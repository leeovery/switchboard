package router

import "net/http"

// newTransport returns the transport requests go upstream on:
// http.DefaultTransport's, with its dial and TLS handshake timeouts, speaking
// HTTP/2 where the upstream does.
func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	t.Protocols.SetHTTP2(true)
	// No response-header timeout: a long thinking request can take minutes
	// to send its first byte.
	t.ResponseHeaderTimeout = 0
	// Without this, a request that asks for no encoding would go upstream
	// asking for gzip, and its response would come back decoded. The client's
	// Accept-Encoding, or its absence, goes upstream as it is.
	t.DisableCompression = true
	return t
}
