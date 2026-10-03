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

	"github.com/leeovery/switchboard/internal/handover"
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
// too, taking up the listeners cfg.Handed names, where they're at those
// addresses, in place of listening afresh. A supervised router also restarts
// once its config file, its binary or the time zone has changed, or it's
// asked to: it replaces itself with its binary, handing its listeners over,
// as cfg.Exec says, or else returns nil, for launchd to start it again. It
// fails when another router already answers there, or when it can't listen.
// On its way out it stops taking requests, gives those in flight up to 30
// seconds to finish, saves its state, and removes the socket, but for one it
// hands over.
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
	ls, err := r.listen(ctx, socket)
	if err != nil {
		return err
	}
	r.proxyAddr = ls.proxy.Addr().String()
	// Only now is the state directory this router's: another starting
	// alongside would have failed by here.
	r.file.load(filepath.Join(r.cfg.StateDir, stateFileName))
	r.openHistory()
	return r.serve(ctx, ls)
}

// openHistory keeps the readings history in the state directory from now
// on, and takes up the readings it holds of the last half hour, so the
// recent rates outlast the router's restart.
func (r *Router) openHistory() {
	r.history.open(filepath.Join(r.cfg.StateDir, historyDirName))
	kept := r.state.seed(r.history.readBack(r.cfg.Now()))
	logger.Info("took up the readings history", "readings", kept)
}

// listeners are the proxy's listener and the control API's.
type listeners struct {
	proxy, control net.Listener
}

// close closes the listeners there are.
func (ls listeners) close() {
	for _, ln := range []net.Listener{ls.proxy, ls.control} {
		if ln != nil {
			_ = ln.Close()
		}
	}
}

// listen takes up the listeners handed over by the router this one replaced,
// where they're at the addresses this one listens on, and listens afresh for
// the rest: the proxy on its address, and the control API on the socket at
// path socket, once no other router answers there. A control socket handed
// over is this router's already.
func (r *Router) listen(ctx context.Context, socket string) (listeners, error) {
	ls := r.inherit(socket)
	var err error
	if ls.control == nil {
		err = ensureAlone(ctx, socket)
	}
	// The proxy's address is taken first: of two routers starting at once,
	// the one that loses it must fail before it touches the other's socket.
	if err == nil && ls.proxy == nil {
		ls.proxy, err = listen(r.cfg.Listen)
	}
	if err == nil && ls.control == nil {
		ls.control, err = listenControl(socket)
	}
	if err != nil {
		ls.close()
		return listeners{}, err
	}
	return ls, nil
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
// itself, keeping the state file, the readings history and what has happened
// lately, and posting notifications, then shuts both down, and restarting,
// replaces itself, handing their listeners over.
func (r *Router) serve(ctx context.Context, ls listeners) error {
	proxySrv, controlSrv := newServer(r.Proxy()), newServer(r.Control())
	var serving sync.WaitGroup
	failed := make(chan error, 2)
	serving.Go(func() { failed <- serveOn(proxySrv, ls.proxy) })
	serving.Go(func() { failed <- serveOn(controlSrv, ls.control) })
	r.logStart(ls.proxy.Addr(), ls.control.Addr())
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
	running.Go(func() { r.history.run(background) })
	running.Go(func() { r.recent.run(background) })
	if r.notifications != nil {
		running.Go(func() { r.notifications.run(background) })
	}

	var err error
	var held *handover.Held
	select {
	case <-ctx.Done():
	case err = <-failed:
	case <-r.upkeep.restarted():
		held = r.hold(ls)
	}
	logger.Info("stopping")
	stopLooking()
	looked.Wait()
	r.probes.stop()
	if held != nil {
		held = r.drainHandingOver(ctx, controlSrv, proxySrv, held)
	} else {
		shutdown(controlSrv, proxySrv)
	}
	serving.Wait()
	stopBackground()
	running.Wait()
	logger.Info("stopped")
	if held != nil {
		r.replace(ctx, held)
	}
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
	drain(proxy)
}

// drain stops the proxy taking requests, giving those in flight DrainTimeout
// to finish, and cuts off any still going then.
func drain(proxy *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), DrainTimeout)
	defer cancel()
	if err := proxy.Shutdown(ctx); err != nil {
		logger.Warn("cut off requests still in flight", "after", DrainTimeout)
		_ = proxy.Close()
	}
}

// logStart notes where the router listens, where it sends requests, and which
// accounts it can send them on, warning of each it can't, and why, and when
// it can send them on none yet.
func (r *Router) logStart(proxy, control net.Addr) {
	tokens := make([]any, len(r.accounts))
	for i, a := range r.accounts {
		tokens[i] = slog.Bool(a.ID, a.hasToken())
		if !a.hasToken() {
			logger.Warn("account has no usable token; nothing will go out on it", "account", a.ID, "error", a.problem())
		}
	}
	if len(r.accounts.sendable()) == 0 {
		logger.Warn("no account has a usable token yet; nothing will be routed until one has")
	}
	logger.Info("listening", "address", proxy.String(), "control", control.String(),
		"upstream", r.upstream.Redacted(), slog.Group("token_set", tokens...))
}
