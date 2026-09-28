package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// maxSocketPath is the longest path a unix socket can have: the room its
// address has, less the NUL that ends the path. On macOS, that's 103 bytes.
const maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// SocketPath is where the control socket of a router whose state directory is
// stateDir lives.
func SocketPath(stateDir string) string {
	return filepath.Join(stateDir, "control.sock")
}

// Health is what GET /health answers: that the router is alive, and which it
// is.
type Health struct {
	OK        bool      `json:"ok"`
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Control is the control API: GET /health says the router is alive, and GET
// /status gives its status document.
func (r *Router) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, Health{OK: true, Version: r.cfg.Version, PID: os.Getpid(), StartedAt: r.started})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.Status())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// checkSocketPath fails when path is too long for a unix socket, which
// listening would only report as an invalid argument.
func checkSocketPath(path string) error {
	if len(path) > maxSocketPath {
		return fmt.Errorf("the control socket's path, %s, is %d bytes, and a unix socket's can be %d at most: set XDG_STATE_HOME to a shorter directory",
			path, len(path), maxSocketPath)
	}
	return nil
}

// listenControl listens on the control socket at path, replacing one a
// router that's gone left behind. File permissions are all that guards the
// socket, so its directory is made its owner's alone before the socket exists,
// leaving no moment when anyone else could reach it, and then the socket is.
func listenControl(path string) (net.Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make the state directory private: %w", err)
	}
	if err := removeStale(path); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on the control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("make the control socket private: %w", err)
	}
	return ln, nil
}

// removeStale removes the socket a router that's gone left at path, if any.
func removeStale(path string) error {
	err := os.Remove(path)
	switch {
	case err == nil:
		logger.Info("removed a stale control socket", "path", path)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("remove the stale control socket: %w", err)
	}
	return nil
}
