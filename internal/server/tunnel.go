package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/protocol"
	"vexlo/internal/storage"
)

type forwarderFunc func(context.Context, *http.Request, []byte, int) (*protocol.ForwardResponse, error)

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
	forwarder      forwarderFunc
	writeMu        sync.Mutex
	mu             sync.RWMutex
	pending        map[string]chan *protocol.ForwardResponse
	rules          []RoutingRule
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

func newForwarderTunnel(session Session, forwarder forwarderFunc, closeConn func() error) *Tunnel {
	return &Tunnel{
		ID:             session.ID,
		Subdomain:      session.Subdomain,
		LocalPort:      session.LocalPort,
		ConnectionType: session.ConnectionType,
		session:        session,
		forwarder:      forwarder,
		pending:        map[string]chan *protocol.ForwardResponse{},
		createdAt:      session.StartedAt,
		authToken:      session.AuthToken,
		closed:         make(chan struct{}),
		conn:           &closerConn{closeFn: closeConn},
	}
}

func (m *TunnelManager) Register(conn net.Conn, reg protocol.Register) (*protocol.Registered, *Tunnel, error) {
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

func (m *TunnelManager) RegisterForwarder(reg protocol.Register, forwarder forwarderFunc, closeConn func() error) (*protocol.Registered, *Tunnel, error) {
	session, err := m.resolveSession(reg)
	if err != nil {
		return nil, nil, err
	}
	if err := m.storage.UpsertSession(session); err != nil {
		return nil, nil, err
	}
	tunnel := newForwarderTunnel(session, forwarder, closeConn)
	m.installTunnel(tunnel)
	return m.registered(session), tunnel, nil
}

func (m *TunnelManager) resolveSession(reg protocol.Register) (Session, error) {
	session := Session{
		ID:             randomID(8),
		Subdomain:      randomID(3),
		LocalPort:      reg.LocalPort,
		ConnectionType: reg.ConnectionType,
		StartedAt:      time.Now().UTC(),
		AuthToken:      randomID(16),
	}
	if reg.SessionID != "" {
		if existing, err := m.storage.GetSession(reg.SessionID); err == nil {
			session = *existing
			session.LocalPort = reg.LocalPort
			session.ConnectionType = reg.ConnectionType
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

func (t *Tunnel) send(kind string, v any) error {
	if t.conn == nil {
		return errors.New("no transport connection")
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return protocol.Encode(t.conn, kind, v)
}

func (m *TunnelManager) Forward(ctx context.Context, tunnel *Tunnel, r *http.Request, body []byte, targetPort int) (*protocol.ForwardResponse, error) {
	if tunnel.forwarder != nil {
		return tunnel.forwarder(ctx, r, body, targetPort)
	}
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

func marshalHeaders(h http.Header) string {
	buf, _ := json.Marshal(h)
	return string(buf)
}

type closerConn struct {
	closeFn func() error
}

func (c *closerConn) Read([]byte) (int, error)  { return 0, io.EOF }
func (c *closerConn) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (c *closerConn) Close() error {
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}
func (c *closerConn) LocalAddr() net.Addr              { return dummyAddr("forwarder-local") }
func (c *closerConn) RemoteAddr() net.Addr             { return dummyAddr("forwarder-remote") }
func (c *closerConn) SetDeadline(time.Time) error      { return nil }
func (c *closerConn) SetReadDeadline(time.Time) error  { return nil }
func (c *closerConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return string(d) }
func (d dummyAddr) String() string  { return string(d) }
