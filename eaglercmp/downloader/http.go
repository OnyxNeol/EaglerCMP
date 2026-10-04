// Package downloader fetches, unpacks and verifies Eaglercraft web client
// bundles (single-file HTML builds or zipped/tarred static folders).
package downloader

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserAgent identifies the launcher to remote hosts.
const UserAgent = "EaglerCMP/0.1.0 (+https://github.com/OnyxNeol/eaglercmp)"

// Hash is an expected digest for a file. Algo is "sha1", "sha256" or "sha512".
type Hash struct {
	Algo  string
	Value string
}

// Empty reports whether no digest is known.
func (h Hash) Empty() bool { return h.Value == "" }

func (h Hash) newHasher() (hash.Hash, error) {
	switch strings.ToLower(h.Algo) {
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	}
	return nil, fmt.Errorf("unsupported hash algorithm %q", h.Algo)
}

// Task is one file to download.
type Task struct {
	URL  string
	Path string
	Hash Hash
	Size int64
	// TrustSize skips re-hashing an existing file whose size matches. Used for
	// content-addressed assets, which are still hash-verified when downloaded.
	TrustSize bool
}

// Client wraps an http.Client with retries and verification.
type Client struct {
	HTTP    *http.Client
	Retries int
	Log     func(format string, args ...any)
}

// New returns a Client with sane defaults.
func New(logf func(string, ...any)) *Client {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Minute}, Retries: 3, Log: logf}
}

func (c *Client) get(ctx context.Context, url string, header map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// VerifyFile checks that path exists and matches h (and size, if > 0).
func VerifyFile(path string, h Hash, size int64) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if size > 0 && st.Size() != size {
		return false, nil
	}
	if h.Empty() {
		return true, nil
	}
	hs, err := h.newHasher()
	if err != nil {
		return false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := io.Copy(hs, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(hs.Sum(nil)), h.Value), nil
}

// Fetch downloads t unless an up-to-date, verified copy already exists.
// It returns true if a download happened.
func (c *Client) Fetch(ctx context.Context, t Task) (bool, error) {
	if t.TrustSize && t.Size > 0 {
		if st, err := os.Stat(t.Path); err == nil && st.Size() == t.Size {
			return false, nil
		}
	}
	if ok, err := VerifyFile(t.Path, t.Hash, t.Size); err != nil {
		return false, err
	} else if ok && (!t.Hash.Empty() || t.Size > 0) {
		return false, nil
	}
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, attempt); err != nil {
				return false, err
			}
		}
		if lastErr = c.fetchOnce(ctx, t); lastErr == nil {
			return true, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
	}
	return false, fmt.Errorf("download %s: %w", t.URL, lastErr)
}

func (c *Client) fetchOnce(ctx context.Context, t Task) error {
	resp, err := c.get(ctx, t.URL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(t.Path), ".dl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	var w io.Writer = tmp
	var hs hash.Hash
	if !t.Hash.Empty() {
		if hs, err = t.Hash.newHasher(); err != nil {
			tmp.Close()
			return err
		}
		w = io.MultiWriter(tmp, hs)
	}
	n, err := io.Copy(w, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if t.Size > 0 && n != t.Size {
		return fmt.Errorf("size mismatch: got %d, want %d", n, t.Size)
	}
	if hs != nil {
		if got := hex.EncodeToString(hs.Sum(nil)); !strings.EqualFold(got, t.Hash.Value) {
			return fmt.Errorf("%s mismatch: got %s, want %s", t.Hash.Algo, got, t.Hash.Value)
		}
	}
	return os.Rename(tmp.Name(), t.Path)
}

func sleep(ctx context.Context, attempt int) error {
	select {
	case <-time.After(time.Duration(attempt*attempt) * 500 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
