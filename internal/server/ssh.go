package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"

	"vexlo/internal/protocol"
)

type sshConnState struct {
	raw        net.Conn
	conn       *ssh.ServerConn
	pubkey     string
	session    *protocol.Registered
	tunnel     *Tunnel
	localPort  int
	cancelConn context.CancelFunc
}

func (s *Server) acceptSSH(ctxDone interface{ Done() <-chan struct{} }) {
	signer, err := sshServerSigner()
	if err != nil {
		slog.Error("ssh signer error", "error", err)
		return
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{Extensions: map[string]string{"pubkey": string(ssh.MarshalAuthorizedKey(key))}}, nil
		},
	}
	cfg.AddHostKey(signer)

	go func() {
		<-ctxDone.Done()
		_ = s.sshLn.Close()
	}()

	for {
		conn, err := s.sshLn.Accept()
		if err != nil {
			return
		}
		go s.handleSSHConn(conn, cfg)
	}
}

func (s *Server) handleSSHConn(raw net.Conn, cfg *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		slog.Warn("ssh connection failed", "remote_addr", raw.RemoteAddr().String(), "error", err)
		_ = raw.Close()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &sshConnState{
		raw:        raw,
		conn:       sshConn,
		pubkey:     sshConn.Permissions.Extensions["pubkey"],
		localPort:  3000,
		cancelConn: cancel,
	}

	go func() {
		defer cancel()
		_ = sshConn.Wait()
		if state.tunnel != nil {
			s.manager.closeTunnel(state.tunnel)
		}
	}()
	slog.Info("ssh connection accepted", "remote_addr", raw.RemoteAddr().String())

	go s.handleSSHRequests(ctx, state, reqs)
	for ch := range chans {
		switch ch.ChannelType() {
		case "session":
			go s.handleSSHSessionChannel(state, ch)
		case "tunnel":
			go s.handleSSHCustomTunnel(state, ch)
		default:
			_ = ch.Reject(ssh.UnknownChannelType, "unsupported channel")
		}
	}
}

func (s *Server) handleSSHRequests(ctx context.Context, state *sshConnState, reqs <-chan *ssh.Request) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-reqs:
			if !ok {
				return
			}
			switch req.Type {
			case "tcpip-forward":
				s.handleSSHTCPIPForward(state, req)
			case "cancel-tcpip-forward", "keepalive@openssh.com":
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
			default:
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
			}
		}
	}
}

func (s *Server) handleSSHTCPIPForward(state *sshConnState, req *ssh.Request) {
	var payload struct {
		BindAddr string
		BindPort uint32
	}
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		if req.WantReply {
			_ = req.Reply(false, nil)
		}
		return
	}

	reg := protocol.Register{
		LocalPort:      int(payload.BindPort),
		ConnectionType: "ssh",
		PublicKey:      state.pubkey,
	}
	registered, tunnel, err := s.manager.RegisterForwarder(reg, sshRemoteForwarder(state, payload.BindAddr, payload.BindPort), func() error {
		state.cancelConn()
		return state.raw.Close()
	})
	if err != nil {
		if req.WantReply {
			_ = req.Reply(false, nil)
		}
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	state.session = registered
	state.tunnel = tunnel
	state.localPort = int(payload.BindPort)
	if req.WantReply {
		_ = req.Reply(true, ssh.Marshal(struct{ Port uint32 }{Port: payload.BindPort}))
	}
	slog.Info("ssh remote forward ready",
		"session_id", tunnel.session.ID,
		"connect_url", registered.ConnectURL,
		"local_port", state.localPort,
	)
}

func sshRemoteForwarder(state *sshConnState, bindAddr string, bindPort uint32) forwarderFunc {
	if bindAddr == "" {
		bindAddr = "0.0.0.0"
	}
	return func(ctx context.Context, r *http.Request, body []byte, _ int) (*protocol.ForwardResponse, error) {
		type forwarded struct {
			ConnectedAddress  string
			ConnectedPort     uint32
			OriginatorAddress string
			OriginatorPort    uint32
		}
		channel, reqs, err := state.conn.OpenChannel("forwarded-tcpip", ssh.Marshal(forwarded{
			ConnectedAddress:  bindAddr,
			ConnectedPort:     bindPort,
			OriginatorAddress: "127.0.0.1",
			OriginatorPort:    0,
		}))
		if err != nil {
			return nil, err
		}
		go ssh.DiscardRequests(reqs)
		defer func() { _ = channel.Close() }()

		writeReq, err := cloneHTTPRequest(ctx, r, body)
		if err != nil {
			return nil, err
		}
		start := time.Now()
		if err := writeReq.Write(channel); err != nil {
			return nil, err
		}
		resp, err := http.ReadResponse(bufio.NewReader(channel), writeReq)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return &protocol.ForwardResponse{
			ID:         randomID(8),
			StatusCode: resp.StatusCode,
			Headers:    resp.Header.Clone(),
			Body:       respBody,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}
}

func cloneHTTPRequest(ctx context.Context, r *http.Request, body []byte) (*http.Request, error) {
	target := "http://vexlo" + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = r.Header.Clone()
	req.Host = r.Host
	return req, nil
}

func (s *Server) handleSSHSessionChannel(state *sshConnState, newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer func() { _ = channel.Close() }()

	for req := range requests {
		switch req.Type {
		case "pty-req", "env":
			_ = req.Reply(true, nil)
		case "shell", "exec":
			_ = req.Reply(true, nil)
			s.writeSSHBanner(channel, state)
		default:
			_ = req.Reply(false, nil)
		}
	}
}

func (s *Server) writeSSHBanner(w io.Writer, state *sshConnState) {
	if state.session == nil {
		_, _ = io.WriteString(w, "Vexlo connected. Waiting for remote forward registration.\nUse: ssh -N -R 80:localhost:3000 <host>\n")
		return
	}
	_, _ = io.WriteString(w, fmt.Sprintf("Vexlo tunnel ready\nPublic URL: %s\nDashboard: %s\n", state.session.ConnectURL, state.session.DashboardURL))
}

func (s *Server) handleSSHCustomTunnel(state *sshConnState, newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	reg := protocol.Register{LocalPort: state.localPort, ConnectionType: "ssh", PublicKey: state.pubkey}
	registered, tunnel, err := s.manager.Register(sshServerChannelConn{Channel: channel, parent: state.raw}, reg)
	if err != nil {
		_ = channel.Close()
		return
	}
	rules, _ := s.db.ListRules(tunnel.session.ID)
	tunnel.SetRules(rules)
	state.session = registered
	state.tunnel = tunnel
	_ = protocol.Encode(channel, protocol.TypeRegistered, registered)
}

func sshServerSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}

type sshServerChannelConn struct {
	ssh.Channel
	parent net.Conn
}

func (c sshServerChannelConn) LocalAddr() net.Addr               { return c.parent.LocalAddr() }
func (c sshServerChannelConn) RemoteAddr() net.Addr              { return c.parent.RemoteAddr() }
func (c sshServerChannelConn) SetDeadline(t time.Time) error     { return c.parent.SetDeadline(t) }
func (c sshServerChannelConn) SetReadDeadline(t time.Time) error { return c.parent.SetReadDeadline(t) }
func (c sshServerChannelConn) SetWriteDeadline(t time.Time) error {
	return c.parent.SetWriteDeadline(t)
}
