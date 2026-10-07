package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestServers(t *testing.T) {
	log := logstest.Capture(t)
	srv := newServer(http.NotFoundHandler())

	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want none: responses stream for as long as they take", srv.WriteTimeout)
	}
	srv.ErrorLog.Print("http: TLS handshake error from 127.0.0.1:52100: EOF")
	if !log.Has("level=WARN", `msg="http: TLS handshake error from 127.0.0.1:52100: EOF"`, "component=router") {
		t.Errorf("log reads\n%s\nwant the server's error, as the router's warning", log)
	}
}

func TestAStoppingRouterWaitsAWhileForItsRoutedRequestsToUnwind(t *testing.T) {
	tests := []struct {
		name string
		// routed and passed are how long a routed request in flight, and one
		// passed through, take to finish: zero where there's none.
		routed, passed time.Duration
		wantWait       time.Duration
		// wantLeft is how many requests the router warns are still in flight,
		// once it gives up waiting: none for no warning.
		wantLeft int
	}{
		{name: "none in flight"},
		{name: "one passed through, which has no line", passed: time.Hour},
		{name: "a routed one finishing in time", routed: 3 * time.Second, wantWait: 3 * time.Second},
		{name: "a routed one still in flight once the time has passed, beside one passed through", routed: time.Hour, passed: time.Hour,
			wantWait: 5 * time.Second, wantLeft: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				r := newTestRouter(t, at(start), &stubProber{})
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "side", reason: reasonNew}, start)
				// The routed request goes out on side, and the one passed through
				// on work's token, which it carries.
				r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{sideToken: {after(tt.routed, served)}, workToken: {after(tt.passed, served)}}}
				// The clients of any requests still in flight go as the test ends.
				clients, gone := context.WithCancel(t.Context())
				defer gone()
				if tt.routed > 0 {
					go routeAs(clients, r, "/v1/messages", "one", opusAsked)
				}
				if tt.passed > 0 {
					upload := httptest.NewRequestWithContext(clients, http.MethodPost, "/v1/files", strings.NewReader("a file"))
					upload.Header.Set("Authorization", "Bearer "+workToken)
					go r.Proxy().ServeHTTP(httptest.NewRecorder(), upload)
				}
				synctest.Wait()

				began := time.Now()
				r.awaitUnwinding()
				if waited := time.Since(began); waited != tt.wantWait {
					t.Errorf("waited %v, want %v", waited, tt.wantWait)
				}
				warned := log.Has("level=WARN", `msg="requests still in flight as the router stops; their lines go unwritten"`,
					"requests="+strconv.Itoa(tt.wantLeft), "after=5s")
				if warned != (tt.wantLeft > 0) {
					t.Errorf("log reads\n%s\nwant a warning of %d requests still in flight", log, tt.wantLeft)
				}
			})
		})
	}
}
