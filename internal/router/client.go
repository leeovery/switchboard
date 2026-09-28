package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// ErrNotRunning is what a Client's calls fail with, wrapped, when no router
// listens on its socket.
var ErrNotRunning = errors.New("switchboard isn't running")

// clientTimeout bounds each of a Client's calls, so a router that's stuck
// can't hang its caller.
const clientTimeout = 5 * time.Second

// Client asks a running router about itself over its control socket.
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
	err := c.get(ctx, "/health", &h)
	return h, err
}

// Status asks for the router's status document.
func (c *Client) Status(ctx context.Context) (status.Document, error) {
	var doc status.Document
	err := c.get(ctx, "/status", &doc)
	return doc, err
}

// get asks the router for path, and decodes its answer into answer.
func (c *Client) get(ctx context.Context, path string, answer any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://switchboard"+path, nil)
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
		return fmt.Errorf("the router answered GET %s with %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(answer); err != nil {
		return fmt.Errorf("read the router's answer to GET %s: %w", path, err)
	}
	return nil
}

// isDial reports whether err is a failure to connect at all: there's no
// socket, or nothing listens on it.
func isDial(err error) bool {
	op, ok := errors.AsType[*net.OpError](err)
	return ok && op.Op == "dial"
}
