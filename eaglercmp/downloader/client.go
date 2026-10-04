package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ManifestName is written into the installed client directory. It records
// where the client came from, its entry page and the SHA-256 of every file.
const ManifestName = ".eaglercmp-client.json"

// Manifest describes an installed Eaglercraft web client.
type Manifest struct {
	Source     string            `json:"source"`
	ImportedAt time.Time         `json:"importedAt"`
	Entry      string            `json:"entry"`
	Bytes      int64             `json:"bytes"`
	Files      map[string]string `json:"files"`
}

// Import installs the Eaglercraft web client at src into dest, replacing any
// previous client atomically. src may be an http(s) URL or a local path to a
// single-file .html build, a static directory, or a .zip/.tar.gz archive.
// If sha256 is set, the downloaded or local source file must match it.
func (c *Client) Import(ctx context.Context, src, sum, dest, cacheDir string) (*Manifest, error) {
	if src == "" {
		return nil, errors.New("no client source given")
	}
	local, err := c.fetchSource(ctx, src, cacheDir)
	if err != nil {
		return nil, err
	}
	// Follow <meta refresh> stubs such as the upstream repo's 262/index.html,
	// which points at the current self-contained client build.
	for cur, hop := src, 0; ; hop++ {
		t := redirectTarget(local)
		if t == "" {
			break
		}
		next, ok := resolveRef(cur, t)
		if !ok {
			return nil, fmt.Errorf("%s only redirects to %s; import that client instead", cur, t)
		}
		if hop == maxRedirects {
			return nil, fmt.Errorf("too many redirects importing %s", src)
		}
		c.Log("%s redirects to %s", cur, next)
		cur = next
		if local, err = c.fetchSource(ctx, cur, cacheDir); err != nil {
			return nil, err
		}
	}
	st, err := os.Stat(local)
	if err != nil {
		return nil, fmt.Errorf("client source: %w", err)
	}
	if sum != "" && !st.IsDir() {
		ok, err := VerifyFile(local, Hash{"sha256", sum}, 0)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s does not match sha256 %s", local, sum)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".client-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)

	lower := strings.ToLower(local)
	switch {
	case st.IsDir():
		err = copyTree(local, stage)
	case strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm"):
		err = copyFile(local, filepath.Join(stage, filepath.Base(local)))
	case strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		err = Extract(local, stage)
	default:
		err = fmt.Errorf("unsupported client source %q (want .html, directory, .zip or .tar.gz)", src)
	}
	if err != nil {
		return nil, err
	}

	root := singleSubdir(stage)
	entry, err := findEntry(root)
	if err != nil {
		return nil, err
	}
	m, err := hashTree(root)
	if err != nil {
		return nil, err
	}
	m.Source, m.Entry, m.ImportedAt = src, entry, time.Now().UTC()
	if err := writeManifest(root, m); err != nil {
		return nil, err
	}

	old := dest + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, old); err != nil {
			return nil, err
		}
	}
	if err := os.Rename(root, dest); err != nil {
		os.Rename(old, dest)
		return nil, err
	}
	os.RemoveAll(old)
	return m, nil
}

const maxRedirects = 3

// fetchSource downloads an http(s) source into cacheDir and returns the local
// path; local paths are returned unchanged.
func (c *Client) fetchSource(ctx context.Context, src, cacheDir string) (string, error) {
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return src, nil
	}
	name := path.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		name = "client.html"
	}
	local := filepath.Join(cacheDir, name)
	os.Remove(local)
	c.Log("downloading client from %s", src)
	if _, err := c.Fetch(ctx, Task{URL: src, Path: local}); err != nil {
		return "", err
	}
	return local, nil
}

// resolveRef resolves a redirect target against the source it came from and
// reports whether the result is a downloadable http(s) URL.
func resolveRef(base, ref string) (string, bool) {
	r, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if b, err := url.Parse(base); err == nil && (b.Scheme == "http" || b.Scheme == "https") {
		r = b.ResolveReference(r)
	}
	if r.Scheme != "http" && r.Scheme != "https" {
		return "", false
	}
	return r.String(), true
}

// SetSource records a human-readable source in an installed client's manifest.
func SetSource(dir, source string) error {
	m, err := ReadManifest(dir)
	if err != nil {
		return err
	}
	m.Source = source
	return writeManifest(dir, m)
}

// ReadManifest loads the manifest of an installed client.
func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ManifestName, err)
	}
	return m, nil
}

// VerifyClient re-hashes every file listed in the manifest.
func VerifyClient(dir string) (*Manifest, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	for rel, want := range m.Files {
		ok, err := VerifyFile(filepath.Join(dir, filepath.FromSlash(rel)), Hash{"sha256", want}, 0)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("client file %s is missing or modified; re-run `eaglercmp import`", rel)
		}
	}
	return m, nil
}

var refreshRe = regexp.MustCompile(`(?i)http-equiv=["']?refresh["']?[^>]*url=([^"'>\s]+)`)

// redirectTarget reports the URL an HTML stub only redirects to, if any.
func redirectTarget(p string) string {
	st, err := os.Stat(p)
	if err != nil || st.Size() > 8<<10 {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	if m := refreshRe.FindSubmatch(data); m != nil && !bytes.Contains(data, []byte("eaglercraftXOpts")) {
		return string(m[1])
	}
	return ""
}

// IsGamePage reports whether an HTML file boots the Eaglercraft client (it
// defines eaglercraftXOpts), as opposed to a launcher site or redirect page.
func IsGamePage(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	needle := []byte("eaglercraftXOpts")
	buf := make([]byte, 1<<20)
	carry := 0
	for {
		n, err := f.Read(buf[carry:])
		if bytes.Contains(buf[:carry+n], needle) {
			return true
		}
		if err != nil {
			return false
		}
		if carry+n >= len(needle) {
			carry = copy(buf, buf[carry+n-len(needle)+1:carry+n])
		} else {
			carry += n
		}
	}
}

// findEntry picks the page that boots the game: index.html if it is one,
// otherwise the largest top-level game page. Launcher sites, menus and
// redirect stubs are never used as the entry.
func findEntry(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var best, stub string
	var bestSize int64 = -1
	for _, e := range entries {
		name := e.Name()
		lower := strings.ToLower(name)
		if e.IsDir() || !(strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm")) {
			continue
		}
		p := filepath.Join(root, name)
		if t := redirectTarget(p); t != "" {
			stub = t
			continue
		}
		if !IsGamePage(p) {
			continue
		}
		if lower == "index.html" {
			return name, nil
		}
		if info, err := e.Info(); err == nil && info.Size() > bestSize {
			best, bestSize = name, info.Size()
		}
	}
	if best != "" {
		return best, nil
	}
	if stub != "" {
		return "", fmt.Errorf("the client's HTML only redirects to %s; import that client instead", stub)
	}
	return "", errors.New("no Eaglercraft game page (an HTML file defining eaglercraftXOpts) found in client source")
}

func singleSubdir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return dir
	}
	return filepath.Join(dir, entries[0].Name())
}

func hashTree(root string) (*Manifest, error) {
	m := &Manifest{Files: map[string]string{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("client contains non-regular file %s", p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ManifestName {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		m.Files[rel] = hex.EncodeToString(h.Sum(nil))
		m.Bytes += n
		return nil
	})
	return m, err
}

func writeManifest(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestName), append(data, '\n'), 0o644)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			if d.Name() == ManifestName {
				return nil
			}
			return copyFile(p, target)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeFile(dst, in, 0o644)
}
