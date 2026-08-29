package protocol

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxMessageSize bounds a single tunnel frame, including JSON/base64 overhead.
// It is deliberately independent of HTTP limits: tunnel peers are remote and
// must never be able to dictate an allocation size to the server.
const MaxMessageSize = 8 * 1024 * 1024

const (
	TypeRegister        = "register"
	TypeRegistered      = "registered"
	TypeForwardRequest  = "forward_request"
	TypeForwardResponse = "forward_response"
	TypeError           = "error"
	TypePing            = "ping"
	TypePong            = "pong"
)

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Register struct {
	SessionID      string `json:"session_id,omitempty"`
	LocalPort      int    `json:"local_port"`
	ConnectionType string `json:"connection_type"`
	ClientToken    string `json:"client_token,omitempty"`
	ResumeToken    string `json:"resume_token,omitempty"`
}

type Registered struct {
	SessionID      string    `json:"session_id"`
	Subdomain      string    `json:"subdomain"`
	ConnectURL     string    `json:"connect_url"`
	DashboardURL   string    `json:"dashboard_url"`
	ConnectionType string    `json:"connection_type"`
	StartedAt      time.Time `json:"started_at"`
	AuthToken      string    `json:"auth_token"`
	TunnelToken    string    `json:"tunnel_token"`
}

type ForwardRequest struct {
	ID         string      `json:"id"`
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	Query      string      `json:"query"`
	Headers    http.Header `json:"headers"`
	Body       []byte      `json:"body"`
	TargetPort int         `json:"target_port"`
	ReceivedAt time.Time   `json:"received_at"`
}

type ForwardResponse struct {
	ID         string      `json:"id"`
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	Body       []byte      `json:"body"`
	DurationMs int64       `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
}

type Error struct {
	Message string `json:"message"`
}

func Encode(w io.Writer, kind string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	env, err := json.Marshal(Envelope{Type: kind, Data: payload})
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(env)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(env)
	return err
}

func Decode(r *bufio.Reader, out any) (string, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return "", err
	}
	size := binary.BigEndian.Uint32(lenBuf[:])
	if size == 0 || size > MaxMessageSize {
		return "", fmt.Errorf("invalid tunnel frame size %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	var env Envelope
	if err := json.Unmarshal(buf, &env); err != nil {
		return "", err
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return "", err
		}
	}
	return env.Type, nil
}
