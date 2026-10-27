package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestARequestClosedUnansweredIsToldApart(t *testing.T) {
	asked := func(err error) error { return &url.Error{Op: "Get", URL: "http://switchboard/health", Err: err} }
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "closed at its end", err: asked(io.EOF), want: true},
		{name: "reset as it was read", err: asked(&net.OpError{Op: "read", Net: "unix", Err: syscall.ECONNRESET}), want: true},
		{name: "closed as the request was written", err: asked(&net.OpError{Op: "write", Net: "unix", Err: syscall.EPIPE}), want: true},
		{name: "disconnected as the request was written", err: asked(&net.OpError{Op: "write", Net: "unix", Err: os.NewSyscallError("write", syscall.ENOTCONN)}), want: true},
		{name: "closed by the transport as it read", err: asked(&net.OpError{Op: "read", Net: "unix", Err: net.ErrClosed}), want: true},
		{name: "closed before the request was read", err: asked(errors.New(serverClosedIdle)), want: true},
		{name: "cut off by its deadline", err: asked(context.DeadlineExceeded)},
		{name: "answered with what isn't HTTP", err: asked(errors.New(`net/http: HTTP/1.x transport connection broken: malformed HTTP status code "2"`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := closedUnanswered(tt.err); got != tt.want {
				t.Errorf("closedUnanswered(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAClientWhoseRequestTheRouterClosedUnreadFindsItNotRunning(t *testing.T) {
	// The transport fails the request as net/http's does where the server
	// closes the connection before reading the request, as a router from
	// before cut those it had accepted as it stopped.
	closedUnread := roundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New(serverClosedIdle) })
	client := &Client{http: &http.Client{Transport: closedUnread}}

	_, err := client.Health(t.Context())
	if !errors.Is(err, ErrUnanswered) || !IsNotRunning(err) {
		t.Errorf("Health() error = %v, want ErrUnanswered, the router not running at that moment", err)
	}
}

func TestARouterClosingEachConnectionAsItAcceptsItIsNotRunningToItsClient(t *testing.T) {
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "control.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	// The close races the request's write and the transport's read, so it
	// fails as each of them, by turns: net/http's own words for a close
	// before the request was read come in about one ask in six to nine with
	// several processors, but about one in 1,500 with one; a disconnected
	// write only with several. So it asks at least atLeast times, then on
	// until those words have come, within atMost asks or the time it's given.
	const atLeast, atMost, given = 300, 20_000, 10 * time.Second
	client := NewClient(path)
	closedIdle := false
	asked := 0
	for start := time.Now(); asked < atMost && time.Since(start) < given && (asked < atLeast || !closedIdle); asked++ {
		_, err := client.Health(t.Context())
		if !IsNotRunning(err) {
			t.Fatalf("Health() error = %v, want the router not running at that moment", err)
		}
		closedIdle = closedIdle || strings.Contains(err.Error(), serverClosedIdle)
	}
	if !closedIdle {
		t.Errorf("no ask in %d failed with %q, the words net/http's transport gives a close before the request was read", asked, serverClosedIdle)
	}
}

// roundTripper is a transport that answers each request as it says.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
