package server

import (
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"github.com/sundaramrai/vexlo/internal/dashboard"
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
	closeCtx := conn.CloseRead(r.Context())

	client := &dashboard.Client{SessionID: sessionID, Send: make(chan []byte, 128)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)
	slog.InfoContext(r.Context(), "events websocket connected", logAttrs(r, "session_id", sessionID)...)

	for {
		select {
		case <-closeCtx.Done():
			slog.InfoContext(r.Context(), "events websocket disconnected", logAttrs(r, "session_id", sessionID)...)
			return
		case msg := <-client.Send:
			if err := conn.Write(closeCtx, websocket.MessageText, msg); err != nil {
				slog.WarnContext(r.Context(), "events websocket write failed", logAttrs(r, "session_id", sessionID, "error", err.Error())...)
				return
			}
		}
	}
}
