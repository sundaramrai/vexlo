package storage

import (
	"context"
	"database/sql"
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
			diff_result TEXT,
			duration_ms INTEGER,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (request_id) REFERENCES requests(id)
		)`,
		`CREATE TABLE IF NOT EXISTS routing_rules (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			match_method TEXT,
			match_path TEXT,
			match_header_key TEXT,
			match_header_value TEXT,
			target_port INTEGER NOT NULL,
			priority INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_session_created_at ON requests(session_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_replays_request_created_at ON replays(request_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_routing_rules_session_priority ON routing_rules(session_id, priority)`,
	}
	for _, stmt := range stmts {
		if _, err := db.sql.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
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
