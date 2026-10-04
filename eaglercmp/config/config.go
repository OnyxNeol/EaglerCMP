// Package config holds the on-disk configuration for the EaglerCMP desktop
// client and the directory layout of a managed instance.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Server is a multiplayer entry handed to the Eaglercraft client. Addr is a
// WebSocket URL such as "wss://play.example.net".
type Server struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// Config is persisted as eaglercmp.json in the instance root.
type Config struct {
	// ClientSource is where `init`/`launch`/`update` import the Eaglercraft
	// web client from: a local .html file, a directory, a .zip/.tar.gz
	// archive, or an http(s) URL to any of those. Empty means the client
	// bundled into the binary, then UpstreamClient.
	ClientSource string `json:"clientSource,omitempty"`
	// ClientSHA256 optionally pins the SHA-256 of a single-file source.
	ClientSHA256 string `json:"clientSha256,omitempty"`

	// Port is the fixed loopback port of the local asset server. It must stay
	// stable: worlds, settings and resource packs live in the window's
	// IndexedDB/localStorage, which the browser engine scopes per origin.
	Port int `json:"port"`

	// Browser is the path to a Chromium-based browser (Chrome, Edge, Chromium,
	// Brave) used as the app window. Empty means auto-detect; "system" opens
	// the default browser instead of a dedicated window.
	Browser     string   `json:"browser,omitempty"`
	BrowserArgs []string `json:"browserArgs,omitempty"`

	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Fullscreen bool `json:"fullscreen,omitempty"`

	// Performance enables the Sodium HX Graphics (NaOHX) window profile.
	Performance bool `json:"performance"`

	// Servers are merged into eaglercraftXOpts.servers before the client boots.
	Servers []Server `json:"servers,omitempty"`
	// Backend is the optional local Java (Fabric/Paper) server bridged to the client.
	Backend Backend `json:"backend"`
	// ClientOptions are merged into eaglercraftXOpts (e.g. {"demoMode": false}).
	ClientOptions map[string]any `json:"clientOptions,omitempty"`
}

// Backend configures the local JVM server supervised by the daemon. The server
// must speak the Eaglercraft WebSocket protocol itself (e.g. an EaglerXServer /
// Eaglercraft-for-Fabric plugin), listening on 127.0.0.1:WSPort; the daemon
// only supervises it and tunnels the client's WebSocket to it.
type Backend struct {
	Enabled bool     `json:"enabled"`
	Name    string   `json:"name,omitempty"`    // label in the multiplayer list
	Java    string   `json:"java,omitempty"`    // java binary; default "java"
	Dir     string   `json:"dir,omitempty"`     // server directory (world, mods/); default <instance>/backend
	Jar     string   `json:"jar,omitempty"`     // server jar, relative to Dir (e.g. fabric-server-launch.jar)
	JVMArgs []string `json:"jvmArgs,omitempty"` // e.g. ["-Xmx2G"]
	Args    []string `json:"args,omitempty"`    // after -jar <jar>; default ["nogui"]
	WSPort  int      `json:"wsPort,omitempty"`  // loopback Eaglercraft WebSocket port; default 8081
}

// FileName is the config file name inside the instance root.
const FileName = "eaglercmp.json"

// UpstreamClient is the Eaglercraft 26.2 client of the upstream project
// (github.com/eymenwsmc/playopspt.github.io, 262/). Its index.html redirects
// to the current self-contained build, which the importer follows.
const UpstreamClient = "https://raw.githubusercontent.com/eymenwsmc/playopspt.github.io/main/262/index.html"

// DefaultPort is the loopback port used unless configured otherwise.
const DefaultPort = 47262

// Default returns the configuration written on first run.
func Default() *Config {
	return &Config{
		Port:        DefaultPort,
		Width:       1280,
		Height:      720,
		Performance: true,
	}
}

// Load reads the config at path, creating it with defaults if it does not exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg := Default()
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

// Save writes the config as indented JSON.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Validate checks for values that would produce a broken launch.
func (c *Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.Width <= 0 || c.Height <= 0 {
		return fmt.Errorf("invalid window size %dx%d", c.Width, c.Height)
	}
	for _, s := range c.Servers {
		if s.Addr == "" {
			return fmt.Errorf("server %q has no addr", s.Name)
		}
	}
	if b := c.Backend; b.Enabled {
		if b.Jar == "" {
			return fmt.Errorf("backend.jar is required when the backend is enabled")
		}
		if !strings.HasSuffix(strings.ToLower(b.Jar), ".jar") || filepath.IsAbs(b.Jar) || strings.Contains(b.Jar, "..") {
			return fmt.Errorf("backend.jar %q must be a .jar path relative to the server directory", b.Jar)
		}
		if b.WSPort == c.Port {
			return fmt.Errorf("backend.wsPort must differ from port %d", c.Port)
		}
		if b.WSPort < 0 || b.WSPort > 65535 {
			return fmt.Errorf("invalid backend.wsPort %d", b.WSPort)
		}
	}
	return nil
}

// Paths is the directory layout of an instance.
type Paths struct {
	Root    string // instance root
	Client  string // installed Eaglercraft web client (static files)
	Profile string // dedicated browser-engine profile: IndexedDB worlds, settings
	Cache   string // downloaded client archives
	Logs    string // launcher + client logs
}

// NewPaths derives the layout from an instance root.
func NewPaths(root string) Paths {
	return Paths{
		Root:    root,
		Client:  filepath.Join(root, "client"),
		Profile: filepath.Join(root, "profile"),
		Cache:   filepath.Join(root, "cache"),
		Logs:    filepath.Join(root, "logs"),
	}
}

// Ensure creates every directory in the layout except Client, which is
// created atomically on import.
func (p Paths) Ensure() error {
	for _, d := range []string{p.Root, p.Profile, p.Cache, p.Logs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// DefaultRoot returns the per-user data directory for EaglerCMP.
func DefaultRoot() string {
	if v := os.Getenv("EAGLERCMP_HOME"); v != "" {
		return v
	}
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "EaglerCMP")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "EaglerCMP")
		}
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "eaglercmp")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "eaglercmp")
	}
	return "eaglercmp-data"
}
