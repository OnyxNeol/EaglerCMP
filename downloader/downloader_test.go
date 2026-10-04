package downloader

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchVerifiesHash(t *testing.T) {
	body := []byte("naohx-payload")
	sum := sha1.Sum(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	dir := t.TempDir()
	c := New(nil)
	c.Retries = 0

	good := Task{URL: srv.URL, Path: filepath.Join(dir, "a", "good.bin"), Hash: Hash{"sha1", hex.EncodeToString(sum[:])}}
	if dl, err := c.Fetch(context.Background(), good); err != nil || !dl {
		t.Fatalf("first fetch: dl=%v err=%v", dl, err)
	}
	if dl, err := c.Fetch(context.Background(), good); err != nil || dl {
		t.Fatalf("cached fetch should skip download: dl=%v err=%v", dl, err)
	}

	bad := Task{URL: srv.URL, Path: filepath.Join(dir, "bad.bin"), Hash: Hash{"sha1", "0000000000000000000000000000000000000000"}}
	if _, err := c.Fetch(context.Background(), bad); err == nil {
		t.Fatal("expected hash mismatch error")
	}
	if _, err := os.Stat(bad.Path); !os.IsNotExist(err) {
		t.Fatal("corrupt download must not be left on disk")
	}
}

func TestExtractRejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	f, _ := os.Create(archive)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escape.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if err := Extract(archive, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected zip-slip rejection")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Fatal("file escaped extraction directory")
	}
}

const page = `<!DOCTYPE html><html><head><title>t</title><script>window.eaglercraftXOpts = {container: "game_frame"};</script></head><body></body></html>`

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportSingleFileAndVerify(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "eaglercraft-26.2.html")
	write(t, src, page)
	dest := filepath.Join(dir, "inst", "client")
	m, err := New(nil).Import(context.Background(), src, "", dest, dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Entry != "eaglercraft-26.2.html" || len(m.Files) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	if _, err := VerifyClient(dest); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dest, m.Entry), page+"<!-- tampered -->")
	if _, err := VerifyClient(dest); err == nil {
		t.Fatal("expected verification failure after tampering")
	}
}

func TestImportZipFlattensAndPrefersIndex(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "client.zip")
	f, _ := os.Create(archive)
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"web/index.html":      page,
		"web/other.html":      page + strings.Repeat(" ", 100),
		"web/classes.wasm.br": "wasm",
		"web/lang/en_us.json": "{}",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
	dest := filepath.Join(dir, "client")
	m, err := New(nil).Import(context.Background(), archive, "", dest, dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Entry != "index.html" || m.Files["lang/en_us.json"] == "" {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestImportRejectsRedirectStub(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "262")
	write(t, filepath.Join(src, "index.html"), `<meta http-equiv="refresh" content="0;url=https://cdn.example/client.html">`)
	write(t, filepath.Join(src, "classes.wasm.br"), "x")
	_, err := New(nil).Import(context.Background(), src, "", filepath.Join(dir, "client"), dir)
	if err == nil || !strings.Contains(err.Error(), "https://cdn.example/client.html") {
		t.Fatalf("expected redirect error naming the target, got %v", err)
	}
}

func TestImportURLChecksSHA256(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(page)) }))
	defer srv.Close()
	dir := t.TempDir()
	c := New(nil)
	c.Retries = 0
	sum := sha256.Sum256([]byte(page))
	if _, err := c.Import(context.Background(), srv.URL+"/client.html", hex.EncodeToString(sum[:]), filepath.Join(dir, "client"), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Import(context.Background(), srv.URL+"/bad.html", strings.Repeat("0", 64), filepath.Join(dir, "client2"), dir); err == nil {
		t.Fatal("expected sha256 mismatch")
	}
}

func TestImportFollowsRedirectStub(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/262/index.html":
			w.Write([]byte(`<meta http-equiv="refresh" content="0;url=` + srv.URL + `/cdn/eaglercraft-26.2.html?v=1">`))
		case "/rel/index.html":
			w.Write([]byte(`<meta http-equiv="refresh" content="0;url=../262/index.html">`))
		case "/loop/index.html":
			w.Write([]byte(`<meta http-equiv="refresh" content="0;url=index.html">`))
		case "/cdn/eaglercraft-26.2.html":
			w.Write([]byte(page))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := New(nil)
	c.Retries = 0
	for _, p := range []string{"/262/index.html", "/rel/index.html"} {
		m, err := c.Import(context.Background(), srv.URL+p, "", filepath.Join(dir, "client"), dir)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if m.Entry != "eaglercraft-26.2.html" || m.Source != srv.URL+p {
			t.Fatalf("%s: entry=%q source=%q", p, m.Entry, m.Source)
		}
	}
	if _, err := c.Import(context.Background(), srv.URL+"/loop/index.html", "", filepath.Join(dir, "client"), dir); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("expected redirect loop error, got %v", err)
	}
}

func TestIsGamePage(t *testing.T) {
	dir := t.TempDir()
	site := filepath.Join(dir, "index.html")
	os.WriteFile(site, []byte(`<html><title>Eagler Versions · Web Launcher</title><a href="262/">26.2</a></html>`), 0o644)
	if IsGamePage(site) {
		t.Fatal("launcher site detected as game page")
	}
	// Needle straddling the 1 MiB read boundary.
	big := filepath.Join(dir, "game.html")
	data := append([]byte(strings.Repeat("x", (1<<20)-5)), []byte("window.eaglercraftXOpts = {}")...)
	os.WriteFile(big, data, 0o644)
	if !IsGamePage(big) {
		t.Fatal("game page not detected across read boundary")
	}
	if _, err := findEntry(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(big)
	if _, err := findEntry(dir); err == nil {
		t.Fatal("launcher site accepted as entry")
	}
}
