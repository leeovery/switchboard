package router

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestARouterToldToStopAsItComesToExecDoesntReplaceItself(t *testing.T) {
	log := logstest.Capture(t)
	h := newHolding(t)
	h.r.cfg.Binary = Watched{path: "/test/bin/switchboard"}
	execs := 0
	h.r.cfg.Exec = func(string, []string, []string) error {
		execs++
		return nil
	}
	_, _ = h.proxy.Close(), h.control.Close()
	stopped, stop := context.WithCancel(t.Context())
	stop()

	h.r.replace(stopped, h.held)
	if execs != 0 {
		t.Errorf("exec'd %d times, want none: the router was told to stop", execs)
	}
	for _, want := range []string{`msg="replacing itself"`, `msg="told to stop as it restarted; exiting"`} {
		if !log.Has("level=INFO", want) {
			t.Errorf("log reads\n%s\nwant a line with %s", log, want)
		}
	}
	if _, err := os.Stat(SocketPath(h.dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket once exited: %v, want it gone", err)
	}
	if conn, err := net.Dial("tcp", h.proxy.Addr().String()); err == nil {
		_ = conn.Close()
		t.Error("the proxy's address takes a connection once the router exited, want nothing left holding its socket")
	}
}

func TestARouterToldToStopAsItRestartsLeavesNothingToConnectToAsItsControlAPIGoes(t *testing.T) {
	logstest.Capture(t)
	h := newHolding(t)
	proxySrv, controlSrv := newProxyServer(http.NotFoundHandler()), newServer(h.r.Control())
	// The control API, as it goes, finds whether the proxy's address takes a
	// connection, and whether its own socket is there to connect to; the
	// proxy's listener is slow to close, as on a loaded machine, until the
	// control API has gone or a while has passed.
	type left struct{ proxy, socket bool }
	going, gone := make(chan left, 1), make(chan struct{})
	var serving sync.WaitGroup
	serving.Go(func() {
		_ = controlSrv.Serve(hooked{Listener: h.control, first: func() {
			conn, err := net.Dial("tcp", h.proxy.Addr().String())
			if err == nil {
				_ = conn.Close()
			}
			_, statErr := os.Stat(SocketPath(h.dir))
			going <- left{proxy: err == nil, socket: statErr == nil}
			close(gone)
		}})
	})
	serving.Go(func() {
		_ = proxySrv.Serve(hooked{Listener: h.proxy, first: func() {
			select {
			case <-gone:
			case <-time.After(50 * time.Millisecond):
			}
		}})
	})
	// Both answer before the router is told to stop, as a router's do.
	if _, err := NewClient(SocketPath(h.dir)).Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + h.proxy.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	stopped, stop := context.WithCancel(t.Context())
	stop()

	if h.r.drainHandingOver(stopped, controlSrv, proxySrv, h.held) != nil {
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

func TestARouterRestartingInPlaceAnswersOnItsControlSocketWhileTheRequestsItCutOffUnwind(t *testing.T) {
	logstest.Capture(t)
	h := newHolding(t)
	h.r.cfg.DrainFor = 50 * time.Millisecond
	upstream := &unwinding{sent: make(chan struct{}), cutOff: make(chan struct{}), release: make(chan struct{})}
	h.r.proxy.transport = upstream
	proxySrv, controlSrv := newProxyServer(h.r.Proxy()), newServer(h.r.Control())
	var serving sync.WaitGroup
	serving.Go(func() { _ = proxySrv.Serve(h.proxy) })
	serving.Go(func() { _ = controlSrv.Serve(h.control) })
	go func() {
		req, err := http.NewRequest(http.MethodPost, "http://"+h.proxy.Addr().String()+"/v1/messages", strings.NewReader(opusAsked))
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+workToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-upstream.sent

	handed := make(chan *handover.Held, 1)
	go func() { handed <- h.r.drainHandingOver(t.Context(), controlSrv, proxySrv, h.held) }()
	<-upstream.cutOff
	if _, err := NewClient(SocketPath(h.dir)).Health(t.Context()); err != nil {
		t.Errorf("Health() = %v while the request cut off unwinds, want the router answering: a claude launched meanwhile would go unrouted", err)
	}
	close(upstream.release)
	if <-handed == nil {
		t.Error("drainHandingOver returned no listeners, want those held, to hand over")
	}
	serving.Wait()
}

// holding is a router whose listeners are held to hand over, as one
// restarting in place holds them: the proxy's, and the control API's, in its
// state directory, dir, one of the test's, short enough for the control
// socket.
type holding struct {
	r              *Router
	dir            string
	proxy, control net.Listener
	held           *handover.Held
}

// newHolding returns a router of testConfigured's, holding its listeners, as
// holding says.
func newHolding(t *testing.T) holding {
	t.Helper()
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
	t.Cleanup(held.Close)
	return holding{r: r, dir: dir, proxy: proxy, control: control, held: held}
}

// unwinding is an upstream that holds the request sent to it until its
// context ends, as when the router cuts it off, then gives it up once
// release closes, telling of the request as it's sent, and as it's cut off.
type unwinding struct {
	sent, cutOff, release chan struct{}
}

func (u *unwinding) RoundTrip(r *http.Request) (*http.Response, error) {
	_ = r.Body.Close()
	close(u.sent)
	<-r.Context().Done()
	close(u.cutOff)
	<-u.release
	return nil, r.Context().Err()
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
