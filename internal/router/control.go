package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

// Health is what GET /health answers: that the router is alive, which it is,
// and where its proxy listens. OK is false while the router is failing too
// many of the requests it routes, and Reason says how many.
type Health struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	// Listen is the address the proxy listens on, which may no longer be the
	// one the config names: "" until it listens.
	Listen    string    `json:"listen,omitempty"`
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// PinRequest is what POST /pin takes: the account every new session goes
// to; whether every running session moves there too, on its next request;
// and whether every session's own pin is cleared, the one it was launched
// with included.
type PinRequest struct {
	Account string `json:"account"`
	Move    bool   `json:"move"`
	Force   bool   `json:"force"`
}

// sessionPinRequest is what POST /sessions/{id}/pin takes: the account the
// session's requests go to from its next request on.
type sessionPinRequest struct {
	Account string `json:"account"`
}

// refreshRequest is what POST /refresh takes: how old an account's usage can
// be, as a duration such as "30m", before it's probed.
type refreshRequest struct {
	MaxAge string `json:"max_age"`
}

// refreshWait bounds how long POST /refresh waits for the probes it starts,
// so what asks for fresh usage is never kept waiting long.
const refreshWait = 10 * time.Second

// problem is what the control API answers a request it won't serve with.
type problem struct {
	Error string `json:"error"`
}

// Control is the control API: GET /health says the router is alive, and
// whether it's healthy, and GET /status gives its status document. GET
// /sessions lists the sessions routed in the last hour, and GET
// /sessions/{id} says where a session's requests go, which POST and DELETE
// /sessions/{id}/pin change as they set and clear its own pin, answering with
// the session as they leave it. POST and DELETE /pin set and clear the global
// pin, and POST /refresh probes the accounts whose usage is older than it
// asks, each of the three answering with the status document as it leaves it.
func (r *Router) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		h := r.health.report()
		writeJSON(w, Health{OK: h.Healthy, Reason: h.Reason, Listen: r.proxyAddr, Version: r.cfg.Version, PID: os.Getpid(), StartedAt: r.started})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.Status())
	})
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.running())
	})
	mux.HandleFunc("GET /sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
		r.answerSession(w, req.PathValue("id"), nil)
	})
	mux.HandleFunc("POST /sessions/{id}/pin", func(w http.ResponseWriter, req *http.Request) {
		var pin sessionPinRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxControlBody)).Decode(&pin); err != nil {
			writeProblem(w, http.StatusBadRequest, `give the account to pin the session to as JSON, such as {"account": "work"}`)
			return
		}
		id := req.PathValue("id")
		r.answerSession(w, id, r.pinSession(id, pin.Account))
	})
	mux.HandleFunc("DELETE /sessions/{id}/pin", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		r.answerSession(w, id, r.unpinSession(id))
	})
	mux.HandleFunc("POST /pin", func(w http.ResponseWriter, req *http.Request) {
		var pin PinRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxControlBody)).Decode(&pin); err != nil {
			writeProblem(w, http.StatusBadRequest, `give the account to pin as JSON, such as {"account": "work", "move": false, "force": false}`)
			return
		}
		if err := r.pin(pin); err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, r.Status())
	})
	mux.HandleFunc("DELETE /pin", func(w http.ResponseWriter, req *http.Request) {
		force, err := forceAsked(req)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		r.unpin(force)
		writeJSON(w, r.Status())
	})
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, req *http.Request) {
		age, err := maxAge(w, req)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		r.refresh(req.Context(), age)
		writeJSON(w, r.Status())
	})
	return mux
}

// maxAge reads how old an account's usage can be before POST /refresh probes
// it, from the request's body.
func maxAge(w http.ResponseWriter, req *http.Request) (time.Duration, error) {
	var refresh refreshRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxControlBody)).Decode(&refresh); err != nil {
		return 0, errors.New(`give how old usage can be as JSON, such as {"max_age": "30m"}`)
	}
	age, err := time.ParseDuration(refresh.MaxAge)
	switch {
	case refresh.MaxAge == "":
		return 0, errors.New(`give how old usage can be, such as {"max_age": "30m"}`)
	case err != nil:
		return 0, fmt.Errorf("max_age %q isn't a duration, such as 30m", refresh.MaxAge)
	case age < 0:
		return 0, fmt.Errorf("max_age %s is less than nothing", refresh.MaxAge)
	}
	return age, nil
}

// refresh probes the accounts nothing has been read of for longer than age,
// sharing any probe of them already under way, and waits for those probes:
// for refreshWait at most, or until ctx ends, after which they go on without
// it.
func (r *Router) refresh(ctx context.Context, age time.Duration) {
	underway := r.probes.start(r.accounts.sendable(), r.state.olderThan(age))
	if len(underway) == 0 {
		return
	}
	started := time.Now()
	switch settle(ctx, underway, refreshWait) {
	case settled:
		logger.Debug("refreshed", "accounts", accountsOf(underway), "max_age", age, "duration", time.Since(started).Round(time.Millisecond))
	case timedOut:
		logger.Debug("stopped waiting for probes to refresh", "accounts", accountsOf(underway), "after", refreshWait)
	}
}

// forceAsked reads whether DELETE /pin is to clear every session's own pin
// too, as ?force=true asks.
func forceAsked(req *http.Request) (bool, error) {
	value := req.URL.Query().Get("force")
	if value == "" {
		return false, nil
	}
	force, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("force is true or false, not %q", value)
	}
	return force, nil
}

// answerSession answers as GET /sessions/{id} does, with the session as it
// stands, unless err, from what was asked of the session, says why not: 404
// for a session the router hasn't seen, else 400.
func (r *Router) answerSession(w http.ResponseWriter, id string, err error) {
	var session status.Session
	if err == nil {
		session, err = r.session(id)
	}
	switch {
	case errors.Is(err, ErrUnknownSession):
		writeProblem(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeProblem(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, session)
	}
}

// session reports where the session with the given id has its requests go,
// with the status of the account its last-used model went to. It fails with
// ErrUnknownSession when the router hasn't seen it.
func (r *Router) session(id string) (status.Session, error) {
	s, ok := r.sessions.session(id)
	if !ok {
		return status.Session{}, unknownSession(id)
	}
	s.Account, _ = r.Status().Account(s.Assignments[0].Account)
	return r.named(s), nil
}

// running reports the sessions routed in the last hour, the one seen last
// first, each as session does but for its account's status.
func (r *Router) running() []status.Session {
	listed := r.sessions.running(r.cfg.Now())
	for i, s := range listed {
		listed[i] = r.named(s)
	}
	return listed
}

// named is s with the family of each of its models, as the provider names it.
func (r *Router) named(s status.Session) status.Session {
	for i, a := range s.Assignments {
		s.Assignments[i].Family = r.cfg.Provider.Family(a.Model)
	}
	return s
}

// pin sends every new session to the account the request names, with Move,
// every running session too, and with Force, clears every session's own pin.
// It fails, saying why, for an account nothing can go out on.
func (r *Router) pin(p PinRequest) error {
	if err := r.pinnable(p.Account); err != nil {
		return err
	}
	cleared := r.sessions.setPin(status.Pin{Account: p.Account, Since: r.cfg.Now().UTC(), Move: p.Move}, p.Force)
	attrs := []any{"account", p.Account, "move", p.Move}
	if p.Force {
		attrs = append(attrs, "force", true, "sessions_unpinned", cleared)
	}
	logger.Info("pinned", attrs...)
	return nil
}

// unpin clears the global pin, so every session is routed on its merits but
// for those with pins of their own, and with force, clears theirs too.
func (r *Router) unpin(force bool) {
	was, cleared := r.sessions.unpin(force)
	switch {
	case force:
		logger.Info("unpinned", "account", was.Account, "force", true, "sessions_unpinned", cleared)
	case was.Account != "":
		logger.Info("unpinned", "account", was.Account)
	}
}

// pinSession sends the requests of the session with the given id to the
// account with the given id from its next request on, passing over the pin it
// was launched with. It fails, saying why, for an account nothing can go out
// on, and with ErrUnknownSession for a session the router hasn't seen.
func (r *Router) pinSession(id, account string) error {
	if err := r.pinnable(account); err != nil {
		return err
	}
	if !r.sessions.pinSession(id, account) {
		return unknownSession(id)
	}
	logger.Info("pinned session", "session", status.ShortID(id), "account", account)
	return nil
}

// unpinSession clears the own pin of the session with the given id, the one
// it was launched with included, so it's routed like any other from its next
// request. It fails with ErrUnknownSession for a session the router hasn't
// seen.
func (r *Router) unpinSession(id string) error {
	if !r.sessions.pinSession(id, "") {
		return unknownSession(id)
	}
	logger.Info("unpinned session", "session", status.ShortID(id))
	return nil
}

// pinnable fails, saying why, unless requests can go out on the account with
// the given id, as a pin to it needs.
func (r *Router) pinnable(id string) error {
	a, ok := r.accounts.byID(id)
	switch {
	case id == "":
		return errors.New(`give the account to pin, such as {"account": "work"}`)
	case !ok:
		return fmt.Errorf("there's no account %q: pin %s", id, strings.Join(r.accounts.sendable().configured().IDs(), " or "))
	case !a.hasToken():
		return fmt.Errorf("account %s has no usable token, so nothing can go out on it: %s", id, a.problem())
	}
	return nil
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
