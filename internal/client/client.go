package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sundaramrai/vexlo/internal/protocol"
)

type Config struct {
	ServerAddr    string
	LocalPort     int
	RegisterToken string
}

func DefaultConfig() Config {
	return Config{
		ServerAddr: "127.0.0.1:9000",
		LocalPort:  3000,
	}
}

func Run(ctx context.Context, cfg Config) error {
	var sessionID string
	var resumeToken string
	for {
		if err := runOnce(ctx, cfg, &sessionID, &resumeToken); err != nil {
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

func runOnce(ctx context.Context, cfg Config, sessionID *string, resumeToken *string) error {
	_ = ctx
	conn, err := net.DialTimeout("tcp", cfg.ServerAddr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	reg := protocol.Register{
		SessionID:      *sessionID,
		LocalPort:      cfg.LocalPort,
		ConnectionType: "tcp",
		ClientToken:    cfg.RegisterToken,
		ResumeToken:    *resumeToken,
	}
	if err := protocol.Encode(conn, protocol.TypeRegister, reg); err != nil {
		return err
	}

	reader := newReader(conn)
	var registered protocol.Registered
	kind, err := protocol.Decode(reader, &registered)
	if err != nil || kind != protocol.TypeRegistered {
		return fmt.Errorf("register failed: %w", err)
	}
	*sessionID = registered.SessionID
	*resumeToken = registered.TunnelToken
	log.Printf("%s -> localhost:%d", registered.ConnectURL, cfg.LocalPort)
	log.Printf("dashboard: %s", registered.DashboardURL)

	for {
		var msg protocol.ForwardRequest
		kind, err := protocol.Decode(reader, &msg)
		if err != nil {
			return err
		}
		switch kind {
		case protocol.TypeForwardRequest:
			go handleForward(conn, msg)
		case protocol.TypePing:
			_ = protocol.Encode(conn, protocol.TypePong, map[string]string{"at": time.Now().UTC().Format(time.RFC3339Nano)})
		}
	}
}

func handleForward(conn net.Conn, msg protocol.ForwardRequest) {
	start := time.Now()
	url := fmt.Sprintf("http://127.0.0.1:%d%s", msg.TargetPort, msg.Path)
	if msg.Query != "" {
		url += "?" + msg.Query
	}
	req, err := http.NewRequest(msg.Method, url, strings.NewReader(string(msg.Body)))
	if err != nil {
		_ = protocol.Encode(conn, protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: 502, Error: err.Error()})
		return
	}
	req.Header = msg.Headers.Clone()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = protocol.Encode(conn, protocol.TypeForwardResponse, protocol.ForwardResponse{ID: msg.ID, StatusCode: 502, Error: err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	_ = protocol.Encode(conn, protocol.TypeForwardResponse, protocol.ForwardResponse{
		ID:         msg.ID,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		DurationMs: time.Since(start).Milliseconds(),
	})
}
