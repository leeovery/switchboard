package router

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransportSpeaksHTTP2WhereTheUpstreamDoes(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	transport := newTransport()
	// Trust the test server's certificate, as the test server's own client does.
	transport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	t.Cleanup(transport.CloseIdleConnections)

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "HTTP/2.0" {
		t.Errorf("the upstream was asked over %s, want HTTP/2.0", body)
	}
}

func TestTransportChecksOnQuietHTTP2Connections(t *testing.T) {
	h2 := newTransport().HTTP2
	if h2 == nil || h2.SendPingTimeout != 30*time.Second || h2.PingTimeout != 15*time.Second {
		t.Errorf("HTTP2 = %+v, want a ping sent after 30s without a frame, and the connection closed if it's unanswered for 15s", h2)
	}
}

func TestTransportDialsAsTheDefaultTransportDoes(t *testing.T) {
	// Clone copies the default's dial function itself, so their code pointers
	// match. In tests that function is testguard's, which is what keeps the
	// upstream on this machine.
	want := reflect.ValueOf(http.DefaultTransport.(*http.Transport).DialContext).Pointer()
	if got := reflect.ValueOf(newTransport().DialContext).Pointer(); got != want {
		t.Error("the upstream transport dials otherwise than http.DefaultTransport, so it loses the default's dial timeouts, and in tests testguard's check of where it dials")
	}
}

func TestTransportWaitsForAResponseAsLongAsItTakes(t *testing.T) {
	transport := newTransport()
	if transport.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want none: a long thinking request takes minutes to answer", transport.ResponseHeaderTimeout)
	}
	if want := 10 * time.Second; transport.TLSHandshakeTimeout != want {
		t.Errorf("TLSHandshakeTimeout = %v, want the default's %v", transport.TLSHandshakeTimeout, want)
	}
}

func TestARenewedPoolKeepsNoConnectionFromBefore(t *testing.T) {
	srv := newCountingServer(t)
	p := newPool()
	t.Cleanup(func() { p.current().CloseIdleConnections() })
	get := func() {
		t.Helper()
		if err := getOver(t.Context(), p, srv.URL); err != nil {
			t.Fatal(err)
		}
	}

	get()
	get()
	if n := srv.opened.Load(); n != 1 {
		t.Fatalf("two requests opened %d connections, want one, kept between them", n)
	}
	p.renew()
	srv.awaitClose(t, "the connection kept from before the pool was renewed")
	get()
	if n := srv.opened.Load(); n != 2 {
		t.Errorf("once the pool is renewed, requests have opened %d connections, want one more", n)
	}
}

// countingServer answers every request, counting the connections opened to
// it, and telling of each as it closes.
type countingServer struct {
	*httptest.Server
	opened atomic.Int32
	closed chan struct{}
}

func newCountingServer(t *testing.T) *countingServer {
	t.Helper()
	s := &countingServer{closed: make(chan struct{}, 2)}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			s.opened.Add(1)
		case http.StateClosed:
			s.closed <- struct{}{}
		}
	}
	s.Start()
	t.Cleanup(s.Close)
	return s
}

// awaitClose fails t unless a connection to s closes within 5 seconds, which
// names.
func (s *countingServer) awaitClose(t *testing.T, which string) {
	t.Helper()
	select {
	case <-s.closed:
	case <-time.After(5 * time.Second):
		t.Fatal(which + " is open still, want it closed, as it was idle")
	}
}

// getOver gets url over transport, reading its answer to the end, so the
// connection it came on is kept for the next request.
func getOver(ctx context.Context, transport http.RoundTripper, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}
