package client

import (
	"bufio"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func newReader(conn net.Conn) *bufio.Reader {
	return bufio.NewReader(conn)
}

type sshChannelConn struct {
	ssh.Channel
	client *ssh.Client
}

func (c sshChannelConn) LocalAddr() net.Addr                { return dummyAddr("ssh-local") }
func (c sshChannelConn) RemoteAddr() net.Addr               { return dummyAddr("ssh-remote") }
func (c sshChannelConn) SetDeadline(t time.Time) error      { return nil }
func (c sshChannelConn) SetReadDeadline(t time.Time) error  { return nil }
func (c sshChannelConn) SetWriteDeadline(t time.Time) error { return nil }
func (c sshChannelConn) Close() error {
	_ = c.Channel.Close()
	return c.client.Close()
}

type dummyAddr string

func (d dummyAddr) Network() string { return string(d) }
func (d dummyAddr) String() string  { return string(d) }
