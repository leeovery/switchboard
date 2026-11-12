package router

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

var (
	// ErrNotRunning is what a Client's calls fail with, wrapped, when no
	// router listens on its socket.
	ErrNotRunning = errors.New("the router isn't running")
	// ErrUnanswered is what a Client's calls fail with, wrapped, when the
	// router closes the connection before it begins to answer, as one
	// stopping does with what it hasn't answered in time.
	ErrUnanswered = errors.New("the router closed the connection without answering")
	// ErrUnknownSession is what Session fails with, followed by the session's
	// id, when the router hasn't seen the session.
	ErrUnknownSession = errors.New("the router hasn't seen session")
	// ErrNoRestart is what Restart fails with, wrapped, when the router can't
	// be asked to restart: it doesn't answer, or doesn't know how, as one from
	// before routers restarted when asked doesn't.
	ErrNoRestart = errors.New("the router can't be asked to restart")
	// ErrNoHistory is what History fails with, wrapped, when the router can't
	// be asked for a window's history: one from before GET /history doesn't
	// know the path.
	ErrNoHistory = errors.New("the router can't be asked for a window's history")
	// ErrNoStream is what Stream fails with, wrapped, when the router can't
	// be asked for its request stream: one from before GET /stream doesn't
	// know the path.
	ErrNoStream = errors.New("the router can't be asked for its request stream")
	// ErrNoImport is what Import fails with, wrapped, when the router can't
	// be asked to import: one from before POST /import doesn't know the path.
	ErrNoImport = errors.New("the router can't be asked to import")
)

// ExportUnderWay is why a router refuses to restart while its export of the
// history into Dir is under way, which it never restarts in place over: a
// router up and answering, which restarts when asked again once the export
// ends.
type ExportUnderWay struct {
	Dir string
}

func (e ExportUnderWay) Error() string {
	return "an export into " + e.Dir + " is under way: try again once it ends"
}

const (
	// clientTimeout bounds each of a Client's calls, so a router that's stuck
	// can't hang its caller.
	clientTimeout = 5 * time.Second
	// refreshTimeout bounds Refresh, which waits for the router's probes.
	refreshTimeout = refreshWait + clientTimeout
	// restartTimeout bounds Restart, which waits for the router's binary to
	// answer whether it could start.
	restartTimeout = checkTimeout + clientTimeout
	// streamLineMost is the longest line of the request stream Stream reads:
	// a router writes a few kilobytes at most, an event's session and model
	// cut to 200 bytes each, so this leaves room for any field to come.
	streamLineMost = 1 << 20
)

// Client asks a running router about itself, and tells it what to do, over
// its control socket.
type Client struct {
	http *http.Client
	// pool is the id of the pool the router's answers are asked for, as
	// InPool names it: "" for the one the router answers for without.
	pool string
}

// NewClient returns a client of the router whose control socket is at path.
// It dials the socket with http.DefaultTransport's dialer, which tests guard
// every dial of, and asks no proxy.
func NewClient(path string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dial(ctx, "unix", path)
	}
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	return &Client{http: &http.Client{Transport: transport}}
}

// InPool returns a client of the same router whose questions of a pool, of
// its status document, sessions, windows' history and request stream, ask
// for the pool with the given id, as ?pool= names it: "" asks for none, which
// the router answers for the pool the verbs act on.
func (c *Client) InPool(pool string) *Client {
	named := *c
	named.pool = pool
	return &named
}

// inPool is path, with the query given, asking for the pool the client
// names, where it names one.
func (c *Client) inPool(path string, query url.Values) string {
	if c.pool != "" {
		query = maps.Clone(query)
		if query == nil {
			query = url.Values{}
		}
		query.Set(PoolQuery, c.pool)
	}
	return pathOf(path, query)
}

// pathOf is path with the query given, where it has any.
func pathOf(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

// Health asks whether the router is alive, and which it is.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	err := c.call(ctx, clientTimeout, http.MethodGet, "/health", nil, &h)
	return h, err
}

// Status asks for the router's status document, of the pool the client
// names, else of the one the verbs act on.
func (c *Client) Status(ctx context.Context) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodGet, c.inPool("/status", nil), nil, &doc)
	return doc, err
}

// Moving asks how the router's move of the accounts and their history onto
// the ids switchboard makes stands, as its health answer says, which is no
// pool's, or, from a router from before pools, its status document: zero
// where it owes none.
func (c *Client) Moving(ctx context.Context) (status.Moving, error) {
	h, err := c.Health(ctx)
	if err != nil || h.ReportsPools {
		return h.Moving, err
	}
	doc, err := c.InPool("").Status(ctx)
	return doc.Moving, err
}

// SessionsInEveryPool asks which sessions the router has routed in the last
// hour, in every pool its health answer reports, the one seen last first, as
// Sessions asks of each: those of the one pool there is, where it reports
// none, as a router from before pools.
func (c *Client) SessionsInEveryPool(ctx context.Context) ([]status.Session, error) {
	h, err := c.Health(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := inEveryPool(ctx, c, h, (*Client).Sessions)
	if err != nil {
		return nil, err
	}
	all := slices.Concat(listed...)
	slices.SortFunc(all, status.SeenLastFirst)
	return all, nil
}

// StatusInEveryPool asks for the status document of every pool the router's
// health answer h reports, in their order, as Status asks for each: the one
// there is, where it reports none, as a router from before pools.
func (c *Client) StatusInEveryPool(ctx context.Context, h Health) ([]status.Document, error) {
	return inEveryPool(ctx, c, h, (*Client).Status)
}

// inEveryPool asks read of c in every pool the router's health answer h
// reports, in their order, and returns what each answered: once, naming none,
// where it reports none.
func inEveryPool[T any](ctx context.Context, c *Client, h Health, read func(*Client, context.Context) (T, error)) ([]T, error) {
	pools := []string{""}
	if len(h.Pools) > 0 {
		pools = pools[:0]
		for _, p := range h.Pools {
			pools = append(pools, p.ID)
		}
	}
	answers := make([]T, len(pools))
	for i, pool := range pools {
		var err error
		if answers[i], err = read(c.InPool(pool), ctx); err != nil {
			return nil, err
		}
	}
	return answers, nil
}

// Sessions asks which sessions the router has routed in the last hour, in
// the pool the client names, else in the one the verbs act on, the one seen
// last first, and where their requests go.
func (c *Client) Sessions(ctx context.Context) ([]status.Session, error) {
	var sessions []status.Session
	err := c.call(ctx, clientTimeout, http.MethodGet, c.inPool("/sessions", nil), nil, &sessions)
	return sessions, err
}

// Session asks which accounts the session with the given id has its requests
// go to. It fails with ErrUnknownSession for a session the router hasn't
// seen, in the pool the client names, where it names one.
func (c *Client) Session(ctx context.Context, id string) (status.Session, error) {
	return c.session(ctx, http.MethodGet, id, c.inPool("", nil), nil)
}

// PinSession has the router send the requests of the session with the given
// id to the account with the given id from its next request on, passing over
// the pin the session was launched with. It returns the session as that
// leaves it, and fails with ErrUnknownSession for a session the router hasn't
// seen, and saying why for an account the router can't send requests on. By
// says who asks.
func (c *Client) PinSession(ctx context.Context, id, account string, by By) (status.Session, error) {
	return c.session(ctx, http.MethodPost, id, "/pin", sessionPinRequest{Account: account, By: by})
}

// UnpinSession has the router clear the own pin of the session with the
// given id, the one it was launched with included, so the session is routed
// like any other from its next request. It returns the session as that
// leaves it, and fails with ErrUnknownSession for a session the router hasn't
// seen. By says who asks.
func (c *Client) UnpinSession(ctx context.Context, id string, by By) (status.Session, error) {
	return c.session(ctx, http.MethodDelete, id, pathOf("/pin", unpinQuery(by, false)), nil)
}

// session sends the router a request for path under the session with the
// given id, with body as JSON unless it's nil, and returns the session as the
// router answers with it. It fails with ErrUnknownSession for a session the
// router hasn't seen.
func (c *Client) session(ctx context.Context, method, id, path string, body any) (status.Session, error) {
	var s status.Session
	err := c.call(ctx, clientTimeout, method, "/sessions/"+url.PathEscape(id)+path, body, &s)
	// A 404 is the router's for an unknown session only when it says why:
	// anything else answering on the socket is trouble.
	if d, ok := errors.AsType[declined](err); ok && d.status == http.StatusNotFound && d.reason != "" {
		return s, unknownSession(id)
	}
	return s, err
}

// unknownSession is the error for the session with the given id, which the
// router hasn't seen. It hides anything in the id that looks like a token, as
// a token pasted where an id goes would be.
func unknownSession(id string) error {
	return fmt.Errorf("%w %s", ErrUnknownSession, redact.Text(id))
}

// Pin has the router pin as p asks: send every new session to the best of its
// accounts, with Move, every running session on another account too, on its
// next request, and with Force, clear every session's own pin. It returns the
// status document as pinning leaves it, and fails, saying why, for an account
// the router can't send requests on. A pin of one account names it as Account
// too, as a router from before pins named several reads a pin: one still
// running once an upgrade has replaced it, until it restarts. By says who
// asks.
func (c *Client) Pin(ctx context.Context, p PinRequest) (status.Document, error) {
	if len(p.Accounts) == 1 {
		p.Account = p.Accounts[0]
	}
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodPost, c.inPool("/pin", nil), p, &doc)
	return doc, err
}

// Unpin has the router route every session on its merits again, but for
// those with pins of their own, whose pins force clears too. It returns the
// status document as unpinning leaves it. By says who asks.
func (c *Client) Unpin(ctx context.Context, force bool, by By) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodDelete, c.inPool("/pin", unpinQuery(by, force)), nil, &doc)
	return doc, err
}

// unpinQuery is the query clearing a pin takes: who asks, and whether every
// session's own pin is cleared too, each left out where it's not given.
func unpinQuery(by By, force bool) url.Values {
	query := url.Values{}
	if by != "" {
		query.Set("by", string(by))
	}
	if force {
		query.Set("force", "true")
	}
	return query
}

// Refresh has the router probe the accounts it hasn't read for longer than
// maxAge, and those that can take no request anyway, waiting a few seconds at
// most for those probes, and returns the status document as they leave it.
func (c *Client) Refresh(ctx context.Context, maxAge time.Duration) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, refreshTimeout, http.MethodPost, c.inPool("/refresh", nil), refreshRequest{MaxAge: maxAge.String()}, &doc)
	return doc, err
}

// Restart has the router restart at once: it finishes its requests in flight,
// within StopTimeout, cutting off those still going after 30 seconds, and a
// router the service runs replaces itself, in the process it runs in, with
// its binary, reading the config and the tokens afresh, or else exits for
// launchd to start it again. It returns the router that took the request,
// and which it means to do. It fails, saying why, when the router refuses,
// as one run by hand does, or one whose binary refuses to start, with
// ExportUnderWay while an export runs, and with ErrNoRestart when it can't
// be asked.
func (c *Client) Restart(ctx context.Context) (Restart, error) {
	var restart Restart
	err := c.call(ctx, restartTimeout, http.MethodPost, "/restart", nil, &restart)
	d, ok := errors.AsType[declined](err)
	switch {
	case ok && d.exporting != "":
		return Restart{}, ExportUnderWay{Dir: d.exporting}
	case ok && d.reason != "":
		return Restart{}, d
	}
	if err != nil {
		return Restart{}, fmt.Errorf("%w: %w", ErrNoRestart, err)
	}
	return restart, nil
}

// History asks for every account's use of the window with the given key over
// its current length, a point each step, from the readings history and the
// readings since. It fails, saying why, when the router refuses what's asked,
// and with ErrNoHistory when it can't be asked, as a router from before GET
// /history can't.
func (c *Client) History(ctx context.Context, window string, step time.Duration) (History, error) {
	var h History
	query := url.Values{"window": {window}, "step": {step.String()}}
	err := c.call(ctx, clientTimeout, http.MethodGet, c.inPool("/history", query), nil, &h)
	if d, ok := errors.AsType[declined](err); ok && d.status == http.StatusNotFound && d.reason == "" {
		return History{}, fmt.Errorf("%w: %w", ErrNoHistory, err)
	}
	if err != nil {
		return History{}, err
	}
	return h, nil
}

// Import has the router take in the history import staged in its state
// directory, as ImportDir says, and returns what it took in. It waits for the
// answer as long as ctx lets it, as an import takes as long as the history it
// brings in. It fails, saying why, when the router refuses, as while another
// import runs, and with ErrNoImport when it can't be asked, as a router from
// before POST /import can't.
func (c *Client) Import(ctx context.Context) (Imported, error) {
	var imported Imported
	if err := noImport(c.ask(ctx, http.MethodPost, "/import", nil, &imported)); err != nil {
		return Imported{}, err
	}
	return imported, nil
}

// ImportRunning asks whether the router is running an import now. It fails
// with ErrNoImport when it can't be asked, as a router from before POST
// /import can't.
func (c *Client) ImportRunning(ctx context.Context) (bool, error) {
	var running importRunning
	if err := noImport(c.call(ctx, clientTimeout, http.MethodGet, "/import", nil, &running)); err != nil {
		return false, err
	}
	return running.Running, nil
}

// noImport is err, a call's of /import, as ErrNoImport where the router
// doesn't know the path, as one from before POST /import doesn't.
func noImport(err error) error {
	if d, ok := errors.AsType[declined](err); ok && d.status == http.StatusNotFound && d.reason == "" {
		return fmt.Errorf("%w: %w", ErrNoImport, err)
	}
	return err
}

// Stream opens the router's request stream, and returns what befalls each
// routed request as it happens, starting with each request in flight as it
// opens, until ctx ends or the router ends the stream, as it does for a
// reader that falls behind, and as it stops: then the channel closes. Read it
// until it does, or end ctx. Stream waits a few seconds at most for the router
// to answer, and fails with ErrNoStream, wrapped, when it can't be asked for
// its stream, as a router from before GET /stream can't.
func (c *Client) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ctx, cancel := context.WithCancel(ctx)
	resp, err := c.openStream(ctx, cancel)
	if err != nil {
		cancel()
		return nil, err
	}
	events := make(chan StreamEvent)
	go func() {
		defer cancel()
		defer close(events)
		defer func() { _ = resp.Body.Close() }()
		readStream(ctx, resp.Body, events)
	}()
	return events, nil
}

// openStream asks the router for its request stream, which runs as long as
// ctx does, giving up, by cancel, should it not answer within clientTimeout.
func (c *Client) openStream(ctx context.Context, cancel context.CancelFunc) (*http.Response, error) {
	answering := time.AfterFunc(clientTimeout, cancel)
	resp, err := c.send(ctx, http.MethodGet, c.inPool("/stream", nil), nil)
	if !answering.Stop() {
		if err == nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("ask the router: no answer within %v", clientTimeout)
	}
	if d, ok := errors.AsType[declined](err); ok && d.status == http.StatusNotFound && d.reason == "" {
		return nil, fmt.Errorf("%w: %w", ErrNoStream, err)
	}
	return resp, err
}

// readStream reads the events of the request stream from body, a line of JSON
// each, and hands each on to events, until the stream ends, or holds a line
// that isn't an event, or ctx ends.
func readStream(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	lines := bufio.NewScanner(body)
	lines.Buffer(nil, streamLineMost)
	for lines.Scan() {
		var e StreamEvent
		if err := json.Unmarshal(lines.Bytes(), &e); err != nil {
			logger.Debug("the router's request stream held a line that isn't an event; leaving it", "error", err)
			return
		}
		select {
		case events <- e:
		case <-ctx.Done():
			return
		}
	}
	if err := lines.Err(); err != nil && ctx.Err() == nil {
		logger.Debug("the router's request stream broke off", "error", err)
	}
}

// call sends the router a request for path, with body as JSON unless it's
// nil, and decodes its answer into answer, giving up once timeout has passed.
func (c *Client) call(ctx context.Context, timeout time.Duration, method, path string, body, answer any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.ask(ctx, method, path, body, answer)
}

// ask sends the router a request for path, with body as JSON unless it's
// nil, and decodes its answer into answer, as long as ctx lets it.
func (c *Client) ask(ctx context.Context, method, path string, body, answer any) error {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(answer); err != nil {
		return fmt.Errorf("read the router's answer to %s %s: %w", method, path, err)
	}
	return nil
}

// send sends the router a request for path, with body as JSON unless it's
// nil, and returns its answer, which it fails unless it's 200.
func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	req, err := newControlRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	switch {
	case err == nil:
	case isDial(err):
		return nil, fmt.Errorf("%w: %w", ErrNotRunning, err)
	case closedUnanswered(err):
		return nil, fmt.Errorf("%w: %w", ErrUnanswered, err)
	default:
		return nil, fmt.Errorf("ask the router: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, refused(resp, method, path)
	}
	return resp, nil
}

func newControlRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var content io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		content = bytes.NewReader(data)
	}
	return http.NewRequestWithContext(ctx, method, "http://switchboard"+path, content)
}

// declined is a call answered with other than 200: the status, the reason
// the router gave, which is "" when the answer gave none, and the folder of
// the export under way it gave, as problem's Exporting has it.
type declined struct {
	status            int
	reason, exporting string
	method, path      string
}

func (d declined) Error() string {
	if d.reason != "" {
		return d.reason
	}
	return fmt.Sprintf("the router answered %s %s with %d %s", d.method, d.path, d.status, http.StatusText(d.status))
}

// refused is the error for an answer other than 200.
func refused(resp *http.Response, method, path string) error {
	var p problem
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxControlBody)).Decode(&p)
	if resp.StatusCode == http.StatusBadRequest && len(p.Pools) > 0 {
		return PoolNeeded{Named: p.Named, Pools: p.Pools}
	}
	return declined{status: resp.StatusCode, reason: p.Error, exporting: p.Exporting, method: method, path: path}
}

// isDial reports whether err is a failure to connect at all: there's no
// socket, or nothing listens on it.
func isDial(err error) bool {
	op, ok := errors.AsType[*net.OpError](err)
	return ok && op.Op == "dial"
}

// IsNotRunning reports whether err, a Client's call's, found the router not
// running at that moment: nothing listening on its socket, as ErrNotRunning
// says, or the connection closed before any answer, as ErrUnanswered says,
// as a router stopping closes it.
func IsNotRunning(err error) bool {
	return errors.Is(err, ErrNotRunning) || errors.Is(err, ErrUnanswered)
}

// closedUnanswered reports whether err, a request's that got no answer, is the
// connection closing under it, rather than its context ending or an answer
// that isn't HTTP: at its end or by a reset as it was read; as the request
// was written, broken or, on macOS, already disconnected; closed on this side
// as it was read or written, as the transport closes it once it has met the
// router's close, the read and the write racing; or before the request was
// read at all, as the transport words it.
func closedUnanswered(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ENOTCONN) || errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), serverClosedIdle)
}

// serverClosedIdle is how net/http's transport fails a request whose
// connection the server closed before reading it, as a router from before
// stopped its control API, cutting the connections it had accepted: the
// error is unexported, wrapping neither EOF nor a reset, so only its text
// tells it (found as one in six such closes, in a reproduction of 200). It
// comes only where nothing was answered.
const serverClosedIdle = "http: server closed idle connection"
