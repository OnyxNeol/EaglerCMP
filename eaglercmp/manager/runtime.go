package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OnyxNeol/eaglercmp/bundle"
	"github.com/OnyxNeol/eaglercmp/config"
	"github.com/OnyxNeol/eaglercmp/downloader"
)

// Runtime prepares and runs one EaglerCMP instance.
type Runtime struct {
	Cfg   *config.Config
	Paths config.Paths
	Log   *Logger
	DL    *downloader.Client
}

// New returns a Runtime for cfg/paths.
func New(cfg *config.Config, paths config.Paths, log *Logger) *Runtime {
	return &Runtime{Cfg: cfg, Paths: paths, Log: log, DL: downloader.New(log.Printf)}
}

// ErrNoClient means no Eaglercraft client is installed and none is configured.
var ErrNoClient = errors.New("no Eaglercraft client installed")

// Import installs a client from src and remembers src as the client source.
func (r *Runtime) Import(ctx context.Context, src, sum string) (*downloader.Manifest, error) {
	if err := r.Paths.Ensure(); err != nil {
		return nil, err
	}
	if abs, err := filepath.Abs(src); err == nil && !strings.Contains(src, "://") {
		src = abs
	}
	m, err := r.DL.Import(ctx, src, sum, r.Paths.Client, r.Paths.Cache)
	if err != nil {
		return nil, err
	}
	r.Log.Printf("installed client %s: %d files, %.1f MB (source %s)", m.Entry, len(m.Files), float64(m.Bytes)/(1<<20), src)
	r.Cfg.ClientSource, r.Cfg.ClientSHA256 = src, sum
	return m, r.Cfg.Save(filepath.Join(r.Paths.Root, config.FileName))
}

// Prepare creates the instance layout, installs the client from clientSource
// if none is installed, verifies every client file and writes naohx.json.
func (r *Runtime) Prepare(ctx context.Context, skipVerify bool) (*downloader.Manifest, error) {
	if err := r.Paths.Ensure(); err != nil {
		return nil, err
	}
	if err := config.WriteBrandingFile(r.Paths.Root, config.Branding(r.Cfg.Performance, ProfileFlags(r.Cfg))); err != nil {
		return nil, err
	}
	m, err := downloader.ReadManifest(r.Paths.Client)
	if errors.Is(err, fs.ErrNotExist) {
		return r.install(ctx)
	}
	if err != nil {
		return nil, err
	}
	if reason := r.staleReason(m); reason != "" {
		r.Log.Printf("%s; reinstalling", reason)
		if m.Source == r.Cfg.ClientSource {
			// That source produced the broken client; fall back to the default.
			r.Cfg.ClientSource, r.Cfg.ClientSHA256 = "", ""
			if err := r.Cfg.Save(filepath.Join(r.Paths.Root, config.FileName)); err != nil {
				return nil, err
			}
		}
		return r.install(ctx)
	}
	if skipVerify {
		return m, nil
	}
	start := time.Now()
	if m, err = downloader.VerifyClient(r.Paths.Client); err != nil {
		return nil, err
	}
	r.Log.Printf("client verified: %s, %d files (sha256) in %s", m.Entry, len(m.Files), time.Since(start).Round(time.Millisecond))
	return m, nil
}

// staleReason explains why the installed client should be replaced: its entry
// page does not boot the game (e.g. a launcher site was imported), or it is an
// older client bundled with a different EaglerCMP build.
func (r *Runtime) staleReason(m *downloader.Manifest) string {
	if !downloader.IsGamePage(filepath.Join(r.Paths.Client, m.Entry)) {
		return fmt.Sprintf("installed client %s is not an Eaglercraft game page", m.Entry)
	}
	if strings.HasPrefix(m.Source, bundledPrefix) && len(bundle.Archive()) > 0 && m.Source != bundledLabel() {
		return "a newer client is bundled with this build"
	}
	return ""
}

// install picks the first working client: the configured clientSource, the
// client bundled into this binary, then the upstream repo's 26.2 client.
func (r *Runtime) install(ctx context.Context) (*downloader.Manifest, error) {
	if r.Cfg.ClientSource != "" {
		m, err := r.DL.Import(ctx, r.Cfg.ClientSource, r.Cfg.ClientSHA256, r.Paths.Client, r.Paths.Cache)
		if err == nil {
			return m, nil
		}
		r.Log.Printf("clientSource %s: %v; using the default client", r.Cfg.ClientSource, err)
	}
	if data := bundle.Archive(); len(data) > 0 {
		return r.installBundled(ctx, data)
	}
	return r.Update(ctx)
}

const bundledPrefix = "bundled with EaglerCMP "

func bundledLabel() string {
	label := bundledPrefix + config.LauncherVersion
	if s := bundle.Source(); s != "" {
		label += " (" + s + ")"
	}
	return label
}

func (r *Runtime) installBundled(ctx context.Context, data []byte) (*downloader.Manifest, error) {
	p := filepath.Join(r.Paths.Cache, "bundled-client.tar.gz")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return nil, err
	}
	defer os.Remove(p)
	label := bundledLabel()
	r.Log.Printf("installing the Eaglercraft client %s", label)
	m, err := r.DL.Import(ctx, p, "", r.Paths.Client, r.Paths.Cache)
	if err != nil {
		return nil, err
	}
	m.Source = label
	return m, downloader.SetSource(r.Paths.Client, label)
}

// Update (re)installs the client from clientSource, or from the upstream
// repo when none is configured. It needs network access for URL sources.
func (r *Runtime) Update(ctx context.Context) (*downloader.Manifest, error) {
	src, sum := r.Cfg.ClientSource, r.Cfg.ClientSHA256
	if src == "" {
		src, sum = config.UpstreamClient, ""
	}
	m, err := r.DL.Import(ctx, src, sum, r.Paths.Client, r.Paths.Cache)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrNoClient, src, err)
	}
	r.Log.Printf("installed client %s from %s", m.Entry, src)
	return m, nil
}

// URL is the loopback origin the client is served from.
func (r *Runtime) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", r.Cfg.Port)
}

// LaunchOptions tweaks a launch.
type LaunchOptions struct {
	NoWindow bool // serve only; the user opens the URL themselves
	Verbose  bool // stream every browser-engine log line, not just console output
}

// Command returns the browser command line used for the app window.
func (r *Runtime) Command() (string, []string, error) {
	bin, err := FindBrowser(r.Cfg.Browser)
	if err != nil {
		return "", nil, err
	}
	return bin, BrowserArgs(r.Cfg, r.Paths.Profile, r.URL()), nil
}

// Launch serves the client on the loopback interface and opens the app
// window. It returns when the window closes or ctx is cancelled.
func (r *Runtime) Launch(ctx context.Context, m *downloader.Manifest, opts LaunchOptions, console io.Writer) (int, error) {
	h, err := NewHandler(r.Paths.Client, m, r.Cfg, ProfileFlags(r.Cfg))
	if err != nil {
		return 1, err
	}
	be := NewBackend(r.Cfg.Backend, r.Paths.Root, r.Paths.Logs, r.Log)
	h.Mods, h.Restart = &Mods{Backend: be}, RestartHandler(be)
	if r.Cfg.Backend.Enabled {
		h.Bridge = &Bridge{Addr: be.Addr(), Allow: h.allowedHost}
		go be.Run(ctx)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", r.Cfg.Port))
	if err != nil {
		return 1, fmt.Errorf("port %d is busy (is EaglerCMP already running?): %w", r.Cfg.Port, err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	r.Log.Printf("serving %s from %s", m.Entry, r.URL())

	waitForCtrlC := func(msg string) (int, error) {
		r.Log.Printf("%s; press Ctrl+C to stop", msg)
		<-ctx.Done()
		return 0, nil
	}
	if opts.NoWindow {
		return waitForCtrlC("open " + r.URL() + " in a browser with WebAssembly GC support")
	}
	if r.Cfg.Browser == SystemBrowser {
		if err := OpenSystemBrowser(r.URL()); err != nil {
			return 1, err
		}
		return waitForCtrlC("opened the default browser")
	}
	bin, args, err := r.Command()
	if err != nil {
		r.Log.Printf("%v; falling back to the default browser", err)
		if err := OpenSystemBrowser(r.URL()); err != nil {
			return 1, err
		}
		return waitForCtrlC("opened " + r.URL())
	}

	r.Log.Printf("%s profile: %s", config.EngineShort, onOff(r.Cfg.Performance))
	r.Log.Printf("opening window: %s", bin)
	start := time.Now()
	code, err := r.runWindow(ctx, bin, args, opts.Verbose, console)
	if err != nil {
		return 1, err
	}
	// Chromium hands the URL to an already-running process for the same
	// profile and exits at once; keep serving until that browser closes.
	if ctx.Err() == nil && code == 0 && time.Since(start) < 3*time.Second {
		profile := r.Paths.Profile
		if profileInUse(profile) {
			r.Log.Printf("opened in the EaglerCMP window that was already running; serving until it closes")
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for profileInUse(profile) {
				select {
				case <-ctx.Done():
					return 0, nil
				case <-t.C:
				}
			}
			r.Log.Printf("window closed")
			return 0, nil
		}
		return waitForCtrlC("browser handed off to an existing window")
	}
	r.Log.Printf("window closed (exit %d)", code)
	return code, nil
}

func (r *Runtime) runWindow(ctx context.Context, bin string, args []string, verbose bool, console io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 1, err
	}
	logFile, err := os.Create(filepath.Join(r.Paths.Logs, "client-"+time.Now().Format("20060102-150405")+".log"))
	if err != nil {
		return 1, err
	}
	defer logFile.Close()
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamFiltered(stdout, "[client/out]", console, logFile, &mu, verbose) }()
	go func() { defer wg.Done(); streamFiltered(stderr, "[client/err]", console, logFile, &mu, verbose) }()
	wg.Wait()
	err = cmd.Wait()
	if ctx.Err() != nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

func onOff(b bool) string {
	if b {
		return "on (" + strings.Join(NaOHXFlags(), " ") + ")"
	}
	return "off"
}
