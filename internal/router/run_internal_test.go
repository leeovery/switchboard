package router

import (
	"net/http"
	"strconv"
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

func TestAStoppingRouterWaitsAWhileForTheRequestsStillInFlight(t *testing.T) {
	tests := []struct {
		name string
		// unwinding is how long each request in flight takes to finish.
		unwinding []time.Duration
		wantWait  time.Duration
		// wantLeft is how many requests the router warns are still in flight,
		// once it gives up waiting: none for no warning.
		wantLeft int
	}{
		{name: "none in flight"},
		{name: "each finishing in time", unwinding: []time.Duration{time.Second, 3 * time.Second}, wantWait: 3 * time.Second},
		{name: "some still in flight once the time has passed", unwinding: []time.Duration{time.Second, time.Minute, time.Hour}, wantWait: 5 * time.Second, wantLeft: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				r := newTestRouter(t, at(start), &stubProber{})
				for _, d := range tt.unwinding {
					r.inFlight.begin()
					time.AfterFunc(d, r.inFlight.end)
				}

				began := time.Now()
				r.awaitInFlight()
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
