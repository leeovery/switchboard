package watch

import (
	"errors"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/theme"
)

// Prefs is where the dashboard keeps its preferences as the user changes
// them, for the next watch to start with.
type Prefs interface {
	// Update changes the preferences as change says, and keeps them.
	Update(change func(*theme.Prefs)) error
}

// errNowhereToKeep is why a theme chosen isn't kept where the watch has no
// preferences to keep it in.
var errNowhereToKeep = errors.New("there are no preferences to keep it in")

// prefsChange is a change to the preferences the watch keeps: what it is,
// and attrs saying how it stands, for the log, where it can't be kept; and
// where it's a theme picked, the pick, and the listing it was picked from.
type prefsChange struct {
	what    string
	change  func(*theme.Prefs)
	attrs   []any
	pick    pick
	listing theme.Listing
}

// keeping is how the watch keeps its preferences: one write at a time, off
// Update, each of the changes made since the last, in the order made, so the
// latest of each stands, as no write lands after a later one. unkept are the
// changes no write has yet been seen to keep, the first writing of them
// those of the write under way; and gate is what the writes share with the
// last, as the watch stops, as keepLeft has it.
type keeping struct {
	unkept  []prefsChange
	writing int
	gate    *prefsGate
}

// prefsGate lets one write of the preferences through at a time, and none
// once the watch has stopped and kept what was left, which held every
// change of the write under way then.
type prefsGate struct {
	mu      sync.Mutex
	stopped bool
}

// prefsKeptMsg is how a write of the preferences went: the changes it made,
// as many as it says, the first of those unkept; and of a theme picked among
// them, the choice as kept, and pair the themes it draws in; or why they
// weren't kept.
type prefsKeptMsg struct {
	changes []prefsChange
	choice  theme.Choice
	pair    theme.Pair
	err     error
}

// keepPrefs keeps what change changes in the preferences, where there are
// preferences to keep it in, noting what it is, and attrs saying how it
// stands, where it can't be kept. What can't be kept is shown all the same.
func (m Model) keepPrefs(what string, change func(*theme.Prefs), attrs ...any) Model {
	return m.toKeep(prefsChange{what: what, change: change, attrs: attrs})
}

// toKeep has c kept, after the changes before it, as keptPrefs has a write
// taken up.
func (m Model) toKeep(c prefsChange) Model {
	if m.cfg.Prefs == nil {
		return m
	}
	m.keeping.unkept = append(slices.Clip(m.keeping.unkept), c)
	return m
}

// keepPick has the theme picked from listing kept, set in the choice as it
// stands then: where there are no preferences to keep it in, it stands as it
// was, the picker saying so.
func (m Model) keepPick(p pick, listing theme.Listing) Model {
	c := prefsChange{
		what:    "the theme chosen",
		change:  func(prefs *theme.Prefs) { prefs.Choice = p.of(prefs.Choice) },
		attrs:   []any{"theme", p.slug, "half", p.half},
		pick:    p,
		listing: listing,
	}
	if m.cfg.Prefs == nil {
		prefsKeptMsg{changes: []prefsChange{c}, err: errNowhereToKeep}.noteUnkept()
		return m.pickNotKept()
	}
	return m.toKeep(c)
}

// keptPrefs writes the changes unkept, where no write is under way: every
// one, in a command, in one write.
func (m Model) keptPrefs() (Model, tea.Cmd) {
	if m.keeping.writing > 0 || len(m.keeping.unkept) == 0 {
		return m, nil
	}
	m.keeping.writing = len(m.keeping.unkept)
	prefs, gate, changes := m.cfg.Prefs, m.keeping.gate, m.keeping.unkept
	return m, func() tea.Msg {
		return gate.keep(prefs, changes)
	}
}

// keep makes the changes in prefs, in one write, unless the watch has
// stopped, having kept them.
func (g *prefsGate) keep(prefs Prefs, changes []prefsChange) prefsKeptMsg {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return prefsKeptMsg{changes: changes}
	}
	return write(prefs, changes)
}

// write makes the changes in prefs, in one write, and of the last theme
// picked among them, pairs the choice kept from the listing it was picked
// from.
func write(prefs Prefs, changes []prefsChange) prefsKeptMsg {
	var kept theme.Prefs
	err := prefs.Update(func(p *theme.Prefs) {
		for _, c := range changes {
			c.change(p)
		}
		kept = *p
	})
	msg := prefsKeptMsg{changes: changes, err: err}
	if i := lastPick(changes); i >= 0 && err == nil {
		msg.choice, msg.pair = kept.Choice, changes[i].listing.Pair(kept.Choice)
	}
	return msg
}

// lastPick is the index of the last theme picked among changes: -1 for none.
func lastPick(changes []prefsChange) int {
	for i, c := range slices.Backward(changes) {
		if c.pick != (pick{}) {
			return i
		}
	}
	return -1
}

// prefsWritten takes in how a write of the preferences went: the changes it
// made no longer unkept, and those made since left for the next; each it
// couldn't keep noted in the log; and where a theme was picked among them,
// the dashboard drawn by the choice as kept from now on, or, not kept, the
// picker saying so.
func (m Model) prefsWritten(msg prefsKeptMsg) Model {
	m.keeping.unkept, m.keeping.writing = m.keeping.unkept[len(msg.changes):], 0
	msg.noteUnkept()
	switch {
	case lastPick(msg.changes) < 0:
		return m
	case msg.err != nil:
		return m.pickNotKept()
	}
	logger.Info("theme chosen", "theme", msg.choice.Theme, "light", msg.choice.Light, "dark", msg.choice.Dark)
	return m.chosen(msg.choice, msg.pair)
}

// keepLeft keeps the changes the watch, as it stops, has left unkept, those
// of the write under way among them, which, kept here, it lets through to
// write nothing, so none lands after a later change.
func (m Model) keepLeft() {
	gate := m.keeping.gate
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.stopped = true
	if len(m.keeping.unkept) > 0 {
		write(m.cfg.Prefs, m.keeping.unkept).noteUnkept()
	}
}

// noteUnkept notes in the log each change the write couldn't keep, and why.
func (msg prefsKeptMsg) noteUnkept() {
	if msg.err == nil {
		return
	}
	for _, c := range msg.changes {
		logger.Warn("couldn't keep "+c.what, append(slices.Clip(c.attrs), "error", msg.err)...)
	}
}
