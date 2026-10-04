package manager

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OnyxNeol/eaglercmp/config"
)

// Backend states reported to the UI.
const (
	StateDisabled = "disabled"
	StateStarting = "starting"
	StateReady    = "ready"
	StateError    = "error"
)

const (
	fastFailWindow = 20 * time.Second // exits sooner than this count as a failed start
	maxFastFails   = 3                // then stop retrying until restarted by the user
)

// Backend supervises the local JVM server (Fabric/Paper with Eaglercraft
// support and mods in <Dir>/mods) and restarts it with backoff if it dies.
// Problems are recorded in Status so the UI can report them; a server that
// cannot start is not retried in a loop.
type Backend struct {
	Cfg  config.Backend
	Root string // instance root, for the default Dir
	Logs string
	Log  *Logger

	mu     sync.Mutex
	state  string
	errMsg string
	cancel context.CancelFunc
	kick   chan struct{}
}

// NewBackend creates a supervisor; it only runs once Run is called.
func NewBackend(cfg config.Backend, root, logs string, log *Logger) *Backend {
	st := StateDisabled
	if cfg.Enabled {
		st = StateStarting
	}
	return &Backend{Cfg: cfg, Root: root, Logs: logs, Log: log, state: st, kick: make(chan struct{}, 1)}
}

// Dir is the server directory (world, mods/, jar).
func (b *Backend) Dir() string {
	if b.Cfg.Dir != "" {
		return b.Cfg.Dir
	}
	return filepath.Join(b.Root, "backend")
}

// ModsDir is where uploaded .jar mods are placed.
func (b *Backend) ModsDir() string { return filepath.Join(b.Dir(), "mods") }

// Addr is the loopback Eaglercraft WebSocket endpoint of the server.
func (b *Backend) Addr() string {
	p := b.Cfg.WSPort
	if p == 0 {
		p = 8081
	}
	return fmt.Sprintf("127.0.0.1:%d", p)
}

// Status returns the current state and, for StateError, a human-readable cause.
func (b *Backend) Status() (state, msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state, b.errMsg
}

func (b *Backend) set(state, msg string) {
	b.mu.Lock()
	b.state, b.errMsg = state, msg
	b.mu.Unlock()
	if state == StateError {
		b.Log.Printf("backend: %s", msg)
	}
}

// Restart stops the running server (if any) and starts it again; it also
// recovers a backend that gave up after an error.
func (b *Backend) Restart() {
	select {
	case b.kick <- struct{}{}:
	default:
	}
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Unlock()
}

func (b *Backend) java() string {
	if b.Cfg.Java != "" {
		return b.Cfg.Java
	}
	return "java"
}

// Preflight checks that the server can be started: a runnable Java and a jar.
func (b *Backend) Preflight() error {
	jar := filepath.Join(b.Dir(), b.Cfg.Jar)
	if st, err := os.Stat(jar); err != nil || st.IsDir() {
		return fmt.Errorf("server jar %q not found: put it in %s (see scripts/testserver.sh)", b.Cfg.Jar, b.Dir())
	}
	bin, err := exec.LookPath(b.java())
	if err != nil {
		return fmt.Errorf("Java not found (%q): install a JDK 21+ or set backend.java", b.java())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "-version").CombinedOutput(); err != nil {
		return fmt.Errorf("Java at %s is not runnable: %v: %s", bin, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (b *Backend) command(ctx context.Context) *exec.Cmd {
	args := append([]string{}, b.Cfg.JVMArgs...)
	args = append(args, "-jar", b.Cfg.Jar)
	if b.Cfg.Args != nil {
		args = append(args, b.Cfg.Args...)
	} else {
		args = append(args, "nogui")
	}
	cmd := exec.CommandContext(ctx, b.java(), args...)
	cmd.Dir = b.Dir()
	return cmd
}

// wait blocks until a restart is requested or ctx ends; false means ctx ended.
func (b *Backend) wait(ctx context.Context) bool {
	select {
	case <-b.kick:
		return true
	case <-ctx.Done():
		return false
	}
}

// Run starts the JVM and keeps it running until ctx is cancelled. It blocks.
func (b *Backend) Run(ctx context.Context) {
	if err := os.MkdirAll(b.ModsDir(), 0o755); err != nil {
		b.set(StateError, err.Error())
		return
	}
	logPath := filepath.Join(b.Logs, "backend.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		b.set(StateError, err.Error())
		return
	}
	defer logf.Close()
	backoff, fails := time.Second, 0
	for ctx.Err() == nil {
		if err := b.Preflight(); err != nil {
			b.set(StateError, err.Error())
			if !b.wait(ctx) {
				return
			}
			continue
		}
		b.set(StateStarting, "")
		runCtx, cancel := context.WithCancel(ctx)
		b.mu.Lock()
		b.cancel = cancel
		b.mu.Unlock()
		cmd := b.command(runCtx)
		cmd.Stdout, cmd.Stderr = logf, logf
		start := time.Now()
		b.Log.Printf("backend: starting %s (logs: backend.log)", cmd.String())
		var runErr error
		if runErr = cmd.Start(); runErr == nil {
			go b.waitReady(runCtx)
			runErr = cmd.Wait()
		}
		cancel()
		select {
		case <-b.kick: // user-requested restart
			backoff, fails = time.Second, 0
			continue
		default:
		}
		if ctx.Err() != nil {
			return
		}
		b.Log.Printf("backend: exited: %v", runErr)
		if time.Since(start) < fastFailWindow {
			fails++
		} else {
			fails, backoff = 0, time.Second
		}
		if fails >= maxFastFails {
			b.set(StateError, fmt.Sprintf("server crashed %d times right after starting; last output: %s", fails, tail(logPath, 400)))
			if !b.wait(ctx) {
				return
			}
			backoff, fails = time.Second, 0
			continue
		}
		b.set(StateStarting, "")
		select {
		case <-ctx.Done():
			return
		case <-b.kick:
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (b *Backend) waitReady(ctx context.Context) {
	for i := 0; i < 300 && ctx.Err() == nil; i++ {
		if c, err := net.DialTimeout("tcp", b.Addr(), time.Second); err == nil {
			c.Close()
			b.set(StateReady, "")
			b.Log.Printf("backend: ready on %s", b.Addr())
			return
		}
		time.Sleep(time.Second)
	}
	if ctx.Err() == nil {
		b.set(StateError, "server is running but nothing listens on "+b.Addr()+": is an Eaglercraft plugin installed and wsPort correct?")
	}
}

// tail returns the last n bytes of a file, trimmed, for error messages.
func tail(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(data) > n {
		data = data[len(data)-n:]
	}
	return string(bytes.TrimSpace(data))
}
