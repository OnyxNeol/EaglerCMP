package manager

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// API paths for the NaOHX mod manager UI.
const (
	modsPath           = "/__eaglercmp/mods"
	backendRestartPath = "/__eaglercmp/backend/restart"
	maxModSize         = 128 << 20
)

var errNotJar = errors.New("file is not a valid .jar (missing zip signature or empty)")

var modNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+() -]{0,120}\.jar$`)

// Mods lists, stores and removes .jar mods in the backend's mods directory.
type Mods struct{ Backend *Backend }

type modInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", contentTypes[".json"])
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (m *Mods) list() []modInfo {
	out := []modInfo{}
	entries, _ := os.ReadDir(m.Backend.ModsDir())
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() && filepath.Ext(e.Name()) == ".jar" {
			out = append(out, modInfo{e.Name(), info.Size()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ServeHTTP handles GET (list + backend status), POST ?name= (raw .jar body)
// and DELETE ?name=. Mutating calls need the X-EaglerCMP header, which a
// cross-site page cannot send without a CORS preflight this server never grants.
func (m *Mods) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Header.Get("X-EaglerCMP") == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-EaglerCMP header"})
		return
	}
	name := r.URL.Query().Get("name")
	switch r.Method {
	case http.MethodGet:
		state, msg := m.Backend.Status()
		writeJSON(w, 200, map[string]any{
			"mods":    m.list(),
			"backend": map[string]any{"state": state, "error": msg},
		})
	case http.MethodPost:
		if !modNameRe.MatchString(name) {
			writeJSON(w, 400, map[string]string{"error": "invalid file name: use letters, digits and . _ + - ( ) and end with .jar"})
			return
		}
		if err := m.save(name, http.MaxBytesReader(w, r.Body, maxModSize)); err != nil {
			code := 400
			if os.IsPermission(err) {
				code = 500
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"mods": m.list()})
	case http.MethodDelete:
		if !modNameRe.MatchString(name) {
			writeJSON(w, 400, map[string]string{"error": "invalid file name"})
			return
		}
		if err := os.Remove(filepath.Join(m.Backend.ModsDir(), name)); err != nil && !os.IsNotExist(err) {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"mods": m.list()})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

// save streams body to a temp file, checks the zip signature, then renames it
// into place so the server never sees a partial jar.
func (m *Mods) save(name string, body io.Reader) error {
	dir := m.Backend.ModsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	head := make([]byte, 4)
	f, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	_, rerr := io.ReadFull(f, head)
	f.Close()
	if n == 0 || rerr != nil || string(head) != "PK\x03\x04" {
		return errNotJar
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// RestartHandler restarts the backend JVM so newly uploaded mods load.
func RestartHandler(b *Backend) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-EaglerCMP") == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "POST with X-EaglerCMP header required"})
			return
		}
		if !b.Cfg.Enabled {
			writeJSON(w, 409, map[string]string{"error": "backend is disabled in eaglercmp.json"})
			return
		}
		b.Restart()
		writeJSON(w, 200, map[string]string{"status": "restarting"})
	})
}
