package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sundaramrai/vexlo/internal/dashboard"
	"github.com/sundaramrai/vexlo/internal/model"
	"github.com/sundaramrai/vexlo/internal/protocol"
	"github.com/sundaramrai/vexlo/internal/storage"
	installers "github.com/sundaramrai/vexlo/scripts"
)

const methodNotAllowed = "method not allowed"
const requestNotFound = "request not found"
const quickTunnelBusy = "quick tunnel is busy"
const headerContentType = "Content-Type"

const (
	apiRequestsPath = "/api/requests/"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.withRequestLogging("root", s.handleRoot))
	mux.HandleFunc("/app", s.withRequestLogging("dashboard", s.handleDashboard))
	mux.HandleFunc("/assets/landing.css", s.withRequestLogging("landing_css", dashboard.ServeLandingCSS))
	mux.HandleFunc("/assets/dashboard.css", s.withRequestLogging("dashboard_css", dashboard.ServeCSS))
	mux.HandleFunc("/assets/dashboard.js", s.withRequestLogging("dashboard_js", dashboard.ServeJS))
	mux.HandleFunc("/install.sh", s.withRequestLogging("install_sh", func(w http.ResponseWriter, r *http.Request) {
		serveInstaller(w, r, installers.Shell())
	}))
	mux.HandleFunc("/install.ps1", s.withRequestLogging("install_ps1", func(w http.ResponseWriter, r *http.Request) {
		serveInstaller(w, r, installers.PowerShell())
	}))
	mux.HandleFunc("/healthz", s.withRequestLogging("healthz", s.handleHealthz))
	mux.HandleFunc("/api/sessions", s.withRequestLogging("list_sessions", s.handleSessions))
	mux.HandleFunc("/api/requests", s.withRequestLogging("list_requests", s.handleRequests))
	mux.HandleFunc(apiRequestsPath, s.withRequestLogging("get_request", s.handleRequestByID))
	mux.HandleFunc("/api/replay", s.withRequestLogging("replay_request", s.handleReplay))
	mux.HandleFunc("/ws/events", s.withRequestLogging("events_ws", s.handleEventsWS))
	mux.HandleFunc("/api/hosted/claim", s.withRequestLogging("hosted_claim", s.handleHostedClaim))
	mux.HandleFunc("/api/admin/registrations", s.withRequestLogging("admin_registrations", s.withAdminAuth(s.handleAdminRegistrations)))
	mux.HandleFunc("/api/admin/tunnels/", s.withRequestLogging("admin_tunnels", s.withAdminAuth(s.handleAdminTunnel)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// On a tunnel hostname every path belongs to the developer's app,
		// including names also used by the dashboard on the base hostname.
		if tunnel := s.manager.FindByHost(r.Host); tunnel != nil {
			s.handlePublicRequest(w, r, tunnel)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func serveInstaller(w http.ResponseWriter, r *http.Request, body []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func (s *Server) handleAdminRegistrations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !safeOperatorOrigin(r) {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Paused bool `json:"paused"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid payload", err)
		return
	}
	if err := s.db.SetHostedRegistrationsPaused(payload.Paused); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to update registration state", err)
		return
	}
	s.manager.registrationsPaused.Store(payload.Paused)
	writeJSON(w, map[string]bool{"paused": payload.Paused})
}

func (s *Server) handleAdminTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete || !safeOperatorOrigin(r) {
		http.NotFound(w, r)
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/admin/tunnels/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, r)
		return
	}
	if err := s.manager.revokeHosted(sessionID); err != nil {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func safeOperatorOrigin(r *http.Request) bool {
	return r.TLS != nil && sameOrigin(r)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	dashboard.ServeLanding(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/app" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self' wss:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if r.URL.Query().Has("token") {
		s.writeError(w, r, http.StatusBadRequest, "token URLs are unavailable", nil)
		return
	}
	dashboard.ServeHTML(w, r)
}

type hostedClaimRequest struct {
	Session string `json:"session"`
	Handoff string `json:"handoff"`
}

func (s *Server) handleHostedClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.TLS == nil || !sameOrigin(r) {
		s.writeError(w, r, http.StatusForbidden, "invalid dashboard handoff", nil)
		return
	}
	var claim hostedClaimRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claim); err != nil || claim.Session == "" || claim.Handoff == "" {
		s.writeError(w, r, http.StatusBadRequest, "invalid dashboard handoff", err)
		return
	}
	token, ok := s.manager.claimDashboardHandoff(claim.Session, claim.Handoff)
	if !ok {
		s.writeError(w, r, http.StatusUnauthorized, "dashboard handoff expired or already used", nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{
		Name:     "vexlo_session_" + claim.Session,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.HostedLifetime.Seconds()),
	})
	w.WriteHeader(http.StatusNoContent)
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "https" && parsed.Host == r.Host
}

func (s *Server) handlePublicRequest(w http.ResponseWriter, r *http.Request, tunnel *Tunnel) {
	if !s.admitPublicRequest(w, r, tunnel) {
		return
	}
	defer s.releaseHostedSlots(tunnel)
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		clientMsg := "failed to read request body"
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
			clientMsg = "request body exceeds configured limit"
		}
		s.writeError(w, r, status, clientMsg, err)
		return
	}
	_ = r.Body.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	targetPort := tunnel.ResolveTargetPort(r)
	start := time.Now()
	resp, err := s.manager.Forward(ctx, tunnel, r, body, targetPort)
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "tunnel unavailable", err,
			"session_id", tunnel.session.ID,
			"target_port", targetPort,
		)
		return
	}

	writeForwardResponse(w, resp)

	if tunnel.captureCount.Add(1) > 200 {
		return
	}
	captureLimit := s.hostedCaptureLimit()
	requestHeaders := marshalHeaders(r.Header)
	responseHeaders := marshalHeaders(resp.Headers)
	requestHeaders = boundedHeaderCapture(requestHeaders)
	responseHeaders = boundedHeaderCapture(responseHeaders)
	record := model.CapturedRequest{
		ID:              randomID(8),
		SessionID:       tunnel.session.ID,
		Method:          r.Method,
		Path:            r.URL.Path,
		Query:           r.URL.RawQuery,
		Headers:         requestHeaders,
		Body:            captureBody(body, captureLimit),
		ResponseStatus:  resp.StatusCode,
		ResponseHeaders: responseHeaders,
		ResponseBody:    captureResponseBody(resp.Headers, resp.Body, captureLimit),
		DurationMS:      time.Since(start).Milliseconds(),
		CreatedAt:       time.Now().UTC(),
		DecodedHeaders:  flattenHeaders(r.Header),
	}
	s.db.InsertRequest(record)
	s.hub.Broadcast(dashboard.Event{
		Type:      "new_request",
		SessionID: tunnel.session.ID,
		Payload:   sanitizeCapturedRequest(record),
	})
}

func (s *Server) hostedCaptureLimit() int {
	if s.cfg.CaptureBodyLimit <= 0 || s.cfg.CaptureBodyLimit > 16*1024 {
		return 16 * 1024
	}
	return s.cfg.CaptureBodyLimit
}

func writeForwardResponse(w http.ResponseWriter, resp *protocol.ForwardResponse) {
	for key, values := range resp.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if resp.StatusCode == 0 {
		resp.StatusCode = http.StatusBadGateway
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (s *Server) admitPublicRequest(w http.ResponseWriter, r *http.Request, tunnel *Tunnel) bool {
	sourceIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		sourceIP = r.RemoteAddr
	}
	if !s.manager.allowPublicRequest(sourceIP) {
		s.writeError(w, r, http.StatusTooManyRequests, "quick tunnel request limit reached", nil)
		return false
	}
	if !s.acquireHostedSlots(tunnel) {
		s.writeError(w, r, http.StatusTooManyRequests, quickTunnelBusy, nil)
		return false
	}
	return true
}

func (s *Server) acquireHostedSlots(tunnel *Tunnel) bool {
	if !acquireSlot(tunnel.inFlight) {
		return false
	}
	if !acquireSlot(s.manager.inFlight) {
		releaseSlot(tunnel.inFlight)
		return false
	}
	return true
}

func (s *Server) releaseHostedSlots(tunnel *Tunnel) {
	releaseSlot(tunnel.inFlight)
	releaseSlot(s.manager.inFlight)
}

func acquireSlot(slot chan struct{}) bool {
	if slot == nil {
		return true
	}
	select {
	case slot <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseSlot(slot chan struct{}) {
	if slot == nil {
		return
	}
	select {
	case <-slot:
	default:
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, http.StatusMethodNotAllowed, methodNotAllowed, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "database unavailable", err)
		return
	}
	writeJSON(w, map[string]any{
		"status":         "ok",
		"active_tunnels": s.manager.ActiveTunnelCount(),
		"db_queue_depth": s.db.QueueDepth(),
		"retention_secs": int64(s.cfg.HostedRetention.Seconds()),
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, http.StatusMethodNotAllowed, methodNotAllowed, nil)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	session, err := s.db.GetSession(sessionID)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load session", err, "session_id", sessionID)
		return
	}
	writeJSON(w, []model.Session{sanitizeSession(*session)})
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, http.StatusMethodNotAllowed, methodNotAllowed, nil)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	requests, err := s.db.ListRequests(sessionID, r.URL.Query().Get("method"), r.URL.Query().Get("path"), r.URL.Query().Get("status"), r.URL.Query().Get("search"))
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to list requests", err, "session_id", sessionID)
		return
	}
	sanitized := make([]model.CapturedRequest, 0, len(requests))
	for _, item := range requests {
		sanitized = append(sanitized, sanitizeCapturedRequest(item))
	}
	writeJSON(w, sanitized)
}

func (s *Server) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, http.StatusMethodNotAllowed, methodNotAllowed, nil)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, apiRequestsPath)
	item, err := s.db.GetRequest(id)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, err, "session_id", sessionID, "request_record_id", id)
		return
	}
	if item.SessionID != sessionID {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, nil, "session_id", sessionID, "request_record_id", id)
		return
	}
	writeJSON(w, sanitizeCapturedRequest(*item))
}

type replayPayload struct {
	RequestID string            `json:"request_id"`
	Headers   map[string]string `json:"headers"`
	Body      *string           `json:"body"`
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, http.StatusMethodNotAllowed, methodNotAllowed, nil)
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, r, http.StatusForbidden, "invalid request origin", nil)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	var payload replayPayload
	maxBody := min(s.cfg.MaxAPIBodyBytes, 32*1024)
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid payload", err, "session_id", sessionID)
		return
	}
	original, ok := s.loadReplayOriginal(w, r, sessionID, payload.RequestID)
	if !ok {
		return
	}
	tunnel := s.manager.FindBySession(sessionID)
	if tunnel == nil {
		s.writeError(w, r, http.StatusBadGateway, "session tunnel offline", nil, "session_id", sessionID)
		return
	}
	if !s.acquireHostedSlots(tunnel) {
		s.writeError(w, r, http.StatusTooManyRequests, quickTunnelBusy, nil)
		return
	}
	defer s.releaseHostedSlots(tunnel)
	s.forwardReplay(w, r, tunnel, original, payload, sessionID)
}

func (s *Server) loadReplayOriginal(w http.ResponseWriter, r *http.Request, sessionID, requestID string) (*model.CapturedRequest, bool) {
	original, err := s.db.GetRequest(requestID)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, err, "session_id", sessionID, "request_record_id", requestID)
		return nil, false
	}
	if original.SessionID != sessionID {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, nil, "session_id", sessionID, "request_record_id", requestID)
		return nil, false
	}
	count, err := s.db.CountReplaysForSession(sessionID)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to check replay limit", err)
		return nil, false
	}
	if count >= 200 {
		s.writeError(w, r, http.StatusTooManyRequests, "quick tunnel replay limit reached", nil)
		return nil, false
	}
	return original, true
}

func replayHeaders(payload replayPayload, original *model.CapturedRequest) http.Header {
	headers := make(http.Header)
	if len(payload.Headers) == 0 {
		for key, value := range headerJSONToMap(original.Headers) {
			headers.Set(key, value)
		}
	} else {
		for key, value := range payload.Headers {
			headers.Set(key, value)
		}
	}
	return headers
}

func (s *Server) forwardReplay(w http.ResponseWriter, r *http.Request, tunnel *Tunnel, original *model.CapturedRequest, payload replayPayload, sessionID string) {
	body := original.Body
	if payload.Body != nil {
		body = *payload.Body
	}
	if len(body) > 16*1024 {
		s.writeError(w, r, http.StatusRequestEntityTooLarge, "quick tunnel replay body exceeds limit", nil)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), original.Method, "http://replay"+original.Path, strings.NewReader(body))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "stored request cannot be replayed", err)
		return
	}
	req.URL.RawQuery = original.Query
	req.Header = replayHeaders(payload, original)
	req.Body = io.NopCloser(strings.NewReader(body))

	targetPort := tunnel.ResolveTargetPort(req)
	start := time.Now()
	resp, err := s.manager.Forward(r.Context(), tunnel, req, []byte(body), targetPort)
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "replay forward failed", err,
			"session_id", sessionID,
			"request_record_id", original.ID,
			"target_port", targetPort,
		)
		return
	}
	captureLimit := s.hostedCaptureLimit()
	decodedReplayBody := captureResponseBody(resp.Headers, resp.Body, captureLimit)
	responseHeaders := marshalHeaders(resp.Headers)
	responseHeaders = boundedHeaderCapture(responseHeaders)
	replayRecord := model.CapturedReplay{
		ID:             randomID(8),
		RequestID:      original.ID,
		MutatedHeaders: marshalFlatHeaders(payload.Headers),
		MutatedBody:    body,
		ResponseStatus: resp.StatusCode,
		ResponseHeader: responseHeaders,
		ResponseBody:   decodedReplayBody,
		DurationMS:     time.Since(start).Milliseconds(),
		CreatedAt:      time.Now().UTC(),
	}
	s.db.InsertReplay(replayRecord)
	writeJSON(w, sanitizeCapturedReplay(replayRecord))
}

func boundedHeaderCapture(encoded string) string {
	if len(encoded) > 16*1024 {
		return `{"[truncated]":["headers exceeded the quick tunnel capture limit"]}`
	}
	return encoded
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set(headerContentType, "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func sanitizeSession(session model.Session) model.Session {
	session.AuthToken = ""
	session.TunnelToken = ""
	return session
}

func (s *Server) authorize(r *http.Request) (string, error) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-ID")
	}
	if sessionID == "" {
		return "", errors.New("missing session")
	}
	session, err := s.db.GetSession(sessionID)
	if err != nil {
		return "", errors.New("invalid session")
	}
	if !session.Hosted || time.Since(session.StartedAt) > s.cfg.HostedLifetime+s.cfg.HostedRetention {
		return "", errors.New("invalid session")
	}
	cookie, err := r.Cookie("vexlo_session_" + sessionID)
	if err != nil || !subtleConstantTimeCompare(storage.HashHostedSecret(cookie.Value), session.AuthToken) {
		return "", errors.New("unauthorized")
	}
	return sessionID, nil
}

func marshalFlatHeaders(src map[string]string) string {
	if len(src) == 0 {
		return "{}"
	}
	h := make(http.Header)
	for key, value := range src {
		h.Set(key, value)
	}
	return marshalHeaders(h)
}
