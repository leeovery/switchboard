package watch

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/theme"
)

// askWithin bounds the binary's answer as it's asked its version: one that
// hasn't answered by then isn't restarted into.
const askWithin = 10 * time.Second

// Upgrade is how the dashboard restarts itself onto an upgrade of the binary
// it was started as, so one left open never reads a newer router's document
// by an older version's rules. It restarts where a read finds the path it
// was started by leading to another file, or the same file changed, as
// router.Watched's MovedTo judges it, a router answering that runs another
// version than its own, and the binary answering its version, another than
// its own. With no router answering it stays, however long: it reads its
// own probes then, by its own rules, and only a router moved over has
// checked the upgrade against the config. The zero Upgrade never restarts.
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
	// nothing, as one probing, or that it didn't answer: the zero Health
	// where none answers, as nil has it.
	Health func(ctx context.Context) router.Health
	// Ask runs the binary for its version, until ctx ends, failing where it
	// doesn't say one.
	Ask func(ctx context.Context) (string, error)
	// Exec replaces the process with the binary, run as the dashboard was
	// started, carrying what it shows, returning only where it fails.
	Exec func(shown Shown) error
}

// Shown is what a dashboard shows, which a restart carries to the dashboard
// it restarts into, whatever another dashboard keeps meanwhile: the
// preferences as it shows them, and the pool shown, by its id, "" for none.
type Shown struct {
	Prefs theme.Prefs
	Pool  string
}

// upgrading is what the watch holds of an upgrade of its binary: asking is
// set while it asks whether to restart onto one, as ask asks, and while it
// restarts, restart then noting how it stands; declined is how the binary
// stood when the watch last stayed on its own version, which it asks about
// again only once the binary has moved on from, zero while it hasn't stayed,
// which any binary there has. toward is how the binary stood at the last
// ask, and noted is set once the watch has said it waits on the router for
// the binary standing so.
type upgrading struct {
	asking   bool
	restart  *restartNoted
	declined filestate.State
	toward   filestate.State
	noted    bool
}

// restartNoted is how the watch stands, a restart decided on, as the last
// message taken in left it, which the restart takes as Bubble Tea runs it:
// the terminal's background, the preferences changed and not yet kept, and
// what it shows.
type restartNoted struct {
	backdrop backdrop.Backdrop
	unkept   []prefsChange
	shown    Shown
}

// upgradeAskedMsg is how the binary, standing as binary, was asked about:
// awaiting is set where the watch waits on the router, as movesOver has it;
// else the version the binary answered, or why it didn't.
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
// asks whether to restart onto it, as ask asks, one ask at a time, and none
// while the pool shown is being read, as a turn has it.
func (m Model) upgradeLooked(binary filestate.State, health router.Health) (Model, tea.Cmd) {
	if m.upgrade.asking || m.turning || !m.cfg.Upgrade.Binary.MovedTo(binary) || !m.upgrade.declined.MovedTo(binary) {
		return m, nil
	}
	if !m.upgrade.toward.Same(binary) {
		m.upgrade.toward, m.upgrade.noted = binary, false
	}
	m.upgrade.asking = true
	u, ctx := m.cfg.Upgrade, m.ctx
	return m, func() tea.Msg { return u.ask(ctx, binary, health) }
}

// ask asks whether to restart onto the binary, standing as binary: the
// router first, as health answered the read, or where it didn't answer, as
// Health answers now, waiting on it as movesOver has it; then the binary its
// version, waiting askWithin at most.
func (u Upgrade) ask(ctx context.Context, binary filestate.State, health router.Health) upgradeAskedMsg {
	if !health.Answered() && u.Health != nil {
		health = u.Health(ctx)
	}
	msg := upgradeAskedMsg{binary: binary}
	if !u.movesOver(health) {
		msg.awaiting = true
		return msg
	}
	asking, cancel := context.WithTimeout(ctx, askWithin)
	defer cancel()
	msg.version, msg.err = u.Ask(asking)
	return msg
}

// movesOver reports whether the watch moves over to the upgrade, health
// being the router's answer now: only where a router answers, running
// another version than the dashboard's, so one slow to answer, or none
// running, is waited on, however long.
func (u Upgrade) movesOver(health router.Health) bool {
	return health.Answered() && health.Version != u.Version
}

// upgradeAsked takes in how the binary was asked about: where the watch
// waits on the router, it says so once a binary, and asks again at the next
// read; where a turn is under way, it asks again once it's done; where the
// binary answers a version other than the dashboard's, it restarts onto it,
// as restart has it; and else stays on its own version, noting why, asking
// again only once the binary moves on, so a binary that can't answer, or
// answers the dashboard's own version, as a dev build does, is never
// restarted into, nor asked at every read.
func (m Model) upgradeAsked(msg upgradeAskedMsg) (Model, tea.Cmd) {
	own := m.cfg.Upgrade.Version
	m.upgrade.asking = false
	switch {
	case msg.awaiting:
		if !m.upgrade.noted {
			logger.Info("the binary was upgraded; restarting onto it once a router runs another version", "version", own)
		}
		m.upgrade.noted = true
		return m, nil
	case m.turning:
		return m, nil
	case msg.err != nil:
		logger.Info("the upgraded binary didn't say its version; staying on this one until the binary changes again", "version", own, "error", msg.err)
	case msg.version == own:
		logger.Info("the upgraded binary is this version; staying on it until the binary changes again", "version", own)
	default:
		logger.Info("restarting onto the upgrade", "from", own, "to", msg.version)
		return m.restart(msg.binary)
	}
	m.upgrade.declined = msg.binary
	return m, nil
}

// restart restarts the dashboard onto the binary, standing as binary, as
// restarting has it, once Bubble Tea has given the terminal back, as a quit
// would, taking how the watch stands as it does, as noteRestart notes it;
// where the process isn't replaced, Bubble Tea takes the terminal back and
// the watch carries on, as restartFailed has it.
func (m Model) restart(binary filestate.State) (Model, tea.Cmd) {
	noted := &restartNoted{}
	m.upgrade.asking, m.upgrade.restart = true, noted
	r := &restarting{noted: noted, unread: m.cfg.unread, gate: m.keeping.gate, prefs: m.cfg.Prefs, exec: m.cfg.Upgrade.Exec}
	handOver := m.cfg.handOver
	if handOver == nil {
		handOver = tea.Exec
	}
	return m, handOver(r, func(err error) tea.Msg { return restartFailedMsg{binary: binary, err: err} })
}

// noteRestart notes how the watch stands for the restart decided on, if
// any, as it stands once a message is taken in: Bubble Tea runs the restart
// between messages, so it takes what the last left.
func (m Model) noteRestart() {
	r := m.upgrade.restart
	if r == nil {
		return
	}
	r.backdrop, r.unkept, r.shown = m.backdrop, m.keeping.unkept, m.shownNow()
}

// shownNow is what the watch shows, as a restart carries it: the
// preferences as it shows them, drawn, where it's drawn in colour, in the
// theme chosen, each theme picked and not yet kept made in it, as the
// restart keeps them; and the pool shown.
func (m Model) shownNow() Shown {
	prefs := m.keeping.shown
	if m.coloured() {
		prefs.Choice = m.choice
		for _, c := range m.keeping.unkept {
			if c.pick != (pick{}) {
				c.change(&prefs)
			}
		}
	}
	return Shown{Prefs: prefs, Pool: m.cfg.Pool.ID}
}

// restartFailed takes in a restart that didn't replace the process: the
// watch carries on as its own version, saying why, and asks again only once
// the binary moves on.
func (m Model) restartFailed(msg restartFailedMsg) Model {
	logger.Warn("can't restart onto the upgrade; carrying on as this version until the binary changes again", "version", m.cfg.Upgrade.Version, "error", msg.err)
	m.upgrade.asking, m.upgrade.restart, m.upgrade.declined = false, nil, msg.binary
	return m
}

// restarting is a restart onto an upgrade, as Bubble Tea runs it with the
// terminal given back, the watch standing as noted: it puts back the
// terminal's background and throws away what was typed and never read, as a
// watch stopping does, keeps the preferences changed and not yet kept, then
// replaces the process with exec, carrying what the watch shows.
type restarting struct {
	out    io.Writer
	noted  *restartNoted
	unread func()
	gate   *prefsGate
	prefs  Prefs
	exec   func(Shown) error
}

func (r *restarting) Run() error {
	now := *r.noted
	now.backdrop.PutBack(r.out)
	if r.unread != nil {
		r.unread()
	}
	return r.gate.keepThen(r.prefs, now.unkept, func() error { return r.exec(now.shown) })
}

func (r *restarting) SetStdin(io.Reader) {}

func (r *restarting) SetStdout(out io.Writer) {
	r.out = out
}

func (r *restarting) SetStderr(io.Writer) {}
