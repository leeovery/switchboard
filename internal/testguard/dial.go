package testguard

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type dialFunc = func(ctx context.Context, network, addr string) (net.Conn, error)

// dialGuard lets connections reach this machine alone: loopback, and unix
// sockets in the temporary directory, where tests make theirs, not a live
// router's, nor any other program's. It notes where each dial it blocked was
// going.
type dialGuard struct {
	// tempDir is the temporary directory, and live the directories a live
	// router's control socket can be in, each with its symlinks resolved.
	tempDir string
	live    []string

	mu sync.Mutex
	// blocked counts the dials blocked, by address.
	blocked map[string]int
}

// newDialGuard returns a guard that lets unix sockets in tempDir be dialled,
// but for those in any of the live directories.
func newDialGuard(tempDir string, live ...string) *dialGuard {
	g := &dialGuard{tempDir: resolve(tempDir)}
	for _, dir := range live {
		g.live = append(g.live, resolve(dir))
	}
	return g
}

// install has rt, which must be an *http.Transport, dial through the guard,
// as does every transport cloned from it after.
func (g *dialGuard) install(rt http.RoundTripper) error {
	t, ok := rt.(*http.Transport)
	if !ok {
		return fmt.Errorf("can't guard the dials of a %T, only an *http.Transport's", rt)
	}
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	t.DialContext = g.wrap(dial)
	return nil
}

// wrap returns dial, failing every dial the guard doesn't allow.
func (g *dialGuard) wrap(dial dialFunc) dialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !g.allows(network, addr) {
			g.note(addr)
			return nil, fmt.Errorf("testguard: blocked dial to %s", addr)
		}
		return dial(ctx, network, addr)
	}
}

func (g *dialGuard) note(addr string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blocked == nil {
		g.blocked = make(map[string]int)
	}
	g.blocked[addr]++
}

// escapes lists the dials blocked, a line an address.
func (g *dialGuard) escapes() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var lines []string
	for _, addr := range slices.Sorted(maps.Keys(g.blocked)) {
		line := "blocked dial to " + addr
		if n := g.blocked[addr]; n > 1 {
			line += fmt.Sprintf(" (%d times)", n)
		}
		lines = append(lines, line)
	}
	return lines
}

// allows reports whether a dial of network to addr stays on this machine,
// and away from what's live on it: a unix socket in the temporary directory,
// where a link there leads, but not in a live directory, or a loopback
// address or localhost. Any other name counts as off the machine, as
// resolving it could reach the network.
func (g *dialGuard) allows(network, addr string) bool {
	if strings.HasPrefix(network, "unix") {
		return g.allowsSocket(addr)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// allowsSocket reports whether the unix socket at path, where a link there
// leads, is in the temporary directory, and not in a live directory.
func (g *dialGuard) allowsSocket(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = resolve(path)
	inLive := func(dir string) bool { return within(dir, path) }
	return within(g.tempDir, path) && !slices.ContainsFunc(g.live, inLive)
}
