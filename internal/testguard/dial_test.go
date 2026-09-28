package testguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestOnThisMachine(t *testing.T) {
	tests := []struct {
		network string
		addr    string
		want    bool
	}{
		{network: "tcp", addr: "127.0.0.1:4747", want: true},
		{network: "tcp", addr: "127.8.9.10:80", want: true},
		{network: "tcp4", addr: "127.0.0.1:0", want: true},
		{network: "tcp", addr: "[::1]:443", want: true},
		{network: "tcp6", addr: "[::1%lo0]:443", want: true},
		{network: "tcp", addr: "localhost:4747", want: true},
		{network: "tcp", addr: "LocalHost:4747", want: true},
		{network: "unix", addr: "/tmp/switchboard/control.sock", want: true},
		{network: "unixgram", addr: "/tmp/switchboard/log.sock", want: true},
		{network: "tcp", addr: "192.0.2.1:443", want: false},
		{network: "tcp", addr: "api.anthropic.com:443", want: false},
		{network: "tcp", addr: "localhost.example.com:80", want: false},
		{network: "tcp", addr: "[2001:db8::1]:443", want: false},
		{network: "tcp", addr: "0.0.0.0:80", want: false},
		{network: "udp", addr: "192.0.2.1:53", want: false},
		{network: "tcp", addr: "127.0.0.1", want: false},
	}
	for _, tt := range tests {
		if got := onThisMachine(tt.network, tt.addr); got != tt.want {
			t.Errorf("onThisMachine(%q, %q) = %v, want %v", tt.network, tt.addr, got, tt.want)
		}
	}
}

func TestDialGuardBlocksDialsOffTheMachine(t *testing.T) {
	var dialled []string
	guard := &dialGuard{}
	dial := guard.wrap(func(_ context.Context, _, addr string) (net.Conn, error) {
		dialled = append(dialled, addr)
		return nil, errors.New("a stand-in never connects")
	})

	_, err := dial(t.Context(), "tcp", "192.0.2.1:443")
	if want := "testguard: blocked dial to 192.0.2.1:443"; err == nil || err.Error() != want {
		t.Errorf("dial to 192.0.2.1:443: error = %v, want %q", err, want)
	}
	_, _ = dial(t.Context(), "tcp", "api.anthropic.com:443")
	_, _ = dial(t.Context(), "tcp", "192.0.2.1:443")
	_, _ = dial(t.Context(), "tcp", "127.0.0.1:4747")

	if want := []string{"127.0.0.1:4747"}; !slices.Equal(dialled, want) {
		t.Errorf("dials passed on = %q, want %q alone", dialled, want)
	}
	want := []string{"blocked dial to 192.0.2.1:443 (2 times)", "blocked dial to api.anthropic.com:443"}
	if got := guard.escapes(); !slices.Equal(got, want) {
		t.Errorf("escapes() = %q, want %q", got, want)
	}
}

func TestDialGuardHoldsForTransportsClonedAfter(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	guard := &dialGuard{}
	transport := &http.Transport{}
	if err := guard.install(transport); err != nil {
		t.Fatalf("install() error = %v", err)
	}

	for name, tr := range map[string]*http.Transport{"the transport": transport, "its clone": transport.Clone()} {
		if _, err := tr.DialContext(t.Context(), "tcp", "192.0.2.1:443"); err == nil || !strings.Contains(err.Error(), "blocked dial") {
			t.Errorf("%s dialled 192.0.2.1:443: error = %v, want it blocked", name, err)
		}
		conn, err := tr.DialContext(t.Context(), "tcp", ln.Addr().String())
		if err != nil {
			t.Errorf("%s dialled a loopback listener: error = %v, want a connection", name, err)
			continue
		}
		_ = conn.Close()
	}
}

func TestDialGuardNeedsAnHTTPTransport(t *testing.T) {
	if err := (&dialGuard{}).install(roundTripper(nil)); err == nil {
		t.Error("install() of a RoundTripper that isn't an *http.Transport: no error, want one")
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
