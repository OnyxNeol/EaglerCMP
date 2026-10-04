package manager

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/OnyxNeol/eaglercmp/config"
)

// Backend supervises the local JVM server (Fabric/Paper with Eaglercraft
// support and mods in <Dir>/mods) and restarts it with backoff if it dies.
type Backend struct {
	Cfg  config.Backend
	Root string // instance root, for the default Dir
	Logs string
	Log  *Logger
}

func (b *Backend) dir() string {
	if b.Cfg.Dir != "" {
		return b.Cfg.Dir
	}
	return filepath.Join(b.Root, "backend")
}

// Addr is the loopback Eaglercraft WebSocket endpoint of the server.
func (b *Backend) Addr() string {
	p := b.Cfg.WSPort
	if p == 0 {
		p = 8081
	}
	return fmt.Sprintf("127.0.0.1:%d", p)
}

func (b *Backend) command(ctx context.Context) *exec.Cmd {
	java := b.Cfg.Java
	if java == "" {
		java = "java"
	}
	args := append([]string{}, b.Cfg.JVMArgs...)
	args = append(args, "-jar", b.Cfg.Jar)
	if b.Cfg.Args != nil {
		args = append(args, b.Cfg.Args...)
	} else {
		args = append(args, "nogui")
	}
	cmd := exec.CommandContext(ctx, java, args...)
	cmd.Dir = b.dir()
	return cmd
}

// Run starts the JVM and keeps it running until ctx is cancelled. It blocks.
func (b *Backend) Run(ctx context.Context) {
	if err := os.MkdirAll(filepath.Join(b.dir(), "mods"), 0o755); err != nil {
		b.Log.Printf("backend: %v", err)
		return
	}
	if _, err := os.Stat(filepath.Join(b.dir(), b.Cfg.Jar)); err != nil {
		b.Log.Printf("backend: server jar not found: %v (place it in %s)", err, b.dir())
		return
	}
	if _, err := exec.LookPath(orDefault(b.Cfg.Java, "java")); err != nil {
		b.Log.Printf("backend: java not found: %v", err)
		return
	}
	logf, err := os.OpenFile(filepath.Join(b.Logs, "backend.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		b.Log.Printf("backend: %v", err)
		return
	}
	defer logf.Close()
	backoff := time.Second
	for ctx.Err() == nil {
		cmd := b.command(ctx)
		cmd.Stdout, cmd.Stderr = logf, logf
		start := time.Now()
		b.Log.Printf("backend: starting %s (logs: backend.log)", cmd.String())
		if err := cmd.Start(); err != nil {
			b.Log.Printf("backend: %v", err)
		} else {
			go b.waitReady(ctx)
			err = cmd.Wait()
			b.Log.Printf("backend: exited: %v", err)
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (b *Backend) waitReady(ctx context.Context) {
	for i := 0; i < 120 && ctx.Err() == nil; i++ {
		if c, err := net.DialTimeout("tcp", b.Addr(), time.Second); err == nil {
			c.Close()
			b.Log.Printf("backend: ready on %s", b.Addr())
			return
		}
		time.Sleep(time.Second)
	}
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
