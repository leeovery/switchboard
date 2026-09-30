package router

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"testing"

	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestARouterToldToStopAsItComesToExecDoesntReplaceItself(t *testing.T) {
	log := logstest.Capture(t)
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := newTestRouter(t, at(start), &stubProber{})
	r.cfg.StateDir, r.cfg.Binary = dir, Watched{path: "/test/bin/switchboard"}
	execs := 0
	r.cfg.Exec = func(string, []string, []string) error {
		execs++
		return nil
	}
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	control, err := net.Listen("unix", SocketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	held, err := handover.Hold(map[string]net.Listener{proxyListener: proxy, controlListener: control})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = proxy.Close(), control.Close()
	stopped, stop := context.WithCancel(t.Context())
	stop()

	r.replace(stopped, held)
	if execs != 0 {
		t.Errorf("exec'd %d times, want none: the router was told to stop", execs)
	}
	for _, want := range []string{`msg="replacing itself"`, `msg="told to stop as it restarted; exiting"`} {
		if !log.Has("level=INFO", want) {
			t.Errorf("log reads\n%s\nwant a line with %s", log, want)
		}
	}
	if _, err := os.Stat(SocketPath(dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket once exited: %v, want it gone", err)
	}
	if conn, err := net.Dial("tcp", proxy.Addr().String()); err == nil {
		_ = conn.Close()
		t.Error("the proxy's address takes a connection once the router exited, want nothing left holding its socket")
	}
}
