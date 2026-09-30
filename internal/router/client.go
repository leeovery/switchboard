package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

var (
	// ErrNotRunning is what a Client's calls fail with, wrapped, when no
	// router listens on its socket.
	ErrNotRunning = errors.New("the router isn't running")
	// ErrUnknownSession is what Session fails with, followed by the session's
	// id, when the router hasn't seen the session.
	ErrUnknownSession = errors.New("the router hasn't seen session")
)

const (
	// clientTimeout bounds each of a Client's calls, so a router that's stuck
	// can't hang its caller.
	clientTimeout = 5 * time.Second
	// refreshTimeout bounds Refresh, which waits for the router's probes.
	refreshTimeout = refreshWait + clientTimeout
)

// Client asks a running router about itself, and tells it what to do, over
// its control socket.
type Client struct {
	http *http.Client
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

// Health asks whether the router is alive, and which it is.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	err := c.call(ctx, clientTimeout, http.MethodGet, "/health", nil, &h)
	return h, err
}

// Status asks for the router's status document.
func (c *Client) Status(ctx context.Context) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodGet, "/status", nil, &doc)
	return doc, err
}

// Sessions asks which sessions the router has routed in the last hour, the
// one seen last first, and where their requests go.
func (c *Client) Sessions(ctx context.Context) ([]status.Session, error) {
	var sessions []status.Session
	err := c.call(ctx, clientTimeout, http.MethodGet, "/sessions", nil, &sessions)
	return sessions, err
}

// Session asks which accounts the session with the given id has its requests
// go to. It fails with ErrUnknownSession for a session the router hasn't
// seen.
func (c *Client) Session(ctx context.Context, id string) (status.Session, error) {
	return c.session(ctx, http.MethodGet, id, "", nil)
}

// PinSession has the router send the requests of the session with the given
// id to the account with the given id from its next request on, passing over
// the pin the session was launched with. It returns the session as that
// leaves it, and fails with ErrUnknownSession for a session the router hasn't
// seen, and saying why for an account the router can't send requests on.
func (c *Client) PinSession(ctx context.Context, id, account string) (status.Session, error) {
	return c.session(ctx, http.MethodPost, id, "/pin", sessionPinRequest{Account: account})
}

// UnpinSession has the router clear the own pin of the session with the
// given id, the one it was launched with included, so the session is routed
// like any other from its next request. It returns the session as that
// leaves it, and fails with ErrUnknownSession for a session the router hasn't
// seen.
func (c *Client) UnpinSession(ctx context.Context, id string) (status.Session, error) {
	return c.session(ctx, http.MethodDelete, id, "/pin", nil)
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
// the router can't send requests on.
func (c *Client) Pin(ctx context.Context, p PinRequest) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodPost, "/pin", p, &doc)
	return doc, err
}

// Unpin has the router route every session on its merits again, but for
// those with pins of their own, whose pins force clears too. It returns the
// status document as unpinning leaves it.
func (c *Client) Unpin(ctx context.Context, force bool) (status.Document, error) {
	path := "/pin"
	if force {
		path += "?force=true"
	}
	var doc status.Document
	err := c.call(ctx, clientTimeout, http.MethodDelete, path, nil, &doc)
	return doc, err
}

// Refresh has the router probe the accounts it hasn't read for longer than
// maxAge, waiting a few seconds at most for those probes, and returns the
// status document as they leave it.
func (c *Client) Refresh(ctx context.Context, maxAge time.Duration) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, refreshTimeout, http.MethodPost, "/refresh", refreshRequest{MaxAge: maxAge.String()}, &doc)
	return doc, err
}

// call sends the router a request for path, with body as JSON unless it's
// nil, and decodes its answer into answer, giving up once timeout has passed.
func (c *Client) call(ctx context.Context, timeout time.Duration, method, path string, body, answer any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := newControlRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if isDial(err) {
			return fmt.Errorf("%w: %w", ErrNotRunning, err)
		}
		return fmt.Errorf("ask the router: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return refused(resp, method, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(answer); err != nil {
		return fmt.Errorf("read the router's answer to %s %s: %w", method, path, err)
	}
	return nil
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

// declined is a call answered with other than 200: the status, and the
// reason the router gave, which is "" when the answer gave none.
type declined struct {
	status       int
	reason       string
	method, path string
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
	return declined{status: resp.StatusCode, reason: p.Error, method: method, path: path}
}

// isDial reports whether err is a failure to connect at all: there's no
// socket, or nothing listens on it.
func isDial(err error) bool {
	op, ok := errors.AsType[*net.OpError](err)
	return ok && op.Op == "dial"
}
