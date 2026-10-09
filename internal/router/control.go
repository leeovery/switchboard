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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/redact"
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
	// StripsOwnHeaders is set by a router that takes every header of
	// switchboard's own, named X-Switchboard-…, off a request going upstream,
	// whatever follows the prefix. run tells only such a router the directory
	// Claude Code starts in: one from before, whose answer decodes without it,
	// took the pin off alone, by its name, and would send the directory on to
	// the API.
	StripsOwnHeaders bool `json:"strips_own_headers"`
}

// Same reports whether h and o are the answers of one router: the same
// process, started at the same time. A router that restarted in place keeps
// its process, but starts again.
func (h Health) Same(o Health) bool {
	return h.PID == o.PID && h.StartedAt.Equal(o.StartedAt)
}

// History is what GET /history answers: every account's use of a window over
// its current length, from the readings history and the readings since, a
// point each step from when the window started, which the dashboard's charts
// draw.
type History struct {
	// Window is the key of the window asked for, and Step the step between
	// points, as asked.
	Window string `json:"window"`
	Step   string `json:"step"`
	// Accounts are every configured account's, in the config's order.
	Accounts []AccountHistory `json:"accounts"`
}

// AccountHistory is an account's use of a window over its current length, as
// GET /history gives it.
type AccountHistory struct {
	ID string `json:"id"`
	// Start is when the account's window, as it runs now, started: zero when
	// it can't be placed as running now, as when it hasn't been read, has
	// lapsed, or has reset since it was read, and then it has no points.
	Start time.Time `json:"start,omitzero"`
	// Points are the window's use a step apart from Start until now, but for
	// those before it was first read.
	Points []HistoryPoint `json:"points,omitempty"`
}

// HistoryPoint is a window's use at a time: as it was last read at or before
// it.
type HistoryPoint struct {
	At          time.Time `json:"at"`
	Utilization float64   `json:"utilization"`
}

// Restart is what POST /restart answers: the router taking the request, as
// GET /health gives it, and whether it means to replace itself in place,
// rather than exit for launchd to start it again.
type Restart struct {
	Health
	InPlace bool `json:"in_place"`
}

// By says who set or cleared a pin, for the event the router tells it as:
// ByCLI or ByDashboard, or "" where it doesn't say, as a switchboard from
// before doesn't.
type By string

const (
	// ByCLI is the pin command.
	ByCLI By = "cli"
	// ByDashboard is the dashboard's routing card.
	ByDashboard By = "dashboard"
)

// check fails, saying what to give, for anyone but ByCLI, ByDashboard or no
// one.
func (b By) check() error {
	switch b {
	case "", ByCLI, ByDashboard:
		return nil
	}
	return fmt.Errorf("by is %s or %s, not %q", ByCLI, ByDashboard, redact.Text(string(b)))
}

// PinRequest is what POST /pin takes: the accounts every new session goes to
// the best of; whether every running session on another account moves there
// too, on its next request; whether every session's own pin is cleared, the
// one it was launched with included; and who asks. Account names one account
// more, as a switchboard from before pins named several asks.
type PinRequest struct {
	Accounts []string `json:"accounts"`
	Account  string   `json:"account,omitempty"`
	Move     bool     `json:"move"`
	Force    bool     `json:"force"`
	By       By       `json:"by,omitempty"`
}

// ids returns the ids of the accounts the request names, as it names them.
func (p PinRequest) ids() []string {
	if p.Account == "" {
		return p.Accounts
	}
	return append(slices.Clone(p.Accounts), p.Account)
}

// sessionPinRequest is what POST /sessions/{id}/pin takes: the account the
// session's requests go to from its next request on, and who asks.
type sessionPinRequest struct {
	Account string `json:"account"`
	By      By     `json:"by,omitempty"`
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
// asks, and those that can take no request anyway, each of the three
// answering with the status document as it leaves it. GET /history gives
// every account's use of a window over its current length, a point each step
// asked, from the readings history and the readings since. GET /stream tells
// of what befalls each routed request as it happens, held open, a line of
// JSON an event, starting with the requests in flight. POST /restart
// restarts the router at once, as it restarts itself but for waiting for a
// moment with no request in flight, answering, before it goes, as GET /health
// does, and whether it means to restart in place; it refuses, saying why, when
// it can't restart, as run by hand.
func (r *Router) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.healthNow())
	})
	mux.HandleFunc("POST /restart", func(w http.ResponseWriter, _ *http.Request) {
		if err := r.upkeep.restartable(); err != nil {
			writeProblem(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, Restart{Health: r.healthNow(), InPlace: r.replaceable()})
		// The answer goes out before the restart begins: the router can stop
		// answering at once.
		_ = http.NewResponseController(w).Flush()
		r.upkeep.restartNow()
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
		r.answerSession(w, id, r.pinSession(id, pin.Account, pin.By))
	})
	mux.HandleFunc("DELETE /sessions/{id}/pin", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		by, err := byAsked(req)
		if err == nil {
			err = r.unpinSession(id, by)
		}
		r.answerSession(w, id, err)
	})
	mux.HandleFunc("POST /pin", func(w http.ResponseWriter, req *http.Request) {
		var pin PinRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxControlBody)).Decode(&pin); err != nil {
			writeProblem(w, http.StatusBadRequest, `give the accounts to pin as JSON, such as {"accounts": ["work"], "move": false, "force": false}`)
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
		by, err := byAsked(req)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		r.unpin(force, by)
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
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, req *http.Request) {
		asked, err := historyAsked(req)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, r.windowHistory(asked))
	})
	mux.HandleFunc("GET /stream", r.serveStream)
	return mux
}

// healthNow is what GET /health answers now.
func (r *Router) healthNow() Health {
	h := r.health.report()
	return Health{OK: h.Healthy, Reason: h.Reason, Listen: r.proxyAddr, Version: r.cfg.Version, PID: os.Getpid(), StartedAt: r.started, StripsOwnHeaders: true}
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

// historyAsked reads what GET /history asks for from its query: a window
// whose length can be read, and a step more than 0 that covers the window's
// whole length in maxPoints steps at most. It fails, saying what to give,
// otherwise, hiding anything that looks like a token in what it quotes.
func historyAsked(req *http.Request) (historyAsk, error) {
	query := req.URL.Query()
	window, asked := query.Get("window"), query.Get("step")
	length, ok := quota.Length(window)
	switch {
	case window == "":
		return historyAsk{}, errors.New("give the window, such as window=5h")
	case !ok:
		return historyAsk{}, fmt.Errorf("window %q has no length to read: give one such as 5h or 7d", redact.Text(window))
	}
	step, err := time.ParseDuration(asked)
	switch {
	case asked == "":
		return historyAsk{}, errors.New("give the step between points, such as step=5m")
	case err != nil:
		return historyAsk{}, fmt.Errorf("step %q isn't a duration, such as 5m", redact.Text(asked))
	case step <= 0:
		return historyAsk{}, fmt.Errorf("step %s isn't more than 0", asked)
	case step < leastStep(length):
		return historyAsk{}, fmt.Errorf("step %s is too short for window %s: its whole length would take more than %d steps, so give %v or more",
			asked, redact.Text(window), maxPoints, leastStep(length))
	}
	return historyAsk{window: window, step: step, asked: asked}, nil
}

// refresh probes the accounts nothing has been read of for longer than age,
// and those that can take no request anyway, as refreshing says, sharing any
// probe of them already under way, and waits for those probes: for
// refreshWait at most, or until ctx ends, after which they go on without it.
func (r *Router) refresh(ctx context.Context, age time.Duration) {
	underway := r.probes.start(r.accounts.sendable(), r.state.refreshing(age))
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

// byAsked reads who DELETE /pin or /sessions/{id}/pin says asks, as ?by=
// gives it, failing, saying what to give, for anyone but ByCLI or
// ByDashboard.
func byAsked(req *http.Request) (By, error) {
	by := By(req.URL.Query().Get("by"))
	return by, by.check()
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
	s.Account = r.accountStatus(s.Assignments[0].Account)
	return r.described(s, r.stream.doing()), nil
}

// running reports the sessions routed in the last hour, the one seen last
// first, each as session does but for its account's status.
func (r *Router) running() []status.Session {
	listed := r.sessions.running(r.cfg.Now())
	doing := r.stream.doing()
	for i, s := range listed {
		listed[i] = r.described(s, doing)
	}
	return listed
}

// described is s with the family of each of its models, as the provider
// names it, and what a request of each in flight is doing, as doing, the
// stream's, has it.
func (r *Router) described(s status.Session, doing map[key]string) status.Session {
	for i, a := range s.Assignments {
		s.Assignments[i].Family = r.cfg.Provider.Family(a.Model)
		s.Assignments[i].InFlight = doing[key{session: s.ID, model: a.Model}]
	}
	return s
}

// pin sends every new session to the best of the accounts the request names,
// with Move, every running session on another account too, and with Force,
// clears every session's own pin, telling it as Pinned. It fails, saying why,
// for anyone asking but ByCLI or ByDashboard, for no account, or for one
// nothing can go out on, and then tells nothing.
func (r *Router) pin(p PinRequest) error {
	if err := p.By.check(); err != nil {
		return err
	}
	ids, err := r.pinnableAll(p.ids())
	if err != nil {
		return err
	}
	cleared := r.sessions.setPin(status.Pin{Accounts: ids, Since: r.cfg.Now().UTC(), Move: p.Move}, p.Force)
	attrs := []any{"accounts", strings.Join(ids, ","), "move", p.Move}
	if p.Force {
		attrs = append(attrs, "force", true, "sessions_unpinned", cleared)
	}
	logger.Info("pinned", attrs...)
	r.emit(Pinned{Accounts: ids, Account: ids[0], Move: p.Move, Force: p.Force, By: p.By})
	return nil
}

// unpin clears the global pin, so every session is routed on its merits but
// for those with pins of their own, and with force, clears theirs too,
// telling it as Unpinned unless it cleared nothing.
func (r *Router) unpin(force bool, by By) {
	was, cleared := r.sessions.unpin(force)
	accounts := strings.Join(was.Accounts, ",")
	switch {
	case force:
		logger.Info("unpinned", "accounts", accounts, "force", true, "sessions_unpinned", cleared)
	case !was.IsZero():
		logger.Info("unpinned", "accounts", accounts)
	}
	if !was.IsZero() || cleared > 0 {
		r.emit(Unpinned{Accounts: was.Accounts, Force: force, By: by})
	}
}

// pinSession sends the requests of the session with the given id to the
// account with the given id from its next request on, passing over the pin it
// was launched with, telling it as Pinned. It fails, saying why, for anyone
// asking but ByCLI or ByDashboard, or for an account nothing can go out on,
// and with ErrUnknownSession for a session the router hasn't seen, and then
// tells nothing.
func (r *Router) pinSession(id, account string, by By) error {
	if err := by.check(); err != nil {
		return err
	}
	if err := r.pinnable(account); err != nil {
		return err
	}
	if _, seen := r.sessions.pinSession(id, account); !seen {
		return unknownSession(id)
	}
	logger.Info("pinned session", "session", status.ShortID(id), "account", account)
	r.emit(Pinned{Account: account, Session: id, By: by})
	return nil
}

// unpinSession clears the own pin of the session with the given id, the one
// it was launched with included, so it's routed like any other from its next
// request, telling it as Unpinned unless it had none. It fails with
// ErrUnknownSession for a session the router hasn't seen.
func (r *Router) unpinSession(id string, by By) error {
	was, seen := r.sessions.pinSession(id, "")
	if !seen {
		return unknownSession(id)
	}
	logger.Info("unpinned session", "session", status.ShortID(id))
	if was != "" {
		r.emit(Unpinned{Account: was, Session: id, By: by})
	}
	return nil
}

// pinnableAll returns the accounts with the given ids once each, in the order
// configured, as the global pin names them. It fails, saying why, for none,
// or unless requests can go out on each, as pinnable says.
func (r *Router) pinnableAll(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New(`give the accounts to pin, such as {"accounts": ["work"]}`)
	}
	for _, id := range ids {
		if err := r.pinnable(id); err != nil {
			return nil, err
		}
	}
	return r.accounts.only(ids).configured().IDs(), nil
}

// pinnable fails, saying why, unless requests can go out on the account with
// the given id, as a pin to it needs.
func (r *Router) pinnable(id string) error {
	a, ok := r.accounts.byID(id)
	switch {
	case id == "":
		return errors.New(`give the account to pin, such as {"account": "work"}`)
	case !ok:
		return UnknownAccount(id, r.accounts.sendable().configured().IDs())
	case !a.hasToken():
		return fmt.Errorf("account %s has no usable token, so nothing can go out on it: %s", id, a.problem())
	}
	return nil
}

// UnknownAccount says there's no account with the given id to pin, naming
// the accounts that can be pinned instead, pinnable, those with a usable
// token, or saying there are none: pin and run --account say it alike. It
// hides anything in the id that looks like a token, as a token pasted where
// an id goes would be.
func UnknownAccount(id string, pinnable []string) error {
	if len(pinnable) == 0 {
		return fmt.Errorf("there's no account %q, and no account has a usable token to pin", redact.Text(id))
	}
	return fmt.Errorf("there's no account %q: pin %s", redact.Text(id), strings.Join(pinnable, " or "))
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
