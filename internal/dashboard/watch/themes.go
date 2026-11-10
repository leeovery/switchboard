package watch

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// Themes is where the theme picker finds the themes to pick from, and the
// choice kept, which the picker keeps its pick in, in the preferences.
type Themes interface {
	// List lists every theme there is to pick from, the themes directory read
	// afresh.
	List() theme.Listing
	// Chosen is the choice kept, as it stands now: another dashboard's, should
	// one have kept its own since this one read it.
	Chosen() theme.Choice
}

// tooSmall is what the key line says where the theme picker doesn't fit.
const tooSmall = "the terminal's too small for the theme picker"

// coloured reports whether the dashboard is drawn in colour, as it is but
// under NO_COLOR, which gives it no themes.
func (m Model) coloured() bool {
	return m.cfg.Themes != nil
}

// look is how the dashboard is drawn: in the theme shown, full screen, or
// without colour.
func (m Model) look() dashboard.Look {
	if !m.coloured() {
		return dashboard.NoColour()
	}
	return dashboard.Screen(m.showing)
}

// Backdrop is the terminal's background as the dashboard has it.
func (m Model) Backdrop() backdrop.Backdrop {
	return m.backdrop
}

// drawInForce draws the screen from now on in the theme in force, by the
// user's choice and the terminal's background, as drawIn has it.
func (m Model) drawInForce() Model {
	m.backdrop, m.showing = m.backdrop.DrawInForce(m.pair)
	return m
}

// drawIn draws the screen in t from now on: should t paint its canvas, the
// terminal's background is set to it, and so is to be put back.
func (m Model) drawIn(t theme.Theme) Model {
	m.showing = t
	m.backdrop = m.backdrop.DrawIn(t)
	return m
}

// settled has the dashboard drawn, from now on, as b says the terminal's
// background is, in the theme in force, should afresh say so: but for one
// the picker shows.
func (m Model) settled(b backdrop.Backdrop, afresh bool) Model {
	m.backdrop = b
	if !afresh || m.picker.open {
		return m
	}
	return m.drawInForce()
}

// picker is the theme picker, while it's open.
type picker struct {
	open bool
	// opening is set while the themes are listed for it to open on.
	opening bool
	// listing is the themes listed as it opened, and rows what it lists for
	// the user's choice.
	listing theme.Listing
	rows    []theme.Entry
	cursor  int
	// asking is the half of the pair it asks whether to set, clearing the one
	// theme chosen, while it asks.
	asking pick
	// note says what went wrong, till the next key.
	note string
}

// pick is what a key in the picker sets: the theme slug names, as the one
// theme, or as the pair's dark half, or its light. The zero pick sets
// nothing.
type pick struct {
	slug string
	// half is the half of the pair it sets, dark or light: "" for the one
	// theme.
	half string
}

// The halves of the pair a pick sets.
const (
	darkHalf  = "dark"
	lightHalf = "light"
)

// of is the choice c with the pick set: the one theme, the pair cleared, or a
// half of the pair, the one theme cleared.
func (p pick) of(c theme.Choice) theme.Choice {
	if p.half == "" {
		return theme.One(p.slug)
	}
	return c.WithHalf(p.half == darkHalf, p.slug)
}

// listedMsg is the themes listed afresh, for the picker to open on.
type listedMsg struct {
	listing theme.Listing
}

// askMsg is the choice kept, as it stands, read for a pick of a half of the
// pair: one theme, which the picker asks first whether to clear, else none,
// and the pick is kept; listing is the themes the picker listed as it was
// picked, and pair the themes the choice draws in, as they list them.
type askMsg struct {
	pick    pick
	chosen  theme.Choice
	listing theme.Listing
	pair    theme.Pair
}

// openPicker lists the themes afresh, for the picker to open on. Under
// NO_COLOR, before the dashboard is drawn, or while they're being listed,
// it does nothing; on a terminal too small for it, it says so.
func (m Model) openPicker() (Model, tea.Cmd) {
	switch {
	case !m.coloured() || !m.backdrop.Settled() || m.picker.opening:
		return m, nil
	case !dashboard.PickerFits(m.size.Width, m.size.Height):
		return m.noting(tooSmall)
	}
	m.picker.opening = true
	themes := m.cfg.Themes
	return m, func() tea.Msg {
		return listedMsg{listing: themes.List()}
	}
}

// listed opens the picker on the themes listed, the theme in force found
// again among them, and the cursor on it: on a terminal now too small for
// it, it says so instead; and with a card opened over the page since t was
// pressed, it stays shut, as the card's keys alone act.
func (m Model) listed(msg listedMsg) (Model, tea.Cmd) {
	m.picker.opening = false
	switch {
	case m.open != nothingOpen:
		return m, nil
	case !dashboard.PickerFits(m.size.Width, m.size.Height):
		return m.noting(tooSmall)
	}
	m.pair = msg.listing.Pair(m.choice)
	m = m.drawInForce()
	m.picker = picker{open: true, listing: msg.listing, rows: msg.listing.Rows(m.choice)}
	m.picker.cursor = m.picker.find(m.showing.Slug)
	return m, nil
}

// find is the row of the theme slug names, where it can be picked, else
// the first that can, else 0.
func (p picker) find(slug string) int {
	if i := slices.IndexFunc(p.rows, func(e theme.Entry) bool { return e.Slug == slug && e.Problem == nil }); i >= 0 {
		return i
	}
	return max(slices.IndexFunc(p.rows, func(e theme.Entry) bool { return e.Problem == nil }), 0)
}

// pickerKey acts on a key while the picker is open, which takes them all but
// those that quit: while it asks its question, y and n answer it; else the
// arrows move the cursor, each theme shown as it's reached; enter sets the
// cursor's theme as the one theme; d and l set it as the pair's dark or light
// half, asking first, over one theme, to clear it; and esc closes the picker.
func (m Model) pickerKey(key string) (tea.Model, tea.Cmd) {
	m.picker.note = ""
	if m.picker.asking != (pick{}) {
		return m.answer(key)
	}
	switch key {
	case "up":
		return m.moveCursor(-1), nil
	case "down":
		return m.moveCursor(1), nil
	case "enter":
		return m.choose("")
	case "d", "D":
		return m.choose(darkHalf)
	case "l", "L":
		return m.choose(lightHalf)
	case "esc":
		return m.closePicker(), nil
	}
	return m, nil
}

// moveCursor moves the picker's cursor by one row up or down, past the
// themes that don't load, and shows the theme it reaches. At either end it
// stays where it is.
func (m Model) moveCursor(by int) Model {
	for i := m.picker.cursor + by; 0 <= i && i < len(m.picker.rows); i += by {
		if m.picker.rows[i].Problem == nil {
			m.picker.cursor = i
			return m.drawIn(m.picker.rows[i].Theme)
		}
	}
	return m
}

// cursorTheme is the slug of the theme the picker's cursor is on, and
// whether it can be picked.
func (m Model) cursorTheme() (string, bool) {
	if m.picker.cursor >= len(m.picker.rows) {
		return "", false
	}
	row := m.picker.rows[m.picker.cursor]
	return row.Slug, row.Slug != "" && row.Problem == nil
}

// choose sets the cursor's theme as the one theme, with half "", or as that
// half of the pair: where the choice kept is one theme as it stands now,
// asking first whether to clear it.
func (m Model) choose(half string) (Model, tea.Cmd) {
	slug, ok := m.cursorTheme()
	if !ok {
		return m, nil
	}
	p := pick{slug: slug, half: half}
	if half == "" {
		return m.keepPick(p, m.picker.listing), nil
	}
	themes, listing := m.cfg.Themes, m.picker.listing
	return m, func() tea.Msg {
		chosen := themes.Chosen()
		return askMsg{pick: p, chosen: chosen, listing: listing, pair: listing.Pair(chosen)}
	}
}

// answer takes in the answer to the picker's question: y clears the one
// theme and sets the half asked of, while n or esc keeps it.
func (m Model) answer(key string) (Model, tea.Cmd) {
	p := m.picker.asking
	switch key {
	case "y", "Y":
		m.picker.asking = pick{}
		return m.keepPick(p, m.picker.listing), nil
	case "n", "N", "esc":
		m.picker.asking = pick{}
	}
	return m, nil
}

// asked takes in the choice kept, as it stands now, for a half of the pair
// picked: over one theme, it asks, in the picker, whether to clear it to set
// the half, the dashboard drawn by it from then on; over none, it keeps the
// half.
func (m Model) asked(msg askMsg) Model {
	switch {
	case !msg.chosen.IsOne():
		return m.keepPick(msg.pick, msg.listing)
	case !m.picker.open:
		return m
	}
	m = m.chosen(msg.chosen, msg.pair)
	m.picker.asking = msg.pick
	return m
}

// pickNotKept takes in a pick not kept: the choice stands as it was, and the
// picker says so.
func (m Model) pickNotKept() Model {
	if m.picker.open {
		m.picker.note = "not kept: see the log"
	}
	return m
}

// chosen has the dashboard drawn by choice, in pair, from now on: while the
// picker is open, which shows the theme the cursor is on, its rows badged by
// it, the cursor where it was; else in the theme in force by it.
func (m Model) chosen(choice theme.Choice, pair theme.Pair) Model {
	m.choice, m.pair = choice, pair
	if !m.picker.open {
		return m.drawInForce()
	}
	slug, _ := m.cursorTheme()
	m.picker.rows = m.picker.listing.Rows(choice)
	m.picker.cursor = m.picker.find(slug)
	return m
}

// closePicker closes the picker, putting back the theme in force, by the
// choice as it stands.
func (m Model) closePicker() Model {
	m.picker = picker{}
	return m.drawInForce()
}

// fitPicker closes the picker on a terminal now too small for it, saying so.
func (m Model) fitPicker() (Model, tea.Cmd) {
	if !m.picker.open || dashboard.PickerFits(m.size.Width, m.size.Height) {
		return m, nil
	}
	return m.closePicker().noting(tooSmall)
}

// drawn is the picker as it's drawn for the user's choice: its note, or
// else that the themes directory can't be read, while it can't.
func (p picker) drawn(choice theme.Choice) dashboard.Picker {
	d := dashboard.Picker{Cursor: p.cursor, Note: p.note}
	if d.Note == "" && p.listing.Problem != nil {
		d.Note = "can't read the themes directory"
	}
	for _, row := range p.rows {
		r := dashboard.PickerRow{Name: prose.Clean(row.Name)}
		if row.Slug != "" {
			r.Badge = choice.Badge(row.Slug)
		}
		if row.Problem != nil {
			r.Problem = row.Problem.Reason
		}
		d.Rows = append(d.Rows, r)
	}
	if p.asking != (pick{}) {
		d.Clearing = prose.Clean(choice.Theme)
	}
	return d
}

// keys are the picker's own keys, as the key line lists them while it's
// open: while it asks its question, y and n; else the arrows, through the
// themes, enter, d and l, to set one, and esc, to close it.
func (p picker) keys() []dashboard.Key {
	if p.asking != (pick{}) {
		return []dashboard.Key{{Key: "y", Does: "confirm"}, {Key: "n", Does: "cancel"}}
	}
	return []dashboard.Key{
		{Key: "↑↓", Does: "preview"},
		{Key: "enter", Does: "set theme"},
		{Key: "d", Does: "set as dark"},
		{Key: "l", Does: "set as light"},
		{Key: "esc", Does: "close"},
	}
}
