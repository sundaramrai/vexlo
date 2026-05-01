package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"vexlo/internal/protocol"
)

type Config struct {
	ServerAddr string
	Scheme     string
	Mode       string
	HostURL    string
	SSHAddr    string
	WSURL      string
	LocalPort  int
}

func DefaultConfig() Config {
	return Config{
		ServerAddr: "127.0.0.1:9000",
		Scheme:     "http",
		Mode:       "tcp",
		HostURL:    "http://localhost:8080",
		SSHAddr:    "127.0.0.1:2222",
		WSURL:      "ws://127.0.0.1:8080/ws/tunnel",
		LocalPort:  3000,
	}
}

func Run(ctx context.Context, cfg Config) error {
	var sessionID string
	for {
		if err := runOnce(ctx, cfg, &sessionID); err != nil {
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

func runOnce(ctx context.Context, cfg Config, sessionID *string) error {
	var conn net.Conn
	var err error
	switch cfg.Mode {
	case "ssh":
		conn, err = dialSSH(cfg, *sessionID)
	case "ws":
		conn, err = dialWS(ctx, cfg)
	default:
		conn, err = net.DialTimeout("tcp", cfg.ServerAddr, 5*time.Second)
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if cfg.Mode != "ws" {
		reg := protocol.Register{SessionID: *sessionID, LocalPort: cfg.LocalPort, ConnectionType: cfg.Mode}
		if err := protocol.Encode(conn, protocol.TypeRegister, reg); err != nil {
			return err
		}
	}

	reader := newReader(conn)
	var registered protocol.Registered
	kind, err := protocol.Decode(reader, &registered)
	if err != nil || kind != protocol.TypeRegistered {
		return fmt.Errorf("register failed: %w", err)
	}
	*sessionID = registered.SessionID
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

func dialWS(ctx context.Context, cfg Config) (net.Conn, error) {
	conn, _, err := websocket.Dial(ctx, cfg.WSURL+"?port="+fmt.Sprint(cfg.LocalPort), nil)
	if err != nil {
		return nil, err
	}
	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

func dialSSH(cfg Config, sessionID string) (net.Conn, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, err
	}
	clientCfg := &ssh.ClientConfig{
		User:            "vexlo",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	raw, err := net.DialTimeout("tcp", cfg.SSHAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn, chans, reqs, err := ssh.NewClientConn(raw, cfg.SSHAddr, clientCfg)
	if err != nil {
		return nil, err
	}
	client := ssh.NewClient(conn, chans, reqs)
	ch, _, err := client.OpenChannel("tunnel", []byte(fmt.Sprintf(`{"port":%d,"session_id":"%s"}`, cfg.LocalPort, sessionID)))
	if err != nil {
		return nil, err
	}
	return sshChannelConn{Channel: ch, client: client}, nil
}
