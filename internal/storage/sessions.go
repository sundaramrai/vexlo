package storage

import (
	"database/sql"
	"time"

	"github.com/sundaramrai/vexlo/internal/model"
)

func (db *DB) UpsertSession(session model.Session) error {
	_, err := db.sql.Exec(`INSERT INTO sessions (id, subdomain, local_port, connection_type, auth_token, tunnel_token, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			subdomain = excluded.subdomain,
			local_port = excluded.local_port,
			connection_type = excluded.connection_type,
			auth_token = excluded.auth_token,
			tunnel_token = excluded.tunnel_token,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at`,
		session.ID, session.Subdomain, session.LocalPort, session.ConnectionType, session.AuthToken, session.TunnelToken, session.StartedAt, nil)
	return err
}

func (db *DB) EndSession(id string, endedAt time.Time) error {
	_, err := db.sql.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, endedAt, id)
	return err
}

func (db *DB) GetSession(id string) (*model.Session, error) {
	var sess model.Session
	var endedAt sql.NullTime
	err := db.sql.QueryRow(`SELECT id, subdomain, local_port, connection_type, auth_token, tunnel_token, started_at, ended_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Subdomain, &sess.LocalPort, &sess.ConnectionType, &sess.AuthToken, &sess.TunnelToken, &sess.StartedAt, &endedAt)
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		sess.EndedAt = &endedAt.Time
	}
	return &sess, nil
}

func (db *DB) ListSessions() ([]model.Session, error) {
	rows, err := db.sql.Query(`SELECT id, subdomain, local_port, connection_type, auth_token, tunnel_token, started_at, ended_at FROM sessions ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		var endedAt sql.NullTime
		if err := rows.Scan(&sess.ID, &sess.Subdomain, &sess.LocalPort, &sess.ConnectionType, &sess.AuthToken, &sess.TunnelToken, &sess.StartedAt, &endedAt); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (db *DB) PruneBefore(cutoff time.Time) error {
	_, err := db.sql.Exec(`DELETE FROM replays WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM requests WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM sessions WHERE ended_at IS NOT NULL AND ended_at < ?`, cutoff)
	return err
}
