package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
)

const (
	// DrainTimeout is how long requests in flight get to finish once the
	// router is stopping. They can be long streams.
	DrainTimeout = 30 * time.Second
	// readHeaderTimeout bounds how long a client takes to send a request's
	// headers. Nothing bounds a response: it streams for as long as it takes.
	readHeaderTimeout = 10 * time.Second
)

// Run builds a router and serves until ctx ends: the proxy on cfg.Listen, and
// the control API on a socket in cfg.StateDir, where it keeps its state file
// too. A supervised router also stops, returning nil, to restart, once its
// config file or its binary has changed. It fails when another router
// already answers there, or when it can't listen. On its way out it stops
// taking requests, gives those in flight up to 30 seconds to finish, saves
// its state, and removes the socket.
func Run(ctx context.Context, cfg Config) error {
	r, err := New(cfg)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func (r *Router) run(ctx context.Context) error {
	socket := SocketPath(r.cfg.StateDir)
	if err := checkSocketPath(socket); err != nil {
		return err
	}
	if err := ensureAlone(ctx, socket); err != nil {
		return err
	}
	// The proxy's address is taken first: of two routers starting at once,
	// the one that loses it must fail before it touches the other's socket.
	proxyLn, err := listen(r.cfg.Listen)
	if err != nil {
		return err
	}
	r.proxyAddr = proxyLn.Addr().String()
	controlLn, err := listenControl(socket)
	if err != nil {
		_ = proxyLn.Close()
		return err
	}
	// Only now is the state directory this router's: another starting
	// alongside would have failed by here.
	r.file.load(filepath.Join(r.cfg.StateDir, stateFileName))
	return r.serve(ctx, proxyLn, controlLn)
}

// ensureAlone fails when a router already answers on the control socket at
// path. The check runs to its end even if ctx ends first, which would
// otherwise read as something on the socket failing to answer.
func ensureAlone(ctx context.Context, path string) error {
	h, err := NewClient(path).Health(context.WithoutCancel(ctx))
	switch {
	case err == nil:
		return fmt.Errorf("switchboard is already running (pid %d)", h.PID)
	case errors.Is(err, ErrNotRunning):
		return nil
	default:
		return fmt.Errorf("something is listening on %s, but not answering as switchboard does: %w", path, err)
	}
}

// listen listens on the proxy's address, saying plainly when another program
// already does.
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	switch {
	case errors.Is(err, syscall.EADDRINUSE):
		return nil, fmt.Errorf("can't listen on %s, as another program already is: stop it, or set listen in the config to another address (%w)", addr, err)
	case err != nil:
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, nil
}

// serve serves the proxy and the control API until ctx ends, either fails,
// or the router restarts itself, probing each account nothing has been read
// of in the meantime, priming the accounts on the schedule, looking after
// itself, keeping the state file and posting notifications, then shuts both
// down.
func (r *Router) serve(ctx context.Context, proxyLn, controlLn net.Listener) error {
	proxySrv, controlSrv := newServer(r.Proxy()), newServer(r.Control())
	var serving sync.WaitGroup
	failed := make(chan error, 2)
	serving.Go(func() { failed <- serveOn(proxySrv, proxyLn) })
	serving.Go(func() { failed <- serveOn(controlSrv, controlLn) })
	r.logStart(proxyLn.Addr(), controlLn.Addr())
	r.probes.start(r.accounts.sendable(), r.state.unread)
	looking, stopLooking := context.WithCancel(ctx)
	var looked sync.WaitGroup
	looked.Go(func() { r.upkeep.run(looking) })
	if r.primer != nil {
		looked.Go(func() { r.primer.run(looking) })
	}
	// The requests still in flight as the router stops change what's to be
	// saved, and what's to be told of, so keeping and notifying outlast ctx.
	background, stopBackground := context.WithCancel(context.WithoutCancel(ctx))
	var running sync.WaitGroup
	running.Go(func() { r.file.keep(background) })
	if r.notifications != nil {
		running.Go(func() { r.notifications.run(background) })
	}

	var err error
	select {
	case <-ctx.Done():
	case err = <-failed:
	case <-r.upkeep.restarted():
	}
	logger.Info("stopping")
	stopLooking()
	looked.Wait()
	r.probes.stop()
	shutdown(controlSrv, proxySrv)
	serving.Wait()
	stopBackground()
	running.Wait()
	logger.Info("stopped")
	return err
}

func newServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          logs.StdLogger("router", slog.LevelWarn),
	}
}

// serveOn serves srv on ln until it's shut down, which isn't a failure.
func serveOn(srv *http.Server, ln net.Listener) error {
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve on %s: %w", ln.Addr(), err)
	}
	return nil
}

// shutdown stops the servers taking requests. The control API goes at once,
// so a launcher that checks the router's health finds it gone and connects
// directly, and its listener's closing removes the socket. The proxy's
// requests in flight get DrainTimeout to finish.
func shutdown(control, proxy *http.Server) {
	_ = control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), DrainTimeout)
	defer cancel()
	if err := proxy.Shutdown(ctx); err != nil {
		logger.Warn("cut off requests still in flight", "after", DrainTimeout)
		_ = proxy.Close()
	}
}

// logStart notes where the router listens, where it sends requests, and which
// accounts it can send them on, warning of each it can't, and why.
func (r *Router) logStart(proxy, control net.Addr) {
	tokens := make([]any, len(r.accounts))
	for i, a := range r.accounts {
		tokens[i] = slog.Bool(a.ID, a.hasToken())
		if !a.hasToken() {
			logger.Warn("account has no usable token; nothing will go out on it", "account", a.ID, "error", a.problem())
		}
	}
	logger.Info("listening", "address", proxy.String(), "control", control.String(),
		"upstream", r.upstream.Redacted(), slog.Group("token_set", tokens...))
}
