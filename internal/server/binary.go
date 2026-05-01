package server

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"

	"vexlo/internal/protocol"
)

func (s *Server) acceptBinary(ctxDone interface{ Done() <-chan struct{} }) {
	go func() {
		<-ctxDone.Done()
		_ = s.tcpLn.Close()
	}()
	for {
		conn, err := s.tcpLn.Accept()
		if err != nil {
			return
		}
		go s.handleBinaryConn(conn)
	}
}

func (s *Server) handleBinaryConn(conn net.Conn) {
	defer func() {
		if recover() != nil {
			_ = conn.Close()
		}
	}()
	reader := bufioReader(conn)
	var reg protocol.Register
	kind, err := protocol.Decode(reader, &reg)
	if err != nil || kind != protocol.TypeRegister {
		slog.Warn("binary register failed", "remote_addr", conn.RemoteAddr().String(), "error", err)
		_ = conn.Close()
		return
	}
	registered, tunnel, err := s.manager.Register(conn, reg)
	if err != nil {
		slog.Warn("binary tunnel registration failed", "remote_addr", conn.RemoteAddr().String(), "error", err)
		_ = conn.Close()
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	_ = protocol.Encode(conn, protocol.TypeRegistered, registered)
	<-tunnel.closed
}

func (s *Server) handleBinaryRegister(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, "use raw TCP or websocket tunnel registration", nil)
}

func bufioReader(conn net.Conn) *bufio.Reader {
	return bufio.NewReader(conn)
}
