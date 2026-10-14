package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxFileSize = 10 << 20 // 10 MB
	maxBackups  = 5
)

type rotatingFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func newRotatingFile(path string) (*rotatingFile, error) {
	r := &rotatingFile{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f = f
	r.size = st.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > maxFileSize {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate shifts proxity.log -> proxity.log.1 -> ... keeping maxBackups.
func (r *rotatingFile) rotate() error {
	r.f.Close()
	for i := maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", r.path, i)
		dst := fmt.Sprintf("%s.%d", r.path, i+1)
		os.Remove(dst)
		os.Rename(src, dst)
	}
	os.Remove(r.path + ".1")
	os.Rename(r.path, r.path+".1")
	r.size = 0
	return r.open()
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		err := r.f.Close()
		r.f = nil
		return err
	}
	return nil
}

func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init creates the service logger writing to dir\proxity.log (rotated at 10MB,
// 5 backups kept). console=true additionally writes to stderr for -console
// dev mode. Returns the logger and a closer for the file.
func Init(level string, dir string, console bool) (*slog.Logger, io.Closer, error) {
	rf, err := newRotatingFile(filepath.Join(dir, "proxity.log"))
	if err != nil {
		return nil, nil, err
	}
	var w io.Writer = rf
	if console {
		w = io.MultiWriter(rf, os.Stderr)
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level)})
	l := slog.New(h)
	slog.SetDefault(l)
	return l, rf, nil
}

// Blocked logs a structured "blocked connection" security event.
func Blocked(l *slog.Logger, exePath string, pid int, dst string, reason string) {
	l.Warn("connection blocked",
		slog.String("event", "blocked"),
		slog.String("exe", exePath),
		slog.Int("pid", pid),
		slog.String("dst", dst),
		slog.String("reason", reason))
}

// Security logs a structured security-relevant event (kill switch, tamper, etc.).
func Security(l *slog.Logger, msg string, attrs ...any) {
	args := append([]any{slog.String("event", "security")}, attrs...)
	l.Warn(msg, args...)
}
