package router

import (
	"net/http"
	"testing"
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
