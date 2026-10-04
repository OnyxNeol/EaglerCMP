package manager

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OnyxNeol/eaglercmp/config"
)

func newMods(t *testing.T) (*Mods, string) {
	root := t.TempDir()
	be := NewBackend("backend", 25566, config.Backend{Enabled: true, Jar: "s.jar"}, root, root, testLog(t))
	return &Mods{Backend: be}, be.ModsDir()
}

func do(m *Mods, method, url, body string, hdr bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	if hdr {
		r.Header.Set("X-EaglerCMP", "1")
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	return w
}

func TestModUploadListDelete(t *testing.T) {
	m, dir := newMods(t)
	if w := do(m, "POST", modsPath+"?name=a.jar", "PK\x03\x04zz", false); w.Code != 403 {
		t.Fatalf("missing header: %d", w.Code)
	}
	if w := do(m, "POST", modsPath+"?name=a.jar", "PK\x03\x04zz", true); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.jar")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../x.jar", "x.txt", "a/b.jar"} {
		if w := do(m, "POST", modsPath+"?name="+bad, "PK\x03\x04", true); w.Code != 400 {
			t.Fatalf("%s accepted: %d", bad, w.Code)
		}
	}
	if w := do(m, "POST", modsPath+"?name=b.jar", "not a zip", true); w.Code != 400 {
		t.Fatalf("non-zip accepted: %d", w.Code)
	}
	if w := do(m, "DELETE", modsPath+"?name=a.jar", "", true); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Fatalf("leftover files: %v", es)
	}
	_ = http.StatusOK
}

func TestBackendPreflight(t *testing.T) {
	root := t.TempDir()
	be := NewBackend("backend", 25566, config.Backend{Enabled: true, Jar: "s.jar", Java: "no-such-java"}, root, root, testLog(t))
	if err := be.Preflight(); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want missing jar error, got %v", err)
	}
	os.MkdirAll(be.Dir(), 0o755)
	os.WriteFile(filepath.Join(be.Dir(), "s.jar"), []byte("x"), 0o644)
	if err := be.Preflight(); err == nil || !strings.Contains(err.Error(), "Java not found") {
		t.Fatalf("want missing java error, got %v", err)
	}
}

func testLog(t *testing.T) *Logger {
	l, err := NewLogger(os.Stderr, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}
