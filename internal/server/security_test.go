package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/model"
	"vexlo/internal/protocol"
	"vexlo/internal/storage"
)

func newTestManager(t *testing.T, cfg Config) (*TunnelManager, *storage.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewTunnelManager(cfg, db, dashboard.NewHub(), map[string]struct{}{}), db
}

func TestValidateRegistrationRequiresClientTokenForNewTCPRegistrations(t *testing.T) {
	manager, _ := newTestManager(t, Config{RegistrationToken: "shared-token"})

	err := manager.validateRegistration(protocol.Register{
		LocalPort:      3000,
		ConnectionType: "tcp",
		ClientToken:    "shared-token",
	})
	if err != nil {
		t.Fatalf("expected registration to succeed, got %v", err)
	}

	err = manager.validateRegistration(protocol.Register{
		LocalPort:      3000,
		ConnectionType: "tcp",
		ClientToken:    "wrong-token",
	})
	if err == nil {
		t.Fatal("expected invalid token to fail")
	}
}

func TestValidateRegistrationRequiresResumeTokenForExistingSession(t *testing.T) {
	manager, db := newTestManager(t, Config{RegistrationToken: "shared-token"})
	session := model.Session{
		ID:             "sess-1",
		Subdomain:      "abc123",
		LocalPort:      3000,
		ConnectionType: "tcp",
		StartedAt:      time.Now().UTC(),
		AuthToken:      "dashboard-token",
		TunnelToken:    "resume-secret",
	}
	if err := db.UpsertSession(session); err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	err := manager.validateRegistration(protocol.Register{
		SessionID:      session.ID,
		LocalPort:      3000,
		ConnectionType: "tcp",
		ResumeToken:    "resume-secret",
	})
	if err != nil {
		t.Fatalf("expected resume to succeed, got %v", err)
	}

	err = manager.validateRegistration(protocol.Register{
		SessionID:      session.ID,
		LocalPort:      3000,
		ConnectionType: "tcp",
		ResumeToken:    "wrong-secret",
	})
	if err == nil {
		t.Fatal("expected invalid resume token to fail")
	}
}

func TestSanitizeSessionStripsSecrets(t *testing.T) {
	session := model.Session{
		ID:          "sess-1",
		AuthToken:   "dashboard-token",
		TunnelToken: "resume-secret",
	}
	sanitized := sanitizeSession(session)
	if sanitized.AuthToken != "" {
		t.Fatalf("expected auth token to be stripped, got %q", sanitized.AuthToken)
	}
	if sanitized.TunnelToken != "" {
		t.Fatalf("expected tunnel token to be stripped, got %q", sanitized.TunnelToken)
	}
}

func TestHandleRootStripsTokenFromRedirectURL(t *testing.T) {
	cfg := DefaultConfig()
	manager, db := newTestManager(t, cfg)
	server := &Server{
		cfg:     cfg,
		db:      db,
		hub:     dashboard.NewHub(),
		manager: manager,
	}

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/?session=session-1&token=secret-token", nil)
	rec := httptest.NewRecorder()

	server.handleRoot(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status %d, got %d", http.StatusSeeOther, rec.Code)
	}
	location := rec.Header().Get("Location")
	if location != "http://localhost:8080/?session=session-1" {
		t.Fatalf("expected token-stripped redirect location, got %q", location)
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie == "" {
		t.Fatal("expected auth cookie to be set")
	}
}
