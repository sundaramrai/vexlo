package server

import (
	"context"
	"net/http"

	"github.com/coder/websocket"

	"vexlo/internal/dashboard"
	"vexlo/internal/protocol"
)

func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.authorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	client := &dashboard.Client{SessionID: sessionID, Send: make(chan []byte, 128)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-client.Send:
			if err := conn.Write(r.Context(), websocket.MessageText, msg); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleTunnelWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
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
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	_ = protocol.Encode(netConn, protocol.TypeRegistered, registered)
	<-tunnel.closed
}
