package server

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sundaramrai/vexlo/internal/dashboard"
	"github.com/sundaramrai/vexlo/internal/model"
	"github.com/sundaramrai/vexlo/internal/protocol"
)

func hostedTestConfig() Config {
	cfg := DefaultConfig()
	cfg.Hosted = true
	cfg.EnableTLS = true
	cfg.EnableTunnelTLS = true
	cfg.BaseDomain = "vexlo.example.com"
	cfg.HostURL = "https://vexlo.example.com"
	cfg.AdminUsername = "operator"
	cfg.AdminPassword = "test-pass"
	return cfg
}

func registerHostedTestTunnel(t *testing.T, manager *TunnelManager) (*protocol.Registered, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })
	registered, _, err := manager.Register(serverConn, protocol.Register{LocalPort: 3000, Quick: true})
	if err != nil {
		t.Fatalf("register hosted tunnel: %v", err)
	}
	return registered, clientConn
}

func TestHostedRegistrationStoresOnlyHashesAndSeparatesSessions(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	first, _ := registerHostedTestTunnel(t, manager)
	second, _ := registerHostedTestTunnel(t, manager)
	if first.SessionID == second.SessionID || first.Subdomain == second.Subdomain {
		t.Fatal("hosted sessions were not unique")
	}
	if !strings.Contains(first.DashboardURL, "#session=") || strings.Contains(first.DashboardURL, "?token=") {
		t.Fatal("hosted dashboard link lacks a fragment handoff")
	}
	stored, err := db.GetSession(first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Hosted || stored.TunnelToken == first.TunnelToken || !strings.HasPrefix(stored.TunnelToken, "sha256:") || !strings.HasPrefix(stored.AuthToken, "sha256:") {
		t.Fatal("hosted session did not store hashed secrets")
	}
	if err := manager.validateRegistration(protocol.Register{SessionID: first.SessionID, LocalPort: 3000, Quick: true, ResumeToken: second.TunnelToken}); err == nil {
		t.Fatal("another tunnel's resume secret was accepted")
	}
	if err := manager.validateRegistration(protocol.Register{SessionID: first.SessionID, LocalPort: 3000, Quick: true, ResumeToken: first.TunnelToken}); err != nil {
		t.Fatalf("correct resume secret rejected: %v", err)
	}
}

func TestHostedDashboardHandoffIsSingleUseAndSessionScoped(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	first, _ := registerHostedTestTunnel(t, manager)
	second, _ := registerHostedTestTunnel(t, manager)
	srv := &Server{cfg: cfg, db: db, manager: manager, hub: dashboard.NewHub()}
	firstURL, err := url.Parse(first.DashboardURL)
	if err != nil {
		t.Fatal(err)
	}
	if firstURL.Query().Get("handoff") != "" {
		t.Fatal("handoff leaked into query")
	}
	fragment, err := url.ParseQuery(firstURL.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	handoff := fragment.Get("handoff")
	if handoff == "" {
		t.Fatal("missing handoff")
	}
	claim := func(sessionID string) *httptest.ResponseRecorder {
		claimBody, _ := json.Marshal(hostedClaimRequest{Session: sessionID, Handoff: handoff})
		req := httptest.NewRequest(http.MethodPost, "https://vexlo.example.com/api/hosted/claim", strings.NewReader(string(claimBody)))
		req.TLS = &tls.ConnectionState{}
		req.Header.Set("Origin", "https://vexlo.example.com")
		rec := httptest.NewRecorder()
		srv.handleHostedClaim(rec, req)
		return rec
	}
	if rec := claim(second.SessionID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("another session claimed handoff: %d", rec.Code)
	}
	accepted := claim(first.SessionID)
	if accepted.Code != http.StatusNoContent || len(accepted.Result().Cookies()) != 1 {
		t.Fatalf("valid handoff rejected: %d", accepted.Code)
	}
	if rec := claim(first.SessionID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("handoff was reusable: %d", rec.Code)
	}
	cookie := accepted.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.Name != "vexlo_session_"+first.SessionID {
		t.Fatal("hosted dashboard cookie was not scoped and secure")
	}
	requestFor := func(sessionID string) int {
		req := httptest.NewRequest(http.MethodGet, "https://vexlo.example.com/api/sessions?session="+sessionID, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		srv.handleSessions(rec, req)
		return rec.Code
	}
	if code := requestFor(first.SessionID); code != http.StatusOK {
		t.Fatalf("own session denied: %d", code)
	}
	if code := requestFor(second.SessionID); code != http.StatusUnauthorized {
		t.Fatalf("other session allowed: %d", code)
	}
}

func TestHostedModeMustBeExplicitAndSecure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBPath = t.TempDir() + "/test.db"
	if _, err := New(cfg); err == nil {
		t.Fatal("server started without explicit hosted activation")
	}
	cfg.Hosted = true
	if _, err := New(cfg); err == nil {
		t.Fatal("hosted mode started without TLS and domain settings")
	}
	cfg = hostedTestConfig()
	cfg.DBPath = t.TempDir() + "/test.db"
	server, err := New(cfg)
	if err != nil {
		t.Fatalf("secure hosted config rejected: %v", err)
	}
	_ = server.db.Close()
}

func TestHostedResumeExpires(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	session := model.Session{
		ID: "expired", Subdomain: "expired", LocalPort: 3000,
		ConnectionType: "tcp", StartedAt: time.Now().Add(-cfg.HostedLifetime - time.Minute),
		AuthToken: "dashboard-secret", TunnelToken: "resume-secret", Hosted: true,
	}
	if err := db.UpsertSession(session); err != nil {
		t.Fatal(err)
	}
	if err := manager.validateRegistration(protocol.Register{Quick: true, SessionID: session.ID, ResumeToken: session.TunnelToken, LocalPort: 3000}); err == nil {
		t.Fatal("expired hosted tunnel resumed")
	}
}

func TestHostedOperatorCanPauseAndRevoke(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	registered, _ := registerHostedTestTunnel(t, manager)
	srv := &Server{cfg: cfg, db: db, manager: manager}
	post := func(paused bool) int {
		body, _ := json.Marshal(map[string]bool{"paused": paused})
		req := httptest.NewRequest(http.MethodPost, "https://vexlo.example.com/api/admin/registrations", strings.NewReader(string(body)))
		req.TLS = &tls.ConnectionState{}
		req.Header.Set("Origin", "https://vexlo.example.com")
		req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := post(true); got != http.StatusOK {
		t.Fatalf("pause returned %d", got)
	}
	if persisted, err := db.HostedRegistrationsPaused(); err != nil || !persisted {
		t.Fatalf("pause was not persisted: %t, %v", persisted, err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	if _, _, err := manager.Register(serverConn, protocol.Register{LocalPort: 3000, Quick: true}); err == nil {
		t.Fatal("new hosted registration allowed while paused")
	}
	if got := post(false); got != http.StatusOK {
		t.Fatalf("unpause returned %d", got)
	}
	req := httptest.NewRequest(http.MethodDelete, "https://vexlo.example.com/api/admin/tunnels/"+registered.SessionID, nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Origin", "https://vexlo.example.com")
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke returned %d", rec.Code)
	}
	if err := manager.validateRegistration(protocol.Register{Quick: true, SessionID: registered.SessionID, ResumeToken: registered.TunnelToken, LocalPort: 3000}); err == nil {
		t.Fatal("revoked hosted tunnel resumed")
	}
	if session, err := db.GetSession(registered.SessionID); err != nil || session.AuthToken != "" {
		t.Fatalf("revoked hosted dashboard credential remained usable: %v", err)
	}
}

func TestHostedTunnelHostOwnsReservedPaths(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	registered, peer := registerHostedTestTunnel(t, manager)
	srv := &Server{cfg: cfg, db: db, manager: manager, hub: dashboard.NewHub()}
	// A reserved path on a tunnel host must not serve Vexlo's dashboard.
	go func() {
		var forwarded protocol.ForwardRequest
		kind, err := protocol.Decode(bufio.NewReader(peer), &forwarded)
		if err == nil && kind == protocol.TypeForwardRequest {
			_ = protocol.Encode(peer, protocol.TypeForwardResponse, protocol.ForwardResponse{ID: forwarded.ID, StatusCode: http.StatusOK, Body: []byte("local-app")})
		}
	}()
	req := httptest.NewRequest(http.MethodGet, "https://"+registered.Subdomain+".vexlo.example.com/app", nil)
	req.RemoteAddr = "203.0.113.1:54321"
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "local-app" {
		t.Fatalf("reserved path did not reach local app: %d %q", rec.Code, rec.Body.String())
	}
}
