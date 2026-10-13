package watch

import (
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// clicked acts on a click of the left button, as what it lands on, of the
// targets the page on screen gives, says: as clickedOn has it. While a card
// is open, a click does nothing, as its keys alone act; and on the theme
// picker, nothing, as it takes keys alone.
func (m Model) clicked(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	click := msg.Mouse()
	if click.Button != tea.MouseLeft || m.open != nothingOpen || m.onPicker(click.X) {
		return m, nil
	}
	f, doc, at := m.screen(m.now())
	t, ok := targetAt(f.Targets(doc, at), click.X, click.Y)
	if !ok {
		return m, nil
	}
	return m.clickedOn(t)
}

// targetAt is the target of those given that takes in the cell at x along
// row y, reporting false where none does.
func targetAt(targets []dashboard.Target, x, y int) (dashboard.Target, bool) {
	for _, t := range targets {
		if t.Y == y && t.X <= x && x < t.X+t.Width {
			return t, true
		}
	}
	return dashboard.Target{}, false
}

// clickedOn does what a click on the target does, which is what its key
// does, the page acting as acting has it: on a tab, it shows it, unless the
// theme picker is open, as the tab key does nothing then; on a sub-tab, it
// shows it, as showSubTab has it, or the Log's; on < or > by a period, it
// presses it; on a key of the key line, it presses it, as pressOf says, as
// on the Log's ▼ N new, which presses end; on an Overview's section's
// heading, it hides the section, as its key does; on an account's row
// there, it moves the focus to it, as focusOn has it; and on a line of the
// Log's table, it chooses it, as clickedLine has it. Nothing else on the
// page takes a click yet.
func (m Model) clickedOn(t dashboard.Target) (tea.Model, tea.Cmd) {
	switch t.On {
	case dashboard.OnTab:
		if m.picker.open {
			return m, nil
		}
		return m.acting().showTab(t.Tab), nil
	case dashboard.OnSubTab:
		if m.view == dashboard.Log {
			m.log.page = m.log.page.Showing(dashboard.LogView(t.SubTab))
			return m, nil
		}
		return m.acting().showSubTab(t.SubTab), nil
	case dashboard.OnStep:
		return m.pressed(stepPress(t.Step))
	case dashboard.OnKey:
		if press, ok := pressOf(t.Key); ok {
			return m.pressed(press)
		}
	case dashboard.OnSection:
		return m.acting().toggle(t.Section), nil
	case dashboard.OnAccount:
		return m.acting().focusOn(t.Account), nil
	case dashboard.OnRow:
		return m.clickedLine(t.Row)
	}
	return m, nil
}

// clickedLine acts on a click on the line of the Log's table shown at the
// place given among its lines: on the line chosen, it opens its details, as
// enter does; on another row, it chooses it, pausing the table where it
// follows, as ChoseLine has it.
func (m Model) clickedLine(at int) (tea.Model, tea.Cmd) {
	if m.log.page.Lit(at) {
		return m.openLogCard()
	}
	m.log.page = m.log.page.ChoseLine(at, m.logRows())
	return m, nil
}

// onPicker reports whether the column x is on the theme picker, while it's
// open over the page's right.
func (m Model) onPicker(x int) bool {
	return m.picker.open && x >= m.size.Width-dashboard.PickerWidth(m.size.Width)
}

// namedPresses are the presses of the keys the key line names by a word or
// by a pair. A pair, which moves either way, is pressed as the way on: ↓, ],
// > and →.
var namedPresses = map[string]tea.KeyPressMsg{
	"tab":   {Code: tea.KeyTab},
	"enter": {Code: tea.KeyEnter},
	"esc":   {Code: tea.KeyEscape},
	"end":   {Code: tea.KeyEnd},
	"↑↓":    {Code: tea.KeyDown},
	"[ ]":   {Code: ']', Text: "]"},
	"< >":   {Code: '>', Text: ">"},
	"←→":    {Code: tea.KeyRight},
}

// stepPress is the press of the key that steps a period the way given, as
// a click on < or > by it does: < back, for -1, and > on.
func stepPress(way int) tea.KeyPressMsg {
	if way < 0 {
		return tea.KeyPressMsg{Code: '<', Text: "<"}
	}
	return namedPresses["< >"]
}

// pressOf is the key press a click on a key of the key line, named as the
// line names it, is taken as, reporting false for a name that's no key. The
// key line gives p and q no target, so a stray click neither routes nor
// quits.
func pressOf(name string) (tea.KeyPressMsg, bool) {
	if press, ok := namedPresses[name]; ok {
		return press, true
	}
	r, size := utf8.DecodeRuneInString(name)
	if size == 0 || size != len(name) {
		return tea.KeyPressMsg{}, false
	}
	return tea.KeyPressMsg{Code: r, Text: name}, true
}
