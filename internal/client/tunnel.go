package client

import (
	"bufio"
	"net"
)

func newReader(conn net.Conn) *bufio.Reader {
	return bufio.NewReader(conn)
}
