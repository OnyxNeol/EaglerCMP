package manager

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OnyxNeol/eaglercmp/config"
	"github.com/OnyxNeol/eaglercmp/downloader"
)

func testHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "game.html"), []byte("<!DOCTYPE html><html><HEAD lang=en><title>x</title></head><body>ok</body></html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "classes.wasm.br"), []byte("br"), 0o644)
	os.WriteFile(filepath.Join(dir, downloader.ManifestName), []byte("{}"), 0o644)
	cfg := config.Default()
	cfg.Servers = []config.Server{{Name: "Arch", Addr: "wss://arch.example"}}
	h, err := NewHandler(dir, &downloader.Manifest{Entry: "game.html"}, cfg, ProfileFlags(cfg))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEntryGetsBootstrapInjected(t *testing.T) {
	h := testHandler(t)
	host := "127.0.0.1:47262"
	body := get(h, host, "/").Body.String()
	want := `<HEAD lang=en><script src="` + scriptPath + `"></script><title>`
	if !strings.Contains(body, want) {
		t.Fatalf("bootstrap not injected after <head>:\n%s", body)
	}
	js := get(h, host, scriptPath).Body.String()
	for _, s := range []string{"wss://arch.example", "eaglercraftXOpts", "Sodium HX Graphics (NaOHX)"} {
		if !strings.Contains(js, s) {
			t.Errorf("bootstrap missing %q", s)
		}
	}
}

func TestServerHeadersAndRestrictions(t *testing.T) {
	h := testHandler(t)
	host := "127.0.0.1:47262"
	if rec := get(h, host, "/classes.wasm.br"); rec.Code != 200 || rec.Header().Get("Content-Type") != "application/octet-stream" || rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("wasm.br: code=%d headers=%v", rec.Code, rec.Header())
	}
	if rec := get(h, host, "/"+downloader.ManifestName); rec.Code != 404 {
		t.Fatalf("manifest should be hidden, got %d", rec.Code)
	}
	if rec := get(h, "evil.example:47262", "/"); rec.Code != 403 {
		t.Fatalf("foreign Host should be rejected, got %d", rec.Code)
	}
	if rec := get(h, host, "/../../etc/passwd"); rec.Code != 404 {
		t.Fatalf("traversal should 404, got %d", rec.Code)
	}
}

func TestBrowserArgs(t *testing.T) {
	cfg := config.Default()
	args := BrowserArgs(cfg, "/p", "http://127.0.0.1:47262/")
	for _, want := range []string{"--app=http://127.0.0.1:47262/", "--user-data-dir=/p", "--enable-gpu-rasterization", "--window-size=1280,720"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	cfg.Performance = false
	if slices.Contains(BrowserArgs(cfg, "/p", "u"), "--enable-gpu-rasterization") {
		t.Error("NaOHX flags present with performance disabled")
	}
}

func TestConsoleMessage(t *testing.T) {
	msg, ok := consoleMessage(`[1:1:1003/071300.1:INFO:CONSOLE(12)] "hello world", source: http://127.0.0.1:47262/ (12)`)
	if !ok || msg != "hello world" {
		t.Fatalf("got %q %v", msg, ok)
	}
	if _, ok := consoleMessage("[1:1:INFO:gpu_init.cc] noise"); ok {
		t.Fatal("non-console line matched")
	}
}
