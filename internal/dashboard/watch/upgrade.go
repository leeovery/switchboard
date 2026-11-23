package watch

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/router"
)

// askWithin bounds the binary's answer as it's asked its version: one that
// hasn't answered by then isn't restarted into.
const askWithin = 10 * time.Second

// Upgrade is how the dashboard restarts itself onto an upgrade of the binary
// it was started as, so one left open never reads a newer router's document
// by an older version's rules. It restarts where a read finds the path it
// was started by leading to another file, or the same file changed, as
// router.Watched's MovedTo judges it, the router running another version
// than its own, or none answering, and the binary answering its version,
// another than its own. The zero Upgrade never restarts.
type Upgrade struct {
	// Version is the dashboard's own version.
	Version string
	// Binary is the binary the dashboard was started as, by the path it was
	// run by, as router.Watch found it as the dashboard started; and Look
	// finds how that path leads now, as filestate.Of does, once each read is
	// made: nil looks at nothing.
	Binary router.Watched
	Look   func() filestate.State
	// Health asks the router's health check, for a read that asked it
	// nothing, as one probing: the zero Health where none answers, as nil
	// has it.
	Health func(ctx context.Context) router.Health
	// Ask runs the binary for its version, until ctx ends, failing where it
	// doesn't say one.
	Ask func(ctx context.Context) (string, error)
	// Exec replaces the process with the binary, run as the dashboard was
	// started but showing the pool with the given id, or none for "",
	// returning only where it fails.
	Exec func(pool string) error
}

// upgrading is what the watch holds of an upgrade of its binary: asking is
// set while it asks whether to restart onto one, as ask asks, and while it
// restarts; declined is how the binary stood when the watch last stayed on
// its own version, which it asks about again only once the binary has moved
// on from, zero while it hasn't stayed, which any binary there has; and
// awaited is how it stood when the watch last waited on the router to run
// another version, as it notes once.
type upgrading struct {
	asking   bool
	declined filestate.State
	awaited  filestate.State
}

// upgradeAskedMsg is how the binary, standing as binary, was asked about:
// awaiting where the router still runs the dashboard's own version; else
// the version the binary answered, or why it didn't.
type upgradeAskedMsg struct {
	binary   filestate.State
	awaiting bool
	version  string
	err      error
}

// restartFailedMsg is a restart onto the binary, standing as binary, that
// didn't replace the process, and why.
type restartFailedMsg struct {
	binary filestate.State
	err    error
}

// upgradeLooked looks at the binary as a read found it, binary, health being
// the router's answer to the read: where it has moved on from the one the
// dashboard was started as, and from the one the watch last stayed on, it
// asks whether to restart onto it, as ask asks, one ask at a time.
func (m Model) upgradeLooked(binary filestate.State, health router.Health) (Model, tea.Cmd) {
	if m.upgrade.asking || !m.cfg.Upgrade.Binary.MovedTo(binary) || !m.upgrade.declined.MovedTo(binary) {
		return m, nil
	}
	m.upgrade.asking = true
	u, ctx := m.cfg.Upgrade, m.ctx
	return m, func() tea.Msg { return u.ask(ctx, binary, health) }
}

// ask asks whether to restart onto the binary, standing as binary: the
// router first, as health answered the read, or where it didn't answer, as
// Health answers now, so the dashboard moves over only once the router runs
// another version than its own, or none answers; then the binary its
// version, waiting askWithin at most.
func (u Upgrade) ask(ctx context.Context, binary filestate.State, health router.Health) upgradeAskedMsg {
	if !health.Answered() && u.Health != nil {
		health = u.Health(ctx)
	}
	if health.Answered() && health.Version == u.Version {
		return upgradeAskedMsg{binary: binary, awaiting: true}
	}
	asking, cancel := context.WithTimeout(ctx, askWithin)
	defer cancel()
	version, err := u.Ask(asking)
	return upgradeAskedMsg{binary: binary, version: version, err: err}
}

// upgradeAsked takes in how the binary was asked about: where the router
// still runs the dashboard's version, it waits, noting so once a binary,
// and asks again at the next read; where the binary answers a version other
// than the dashboard's, it restarts onto it, as restart has it; and else
// stays on its own version, noting why, asking again only once the binary
// moves on, so a binary that can't answer, or answers the dashboard's own
// version, as a dev build does, is never restarted into, nor asked at every
// read.
func (m Model) upgradeAsked(msg upgradeAskedMsg) (Model, tea.Cmd) {
	own := m.cfg.Upgrade.Version
	switch {
	case msg.awaiting:
		if !m.upgrade.awaited.Same(msg.binary) {
			logger.Info("the binary was upgraded; restarting onto it once the router runs another version", "version", own)
		}
		m.upgrade.asking, m.upgrade.awaited = false, msg.binary
		return m, nil
	case msg.err != nil:
		logger.Info("the upgraded binary didn't say its version; staying on this one until the binary changes again", "version", own, "error", msg.err)
	case msg.version == own:
		logger.Info("the upgraded binary is this version; staying on it until the binary changes again", "version", own)
	default:
		logger.Info("restarting onto the upgrade", "from", own, "to", msg.version)
		return m, m.restart(msg.binary)
	}
	m.upgrade.asking, m.upgrade.declined = false, msg.binary
	return m, nil
}

// restart restarts the dashboard onto the binary, standing as binary, as
// restarting has it, once Bubble Tea has given the terminal back, as a quit
// would; where the process isn't replaced, Bubble Tea takes the terminal back
// and the watch carries on, as restartFailed has it.
func (m Model) restart(binary filestate.State) tea.Cmd {
	exec, pool := m.cfg.Upgrade.Exec, m.cfg.Pool.ID
	r := &restarting{
		backdrop: m.backdrop, unread: m.cfg.unread, gate: m.keeping.gate, prefs: m.cfg.Prefs, unkept: m.keeping.unkept,
		exec: func() error { return exec(pool) },
	}
	handOver := m.cfg.handOver
	if handOver == nil {
		handOver = tea.Exec
	}
	return handOver(r, func(err error) tea.Msg { return restartFailedMsg{binary: binary, err: err} })
}

// restartFailed takes in a restart that didn't replace the process: the
// watch carries on as its own version, saying why, and asks again only once
// the binary moves on.
func (m Model) restartFailed(msg restartFailedMsg) Model {
	logger.Warn("can't restart onto the upgrade; carrying on as this version until the binary changes again", "version", m.cfg.Upgrade.Version, "error", msg.err)
	m.upgrade.asking, m.upgrade.declined = false, msg.binary
	return m
}

// restarting is a restart onto an upgrade, as Bubble Tea runs it with the
// terminal given back: it puts back the terminal's background and throws away
// what was typed and never read, as a watch stopping does, keeps the
// preferences changed and not yet kept, so the dashboard restarted opens on
// what was shown, then replaces the process with exec.
type restarting struct {
	out      io.Writer
	backdrop backdrop.Backdrop
	unread   func()
	gate     *prefsGate
	prefs    Prefs
	unkept   []prefsChange
	exec     func() error
}

func (r *restarting) Run() error {
	r.backdrop.PutBack(r.out)
	if r.unread != nil {
		r.unread()
	}
	return r.gate.keepThen(r.prefs, r.unkept, r.exec)
}

func (r *restarting) SetStdin(io.Reader) {}

func (r *restarting) SetStdout(out io.Writer) {
	r.out = out
}

func (r *restarting) SetStderr(io.Writer) {}
