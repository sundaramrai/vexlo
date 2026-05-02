package storage

import (
	"database/sql"

	"vexlo/internal/model"
)

func (db *DB) InsertRequest(req model.CapturedRequest) {
	db.Enqueue(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO requests
			(id, session_id, method, path, query, headers, body, response_status, response_headers, response_body, duration_ms, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			req.ID, req.SessionID, req.Method, req.Path, req.Query, req.Headers, req.Body,
			req.ResponseStatus, req.ResponseHeaders, req.ResponseBody, req.DurationMS, req.CreatedAt)
		return err
	})
}

func (db *DB) ListRequests(sessionID, method, path, status, search string) ([]model.CapturedRequest, error) {
	query := `SELECT id, session_id, method, path, query, headers, body, response_status, response_headers, response_body, duration_ms, created_at
		FROM requests WHERE session_id = ?`
	args := []any{sessionID}
	if method != "" && method != "ALL" {
		query += ` AND method = ?`
		args = append(args, method)
	}
	if path != "" {
		query += ` AND path LIKE ?`
		args = append(args, "%"+path+"%")
	}
	if status != "" {
		query += ` AND CAST(response_status AS TEXT) LIKE ?`
		args = append(args, status+"%")
	}
	if search != "" {
		query += ` AND (body LIKE ? OR response_body LIKE ?)`
		args = append(args, "%"+search+"%", "%"+search+"%")
	}
	query += ` ORDER BY created_at DESC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.CapturedRequest
	for rows.Next() {
		var item model.CapturedRequest
		if err := rows.Scan(&item.ID, &item.SessionID, &item.Method, &item.Path, &item.Query, &item.Headers, &item.Body,
			&item.ResponseStatus, &item.ResponseHeaders, &item.ResponseBody, &item.DurationMS, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.DecodedHeaders = HeaderJSONToMap(item.Headers)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *DB) GetRequest(id string) (*model.CapturedRequest, error) {
	var item model.CapturedRequest
	err := db.sql.QueryRow(`SELECT id, session_id, method, path, query, headers, body, response_status, response_headers, response_body, duration_ms, created_at
		FROM requests WHERE id = ?`, id).
		Scan(&item.ID, &item.SessionID, &item.Method, &item.Path, &item.Query, &item.Headers, &item.Body,
			&item.ResponseStatus, &item.ResponseHeaders, &item.ResponseBody, &item.DurationMS, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	item.DecodedHeaders = HeaderJSONToMap(item.Headers)
	replay, _ := db.LatestReplay(id)
	item.Replay = replay
	return &item, nil
}

func (db *DB) InsertReplay(replay model.CapturedReplay) {
	db.Enqueue(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO replays
			(id, request_id, mutated_headers, mutated_body, response_status, response_headers, response_body, duration_ms, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			replay.ID, replay.RequestID, replay.MutatedHeaders, replay.MutatedBody, replay.ResponseStatus,
			replay.ResponseHeader, replay.ResponseBody, replay.DurationMS, replay.CreatedAt)
		return err
	})
}

func (db *DB) LatestReplay(requestID string) (*model.CapturedReplay, error) {
	var replay model.CapturedReplay
	err := db.sql.QueryRow(`SELECT id, request_id, mutated_headers, mutated_body, response_status, response_headers, response_body, duration_ms, created_at
		FROM replays WHERE request_id = ? ORDER BY created_at DESC LIMIT 1`, requestID).
		Scan(&replay.ID, &replay.RequestID, &replay.MutatedHeaders, &replay.MutatedBody, &replay.ResponseStatus,
			&replay.ResponseHeader, &replay.ResponseBody, &replay.DurationMS, &replay.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &replay, nil
}
