package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/protocol"
	"vexlo/internal/storage"
)

type TunnelManager struct {
	tunnels   map[string]*Tunnel
	bySession map[string]*Tunnel
	mu        sync.RWMutex
	storage   *storage.DB
	dashboard *dashboard.Hub
	cfg       Config
}

type Tunnel struct {
	ID             string
	Subdomain      string
	LocalPort      int
	ConnectionType string
	session        Session
	conn           net.Conn
	reader         *bufio.Reader
	writeMu        sync.Mutex
	mu             sync.RWMutex
	pending        map[string]chan *protocol.ForwardResponse
	createdAt      time.Time
	authToken      string
	closed         chan struct{}
	closeOnce      sync.Once
}

func NewTunnelManager(cfg Config, db *storage.DB, hub *dashboard.Hub) *TunnelManager {
	return &TunnelManager{
		tunnels:   map[string]*Tunnel{},
		bySession: map[string]*Tunnel{},
		storage:   db,
		dashboard: hub,
		cfg:       cfg,
	}
}

func newTunnel(conn net.Conn, session Session) *Tunnel {
	return &Tunnel{
		ID:             session.ID,
		Subdomain:      session.Subdomain,
		LocalPort:      session.LocalPort,
		ConnectionType: session.ConnectionType,
		session:        session,
		conn:           conn,
		reader:         bufio.NewReader(conn),
		pending:        map[string]chan *protocol.ForwardResponse{},
		createdAt:      session.StartedAt,
		authToken:      session.AuthToken,
		closed:         make(chan struct{}),
	}
}

func (m *TunnelManager) Register(conn net.Conn, reg protocol.Register) (*protocol.Registered, *Tunnel, error) {
	if err := m.validateRegistration(reg); err != nil {
		return nil, nil, err
	}
	session, err := m.resolveSession(reg)
	if err != nil {
		return nil, nil, err
	}
	if err := m.storage.UpsertSession(session); err != nil {
		return nil, nil, err
	}
	tunnel := newTunnel(conn, session)
	m.installTunnel(tunnel)
	go m.readLoop(tunnel)
	return m.registered(session), tunnel, nil
}

func (m *TunnelManager) resolveSession(reg protocol.Register) (Session, error) {
	session := Session{
		ID:             randomID(8),
		Subdomain:      randomID(3),
		LocalPort:      reg.LocalPort,
		ConnectionType: "tcp",
		StartedAt:      time.Now().UTC(),
		AuthToken:      randomID(16),
		TunnelToken:    randomID(16),
	}
	if reg.SessionID != "" {
		if existing, err := m.storage.GetSession(reg.SessionID); err == nil {
			session = *existing
			session.LocalPort = reg.LocalPort
			session.ConnectionType = "tcp"
			session.StartedAt = time.Now().UTC()
		} else {
			session.ID = reg.SessionID
		}
	}
	return session, nil
}

func (m *TunnelManager) installTunnel(tunnel *Tunnel) {
	m.mu.Lock()
	if existing := m.bySession[tunnel.session.ID]; existing != nil && existing.conn != nil {
		_ = existing.conn.Close()
	}
	m.tunnels[tunnel.Subdomain] = tunnel
	m.bySession[tunnel.session.ID] = tunnel
	m.mu.Unlock()
	slog.Info("tunnel registered",
		"session_id", tunnel.session.ID,
		"subdomain", tunnel.Subdomain,
		"local_port", tunnel.LocalPort,
	)
}

func (m *TunnelManager) registered(session Session) *protocol.Registered {
	return &protocol.Registered{
		SessionID:      session.ID,
		Subdomain:      session.Subdomain,
		ConnectURL:     m.publicURL(session.Subdomain),
		DashboardURL:   m.cfg.HostURL + "/?token=" + session.AuthToken + "&session=" + session.ID,
		ConnectionType: session.ConnectionType,
		StartedAt:      session.StartedAt,
		AuthToken:      session.AuthToken,
		TunnelToken:    session.TunnelToken,
	}
}

func (m *TunnelManager) publicURL(subdomain string) string {
	if m.cfg.BaseDomain == "localhost" {
		return m.cfg.HostURL + "/t/" + subdomain
	}
	return "https://" + subdomain + "." + m.cfg.BaseDomain
}

func (m *TunnelManager) readLoop(t *Tunnel) {
	defer m.closeTunnel(t)
	for {
		var resp protocol.ForwardResponse
		kind, err := protocol.Decode(t.reader, &resp)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = t.conn.Close()
			}
			t.closeOnce.Do(func() { close(t.closed) })
			return
		}
		switch kind {
		case protocol.TypeForwardResponse:
			t.mu.Lock()
			ch := t.pending[resp.ID]
			delete(t.pending, resp.ID)
			t.mu.Unlock()
			if ch != nil {
				ch <- &resp
			}
		case protocol.TypePing:
			_ = t.send(protocol.TypePong, map[string]string{"at": time.Now().UTC().Format(time.RFC3339Nano)})
		}
	}
}

func (m *TunnelManager) removeTunnel(t *Tunnel) {
	m.mu.Lock()
	delete(m.tunnels, t.Subdomain)
	delete(m.bySession, t.session.ID)
	m.mu.Unlock()
	ended := time.Now().UTC()
	_ = m.storage.EndSession(t.session.ID, ended)
	slog.Info("tunnel closed",
		"session_id", t.session.ID,
		"subdomain", t.Subdomain,
	)
}

func (m *TunnelManager) closeTunnel(t *Tunnel) {
	t.closeOnce.Do(func() { close(t.closed) })
	m.removeTunnel(t)
}

func (m *TunnelManager) FindByHost(host string) *Tunnel {
	host = strings.Split(host, ":")[0]
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		return nil
	}
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tunnels[parts[0]]
}

func (m *TunnelManager) FindBySubdomain(sub string) *Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tunnels[sub]
}

func (m *TunnelManager) FindBySession(sessionID string) *Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bySession[sessionID]
}

func (m *TunnelManager) ActiveTunnelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.bySession)
}

func (t *Tunnel) send(kind string, v any) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return protocol.Encode(t.conn, kind, v)
}

func (m *TunnelManager) Forward(ctx context.Context, tunnel *Tunnel, r *http.Request, body []byte, targetPort int) (*protocol.ForwardResponse, error) {
	reqID := randomID(8)
	fwd := protocol.ForwardRequest{
		ID:         reqID,
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Headers:    r.Header.Clone(),
		Body:       body,
		TargetPort: targetPort,
		ReceivedAt: time.Now().UTC(),
	}
	ch := make(chan *protocol.ForwardResponse, 1)
	tunnel.mu.Lock()
	tunnel.pending[reqID] = ch
	tunnel.mu.Unlock()
	if err := tunnel.send(protocol.TypeForwardRequest, fwd); err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-tunnel.closed:
		return nil, errors.New("tunnel disconnected")
	}
}

func (t *Tunnel) ResolveTargetPort(*http.Request) int {
	return t.LocalPort
}

func marshalHeaders(h http.Header) string {
	buf, _ := json.Marshal(h)
	return string(buf)
}
