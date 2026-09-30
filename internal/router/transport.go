package router

import (
	"net/http"
	"sync"
	"time"
)

// pool sends requests upstream on the transport newTransport makes, with the
// connections it keeps, until renew makes another in its place. It's safe
// for concurrent use.
type pool struct {
	mu        sync.Mutex
	transport *http.Transport
}

func newPool() *pool {
	return &pool{transport: newTransport()}
}

func (p *pool) RoundTrip(r *http.Request) (*http.Response, error) {
	return p.current().RoundTrip(r)
}

// current is the transport requests go upstream on now.
func (p *pool) current() *http.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.transport
}

// renew has the requests from now on go upstream on a transport of their
// own, with connections of its own, as after the Mac sleeps, when those kept
// may be dead. Of the connections before, those idle close, and those with a
// request under way carry it on, as far as they can, and close once idle.
// Closing the idle alone wouldn't do: HTTP/2 carries every request on one
// connection, and one with a stream under way through the sleep would take
// the next request, and hang it until its pings failed.
func (p *pool) renew() {
	p.mu.Lock()
	before := p.transport
	p.transport = newTransport()
	p.mu.Unlock()
	before.CloseIdleConnections()
}

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
