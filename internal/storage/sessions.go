package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/sundaramrai/vexlo/internal/model"
)

func (db *DB) UpsertSession(session model.Session) error {
	authToken, tunnelToken := session.AuthToken, session.TunnelToken
	if session.Hosted {
		authToken, tunnelToken = HashHostedSecret(authToken), HashHostedSecret(tunnelToken)
	}
	_, err := db.sql.Exec(`INSERT INTO sessions (id, subdomain, local_port, connection_type, auth_token, tunnel_token, started_at, ended_at, hosted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			subdomain = excluded.subdomain,
			local_port = excluded.local_port,
			connection_type = excluded.connection_type,
			auth_token = excluded.auth_token,
			tunnel_token = excluded.tunnel_token,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			hosted = excluded.hosted`,
		session.ID, session.Subdomain, session.LocalPort, session.ConnectionType, authToken, tunnelToken, session.StartedAt, nil, session.Hosted)
	return err
}

func HashHostedSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (db *DB) SetHostedRegistrationsPaused(paused bool) error {
	value := "false"
	if paused {
		value = "true"
	}
	_, err := db.sql.Exec(`INSERT INTO service_settings (key, value) VALUES ('hosted_registrations_paused', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, value)
	return err
}

func (db *DB) HostedRegistrationsPaused() (bool, error) {
	var value string
	err := db.sql.QueryRow(`SELECT value FROM service_settings WHERE key = 'hosted_registrations_paused'`).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return value == "true", err
}

func (db *DB) EndSession(id string, endedAt time.Time) error {
	_, err := db.sql.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, endedAt, id)
	return err
}

func (db *DB) SubdomainExists(subdomain string) (bool, error) {
	var count int
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM sessions WHERE subdomain = ?`, subdomain).Scan(&count)
	return count > 0, err
}

func (db *DB) CountRequests(sessionID string) (int64, error) {
	var count int64
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM requests WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

func (db *DB) CountReplaysForSession(sessionID string) (int64, error) {
	var count int64
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM replays WHERE request_id IN (SELECT id FROM requests WHERE session_id = ?)`, sessionID).Scan(&count)
	return count, err
}

func (db *DB) EndOrphanedHostedSessions(endedAt time.Time) error {
	_, err := db.sql.Exec(`UPDATE sessions SET ended_at = ? WHERE hosted = 1 AND ended_at IS NULL`, endedAt)
	return err
}

func (db *DB) RevokeHostedSession(id string, endedAt time.Time) error {
	result, err := db.sql.Exec(`UPDATE sessions SET auth_token = '', tunnel_token = '', ended_at = ? WHERE id = ? AND hosted = 1`, endedAt, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) GetSession(id string) (*model.Session, error) {
	var sess model.Session
	var endedAt sql.NullTime
	err := db.sql.QueryRow(`SELECT id, subdomain, local_port, connection_type, auth_token, tunnel_token, started_at, ended_at, hosted FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Subdomain, &sess.LocalPort, &sess.ConnectionType, &sess.AuthToken, &sess.TunnelToken, &sess.StartedAt, &endedAt, &sess.Hosted)
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		sess.EndedAt = &endedAt.Time
	}
	return &sess, nil
}

func (db *DB) PruneBefore(cutoff time.Time) error {
	_, err := db.sql.Exec(`DELETE FROM replays WHERE created_at < ? AND request_id IN (SELECT id FROM requests WHERE session_id IN (SELECT id FROM sessions WHERE hosted = 0))`, cutoff)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM requests WHERE created_at < ? AND session_id IN (SELECT id FROM sessions WHERE hosted = 0)`, cutoff)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM sessions WHERE hosted = 0 AND ended_at IS NOT NULL AND ended_at < ?`, cutoff)
	return err
}

func (db *DB) PruneHostedBefore(cutoff time.Time) error {
	if _, err := db.sql.Exec(`DELETE FROM replays WHERE request_id IN (SELECT id FROM requests WHERE session_id IN (SELECT id FROM sessions WHERE hosted = 1 AND ended_at IS NOT NULL AND ended_at < ?))`, cutoff); err != nil {
		return err
	}
	if _, err := db.sql.Exec(`DELETE FROM requests WHERE session_id IN (SELECT id FROM sessions WHERE hosted = 1 AND ended_at IS NOT NULL AND ended_at < ?)`, cutoff); err != nil {
		return err
	}
	_, err := db.sql.Exec(`DELETE FROM sessions WHERE hosted = 1 AND ended_at IS NOT NULL AND ended_at < ?`, cutoff)
	return err
}
