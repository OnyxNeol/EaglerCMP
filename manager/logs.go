// Package manager orchestrates the EaglerCMP lifecycle: installing and
// verifying the Eaglercraft web client, serving it from a loopback HTTP
// server, and spawning/supervising the desktop window process.
package manager

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Logger writes timestamped launcher lines to the console and a log file.
type Logger struct {
	mu   sync.Mutex
	out  io.Writer
	file *os.File
	last string
}

// NewLogger opens logs/launcher.log (appending) alongside out.
func NewLogger(out io.Writer, logDir string) (*Logger, error) {
	l := &Logger{out: out}
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(filepath.Join(logDir, "launcher.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		l.file = f
	}
	return l, nil
}

// Printf logs a launcher message.
func (l *Logger) Printf(format string, args ...any) {
	line := fmt.Sprintf("%s [EaglerCMP/NaOHX] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = fmt.Sprintf(format, args...)
	io.WriteString(l.out, line)
	if l.file != nil {
		io.WriteString(l.file, line)
	}
}

// Last returns the most recent launcher message.
func (l *Logger) Last() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// Path returns the launcher log file path, if any.
func (l *Logger) Path() string {
	if l.file == nil {
		return ""
	}
	return l.file.Name()
}

// Writer exposes the console/log sinks for raw output (e.g. the splash).
func (l *Logger) Writer() io.Writer {
	if l.file == nil {
		return l.out
	}
	return io.MultiWriter(l.out, l.file)
}

// Close closes the log file.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// streamFiltered writes every line to sink. Console output gets the page's
// console messages (Chromium "CONSOLE" lines) unless verbose is set, in which
// case every engine log line is shown.
func streamFiltered(r io.Reader, prefix string, console, sink io.Writer, mu *sync.Mutex, verbose bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		mu.Lock()
		fmt.Fprintf(sink, "%s %s\n", prefix, line)
		if msg, ok := consoleMessage(line); ok {
			fmt.Fprintf(console, "%s %s\n", prefix, msg)
		} else if verbose {
			fmt.Fprintf(console, "%s %s\n", prefix, line)
		}
		mu.Unlock()
	}
	return sc.Err()
}

// consoleMessage extracts the page message from a Chromium log line such as
// `[1:1:1003/071300.1:INFO:CONSOLE(12)] "hello", source: http://... (12)`.
func consoleMessage(line string) (string, bool) {
	i := strings.Index(line, ":CONSOLE")
	if i < 0 {
		return "", false
	}
	j := strings.Index(line[i:], "] ")
	if j < 0 {
		return "", false
	}
	msg := line[i+j+2:]
	if k := strings.LastIndex(msg, `", source: `); k >= 0 && strings.HasPrefix(msg, `"`) {
		msg = msg[1:k]
	}
	return msg, true
}
