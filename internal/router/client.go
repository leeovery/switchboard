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

	"github.com/leeovery/switchboard/internal/status"
)

var (
	// ErrNotRunning is what a Client's calls fail with, wrapped, when no
	// router listens on its socket.
	ErrNotRunning = errors.New("switchboard isn't running")
	// ErrUnknownSession is what Session fails with, followed by the session's
	// id, when the router hasn't seen the session.
	ErrUnknownSession = errors.New("the router hasn't seen session")
)

// clientTimeout bounds each of a Client's calls, so a router that's stuck
// can't hang its caller.
const clientTimeout = 5 * time.Second

// Client asks a running router about itself, and tells it what to do, over
// its control socket.
type Client struct {
	http *http.Client
}

// NewClient returns a client of the router whose control socket is at path.
func NewClient(path string) *Client {
	var dialer net.Dialer
	return &Client{http: &http.Client{
		Timeout: clientTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", path)
			},
			DisableKeepAlives: true,
		},
	}}
}

// Health asks whether the router is alive, and which it is.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	err := c.call(ctx, http.MethodGet, "/health", nil, &h)
	return h, err
}

// Status asks for the router's status document.
func (c *Client) Status(ctx context.Context) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, http.MethodGet, "/status", nil, &doc)
	return doc, err
}

// Session asks which accounts the session with the given id has its requests
// go to. It fails with ErrUnknownSession for a session the router hasn't
// seen.
func (c *Client) Session(ctx context.Context, id string) (Session, error) {
	var s Session
	err := c.call(ctx, http.MethodGet, "/sessions/"+url.PathEscape(id), nil, &s)
	// A 404 is the router's for an unknown session only when it says why:
	// anything else answering on the socket is trouble.
	if d, ok := errors.AsType[declined](err); ok && d.status == http.StatusNotFound && d.reason != "" {
		return s, unknownSession(id)
	}
	return s, err
}

// unknownSession is the error for the session with the given id, which the
// router hasn't seen.
func unknownSession(id string) error {
	return fmt.Errorf("%w %s", ErrUnknownSession, id)
}

// Pin has the router send every new session to the account with the given
// id, and with move, every running session too, on its next request. It
// returns the status document as pinning leaves it, and fails, saying why,
// for an account the router can't send requests on.
func (c *Client) Pin(ctx context.Context, account string, move bool) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, http.MethodPost, "/pin", pinRequest{Account: account, Move: move}, &doc)
	return doc, err
}

// Unpin has the router route every session on its merits again. It returns
// the status document as unpinning leaves it.
func (c *Client) Unpin(ctx context.Context) (status.Document, error) {
	var doc status.Document
	err := c.call(ctx, http.MethodDelete, "/pin", nil, &doc)
	return doc, err
}

// call sends the router a request for path, with body as JSON unless it's
// nil, and decodes its answer into answer.
func (c *Client) call(ctx context.Context, method, path string, body, answer any) error {
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
