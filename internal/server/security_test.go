package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sundaramrai/vexlo/internal/dashboard"
	"github.com/sundaramrai/vexlo/internal/model"
	"github.com/sundaramrai/vexlo/internal/protocol"
	"github.com/sundaramrai/vexlo/internal/storage"
	installers "github.com/sundaramrai/vexlo/scripts"
)

func TestHealthzReportsHostedRetention(t *testing.T) {
	cfg := hostedTestConfig()
	cfg.RetentionPeriod = 7 * 24 * time.Hour
	cfg.HostedRetention = time.Hour
	manager, db := newTestManager(t, cfg)
	server := &Server{cfg: cfg, db: db, manager: manager}
	rec := httptest.NewRecorder()
	server.handleHealthz(rec, httptest.NewRequest(http.MethodGet, "https://vexlo.example.com/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status: %d", rec.Code)
	}
	var response struct {
		RetentionSecs int64 `json:"retention_secs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if response.RetentionSecs != int64(time.Hour.Seconds()) {
		t.Fatalf("healthz retention_secs = %d, want %d", response.RetentionSecs, int64(time.Hour.Seconds()))
	}
}

func newTestManager(t *testing.T, cfg Config) (*TunnelManager, *storage.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewTunnelManager(cfg, db), db
}

func TestInstallerRoutes(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	server := &Server{cfg: cfg, db: db, manager: manager}
	for _, tc := range []struct {
		path string
		body []byte
	}{
		{path: "/install.sh", body: installers.Shell()},
		{path: "/install.ps1", body: installers.PowerShell()},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			rec := httptest.NewRecorder()
			server.routes().ServeHTTP(rec, httptest.NewRequest(method, "https://vexlo.example.com"+tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s status: %d", method, tc.path, rec.Code)
			}
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s missing installer headers", method, tc.path)
			}
			if method == http.MethodGet && !bytes.Equal(rec.Body.Bytes(), tc.body) {
				t.Fatalf("%s did not serve the packaged installer", tc.path)
			}
			if method == http.MethodHead && rec.Body.Len() != 0 {
				t.Fatalf("HEAD %s returned a body", tc.path)
			}
		}
		rec := httptest.NewRecorder()
		server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "https://vexlo.example.com"+tc.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status: %d", tc.path, rec.Code)
		}
	}
}

func TestRegistrationRejectsLegacyClient(t *testing.T) {
	manager, _ := newTestManager(t, hostedTestConfig())
	if err := manager.validateRegistration(protocol.Register{LocalPort: 3000}); err == nil {
		t.Fatal("legacy client registered without quick-tunnel support")
	}
	if err := manager.validateRegistration(protocol.Register{LocalPort: 3000, Quick: true}); err != nil {
		t.Fatalf("quick tunnel registration rejected: %v", err)
	}
}

func TestResumeRejectsLegacyStoredSession(t *testing.T) {
	manager, db := newTestManager(t, hostedTestConfig())
	session := model.Session{
		ID: "old-session", Subdomain: "old", LocalPort: 3000,
		ConnectionType: "tcp", StartedAt: time.Now().UTC(),
		AuthToken: "old-dashboard-token", TunnelToken: "old-resume-token",
	}
	if err := db.UpsertSession(session); err != nil {
		t.Fatal(err)
	}
	if err := manager.validateRegistration(protocol.Register{
		SessionID: session.ID, LocalPort: 3000, ResumeToken: session.TunnelToken, Quick: true,
	}); err == nil {
		t.Fatal("legacy session resumed on hosted-only server")
	}
}

func TestSanitizeSessionStripsSecrets(t *testing.T) {
	session := model.Session{ID: "session", AuthToken: "dashboard", TunnelToken: "resume"}
	sanitized := sanitizeSession(session)
	if sanitized.AuthToken != "" || sanitized.TunnelToken != "" {
		t.Fatal("session API exposed a secret")
	}
}

func TestPublicTunnelMatchesOnlyConfiguredDomain(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	tunnel := newTunnel(nil, Session{ID: "session", Subdomain: "abc", LocalPort: 3000, Hosted: true})
	manager.installTunnel(tunnel)
	server := &Server{cfg: cfg, db: db, hub: dashboard.NewHub(), manager: manager}
	if manager.FindByHost("abc.other.example") != nil {
		t.Fatal("host outside base domain selected a tunnel")
	}
	rec := httptest.NewRecorder()
	server.handleRoot(rec, httptest.NewRequest(http.MethodGet, "https://vexlo.example.com/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("landing page status: %d", rec.Code)
	}
}

func TestDashboardRejectsLegacyTokenURL(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	server := &Server{cfg: cfg, db: db, manager: manager}
	rec := httptest.NewRecorder()
	server.handleDashboard(rec, httptest.NewRequest(http.MethodGet, "https://vexlo.example.com/app?token=old-secret", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("legacy token URL status: %d", rec.Code)
	}
}

func TestLandingAndDashboardAssetsArePublic(t *testing.T) {
	cfg := hostedTestConfig()
	manager, db := newTestManager(t, cfg)
	server := &Server{cfg: cfg, db: db, manager: manager}
	for _, path := range []string{"/assets/landing.css", "/assets/dashboard.css"} {
		rec := httptest.NewRecorder()
		server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://vexlo.example.com"+path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status: %d", path, rec.Code)
		}
	}
}
