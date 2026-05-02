package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/model"
)

const methodNotAllowed = "method not allowed"
const requestNotFound = "request not found"
const headerContentType = "Content-Type"

const (
	apiRequestsPath = "/api/requests/"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.withRequestLogging("root", s.withAdminAuth(s.handleRoot)))
	mux.HandleFunc("/assets/dashboard.css", s.withRequestLogging("dashboard_css", s.withAdminAuth(dashboard.ServeCSS)))
	mux.HandleFunc("/assets/dashboard.js", s.withRequestLogging("dashboard_js", s.withAdminAuth(dashboard.ServeJS)))
	mux.HandleFunc("/healthz", s.withRequestLogging("healthz", s.handleHealthz))
	mux.HandleFunc("/api/sessions", s.withRequestLogging("list_sessions", s.withAdminAuth(s.handleSessions)))
	mux.HandleFunc("/api/requests", s.withRequestLogging("list_requests", s.withAdminAuth(s.handleRequests)))
	mux.HandleFunc(apiRequestsPath, s.withRequestLogging("get_request", s.withAdminAuth(s.handleRequestByID)))
	mux.HandleFunc("/api/replay", s.withRequestLogging("replay_request", s.withAdminAuth(s.handleReplay)))
	mux.HandleFunc("/ws/events", s.withRequestLogging("events_ws", s.withAdminAuth(s.handleEventsWS)))
	mux.HandleFunc("/t/", s.withRequestLogging("path_tunnel", s.handlePathTunnel))
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if tunnel := s.manager.FindByHost(r.Host); tunnel != nil {
		s.handlePublicRequest(w, r, tunnel)
		return
	}
	if token := r.URL.Query().Get("token"); token != "" {
		http.SetCookie(w, &http.Cookie{Name: "vexlo_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		redirectURL := *r.URL
		query := redirectURL.Query()
		query.Del("token")
		redirectURL.RawQuery = query.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusSeeOther)
		return
	}
	dashboard.ServeHTML(w, r)
}

func (s *Server) handlePathTunnel(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/t/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	tunnel := s.manager.FindBySubdomain(parts[0])
	if tunnel == nil {
		http.NotFound(w, r)
		return
	}
	nextPath := "/"
	if len(parts) == 2 {
		nextPath += parts[1]
	}
	r.URL.Path = nextPath
	s.handlePublicRequest(w, r, tunnel)
}

func (s *Server) handlePublicRequest(w http.ResponseWriter, r *http.Request, tunnel *Tunnel) {
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

	record := model.CapturedRequest{
		ID:              randomID(8),
		SessionID:       tunnel.session.ID,
		Method:          r.Method,
		Path:            r.URL.Path,
		Query:           r.URL.RawQuery,
		Headers:         marshalHeaders(r.Header),
		Body:            captureBody(body, s.cfg.CaptureBodyLimit),
		ResponseStatus:  resp.StatusCode,
		ResponseHeaders: marshalHeaders(resp.Headers),
		ResponseBody:    captureResponseBody(resp.Headers, resp.Body, s.cfg.CaptureBodyLimit),
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
		"retention_secs": int64(s.cfg.RetentionPeriod.Seconds()),
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
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	var payload replayPayload
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxAPIBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid payload", err, "session_id", sessionID)
		return
	}
	original, err := s.db.GetRequest(payload.RequestID)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, err, "session_id", sessionID, "request_record_id", payload.RequestID)
		return
	}
	if original.SessionID != sessionID {
		s.writeError(w, r, http.StatusNotFound, requestNotFound, nil, "session_id", sessionID, "request_record_id", payload.RequestID)
		return
	}
	tunnel := s.manager.FindBySession(sessionID)
	if tunnel == nil {
		s.writeError(w, r, http.StatusBadGateway, "session tunnel offline", nil, "session_id", sessionID)
		return
	}

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
	body := original.Body
	if payload.Body != nil {
		body = *payload.Body
	}
	req, _ := http.NewRequestWithContext(r.Context(), original.Method, "http://replay"+original.Path, strings.NewReader(body))
	req.URL.RawQuery = original.Query
	req.Header = headers
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
	decodedReplayBody := captureResponseBody(resp.Headers, resp.Body, s.cfg.CaptureBodyLimit)
	replayRecord := model.CapturedReplay{
		ID:             randomID(8),
		RequestID:      original.ID,
		MutatedHeaders: marshalFlatHeaders(payload.Headers),
		MutatedBody:    body,
		ResponseStatus: resp.StatusCode,
		ResponseHeader: marshalHeaders(resp.Headers),
		ResponseBody:   decodedReplayBody,
		DurationMS:     time.Since(start).Milliseconds(),
		CreatedAt:      time.Now().UTC(),
	}
	s.db.InsertReplay(replayRecord)
	writeJSON(w, sanitizeCapturedReplay(replayRecord))
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
	token := r.URL.Query().Get("token")
	if token == "" {
		if cookie, err := r.Cookie("vexlo_token"); err == nil {
			token = cookie.Value
		}
	}
	if token == "" || token != session.AuthToken {
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
