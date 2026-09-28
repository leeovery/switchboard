package router

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestTransportWaitsForAResponseAsLongAsItTakes(t *testing.T) {
	transport := newTransport()
	if transport.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout = %v, want none: a long thinking request takes minutes to answer", transport.ResponseHeaderTimeout)
	}
	if want := 10 * time.Second; transport.TLSHandshakeTimeout != want {
		t.Errorf("TLSHandshakeTimeout = %v, want the default's %v", transport.TLSHandshakeTimeout, want)
	}
}
