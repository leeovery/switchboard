package router

import (
	"net/http"
	"time"
)

// newTransport returns the transport requests go upstream on:
// http.DefaultTransport's, with its dial and TLS handshake timeouts, speaking
// HTTP/2 where the upstream does.
func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	t.Protocols.SetHTTP2(true)
	// After a Mac sleeps, a pooled HTTP/2 connection can be dead, and a
	// request sent on it would hang until TCP gave up, minutes later. A ping
	// after 30 seconds without a frame finds out, closing the connection if
	// 15 more pass without an answer.
	t.HTTP2 = &http.HTTP2Config{SendPingTimeout: 30 * time.Second, PingTimeout: 15 * time.Second}
	// No response-header timeout: a long thinking request can take minutes
	// to send its first byte.
	t.ResponseHeaderTimeout = 0
	// Without this, a request that asks for no encoding would go upstream
	// asking for gzip, and its response would come back decoded. The client's
	// Accept-Encoding, or its absence, goes upstream as it is.
	t.DisableCompression = true
	return t
}
