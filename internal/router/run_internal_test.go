package router

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
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
		// upgrades is set where the routed request upgrades its connection,
		// which closing the server leaves open.
		upgrades bool
		wantWait time.Duration
		// wantLeft is how many requests the router warns are still in flight,
		// once it gives up waiting: none for no warning.
		wantLeft int
	}{
		{name: "none in flight"},
		{name: "one passed through, which has no line", passed: time.Hour},
		{name: "a routed one finishing in time", routed: 3 * time.Second, wantWait: 3 * time.Second},
		{name: "a routed one still in flight once the time has passed, beside one passed through", routed: time.Hour, passed: time.Hour,
			wantWait: 5 * time.Second, wantLeft: 1},
		{name: "a routed one upgrading its connection", routed: time.Hour, upgrades: true},
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
					req := claudeCodeAsks(clients, "/v1/messages", "one", strings.NewReader(opusAsked))
					if tt.upgrades {
						req.Header.Set("Connection", "Upgrade")
						req.Header.Set("Upgrade", "websocket")
					}
					go r.Proxy().ServeHTTP(httptest.NewRecorder(), req)
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

func TestTheControlAPIAnswersAnAskItAcceptedAsItCloses(t *testing.T) {
	paths := []struct {
		name string
		// close closes the router's control API, as the path it's on does.
		close func(ctx context.Context, r *Router, control controlServer, held *handover.Held)
	}{
		{
			name: "restarting in place",
			close: func(ctx context.Context, r *Router, control controlServer, held *handover.Held) {
				r.drainHandingOver(ctx, control, newProxyServer(http.NotFoundHandler()), held, func() {})
			},
		},
		{
			name: "stopping",
			close: func(_ context.Context, r *Router, control controlServer, _ *handover.Held) {
				r.shutdown(control, newProxyServer(http.NotFoundHandler()))
			},
		},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			held := holdAProxy(t)
			synctest.Test(t, func(t *testing.T) {
				logstest.Capture(t)
				r := newTestRouter(t, at(start), &stubProber{})
				// The ask is accepted, and answered once answer closes, as on a
				// machine far behind.
				asked, answer := make(chan struct{}), make(chan struct{})
				control := newControlServer(delayed("/health", r.Control(), func(*http.Request) {
					close(asked)
					<-answer
				}))
				ln := servePipes(t, control.Server)
				answered := make(chan error, 1)
				go func() {
					_, err := ln.client().Health(t.Context())
					answered <- err
				}()
				<-asked

				closed := make(chan struct{})
				go func() {
					path.close(t.Context(), r, control, held)
					close(closed)
				}()
				synctest.Wait()
				select {
				case <-closed:
					t.Error("the control API closed with an ask it had accepted unanswered, want it answered first")
				default:
				}
				close(answer)
				if err := <-answered; err != nil {
					t.Errorf("Health() = %v, asked as the control API closed, want it answered: a claude launched then would stop", err)
				}
				<-closed
			})
		})
	}
}

func TestTheControlAPIHoldsTheStopUpNoLongerThanItsWait(t *testing.T) {
	tests := []struct {
		name string
		// open opens a request to the control API, served on ln, that the
		// router doesn't answer at once, and returns what's closed as its
		// client finds it ended.
		open func(t *testing.T, ln *pipes) <-chan struct{}
		// wantWait is how long closing the control API takes, but where
		// wantShorter is set, when it's less than controlCloseWait.
		wantWait    time.Duration
		wantShorter bool
	}{
		{
			name: "a reader of the request stream, as a dashboard's",
			open: func(t *testing.T, ln *pipes) <-chan struct{} {
				events, err := ln.client().Stream(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				ended := make(chan struct{})
				go func() {
					for range events {
					}
					close(ended)
				}()
				return ended
			},
			wantShorter: true,
		},
		{
			name: "a request waiting on more, until it's cut off",
			open: func(t *testing.T, ln *pipes) <-chan struct{} {
				ended := make(chan struct{})
				go func() {
					_, _ = ln.client().Refresh(t.Context(), time.Minute)
					close(ended)
				}()
				return ended
			},
			wantWait: controlCloseWait,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				logstest.Capture(t)
				r := newTestRouter(t, at(start), &stubProber{})
				control := newControlServer(delayed("/refresh", r.Control(), func(req *http.Request) { <-req.Context().Done() }))
				ln := servePipes(t, control.Server)
				ended := tt.open(t, ln)
				synctest.Wait()

				began := time.Now()
				r.shutdown(control, newProxyServer(http.NotFoundHandler()))
				took := time.Since(began)
				if tt.wantShorter && took >= controlCloseWait || !tt.wantShorter && took != tt.wantWait {
					t.Errorf("stopping took %v, want %v (less than %v: %v)", took, tt.wantWait, controlCloseWait, tt.wantShorter)
				}
				<-ended
			})
		})
	}
}

func TestAStoppingRouterClosesItsControlAPIWhileItsRequestsDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logstest.Capture(t)
		r := newTestRouter(t, at(start), &stubProber{})
		r.cfg.DrainFor = 10 * time.Second
		// A request in flight at the proxy runs until it's cut off, as does a
		// request the control API has accepted.
		proxy := newProxyServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) { <-req.Context().Done() }))
		proxied := servePipes(t, proxy.Server)
		control := newControlServer(delayed("/sessions", r.Control(), func(req *http.Request) { <-req.Context().Done() }))
		ln := servePipes(t, control.Server)
		go func() {
			if resp, err := (&http.Client{Transport: proxied.transport()}).Get("http://switchboard/v1/messages"); err == nil {
				_ = resp.Body.Close()
			}
		}()
		began := time.Now()
		asked := make(chan time.Duration, 1)
		go func() {
			_, _ = ln.client().Sessions(t.Context())
			asked <- time.Since(began)
		}()
		synctest.Wait()

		stopped := make(chan struct{})
		go func() {
			r.shutdown(control, proxy)
			close(stopped)
		}()
		synctest.Wait()
		if _, err := ln.client().Health(t.Context()); !errors.Is(err, ErrNotRunning) {
			t.Errorf("Health() = %v as the proxy's requests drain, want nothing listening: the control API closes at once", err)
		}
		if took := <-asked; took != controlCloseWait {
			t.Errorf("the request the control API had accepted ended %v on, want %v, its wait, while the proxy's requests drain", took, controlCloseWait)
		}
		<-stopped
		if took := time.Since(began); took != r.cfg.DrainFor {
			t.Errorf("stopping took %v, want %v, the drain's", took, r.cfg.DrainFor)
		}
	})
}

func TestWhatARequestTheControlAPIAcceptedChangesIsChangedOnceItCloses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logstest.Capture(t)
		r := newTestRouter(t, at(start), &stubProber{})
		// The pin is made a moment after the request is cut off, as by a
		// handler held up, as on a lock.
		control := newControlServer(delayed("/pin", r.Control(), func(req *http.Request) {
			<-req.Context().Done()
			time.Sleep(time.Millisecond)
		}))
		ln := servePipes(t, control.Server)
		go func() { _, _ = ln.client().Pin(t.Context(), PinRequest{Accounts: []string{"side"}, By: ByCLI}) }()
		synctest.Wait()

		r.closeControl(control)
		if got := r.sessions.globalPin().Accounts; !slices.Equal(got, []string{"side"}) {
			t.Errorf("once the control API closed, the global pin was to %q, want the one asked for as it closed, to side: the state saved then would miss it", got)
		}
		pinned := func(e status.Event) bool { return e.Kind == status.EventPin && e.Account == "side" }
		if events := r.recent.events(); !slices.ContainsFunc(events, pinned) {
			t.Errorf("once the control API closed, the events kept were %+v, want the pin to side among them: the events filed then would miss it", events)
		}
	})
}

// delayed returns next, but for a request for path, which waits as wait says
// first.
func delayed(path string, next http.Handler, wait func(req *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == path {
			wait(req)
		}
		next.ServeHTTP(w, req)
	})
}

// holdAProxy holds a listener, as a router restarting in place holds its
// proxy's, until the test ends.
func holdAProxy(t *testing.T) *handover.Held {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	held, err := handover.Hold(map[string]net.Listener{proxyListener: ln})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(held.Close)
	return held
}

// servePipes serves srv on a listener of in-memory connections until the
// test ends, and returns the listener.
func servePipes(t *testing.T, srv *http.Server) *pipes {
	t.Helper()
	ln := &pipes{conns: make(chan net.Conn), closed: make(chan struct{})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln
}

// pipes is a listener whose connections are in memory, as net.Pipe makes
// them, so a server and its clients run within a synctest bubble.
type pipes struct {
	conns   chan net.Conn
	closed  chan struct{}
	closing sync.Once
}

func (p *pipes) Accept() (net.Conn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	case <-p.closed:
		return nil, net.ErrClosed
	}
}

func (p *pipes) Close() error {
	p.closing.Do(func() { close(p.closed) })
	return nil
}

func (p *pipes) Addr() net.Addr {
	return pipeAddr{}
}

// client returns a client of the router whose control API is served on p,
// which finds nothing listening once p is closed.
func (p *pipes) client() *Client {
	return &Client{http: &http.Client{Transport: p.transport()}}
}

// transport returns a transport whose every connection is to p, which finds
// nothing listening once p is closed.
func (p *pipes) transport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		server, client := net.Pipe()
		select {
		case p.conns <- server:
			return client, nil
		case <-p.closed:
			return nil, &net.OpError{Op: "dial", Net: "pipe", Err: syscall.ECONNREFUSED}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return transport
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }
