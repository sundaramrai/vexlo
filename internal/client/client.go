package client

import (
	"bytes"
	"context"
	"crypto/tls"
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
	RegisterToken        string
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
	var conn net.Conn
	var err error
	if cfg.EnableTLS {
		serverName := cfg.ServerName
		if serverName == "" {
			host, _, splitErr := net.SplitHostPort(cfg.ServerAddr)
			if splitErr != nil {
				return fmt.Errorf("derive tunnel TLS server name: %w", splitErr)
			}
			serverName = host
		}
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", cfg.ServerAddr, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName})
	} else {
		conn, err = net.DialTimeout("tcp", cfg.ServerAddr, 5*time.Second)
	}
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
