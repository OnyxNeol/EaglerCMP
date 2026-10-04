package manager

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

// Event bridge: a Fabric server mod (or anything on loopback) POSTs a JSON
// object {"type": "...", ...} to eventsEmitPath; every open game page receives
// it over the Server-Sent Events stream at eventsPath and dispatches it to
// window.eaglercmp.on(type, fn) listeners.
const (
	eventsPath     = "/__eaglercmp/events"
	eventsEmitPath = "/__eaglercmp/emit"
	maxEventSize   = 4 << 10
)

// EventHub fans events out to connected pages.
type EventHub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func NewEventHub() *EventHub { return &EventHub{subs: map[chan []byte]struct{}{}} }

func (e *EventHub) publish(msg []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for c := range e.subs {
		select {
		case c <- msg:
		default: // slow page: drop rather than block the emitter
		}
	}
}

// ServeHTTP handles GET (SSE stream) and POST (emit, needs X-EaglerCMP).
func (e *EventHub) emit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("X-EaglerCMP") == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "POST with X-EaglerCMP header required"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventSize))
	var ev struct {
		Type string `json:"type"`
	}
	if err != nil || json.Unmarshal(body, &ev) != nil || ev.Type == "" {
		writeJSON(w, 400, map[string]string{"error": `body must be a JSON object with a "type" string (max 4 KB)`})
		return
	}
	compact, _ := json.Marshal(json.RawMessage(body))
	e.publish(compact)
	writeJSON(w, 200, map[string]string{"status": "sent"})
}

func (e *EventHub) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if r.Method != http.MethodGet || !ok {
		http.Error(w, "GET stream only", http.StatusMethodNotAllowed)
		return
	}
	c := make(chan []byte, 64)
	e.mu.Lock()
	e.subs[c] = struct{}{}
	e.mu.Unlock()
	defer func() { e.mu.Lock(); delete(e.subs, c); e.mu.Unlock() }()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	io.WriteString(w, ": ok\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-c:
			w.Write([]byte("data: "))
			w.Write(m)
			w.Write([]byte("\n\n"))
			fl.Flush()
		}
	}
}
