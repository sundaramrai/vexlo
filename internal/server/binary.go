package server

import (
	"bufio"
	"log"
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
		log.Printf("binary register failed: %v", err)
		_ = conn.Close()
		return
	}
	registered, tunnel, err := s.manager.Register(conn, reg)
	if err != nil {
		_ = conn.Close()
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	_ = protocol.Encode(conn, protocol.TypeRegistered, registered)
	<-tunnel.closed
}

func (s *Server) handleBinaryRegister(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "use raw TCP or websocket tunnel registration", http.StatusNotImplemented)
}

func bufioReader(conn net.Conn) *bufio.Reader {
	return bufio.NewReader(conn)
}
