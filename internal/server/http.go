package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/model"
	"vexlo/internal/replay"
)

const methodNotAllowed = "method not allowed"

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/connect", s.handleConnectPage)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/requests", s.handleRequests)
	mux.HandleFunc("/api/requests/", s.handleRequestByID)
	mux.HandleFunc("/api/replay", s.handleReplay)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/rules/", s.handleRuleByID)
	mux.HandleFunc("/ws/events", s.handleEventsWS)
	mux.HandleFunc("/ws/tunnel", s.handleTunnelWS)
	mux.HandleFunc("/tunnel/connect", s.handleBinaryRegister)
	mux.HandleFunc("/t/", s.handlePathTunnel)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if tunnel := s.manager.FindByHost(r.Host); tunnel != nil {
		s.handlePublicRequest(w, r, tunnel)
		return
	}
	if token := r.URL.Query().Get("token"); token != "" {
		http.SetCookie(w, &http.Cookie{Name: "vexlo_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	dashboard.ServeHTML(w, r)
}

func (s *Server) handleConnectPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Vexlo Connect</title></head><body style="background:#0d0d0d;color:#e5e5e5;font-family:monospace;padding:24px"><h1>Vexlo Browser Client</h1><p>Open the main dashboard and use the browser client script from the footer. This endpoint is reserved for browser tunnel usage.</p></body></html>`))
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	targetPort := tunnel.ResolveTargetPort(r)
	start := time.Now()
	resp, err := s.manager.Forward(ctx, tunnel, r, body, targetPort)
	if err != nil {
		http.Error(w, "tunnel unavailable: "+err.Error(), http.StatusBadGateway)
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
		ResponseBody:    captureBody(resp.Body, s.cfg.CaptureBodyLimit),
		DurationMS:      time.Since(start).Milliseconds(),
		CreatedAt:       time.Now().UTC(),
		DecodedHeaders:  flattenHeaders(r.Header),
	}
	s.db.InsertRequest(record)
	s.hub.Broadcast(dashboard.Event{
		Type:      "new_request",
		SessionID: tunnel.session.ID,
		Payload:   record,
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	sessions, err := s.db.ListSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sessions)
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	requests, err := s.db.ListRequests(sessionID, r.URL.Query().Get("method"), r.URL.Query().Get("path"), r.URL.Query().Get("status"), r.URL.Query().Get("search"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, requests)
}

func (s *Server) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	if _, err := s.authorize(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/requests/")
	item, err := s.db.GetRequest(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, item)
}

type replayPayload struct {
	RequestID string            `json:"request_id"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	sessionID, err := s.authorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var payload replayPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	original, err := s.db.GetRequest(payload.RequestID)
	if err != nil {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}
	tunnel := s.manager.FindBySession(sessionID)
	if tunnel == nil {
		http.Error(w, "session tunnel offline", http.StatusBadGateway)
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
	req, _ := http.NewRequestWithContext(r.Context(), original.Method, "http://replay"+original.Path, strings.NewReader(payload.Body))
	req.URL.RawQuery = original.Query
	req.Header = headers
	if payload.Body == "" {
		payload.Body = original.Body
		req.Body = io.NopCloser(strings.NewReader(payload.Body))
	}

	targetPort := tunnel.ResolveTargetPort(req)
	start := time.Now()
	resp, err := s.manager.Forward(r.Context(), tunnel, req, []byte(payload.Body), targetPort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	diff, _ := replay.CompareBodies(original.ResponseBody, string(resp.Body))
	replayRecord := model.CapturedReplay{
		ID:             randomID(8),
		RequestID:      original.ID,
		MutatedHeaders: marshalFlatHeaders(payload.Headers),
		MutatedBody:    payload.Body,
		ResponseStatus: resp.StatusCode,
		ResponseHeader: marshalHeaders(resp.Headers),
		ResponseBody:   string(resp.Body),
		DiffResult:     replay.MustJSON(diff),
		DurationMS:     time.Since(start).Milliseconds(),
		CreatedAt:      time.Now().UTC(),
	}
	s.db.InsertReplay(replayRecord)
	writeJSON(w, replayRecord)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.authorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules, err := s.db.ListRules(sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, rules)
	case http.MethodPost:
		var rule model.RoutingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		rule.ID = randomID(8)
		rule.SessionID = sessionID
		if rule.Priority == 0 {
			rule.Priority = int(time.Now().UnixMilli() % 100000)
		}
		if err := s.db.InsertRule(rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if tunnel := s.manager.FindBySession(sessionID); tunnel != nil {
			rules, _ := s.db.ListRules(sessionID)
			tunnel.SetRules(rules)
		}
		writeJSON(w, rule)
	default:
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.authorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/rules/")
	if err := s.db.DeleteRule(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tunnel := s.manager.FindBySession(sessionID); tunnel != nil {
		rules, _ := s.db.ListRules(sessionID)
		tunnel.SetRules(rules)
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
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

func parseInt(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}
