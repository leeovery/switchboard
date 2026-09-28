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
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// maxSocketPath is the longest path a unix socket can have: the room its
// address has, less the NUL that ends the path. On macOS, that's 103 bytes.
const maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// SocketPath is where the control socket of a router whose state directory is
// stateDir lives.
func SocketPath(stateDir string) string {
	return filepath.Join(stateDir, "control.sock")
}

// maxControlBody caps the body of a request to the control API.
const maxControlBody = 64 << 10

// Health is what GET /health answers: that the router is alive, and which it
// is.
type Health struct {
	OK        bool      `json:"ok"`
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Session is what GET /sessions/{id} answers: the accounts a session's
// requests go to, as a statusline asks.
type Session struct {
	ID string `json:"session"`
	// Assignments are the session's, a model each, the one used last first.
	Assignments []Assignment `json:"assignments"`
	// Account is the account of the assignment used last.
	Account status.Account `json:"account"`
}

// Assignment is the account a session's requests of one model go to, and why.
type Assignment struct {
	Model   string `json:"model"`
	Account string `json:"account"`
	// Pinned is set when the session's own pin put it on the account.
	Pinned     bool      `json:"pinned"`
	Reason     string    `json:"reason"`
	AssignedAt time.Time `json:"assigned_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// pinRequest is what POST /pin takes.
type pinRequest struct {
	Account string `json:"account"`
	Move    bool   `json:"move"`
}

// problem is what the control API answers a request it won't serve with.
type problem struct {
	Error string `json:"error"`
}

// Control is the control API: GET /health says the router is alive, GET
// /status gives its status document, GET /sessions/{id} says where a
// session's requests go, and POST and DELETE /pin set and clear the global
// pin, each answering with the status document as it leaves it.
func (r *Router) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, Health{OK: true, Version: r.cfg.Version, PID: os.Getpid(), StartedAt: r.started})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.Status())
	})
	mux.HandleFunc("GET /sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		session, ok := r.session(id)
		if !ok {
			writeProblem(w, http.StatusNotFound, "the router hasn't seen session "+id)
			return
		}
		writeJSON(w, session)
	})
	mux.HandleFunc("POST /pin", func(w http.ResponseWriter, req *http.Request) {
		var pin pinRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxControlBody)).Decode(&pin); err != nil {
			writeProblem(w, http.StatusBadRequest, `give the account to pin as JSON, such as {"account": "work", "move": false}`)
			return
		}
		if err := r.pin(pin.Account, pin.Move); err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, r.Status())
	})
	mux.HandleFunc("DELETE /pin", func(w http.ResponseWriter, _ *http.Request) {
		r.unpin()
		writeJSON(w, r.Status())
	})
	return mux
}

// session reports where the session with the given id has its requests go,
// and false when the router hasn't seen it.
func (r *Router) session(id string) (Session, bool) {
	assignments := r.sessions.of(id)
	if len(assignments) == 0 {
		return Session{}, false
	}
	account, _ := r.Status().Account(assignments[0].Account)
	return Session{ID: id, Assignments: assignments, Account: account}, true
}

// pin sends every new session to the account with the given id, and with
// move, every running session too. It fails, saying why, for an account
// nothing can go out on.
func (r *Router) pin(id string, move bool) error {
	a, ok := r.accounts.byID(id)
	switch {
	case id == "":
		return errors.New(`give the account to pin, such as {"account": "work"}`)
	case !ok:
		return fmt.Errorf("there's no account %q: pin %s", id, strings.Join(r.accounts.sendable().ids(), " or "))
	case !a.hasToken:
		return fmt.Errorf("account %s has no token, so nothing can go out on it: set %s", id, a.TokenEnv)
	}
	r.sessions.setPin(status.Pin{Account: id, Since: r.cfg.Now().UTC(), Move: move})
	logger.Info("pinned", "account", id, "move", move)
	return nil
}

// unpin clears the global pin, so every session is routed on its merits.
func (r *Router) unpin() {
	if was := r.sessions.unpin(); was.Account != "" {
		logger.Info("unpinned", "account", was.Account)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeProblem answers with status, and message saying why.
func writeProblem(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Error: message})
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
