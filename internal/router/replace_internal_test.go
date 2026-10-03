package router

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

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

func TestARouterToldToStopAsItRestartsLeavesNothingToConnectToAsItsControlAPIGoes(t *testing.T) {
	logstest.Capture(t)
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := newTestRouter(t, at(start), &stubProber{})
	r.cfg.StateDir = dir
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
	proxySrv, controlSrv := newServer(http.NotFoundHandler()), newServer(r.Control())
	// The control API, as it goes, finds whether the proxy's address takes a
	// connection, and whether its own socket is there to connect to; the
	// proxy's listener is slow to close, as on a loaded machine, until the
	// control API has gone or a while has passed.
	type left struct{ proxy, socket bool }
	going, gone := make(chan left, 1), make(chan struct{})
	var serving sync.WaitGroup
	serving.Go(func() {
		_ = controlSrv.Serve(hooked{Listener: control, first: func() {
			conn, err := net.Dial("tcp", proxy.Addr().String())
			if err == nil {
				_ = conn.Close()
			}
			_, statErr := os.Stat(SocketPath(dir))
			going <- left{proxy: err == nil, socket: statErr == nil}
			close(gone)
		}})
	})
	serving.Go(func() {
		_ = proxySrv.Serve(hooked{Listener: proxy, first: func() {
			select {
			case <-gone:
			case <-time.After(50 * time.Millisecond):
			}
		}})
	})
	// Both answer before the router is told to stop, as a router's do.
	if _, err := NewClient(SocketPath(dir)).Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + proxy.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	stopped, stop := context.WithCancel(t.Context())
	stop()

	if r.drainHandingOver(stopped, controlSrv, proxySrv, held) != nil {
		t.Error("drainHandingOver returned the listeners held, want none: the router was told to stop")
	}
	serving.Wait()
	found := <-going
	if found.proxy {
		t.Error("the proxy's address took a connection as the control API went, want it refused: a launcher that finds the router gone must find nothing on its proxy's address")
	}
	if found.socket {
		t.Error("the control socket was there as the control API went, want it removed first: a launcher connecting to it as it closes can be left hanging")
	}
}

// hooked is a listener that does first as it's closed, then closes.
type hooked struct {
	net.Listener
	first func()
}

func (h hooked) Close() error {
	h.first()
	return h.Listener.Close()
}
