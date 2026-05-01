package model

import "time"

type RoutingRule struct {
	ID               string    `json:"id"`
	SessionID        string    `json:"session_id"`
	MatchMethod      string    `json:"match_method"`
	MatchPath        string    `json:"match_path"`
	MatchHeaderKey   string    `json:"match_header_key"`
	MatchHeaderValue string    `json:"match_header_value"`
	TargetPort       int       `json:"target_port"`
	Priority         int       `json:"priority"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
}

type Session struct {
	ID             string     `json:"id"`
	Subdomain      string     `json:"subdomain"`
	LocalPort      int        `json:"local_port"`
	ConnectionType string     `json:"connection_type"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	AuthToken      string     `json:"auth_token,omitempty"`
}

type CapturedRequest struct {
	ID              string            `json:"id"`
	SessionID       string            `json:"session_id"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Query           string            `json:"query"`
	Headers         string            `json:"headers"`
	Body            string            `json:"body"`
	ResponseStatus  int               `json:"response_status"`
	ResponseHeaders string            `json:"response_headers"`
	ResponseBody    string            `json:"response_body"`
	DurationMS      int64             `json:"duration_ms"`
	CreatedAt       time.Time         `json:"created_at"`
	Replay          *CapturedReplay   `json:"replay,omitempty"`
	DecodedHeaders  map[string]string `json:"decoded_headers,omitempty"`
}

type CapturedReplay struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id"`
	MutatedHeaders string    `json:"mutated_headers"`
	MutatedBody    string    `json:"mutated_body"`
	ResponseStatus int       `json:"response_status"`
	ResponseHeader string    `json:"response_headers"`
	ResponseBody   string    `json:"response_body"`
	DiffResult     string    `json:"diff_result"`
	DurationMS     int64     `json:"duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
}
