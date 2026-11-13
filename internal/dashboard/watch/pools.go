package watch

import (
	"errors"
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Pooled is what the dashboard shows of a pool: the pool, as status's
// PoolShown has it; its accounts, as status's PoolAccounts has them, which
// are Config's Accounts; the source that reads its document, sessions,
// history and request stream; its share of the router's events; what labels
// a pool by its id in the words of those events, as Config's PoolNamed does;
// how the config file stood before it was read, as Config's ConfigStamp
// gives it; how it placed the accounts in the pools, as status's Placed has
// it; and, as the config read has them, the grounds its request ledger is
// made on, as Config's LedgerGrounds, NewLedger making one on them, and the
// accounts removed, as Config's Removed, so nothing the dashboard works out
// of the config is of one gone. NewLedger is called only where the ledger in
// use was made on other grounds, as freshIn has it: one made at every turn
// would read today's lines and the summaries cold.
type Pooled struct {
	Pool      status.ShownPool
	Accounts  config.Accounts
	Source    Source
	Events    views.Events
	PoolNamed func(id string) string
	Stamp     filestate.State
	Placed    status.Placement
	Grounds   ledger.Grounds
	NewLedger func() views.Ledger
	Removed   status.RemovedAccounts
}

// PoolAsk is which pool Config's Turn is asked for.
type PoolAsk int

const (
	// NextPool is the pool after the one shown, as P asks for it.
	NextPool PoolAsk = iota
	// SamePool is the pool shown, read again as the config changes, else,
	// where it's gone, the first, as P would turn to from it.
	SamePool
)

// turnedMsg is what the dashboard shows of the pool asked for, as ask says,
// or why it couldn't tell.
type turnedMsg struct {
	ask    PoolAsk
	pooled Pooled
	err    error
}

// poolKey is P, which turns to the next pool, as the key line and help word
// it.
var poolKey = dashboard.Key{Key: "P", Does: "pool"}

// pool is the pool shown as the moment of the document on screen has it at
// now, as status's AsRead has it, or, while the router refuses to read it,
// as status's Refused has it of the router's answer: the frame, the status
// line and the keys each read whether it's empty here.
func (m Model) pool(now time.Time) status.ShownPool {
	if m.refusing != nil {
		return m.cfg.Pool.Refused(*m.refusing)
	}
	return m.cfg.Pool.AsRead(m.moment(now))
}

// refusesPool reports whether the read msg is the router refusing to read
// the pool shown, by its id, ignoring case, for want of knowing it, as a
// pool added to the config since it started: it routes none there till it
// restarts. A pool needed with no router's answer beside it, as a probe in
// place of the router finds of a pool the config no longer has, isn't.
func (m Model) refusesPool(msg fetchedMsg) bool {
	need, ok := errors.AsType[router.PoolNeeded](msg.err)
	return ok && msg.router.Answered() && m.cfg.Pool.ID != "" && strings.EqualFold(need.Named, m.cfg.Pool.ID)
}

// turnsPools reports whether the key line lists P: where there's a second
// pool, and the watch has a Turn to ask.
func (m Model) turnsPools() bool {
	return m.cfg.Pool.Turns() && m.cfg.Turn != nil
}

// asksPools reports whether P asks for the next pool: wherever there are
// pools, and the watch has a Turn to ask, so a second added to a config of
// one, while the dashboard shows it, is turned to.
func (m Model) asksPools() bool {
	return m.cfg.Pool.Pooled() && m.cfg.Turn != nil
}

// turnPool asks for what the dashboard shows of the pool after the one it
// shows, as P does, as askPool asks.
func (m Model) turnPool() (Model, tea.Cmd) {
	return m.askPool(NextPool)
}

// followConfig reads the pool shown again, as askPool asks, where a read
// finds the config file changed since the pool was read, as the read found
// it standing, as filestate's Same has it; or, as accountsChanged says, the
// accounts the read found other than those the last read found, as an
// account the router took up as it restarted. The stamp found is taken as
// the one acted on once the ask goes out, so a config that can't be read is
// asked again only once it changes again, and one found changed during a
// turn is asked again at the next read. A read that finds neither reads
// nothing again.
func (m Model) followConfig(stamp filestate.State, accountsChanged bool) (Model, tea.Cmd) {
	stamped := m.cfg.ConfigStamp != nil && !stamp.Same(m.cfg.Stamp)
	// An accounts change found during a turn is dropped, accepted: a turn
	// lasts milliseconds, the change costs nothing while the config is as
	// it was, and the stamp, left as it was, catches a config changed.
	if !m.asksPools() || m.turning || !stamped && !accountsChanged {
		return m, nil
	}
	m.cfg.Stamp = stamp
	return m.askPool(SamePool)
}

// askPool asks for what the dashboard shows of the pool ask says, in a
// command, of Config's Turn: nothing while a turn is under way, so a second
// P turns from the pool the first turned to.
func (m Model) askPool(ask PoolAsk) (Model, tea.Cmd) {
	// P during a turn is dropped, accepted: a turn lasts milliseconds, and
	// one dropped fails safe, pressed again.
	if m.turning {
		return m, nil
	}
	m.turning = true
	ctx, turn, from := m.ctx, m.cfg.Turn, m.cfg.Pool.ID
	return m, func() tea.Msg {
		pooled, err := turn(ctx, from, ask)
		return turnedMsg{ask: ask, pooled: pooled, err: err}
	}
}

// turnedTo shows the pool asked for, as shownAfresh shows it, and where P
// asked, with no other to turn to, says so. Where it couldn't be told, it
// keeps to the pool shown, and where P asked, the key line says why; where
// the config changed, it's logged alone, and asked again once the config
// changes again.
func (m Model) turnedTo(msg turnedMsg) (Model, tea.Cmd) {
	m.turning = false
	switch {
	case msg.err != nil && msg.ask == NextPool:
		logger.Warn("can't turn to the next pool", "error", msg.err)
		return m.noting(prose.TurnFailed(msg.err.Error()))
	case msg.err != nil:
		logger.Warn("can't read the pool shown again as the config changed", "error", msg.err)
		return m, nil
	}
	held := strings.EqualFold(msg.pooled.Pool.ID, m.cfg.Pool.ID)
	shown, started := m.shownAfresh(msg.pooled)
	if msg.ask == NextPool && held && !msg.pooled.Pool.Turns() {
		noted, note := shown.noting(prose.OnePool)
		return noted, tea.Batch(started, note)
	}
	return shown, started
}

// shownAfresh is the watch of the pool pooled, started afresh, as freshIn
// starts it, but where it's the pool shown, its config placing the accounts
// in the pools as it did, as status's Placement has it, and its ledger on
// the grounds it was made on: then it's the watch as it was, the config
// file's stamp that of pooled. So a change to the
// config that places anything otherwise, in any pool, starts the pool shown
// afresh, and one that doesn't, as a comment, starts nothing.
func (m Model) shownAfresh(pooled Pooled) (Model, tea.Cmd) {
	if strings.EqualFold(pooled.Pool.ID, m.cfg.Pool.ID) && pooled.Placed.Same(m.cfg.Placed) && pooled.Grounds.Same(m.cfg.LedgerGrounds) {
		m.cfg.Stamp = pooled.Stamp
		return m, nil
	}
	logger.Info("pool shown", "pool", pooled.Pool.ID)
	fresh := m.freshIn(pooled)
	return fresh, fresh.started()
}

// freshIn is the watch started afresh on the pool pooled, as a router
// change starts its view afresh, so nothing it held of the pool before
// carries into it: no page, cursor, session's page, card, alert, held
// moment or read of the pool before, its request stream ended. What it
// keeps is the screen's and the user's, not the pool's: the terminal's size
// and background, the theme and the picker, the preferences being kept, the
// tab shown, under a session's page where one is open, and what each tab
// shows as the preferences keep it; and the request ledger, what it holds
// of the days read, where pooled's is on the grounds it was made on, else
// one made on pooled's. poolTurns counts the pools shown, so what the
// commands of the one before send back is dropped, as turns has it.
func (m Model) freshIn(pooled Pooled) Model {
	m.stream.end()
	cfg := m.cfg
	cfg.Pool, cfg.Accounts, cfg.Source, cfg.Events, cfg.PoolNamed, cfg.Stamp, cfg.Placed = pooled.Pool, pooled.Accounts, pooled.Source, pooled.Events, pooled.PoolNamed, pooled.Stamp, pooled.Placed
	cfg.Removed = pooled.Removed
	if !pooled.Grounds.Same(cfg.LedgerGrounds) {
		cfg.Ledger, cfg.LedgerGrounds = pooled.NewLedger(), pooled.Grounds
	}
	cfg.Size, cfg.View, cfg.Sections, cfg.By, cfg.Models, cfg.Graphs = m.size, m.view, m.sections, m.by, m.models, m.graphs
	cfg.AccountsShown = dashboard.AccountsKept(m.accounts.shown.Kept())
	cfg.SessionsShown = dashboard.SessionsKept(m.sessionsTab.shown.Kept())
	cfg.RunwayShown = dashboard.RunwayKept(m.runwayTab.shown.Kept())
	cfg.History = dashboard.HistoryKept(m.past.page.Kept(), cfg.Accounts)
	cfg.Log = m.log.page.Columns
	fresh := New(m.ctx, cfg)
	fresh.choice, fresh.pair, fresh.showing, fresh.backdrop, fresh.picker = m.choice, m.pair, m.showing, m.backdrop, m.picker
	fresh.keeping, fresh.poolTurns = m.keeping, m.poolTurns+1
	return fresh
}

// turns runs the watch's model as Bubble Tea runs it, each of its own
// messages a command sends back stamped with the pool shown as the command
// was given, as stamped has it, so one sent back once P has turned to
// another pool is dropped: nothing of one pool lands in the next. The
// messages of what the turn keeps, the preferences being kept and the
// theme picker, land whenever they come.
type turns struct {
	Model
}

// ofTurn is a message of the watch's own a command sent back, and the pool
// shown, as Model's poolTurns counts them, as the command was given.
type ofTurn struct {
	turn int
	msg  tea.Msg
}

// Init starts the model as it starts itself, its commands stamped.
func (t turns) Init() tea.Cmd {
	return stamped(t.poolTurns, t.Model.Init())
}

// Update takes in a message as the model does, but one stamped with a pool
// shown before the one it shows now, unless it lasts the turn; and stamps
// the commands it gives with the pool it shows then.
func (t turns) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s, ok := msg.(ofTurn); ok {
		if s.turn != t.poolTurns && !lastsTurns(s.msg) {
			return t, nil
		}
		msg = s.msg
	}
	next, cmd := t.Model.Update(msg)
	t.Model = next.(Model)
	return t, stamped(t.poolTurns, cmd)
}

// ownPackage is the watch's own package, whose messages stamped stamps.
var ownPackage = reflect.TypeFor[Model]().PkgPath()

// stamped is cmd with each message of the watch's own it sends back
// stamped with the pool shown, turn, as ofTurn has it, those of a batch
// each in turn. Any other passes as it is: Bubble Tea's own, which it acts
// on itself, as a quit or a question to the terminal, would be lost to it
// stamped.
func stamped(turn int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			cmds := make(tea.BatchMsg, len(msg))
			for i, c := range msg {
				cmds[i] = stamped(turn, c)
			}
			return cmds
		default:
			if reflect.TypeOf(msg).PkgPath() != ownPackage {
				return msg
			}
			return ofTurn{turn: turn, msg: msg}
		}
	}
}

// lastsTurns reports whether msg lands however the pool shown has turned
// since its command was given: of what a turn keeps, a write of the
// preferences and the theme picker's listing and asking.
func lastsTurns(msg tea.Msg) bool {
	switch msg.(type) {
	case prefsKeptMsg, listedMsg, askMsg:
		return true
	}
	return false
}

// emptyPoolKey acts on a key of the page of a pool with no accounts, which
// has no view to act on: tab and shift-tab show the next tab and the one
// before, as turn has it, and t opens the theme picker, as on any page. The
// rest do nothing.
func (m Model) emptyPoolKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "tab":
		return m.turn(1)
	case "shift+tab":
		return m.turn(-1)
	case "t":
		return m.openPicker()
	}
	return m, nil
}
