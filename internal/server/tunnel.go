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
	"sync/atomic"
	"time"

	"github.com/sundaramrai/vexlo/internal/protocol"
	"github.com/sundaramrai/vexlo/internal/storage"
)

type TunnelManager struct {
	tunnels             map[string]*Tunnel
	bySession           map[string]*Tunnel
	mu                  sync.RWMutex
	storage             *storage.DB
	cfg                 Config
	registerMu          sync.Mutex
	rateGlobal          rateWindow
	rateByIP            map[string]rateWindow
	publicMu            sync.Mutex
	publicGlobal        rateWindow
	publicByIP          map[string]rateWindow
	handoffs            map[string]dashboardHandoff
	handoffMu           sync.Mutex
	inFlight            chan struct{}
	registrationsPaused atomic.Bool
}

type rateWindow struct {
	start time.Time
	count int
}

type dashboardHandoff struct {
	sessionID string
	expires   time.Time
}

type Tunnel struct {
	Subdomain    string
	LocalPort    int
	session      Session
	conn         net.Conn
	reader       *bufio.Reader
	writeMu      sync.Mutex
	mu           sync.RWMutex
	pending      map[string]chan *protocol.ForwardResponse
	authToken    string
	sourceIP     string
	inFlight     chan struct{}
	captureCount atomic.Int64
	closed       chan struct{}
	closeOnce    sync.Once
}

func NewTunnelManager(cfg Config, db *storage.DB) *TunnelManager {
	m := &TunnelManager{
		tunnels:    map[string]*Tunnel{},
		bySession:  map[string]*Tunnel{},
		storage:    db,
		cfg:        cfg,
		rateByIP:   map[string]rateWindow{},
		publicByIP: map[string]rateWindow{},
		handoffs:   map[string]dashboardHandoff{},
	}
	m.inFlight = make(chan struct{}, cfg.HostedMaxInFlight)
	return m
}

func newTunnel(conn net.Conn, session Session) *Tunnel {
	return &Tunnel{
		Subdomain: session.Subdomain,
		LocalPort: session.LocalPort,
		session:   session,
		conn:      conn,
		reader:    bufio.NewReader(conn),
		pending:   map[string]chan *protocol.ForwardResponse{},
		authToken: session.AuthToken,
		inFlight:  make(chan struct{}, 10),
		closed:    make(chan struct{}),
	}
}

func (m *TunnelManager) Register(conn net.Conn, reg protocol.Register) (*protocol.Registered, *Tunnel, error) {
	m.registerMu.Lock()
	defer m.registerMu.Unlock()
	sourceIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		sourceIP = conn.RemoteAddr().String()
	}
	if err := m.admitHosted(sourceIP, reg.SessionID == ""); err != nil {
		return nil, nil, err
	}
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
	count, err := m.storage.CountRequests(session.ID)
	if err != nil {
		return nil, nil, err
	}
	tunnel.captureCount.Store(count)
	tunnel.sourceIP = sourceIP
	m.installTunnel(tunnel)
	go m.readLoop(tunnel)
	return m.registered(session), tunnel, nil
}

func (m *TunnelManager) allowPublicRequest(sourceIP string) bool {
	m.publicMu.Lock()
	defer m.publicMu.Unlock()
	now := time.Now()
	global, ok := advanceRateWindow(m.publicGlobal, now, 6000)
	m.publicGlobal = global
	if !ok {
		return false
	}
	if !rateMapHasCapacity(m.publicByIP, now) {
		return false
	}
	perIP, ok := advanceRateWindow(m.publicByIP[sourceIP], now, 120)
	m.publicByIP[sourceIP] = perIP
	return ok
}

func (m *TunnelManager) admitHosted(sourceIP string, isNew bool) error {
	if isNew && m.registrationsPaused.Load() {
		return errors.New("new quick tunnels are temporarily unavailable")
	}
	if err := m.checkRegistrationRate(sourceIP); err != nil {
		return err
	}
	if !isNew {
		return nil
	}
	return m.checkHostedCapacity(sourceIP)
}

func advanceRateWindow(value rateWindow, now time.Time, limit int) (rateWindow, bool) {
	if now.Sub(value.start) >= time.Minute {
		value = rateWindow{start: now}
	}
	value.count++
	return value, value.count <= limit
}

func rateMapHasCapacity(windows map[string]rateWindow, now time.Time) bool {
	if len(windows) <= 10000 {
		return true
	}
	for ip, value := range windows {
		if now.Sub(value.start) >= time.Minute {
			delete(windows, ip)
		}
	}
	return len(windows) <= 10000
}

func (m *TunnelManager) checkRegistrationRate(sourceIP string) error {
	now := time.Now()
	global, ok := advanceRateWindow(m.rateGlobal, now, 60)
	m.rateGlobal = global
	if !ok {
		return errors.New("quick tunnel registration is busy; try again shortly")
	}
	if !rateMapHasCapacity(m.rateByIP, now) {
		return errors.New("quick tunnel registration is busy; try again shortly")
	}
	perIP, ok := advanceRateWindow(m.rateByIP[sourceIP], now, 10)
	m.rateByIP[sourceIP] = perIP
	if !ok {
		return errors.New("too many quick tunnel registrations from this network")
	}
	return nil
}

func (m *TunnelManager) checkHostedCapacity(sourceIP string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.bySession) >= m.cfg.HostedMaxTunnels {
		return errors.New("quick tunnel service is at capacity")
	}
	activeFromIP := 0
	for _, active := range m.bySession {
		if active.sourceIP == sourceIP {
			activeFromIP++
		}
	}
	if activeFromIP >= m.cfg.HostedMaxPerIP {
		return errors.New("too many active quick tunnels from this network")
	}
	return nil
}

func (m *TunnelManager) revokeHosted(sessionID string) error {
	m.registerMu.Lock()
	defer m.registerMu.Unlock()
	if err := m.storage.RevokeHostedSession(sessionID, time.Now().UTC()); err != nil {
		return err
	}
	if tunnel := m.FindBySession(sessionID); tunnel != nil {
		_ = tunnel.conn.Close()
	}
	return nil
}

func (m *TunnelManager) resolveSession(reg protocol.Register) (Session, error) {
	session := m.newSession(reg.LocalPort)
	if reg.SessionID != "" {
		return m.resumeSession(session, reg)
	}
	if err := m.allocateHostedSubdomain(&session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (m *TunnelManager) newSession(localPort int) Session {
	session := Session{
		ID:             randomID(8),
		Subdomain:      randomID(8),
		LocalPort:      localPort,
		ConnectionType: "tcp",
		StartedAt:      time.Now().UTC(),
		AuthToken:      randomID(32),
		TunnelToken:    randomID(32),
		Hosted:         true,
	}
	return session
}

func (m *TunnelManager) resumeSession(session Session, reg protocol.Register) (Session, error) {
	existing, err := m.storage.GetSession(reg.SessionID)
	if err != nil {
		return Session{}, errors.New("invalid session resume")
	}
	session.ID = existing.ID
	session.Subdomain = existing.Subdomain
	session.StartedAt = existing.StartedAt
	session.LocalPort = reg.LocalPort
	session.ConnectionType = "tcp"
	return session, nil
}

func (m *TunnelManager) allocateHostedSubdomain(session *Session) error {
	for attempts := 0; attempts < 5; attempts++ {
		exists, err := m.storage.SubdomainExists(session.Subdomain)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		session.Subdomain = randomID(8)
	}
	return errors.New("could not allocate quick tunnel URL")
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
	handoff := m.newDashboardHandoff(session.ID)
	return &protocol.Registered{
		SessionID:      session.ID,
		Subdomain:      session.Subdomain,
		ConnectURL:     m.publicURL(session.Subdomain),
		DashboardURL:   m.cfg.HostURL + "/app#session=" + session.ID + "&handoff=" + handoff,
		ConnectionType: session.ConnectionType,
		StartedAt:      session.StartedAt,
		TunnelToken:    session.TunnelToken,
	}
}

func (m *TunnelManager) newDashboardHandoff(sessionID string) string {
	raw := randomID(24)
	m.handoffMu.Lock()
	defer m.handoffMu.Unlock()
	now := time.Now()
	for key, handoff := range m.handoffs {
		if now.After(handoff.expires) {
			delete(m.handoffs, key)
		}
	}
	m.handoffs[storage.HashHostedSecret(raw)] = dashboardHandoff{sessionID: sessionID, expires: now.Add(5 * time.Minute)}
	return raw
}

func (m *TunnelManager) claimDashboardHandoff(sessionID, raw string) (string, bool) {
	m.handoffMu.Lock()
	handoff, ok := m.handoffs[storage.HashHostedSecret(raw)]
	if ok && handoff.sessionID == sessionID {
		delete(m.handoffs, storage.HashHostedSecret(raw))
	}
	m.handoffMu.Unlock()
	if !ok || handoff.sessionID != sessionID || time.Now().After(handoff.expires) {
		return "", false
	}
	tunnel := m.FindBySession(sessionID)
	if tunnel == nil {
		return "", false
	}
	return tunnel.authToken, true
}

func (m *TunnelManager) publicURL(subdomain string) string {
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
	if m.bySession[t.session.ID] != t {
		m.mu.Unlock()
		return
	}
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
	host = strings.ToLower(stripPort(strings.TrimSuffix(host, ".")))
	base := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(m.cfg.BaseDomain), "."))
	if base == "" || host == base || !strings.HasSuffix(host, "."+base) {
		return nil
	}
	subdomain := strings.TrimSuffix(host, "."+base)
	if subdomain == "" || strings.Contains(subdomain, ".") {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tunnels[subdomain]
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

func (m *TunnelManager) expireHostedTunnels(now time.Time) {
	m.mu.RLock()
	var expired []*Tunnel
	for _, tunnel := range m.bySession {
		if now.Sub(tunnel.session.StartedAt) >= m.cfg.HostedLifetime {
			expired = append(expired, tunnel)
		}
	}
	m.mu.RUnlock()
	for _, tunnel := range expired {
		_ = tunnel.conn.Close()
	}
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
	defer func() {
		tunnel.mu.Lock()
		delete(tunnel.pending, reqID)
		tunnel.mu.Unlock()
	}()
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
