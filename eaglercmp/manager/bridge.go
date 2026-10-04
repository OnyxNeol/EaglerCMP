package manager

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// bridgePath is where the client's WebSocket connects; it is tunnelled to the
// local JVM backend's Eaglercraft WebSocket listener.
const bridgePath = "/__eaglercmp/bridge"

// Bridge is a byte-level WebSocket tunnel. The backend (an Eaglercraft-aware
// Java server) performs the WebSocket handshake and speaks the Eaglercraft
// protocol itself, so no frames are parsed or translated here.
type Bridge struct {
	Addr  string // backend host:port, e.g. 127.0.0.1:8081
	Allow func(host string) bool
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket required", http.StatusBadRequest)
		return
	}
	// Block cross-site WebSocket hijacking: Origin must be this server.
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || (b.Allow != nil && !b.Allow(u.Host)) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
	}
	back, err := net.DialTimeout("tcp", b.Addr, 3*time.Second)
	if err != nil {
		http.Error(w, "backend not ready: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer back.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	r.Host = b.Addr
	r.Header.Set("Origin", "http://"+b.Addr)
	r.RequestURI = ""
	if err := r.Write(back); err != nil {
		fmt.Fprint(rw, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		rw.Flush()
		return
	}
	done := make(chan struct{}, 2)
	go func() { io.Copy(back, rw); done <- struct{}{} }()
	go func() { io.Copy(conn, back); done <- struct{}{} }()
	<-done
}
