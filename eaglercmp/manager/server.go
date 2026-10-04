package manager

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/OnyxNeol/eaglercmp/config"
	"github.com/OnyxNeol/eaglercmp/downloader"
)

// Paths served by the launcher itself rather than from the client directory.
const (
	scriptPath   = "/__eaglercmp/naohx.js"
	brandingPath = "/__eaglercmp/naohx.json"
)

// contentTypes covers Eaglercraft's file types. Brotli payloads (.br) are
// decoded by the client itself, so they are served as opaque bytes without
// Content-Encoding, matching the upstream _headers file.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".json": "application/json",
	".wasm": "application/wasm",
	".br":   "application/octet-stream",
	".epk":  "application/octet-stream",
	".epw":  "application/octet-stream",
	".png":  "image/png",
	".css":  "text/css; charset=utf-8",
}

// Handler serves an installed client on the loopback interface. The entry
// page gets the NaOHX bootstrap script injected into its <head>.
type Handler struct {
	Dir      string
	Entry    string
	Port     int
	Script   []byte
	Branding []byte
	// Bridge, when set, tunnels WebSocket connections on bridgePath to the
	// local JVM backend.
	Bridge http.Handler
	// Mods and Restart serve the NaOHX mod manager API.
	Mods    http.Handler
	Restart http.Handler
	files   http.Handler
}

// NewHandler builds the asset server for an installed client.
func NewHandler(dir string, m *downloader.Manifest, cfg *config.Config, flags []string) (*Handler, error) {
	script, err := ClientScript(cfg, flags)
	if err != nil {
		return nil, err
	}
	branding, err := json.Marshal(config.Branding(cfg.Performance, flags))
	if err != nil {
		return nil, err
	}
	return &Handler{
		Dir: dir, Entry: m.Entry, Port: cfg.Port,
		Script: script, Branding: branding,
		files: http.FileServer(http.Dir(dir)),
	}, nil
}

func (h *Handler) allowedHost(host string) bool {
	hostname, port, err := net.SplitHostPort(host)
	if err != nil || port != strconv.Itoa(h.Port) {
		return false
	}
	return hostname == "127.0.0.1" || hostname == "localhost"
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Reject DNS-rebinding requests: only the loopback origin may load assets.
	if !h.allowedHost(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if h.Mods != nil {
		switch path.Clean(r.URL.Path) {
		case modsPath:
			h.checkOriginThen(w, r, h.Mods)
			return
		case backendRestartPath:
			h.checkOriginThen(w, r, h.Restart)
			return
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Bridge != nil && path.Clean(r.URL.Path) == bridgePath {
		h.Bridge.ServeHTTP(w, r)
		return
	}
	hdr := w.Header()
	hdr.Set("X-Content-Type-Options", "nosniff")
	p := path.Clean("/" + r.URL.Path)
	switch {
	case p == "/" || p == "/"+h.Entry:
		h.serveEntry(w, r)
		return
	case p == scriptPath:
		hdr.Set("Content-Type", contentTypes[".js"])
		hdr.Set("Cache-Control", "no-cache")
		w.Write(h.Script)
		return
	case p == introGIFPath:
		hdr.Set("Content-Type", "image/gif")
		hdr.Set("Cache-Control", "no-cache")
		w.Write(introGIF)
		return
	case p == introMP3Path:
		hdr.Set("Content-Type", "audio/mpeg")
		hdr.Set("Cache-Control", "no-cache")
		w.Write(introMP3)
		return
	case p == brandingPath:
		hdr.Set("Content-Type", contentTypes[".json"])
		hdr.Set("Cache-Control", "no-cache")
		w.Write(h.Branding)
		return
	case path.Base(p) == downloader.ManifestName:
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(filepath.Join(h.Dir, filepath.FromSlash(p))); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if ct, ok := contentTypes[strings.ToLower(path.Ext(p))]; ok {
		hdr.Set("Content-Type", ct)
	}
	hdr.Set("Cache-Control", "no-cache")
	h.files.ServeHTTP(w, r)
}

// checkOriginThen rejects cross-origin API calls before delegating.
func (h *Handler) checkOriginThen(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || !h.allowedHost(u.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
	}
	next.ServeHTTP(w, r)
}

var headRe = regexp.MustCompile(`(?i)<head(\s[^>]*)?>`)

// serveEntry streams the entry page with the bootstrap script inserted right
// after <head>, so it runs before any of the client's own scripts.
func (h *Handler) serveEntry(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(filepath.Join(h.Dir, h.Entry))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", contentTypes[".html"])
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		return
	}
	head := make([]byte, 256<<10)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	head = head[:n]
	tag := []byte(`<script src="` + scriptPath + `"></script>`)
	at := 0
	if loc := headRe.FindIndex(head); loc != nil {
		at = loc[1]
	} else if i := bytes.Index(bytes.ToLower(head), []byte("<html")); i >= 0 {
		if j := bytes.IndexByte(head[i:], '>'); j >= 0 {
			at = i + j + 1
		}
	}
	w.Write(head[:at])
	w.Write(tag)
	w.Write(head[at:])
	io.Copy(w, f)
}

// ClientScript is the NaOHX bootstrap injected into the entry page. It hooks
// window.eaglercraftXOpts so configured servers and options are merged in
// before the client boots, and applies the window branding.
func ClientScript(cfg *config.Config, flags []string) ([]byte, error) {
	servers := cfg.Servers
	if servers == nil {
		servers = []config.Server{}
	}
	bridge := ""
	if cfg.Backend.Enabled {
		bridge = cfg.Backend.Name
		if bridge == "" {
			bridge = "Local Modded Server"
		}
	}
	opts := cfg.ClientOptions
	if opts == nil {
		opts = map[string]any{}
	}
	data, err := json.Marshal(map[string]any{
		"title":         config.WindowTitle(),
		"engine":        config.EngineName + " (" + config.EngineShort + ")",
		"performance":   cfg.Performance,
		"servers":       servers,
		"bridge":        bridge,
		"bridgePath":    bridgePath,
		"clientOptions": opts,
		"branding":      config.Branding(cfg.Performance, flags),
	})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("// EaglerCMP - Sodium HX Graphics (NaOHX) bootstrap\n(function () {\n\"use strict\";\nvar naohx = ")
	b.Write(data)
	b.WriteString(`;
var opts;
if (naohx.bridge) naohx.servers = [{ name: naohx.bridge, addr: (location.protocol === "https:" ? "wss://" : "ws://") + location.host + naohx.bridgePath }].concat(naohx.servers);
function merge(v) {
	if (!v || typeof v !== "object") return v;
	for (var k in naohx.clientOptions) v[k] = naohx.clientOptions[k];
	if (naohx.servers.length) v.servers = naohx.servers.concat(Array.isArray(v.servers) ? v.servers : []);
	return v;
}
try {
	Object.defineProperty(window, "eaglercraftXOpts", {
		configurable: true, enumerable: true,
		get: function () { return opts; },
		set: function (v) { opts = merge(v); }
	});
} catch (e) { console.warn("[NaOHX] could not hook eaglercraftXOpts", e); }
window.eaglercmp = naohx.branding;
// Label the WebGL renderer/version strings (shown on the F3 debug screen) with
// the NaOHX profile. The original text stays first so parsers still work.
var tag = " | " + naohx.engine;
function label(proto) {
	if (!proto || !proto.getParameter) return;
	var get = proto.getParameter;
	proto.getParameter = function (p) {
		var v = get.apply(this, arguments);
		if (typeof v === "string" && (p === 0x1F01 || p === 0x1F02 || p === 0x9246)) v += tag;
		return v;
	};
}
if (naohx.performance) {
	label(window.WebGLRenderingContext && WebGLRenderingContext.prototype);
	label(window.WebGL2RenderingContext && WebGL2RenderingContext.prototype);
}
function brand() { document.title = naohx.title; }
document.addEventListener("DOMContentLoaded", brand);
window.addEventListener("load", brand);
console.log("[EaglerCMP] " + naohx.title + " v" + naohx.branding.launcherVersion + " | " + naohx.servers.length + " configured server(s)");
})();
`)
	b.WriteString(modUI)
	b.WriteString(introUI)
	return b.Bytes(), nil
}
