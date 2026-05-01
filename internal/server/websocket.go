package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"vexlo/internal/dashboard"
	"vexlo/internal/protocol"
)

func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.authorize(r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, err.Error(), err)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.WarnContext(r.Context(), "websocket accept failed", logAttrs(r, "error", err.Error())...)
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	client := &dashboard.Client{SessionID: sessionID, Send: make(chan []byte, 128)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)
	slog.InfoContext(r.Context(), "events websocket connected", logAttrs(r, "session_id", sessionID)...)

	for {
		select {
		case <-r.Context().Done():
			slog.InfoContext(r.Context(), "events websocket disconnected", logAttrs(r, "session_id", sessionID)...)
			return
		case msg := <-client.Send:
			if err := conn.Write(r.Context(), websocket.MessageText, msg); err != nil {
				slog.WarnContext(r.Context(), "events websocket write failed", logAttrs(r, "session_id", sessionID, "error", err.Error())...)
				return
			}
		}
	}
}

func (s *Server) handleTunnelWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.WarnContext(r.Context(), "tunnel websocket accept failed", logAttrs(r, "error", err.Error())...)
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	port := parseInt(r.URL.Query().Get("port"))
	if port == 0 {
		port = 3000
	}
	netConn := websocket.NetConn(context.Background(), conn, websocket.MessageBinary)
	reg := protocol.Register{LocalPort: port, ConnectionType: "websocket"}
	registered, tunnel, err := s.manager.Register(netConn, reg)
	if err != nil {
		slog.WarnContext(r.Context(), "tunnel registration failed", logAttrs(r, "error", err.Error())...)
		_ = conn.Close(websocket.StatusInternalError, "registration failed")
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	_ = protocol.Encode(netConn, protocol.TypeRegistered, registered)
	<-tunnel.closed
}
