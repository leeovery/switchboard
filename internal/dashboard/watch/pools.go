package watch

import (
	"reflect"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Pooled is what the dashboard shows of a pool: the pool, as status's
// PoolShown has it; its accounts, as status's PoolAccounts has them, which
// are Config's Accounts; the source that reads its document, sessions,
// history and request stream; its share of the router's events; and what
// labels a pool by its id in the words of those events, as Config's
// PoolNamed does.
type Pooled struct {
	Pool      status.ShownPool
	Accounts  config.Accounts
	Source    Source
	Events    views.Events
	PoolNamed func(id string) string
}

// turnedMsg is what the dashboard shows of the pool P turned to, or why it
// couldn't tell.
type turnedMsg struct {
	pooled Pooled
	err    error
}

// poolKey is P, which turns to the next pool, as the key line and help word
// it.
var poolKey = dashboard.Key{Key: "P", Does: "pool"}

// turnsPools reports whether P turns to another pool: where there's a
// second, and the watch has a Turn to ask.
func (m Model) turnsPools() bool {
	return m.cfg.Pool.Turns() && m.cfg.Turn != nil
}

// turnPool asks for what the dashboard shows of the pool after the one it
// shows, in a command, as P does: nothing while a turn is under way, so a
// second P turns from the pool the first turned to.
func (m Model) turnPool() (Model, tea.Cmd) {
	if m.turning {
		return m, nil
	}
	m.turning = true
	ctx, turn, from := m.ctx, m.cfg.Turn, m.cfg.Pool.ID
	return m, func() tea.Msg {
		pooled, err := turn(ctx, from)
		return turnedMsg{pooled: pooled, err: err}
	}
}

// turnedTo shows the pool P turned to, as freshIn has it; or where it
// couldn't be told, keeps to the pool shown, the key line saying why.
func (m Model) turnedTo(msg turnedMsg) (Model, tea.Cmd) {
	m.turning = false
	if msg.err != nil {
		logger.Warn("can't turn to the next pool", "error", msg.err)
		return m.noting("can't turn to the next pool: " + msg.err.Error())
	}
	logger.Info("pool shown", "pool", msg.pooled.Pool.ID)
	fresh := m.freshIn(msg.pooled)
	return fresh, fresh.started()
}

// freshIn is the watch started afresh on the pool pooled, as a router
// change starts its view afresh, so nothing it held of the pool before
// carries into it: no page, cursor, session's page, card, alert, held
// moment or read of the pool before, its request stream ended. What it
// keeps is the screen's and the user's, not the pool's: the terminal's size
// and background, the theme and the picker, the preferences being kept, the
// tab shown, under a session's page where one is open, and what each tab
// shows as the preferences keep it. poolTurns counts the pools shown, so what
// the commands of the one before send back is dropped, as turns has it.
func (m Model) freshIn(pooled Pooled) Model {
	m.stream.end()
	cfg := m.cfg
	cfg.Pool, cfg.Accounts, cfg.Source, cfg.Events, cfg.PoolNamed = pooled.Pool, pooled.Accounts, pooled.Source, pooled.Events, pooled.PoolNamed
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
