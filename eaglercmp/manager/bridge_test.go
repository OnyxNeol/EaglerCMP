package manager

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A fake backend answers the upgrade then echoes bytes; the tunnel must relay both.
func TestBridgeTunnel(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		for {
			l, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if l == "\r\n" {
				break
			}
		}
		c.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\n"))
		buf := make([]byte, 4)
		n, _ := br.Read(buf)
		c.Write(buf[:n])
	}()
	srv := httptest.NewServer(&Bridge{Addr: ln.Addr().String()})
	defer srv.Close()
	c, _ := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	defer c.Close()
	c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
	br := bufio.NewReader(c)
	st, _ := br.ReadString('\n')
	if !strings.Contains(st, "101") {
		t.Fatalf("status %q", st)
	}
	br.ReadString('\n')
	c.Write([]byte("ping"))
	buf := make([]byte, 4)
	br.Read(buf)
	if string(buf) != "ping" {
		t.Fatalf("got %q", buf)
	}
	_ = http.StatusOK
}
