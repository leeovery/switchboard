package router

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/netip"
	"os"

	"github.com/leeovery/switchboard/internal/handover"
)

// The names a router hands its listeners over by.
const (
	proxyListener   = "proxy"
	controlListener = "control"
)

// hold holds the router's listeners open, to hand over as it replaces
// itself, or returns nil when it can't replace itself, and so exits instead:
// it has nothing to exec with, doesn't know its binary, or can't hold them.
func (r *Router) hold(ls listeners) *handover.Held {
	if r.cfg.Exec == nil {
		return nil
	}
	if r.cfg.Binary.path == "" {
		logger.Warn("can't replace itself, not knowing its binary; exiting for launchd to start it again")
		return nil
	}
	held, err := handover.Hold(map[string]net.Listener{proxyListener: ls.proxy, controlListener: ls.control})
	if err != nil {
		logger.Warn("can't replace itself; exiting for launchd to start it again", "error", err)
		return nil
	}
	return held
}

// replace replaces this process with the router's binary, by the path it was
// started as, which an upgrade leads on to the new version, run as it was,
// handing it the listeners held, for the router it becomes to take up: no
// connection is refused meanwhile, and the process, keeping its id, isn't
// started afresh, which macOS can refuse a binary an upgrade replaced. It
// returns only when that fails, the router then exiting as though it had
// stopped, for launchd to start it again, or once ctx has ended, as at a
// signal to stop while the router stopped to restart, when it exits for good.
func (r *Router) replace(ctx context.Context, held *handover.Held) {
	defer held.Close()
	if ctx.Err() != nil {
		logger.Info("told to stop as it restarted; exiting")
		r.removeSocket()
		return
	}
	path, why := r.cfg.Binary.path, r.upkeep.restartReason()
	err := held.Exec(func(path string, argv, env []string) error {
		logger.Info("replacing itself", "path", path, "reason", why, "listeners", handover.Named(env))
		return r.cfg.Exec(path, argv, env)
	}, path, r.cfg.Args, r.cfg.Environ)
	if err == nil {
		return
	}
	logger.Warn("couldn't replace itself; exiting for launchd to start it again", "path", path, "error", err)
	r.removeSocket()
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
