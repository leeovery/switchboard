package router

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"time"

	"github.com/leeovery/switchboard/internal/handover"
)

// The names a router hands its listeners over by.
const (
	proxyListener   = "proxy"
	controlListener = "control"
)

const (
	// execTries is how many times the router tries to exec its binary while
	// it isn't there, as for a moment while an upgrade moves its link on.
	execTries = 5
	// execRetry is how long it waits between tries, unless its config says
	// otherwise.
	execRetry = 500 * time.Millisecond
)

// errStopped is what an exec is answered with once the router has been told
// to stop, as by a signal: it exits for good rather than replace itself.
var errStopped = errors.New("told to stop")

// replaceable reports whether the router means to replace itself as it
// restarts, rather than exit for launchd to start it again: it has
// something to exec with, and knows its binary.
func (r *Router) replaceable() bool {
	return r.cfg.Exec != nil && r.cfg.Binary.path != ""
}

// hold holds the router's listeners open, to hand over as it replaces
// itself, or returns nil when it can't replace itself, and so exits instead:
// it has nothing to exec with, doesn't know its binary, or can't hold them.
func (r *Router) hold(ls listeners) *handover.Held {
	if !r.replaceable() {
		if r.cfg.Exec != nil {
			logger.Warn("can't replace itself, not knowing its binary; exiting for launchd to start it again")
		}
		return nil
	}
	held, err := handover.Hold(map[string]net.Listener{proxyListener: ls.proxy, controlListener: ls.control})
	if err != nil {
		logger.Warn("can't replace itself; exiting for launchd to start it again", "error", err)
		return nil
	}
	return held
}

// drainHandingOver stops the proxy taking requests, giving those in flight
// DrainTimeout to finish while the control API answers, so a session
// launched meanwhile sends its requests to the proxy's socket, held open,
// where they wait for the router this one becomes; then closes the control
// API, and returns held, to hand over. Told to stop meanwhile, as by a
// signal, it stops as at one after all: the sockets held close, so no
// request waits on one nothing will take up, its control socket is removed,
// and then the control API goes, so a launcher that finds the router gone
// finds nothing on its proxy's address either, and connects directly; it
// returns nil once the requests in flight have finished.
func (r *Router) drainHandingOver(ctx context.Context, control, proxy *http.Server, held *handover.Held) *handover.Held {
	refuse(proxy)
	drained := make(chan struct{})
	go func() {
		drain(proxy)
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
	}
	if ctx.Err() != nil {
		logger.Info("told to stop as it restarted; stopping instead")
		held.Close()
		// Removed before the control API closes its socket: macOS can leave a
		// connection made to a unix socket as it closes hanging, neither
		// answered nor closed (proven by experiment).
		r.removeSocket()
		_ = control.Close()
		<-drained
		return nil
	}
	_ = control.Close()
	return held
}

// replace replaces this process with the router's binary, by the path it was
// started as, which an upgrade leads on to the new version, run as it was,
// handing it the listeners held, for the router it becomes to take up: no
// connection is refused meanwhile, and the process, keeping its id, isn't
// started afresh, which macOS can refuse a binary an upgrade replaced. It
// returns only when that fails, the router then exiting as though it had
// stopped, for launchd to start it again, or once it's told to stop, as by a
// signal, when it exits for good.
func (r *Router) replace(ctx context.Context, held *handover.Held) {
	defer held.Close()
	why := r.upkeep.restartReason()
	exec := func(path string, argv, env []string) error {
		logger.Info("replacing itself", "path", path, "reason", why, "listeners", handover.Named(env))
		// A signal to stop taken from here on is this process's alone, and
		// never reaches the router exec makes of it, so it's looked for as
		// late as can be.
		if ctx.Err() != nil {
			return errStopped
		}
		return r.cfg.Exec(path, argv, env)
	}
	err := r.execute(ctx, held, exec)
	switch {
	case err == nil:
		return
	case errors.Is(err, errStopped):
		logger.Info("told to stop as it restarted; exiting")
	default:
		logger.Warn("couldn't replace itself; exiting for launchd to start it again", "path", r.cfg.Binary.path, "error", err)
	}
	r.removeSocket()
}

// execute has held exec the router's binary, trying again while it isn't
// there, as for a moment while an upgrade moves its link on, execTries times
// in all, until it's told to stop.
func (r *Router) execute(ctx context.Context, held *handover.Held, exec func(path string, argv, env []string) error) error {
	path := r.cfg.Binary.path
	for try := 1; ; try++ {
		err := held.Exec(exec, path, r.cfg.Args, r.cfg.Environ)
		if !errors.Is(err, fs.ErrNotExist) || try == execTries {
			return err
		}
		logger.Info("its binary isn't there; trying again", "path", path, "try", try)
		select {
		case <-ctx.Done():
			return errStopped
		case <-time.After(cmp.Or(r.cfg.ExecRetry, execRetry)):
		}
	}
}

// removeSocket removes the control socket a listener held, rather than
// handed over, leaves, as the listener's closing would have.
func (r *Router) removeSocket() {
	if err := os.Remove(SocketPath(r.cfg.StateDir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("can't remove the control socket", "error", err)
	}
}

// inherit takes up the listeners cfg.Handed names, handed over by the router
// this one replaced: the proxy's, while it's at the address the config asks
// for, and the control API's, at socket. One elsewhere, as when the config
// has moved the proxy, is closed, for this router to listen afresh.
func (r *Router) inherit(socket string) listeners {
	if r.cfg.Handed == "" {
		return listeners{}
	}
	taken, err := handover.Take(r.cfg.Handed)
	if err != nil {
		logger.Warn("couldn't take up the listeners handed over; listening afresh", "handed", r.cfg.Handed, "error", err)
	}
	var ls listeners
	for name, ln := range taken {
		switch {
		case name == proxyListener && listensAt(ln.Addr(), r.cfg.Listen):
			ls.proxy = ln
		case name == controlListener && ln.Addr().String() == socket:
			ls.control = ln
		default:
			logger.Info("closed a listener handed over, as the router doesn't listen there", "listener", name, "address", ln.Addr().String())
			_ = ln.Close()
			continue
		}
		logger.Info("took up the listener handed over", "listener", name, "address", ln.Addr().String())
	}
	return ls
}

// listensAt reports whether addr, a TCP listener's, is address, a host:port
// whose host is an IP address, and whose port 0 is any.
func listensAt(addr net.Addr, address string) bool {
	tcp, ok := addr.(*net.TCPAddr)
	want, err := netip.ParseAddrPort(address)
	if !ok || err != nil {
		return false
	}
	got := tcp.AddrPort()
	return got.Addr().Unmap() == want.Addr().Unmap() && (want.Port() == 0 || got.Port() == want.Port())
}
