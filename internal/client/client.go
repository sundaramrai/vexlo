package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sundaramrai/vexlo/internal/protocol"
)

type Config struct {
	ServerAddr           string
	LocalPort            int
	EnableTLS            bool
	ServerName           string
	RequestTimeout       time.Duration
	MaxResponseBodyBytes int64
}

func DefaultConfig() Config {
	return Config{
		ServerAddr:           "127.0.0.1:9000",
		LocalPort:            3000,
		RequestTimeout:       30 * time.Second,
		MaxResponseBodyBytes: 2 * 1024 * 1024,
	}
}

func Run(ctx context.Context, cfg Config) error {
	var sessionID string
	var resumeToken string
	for {
		if err := runOnce(ctx, cfg, &sessionID, &resumeToken); err != nil {
			var rejected resumeRejectedError
			if errors.As(err, &rejected) {
				sessionID, resumeToken = "", ""
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("connection dropped: %v; reconnecting", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

type resumeRejectedError struct{ reason string }

func (e resumeRejectedError) Error() string { return e.reason }

func runOnce(ctx context.Context, cfg Config, sessionID *string, resumeToken *string) error {
	conn, err := dialServer(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	reader := bufio.NewReader(conn)
	registered, err := registerTunnel(conn, reader, cfg, *sessionID, *resumeToken)
	if err != nil {
		return err
	}
	*sessionID = registered.SessionID
	*resumeToken = registered.TunnelToken
	log.Printf("%s -> localhost:%d", registered.ConnectURL, cfg.LocalPort)
	log.Printf("dashboard: %s", registered.DashboardURL)
	log.Printf("Anyone with the public URL can access your local app. Keep the dashboard link private.")
	return serveTunnel(conn, reader, cfg)
}

func dialServer(cfg Config) (net.Conn, error) {
	if !cfg.EnableTLS {
		return net.DialTimeout("tcp", cfg.ServerAddr, 5*time.Second)
	}
	serverName := cfg.ServerName
	if serverName == "" {
		host, _, err := net.SplitHostPort(cfg.ServerAddr)
		if err != nil {
			return nil, fmt.Errorf("derive tunnel TLS server name: %w", err)
		}
		serverName = host
	}
	return tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", cfg.ServerAddr, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName})
}

func registerTunnel(conn net.Conn, reader *bufio.Reader, cfg Config, sessionID, resumeToken string) (protocol.Registered, error) {
	reg := protocol.Register{
		SessionID:      sessionID,
		LocalPort:      cfg.LocalPort,
		ConnectionType: "tcp",
		ResumeToken:    resumeToken,
		Quick:          true,
	}
	if err := protocol.Encode(conn, protocol.TypeRegister, reg); err != nil {
		return protocol.Registered{}, err
	}
	var payload json.RawMessage
	kind, err := protocol.Decode(reader, &payload)
	if err != nil {
		return protocol.Registered{}, fmt.Errorf("register failed: %w", err)
	}
	if kind == protocol.TypeError {
		var failure protocol.Error
		if err := json.Unmarshal(payload, &failure); err != nil {
			return protocol.Registered{}, fmt.Errorf("decode registration error: %w", err)
		}
		if sessionID != "" && (failure.Message == "quick tunnel expired" || failure.Message == "invalid session resume" || failure.Message == "invalid session mode" || failure.Message == "invalid resume token") {
			return protocol.Registered{}, resumeRejectedError{reason: failure.Message}
		}
		return protocol.Registered{}, fmt.Errorf("register failed: %s", failure.Message)
	}
	if kind != protocol.TypeRegistered {
		return protocol.Registered{}, fmt.Errorf("unexpected registration response: %s", kind)
	}
	var registered protocol.Registered
	if err := json.Unmarshal(payload, &registered); err != nil {
		return protocol.Registered{}, fmt.Errorf("decode registration response: %w", err)
	}
	return registered, nil
}

func serveTunnel(conn net.Conn, reader *bufio.Reader, cfg Config) error {
	var writeMu sync.Mutex
	send := func(kind string, value any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return protocol.Encode(conn, kind, value)
	}

	for {
		var msg protocol.ForwardRequest
		kind, err := protocol.Decode(reader, &msg)
		if err != nil {
			return err
		}
		switch kind {
		case protocol.TypeForwardRequest:
			go handleForward(send, cfg, msg)
		case protocol.TypePing:
			_ = send(protocol.TypePong, map[string]string{"at": time.Now().UTC().Format(time.RFC3339Nano)})
		}
	}
}

func handleForward(send func(string, any) error, cfg Config, msg protocol.ForwardRequest) {
	start := time.Now()
	url := fmt.Sprintf("http://127.0.0.1:%d%s", msg.TargetPort, msg.Path)
	if msg.Query != "" {
		url += "?" + msg.Query
	}
	req, err := http.NewRequest(msg.Method, url, bytes.NewReader(msg.Body))
	if err != nil {
		_ = send(protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: 502, Error: err.Error()})
		return
	}
	req.Header = msg.Headers.Clone()
	resp, err := (&http.Client{Timeout: cfg.RequestTimeout}).Do(req)
	if err != nil {
		_ = send(protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: 502, Error: err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxResponseBodyBytes+1))
	if err != nil {
		_ = send(protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: http.StatusBadGateway, Error: fmt.Sprintf("read local response: %v", err)})
		return
	}
	if int64(len(body)) > cfg.MaxResponseBodyBytes {
		_ = send(protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: http.StatusBadGateway, Error: "local response exceeds configured limit"})
		return
	}
	_ = send(protocol.TypeForwardResponse, protocol.ForwardResponse{
		ID:         msg.ID,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		DurationMs: time.Since(start).Milliseconds(),
	})
}
