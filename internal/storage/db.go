package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql           *sql.DB
	queue         chan func(*sql.Tx) error
	closed        chan struct{}
	writerDone    chan struct{}
	writerStarted atomic.Bool
	closeOnce     sync.Once
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	wrapped := &DB{
		sql:        db,
		queue:      make(chan func(*sql.Tx) error, 1024),
		closed:     make(chan struct{}),
		writerDone: make(chan struct{}),
	}
	if err := wrapped.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return wrapped, nil
}

func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			subdomain TEXT NOT NULL,
			local_port INTEGER NOT NULL,
			connection_type TEXT NOT NULL,
			auth_token TEXT NOT NULL,
			tunnel_token TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			ended_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			query TEXT,
			headers TEXT NOT NULL,
			body TEXT,
			response_status INTEGER,
			response_headers TEXT,
			response_body TEXT,
			duration_ms INTEGER,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS replays (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			mutated_headers TEXT,
			mutated_body TEXT,
			response_status INTEGER,
			response_headers TEXT,
			response_body TEXT,
			duration_ms INTEGER,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (request_id) REFERENCES requests(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_session_created_at ON requests(session_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_replays_request_created_at ON replays(request_id, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.sql.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumn(db.sql, "sessions", "tunnel_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := db.applyPragmas(); err != nil {
		return err
	}
	return nil
}

func (db *DB) applyPragmas() error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`PRAGMA busy_timeout=5000;`,
		`PRAGMA synchronous=NORMAL;`,
	}
	for _, pragma := range pragmas {
		if _, err := db.sql.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl))
	return err
}

func (db *DB) RunWriter(ctx context.Context) {
	db.writerStarted.Store(true)
	defer close(db.writerDone)

	run := func(txCtx context.Context, fn func(*sql.Tx) error) {
		tx, err := db.sql.BeginTx(txCtx, nil)
		if err != nil {
			return
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case fn := <-db.queue:
					run(context.Background(), fn)
				default:
					return
				}
			}
		case fn := <-db.queue:
			run(ctx, fn)
		}
	}
}

func (db *DB) Enqueue(fn func(*sql.Tx) error) {
	select {
	case db.queue <- fn:
	case <-db.closed:
	}
}

func (db *DB) Close() error {
	db.closeOnce.Do(func() {
		close(db.closed)
	})
	if db.writerStarted.Load() {
		<-db.writerDone
	}
	return db.sql.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.sql.PingContext(ctx)
}

func (db *DB) QueueDepth() int {
	return len(db.queue)
}
