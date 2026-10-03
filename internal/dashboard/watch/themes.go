package watch

import (
	"fmt"
	"image/color"
	"io"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// Themes is where the theme picker finds the themes to pick from, and keeps
// the user's pick.
type Themes interface {
	// List lists every theme there is to pick from, the themes directory read
	// afresh.
	List() theme.Listing
	// Chosen is the choice kept, as it stands now: another dashboard's, should
	// one have kept its own since this one read it.
	Chosen() theme.Choice
	// Keep changes the choice kept, as it stands now, as change says, for the
	// dashboard to start in from now on, and returns the choice it keeps.
	Keep(change func(theme.Choice) theme.Choice) (theme.Choice, error)
}

// tooSmall is what the footer says where the theme picker doesn't fit.
const tooSmall = "the terminal's too small for the theme picker"

// unansweredMsg says the terminal has had its time to say what its
// background is.
type unansweredMsg struct{}

// backdrop is the terminal's background: whether it's dark, as the terminal
// said, what it was before the dashboard set its own, and whether it has.
type backdrop struct {
	// settled is set once the terminal has said what its background is, or
	// had its time to: the dashboard is drawn from then on.
	settled bool
	// heard is set once the terminal has answered the one question the
	// dashboard asks it, which it may do after its time.
	heard bool
	// dark is whether the terminal's background is dark: a terminal that
	// didn't say is taken for dark.
	dark bool
	// original is the terminal's background as it said before the dashboard
	// set its own, to set back: nil where it hasn't said, or said a canvas of
	// the dashboard's own.
	original color.Color
	// set is set once the dashboard has set the terminal's background, which
	// is put back as the dashboard stops.
	set bool
}

// putBack puts back the terminal's background as the dashboard found it,
// once the dashboard has set its own: the colour the terminal said it was,
// or, where it didn't say, the terminal's own default, which OSC 111
// restores.
func (b backdrop) putBack(w io.Writer) {
	switch {
	case !b.set:
		return
	case b.original == nil:
		_, _ = io.WriteString(w, ansi.ResetBackgroundColor)
	default:
		_, _ = io.WriteString(w, ansi.SetBackgroundColor(rgbHex(b.original)))
	}
}

// rgbHex is c written #rrggbb.
func rgbHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

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

// background is what the screen sets the terminal's background to: the
// look's canvas, where it paints one; else, once the dashboard has set its
// own, the background the terminal said it found, as the reset nil has Bubble
// Tea write (OSC 111) restores the terminal profile's own, not one set since;
// and nil where the terminal didn't say, or nothing was set.
func (m Model) background(look dashboard.Look) color.Color {
	if canvas := look.Canvas(); canvas != nil || !m.backdrop.set {
		return canvas
	}
	return m.backdrop.original
}

// inForce is the theme the dashboard is drawn in, by the user's choice and
// the terminal's background.
func (m Model) inForce() theme.Theme {
	return m.pair.For(m.backdrop.dark)
}

// drawIn draws the screen in t from now on: should t paint its canvas, the
// terminal's background is set to it, and so is to be put back.
func (m Model) drawIn(t theme.Theme) Model {
	m.showing = t
	m.backdrop.set = m.backdrop.set || t.Paints()
	return m
}

// answered takes in the terminal's answer to the one question the dashboard
// asks it, its background, whenever it comes: as the dashboard asks before
// it sets its own, it's the background the dashboard found, and the half of
// the pair goes by how dark it is. One that's a canvas the dashboard paints
// may be one an exit it couldn't catch left, so it's never set back: OSC 111
// resets the terminal's instead. Should the answer come once the dashboard
// is drawn, and give the other half of the pair, the dashboard is drawn in
// that from then on.
func (m Model) answered(background color.Color) Model {
	if m.backdrop.heard {
		return m
	}
	painted := m.painted(background)
	m.backdrop.heard, m.backdrop.original = true, background
	if painted {
		m.backdrop.original = nil
	}
	logger.Debug("terminal background", "answered", background != nil, "painted", painted, "late", m.backdrop.settled)
	return m.settle(theme.Dark(background))
}

// noAnswer takes the terminal for dark, having had its time to say what its
// background is, unless it has said.
func (m Model) noAnswer() Model {
	if m.backdrop.settled {
		return m
	}
	return m.settle(theme.Dark(nil))
}

// settle has the dashboard drawn, from now on, as on a terminal whose
// background is dark, or light, in the theme for it: but for one the picker
// shows.
func (m Model) settle(dark bool) Model {
	if m.backdrop.settled && m.backdrop.dark == dark {
		return m
	}
	m.backdrop.settled, m.backdrop.dark = true, dark
	if m.picker.open {
		return m
	}
	return m.drawIn(m.inForce())
}

// painted reports whether background is a canvas the dashboard paints: a
// built-in theme's, or one of those it's drawn in.
func (m Model) painted(background color.Color) bool {
	if background == nil {
		return false
	}
	for _, t := range append(theme.Builtins(), m.pair.Light, m.pair.Dark) {
		if canvas := t.Colour(theme.Canvas); canvas != nil && rgbHex(canvas) == rgbHex(background) {
			return true
		}
	}
	return false
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

// askMsg is the choice kept, as it stands, one theme, which setting a half
// of the pair, as pick does, asks first whether to clear; pair is the
// themes it draws in, as the picker lists them.
type askMsg struct {
	pick   pick
	chosen theme.Choice
	pair   theme.Pair
}

// keptMsg is how keeping a pick went: the choice as kept, and pair the themes
// it draws in, as the picker lists them; or why it wasn't kept.
type keptMsg struct {
	pick   pick
	choice theme.Choice
	pair   theme.Pair
	err    error
}

// openPicker lists the themes afresh, for the picker to open on. Under
// NO_COLOR, before the dashboard is drawn, or while they're being listed,
// it does nothing; on a terminal too small for it, it says so.
func (m Model) openPicker() (Model, tea.Cmd) {
	switch {
	case !m.coloured() || !m.backdrop.settled || m.picker.opening:
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
// it, it says so instead.
func (m Model) listed(msg listedMsg) (Model, tea.Cmd) {
	m.picker.opening = false
	if !dashboard.PickerFits(m.size.Width, m.size.Height) {
		return m.noting(tooSmall)
	}
	m.pair = msg.listing.Pair(m.choice)
	m = m.drawIn(m.inForce())
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
		return m, m.keep(p)
	}
	themes, listing := m.cfg.Themes, m.picker.listing
	return m, func() tea.Msg {
		if chosen := themes.Chosen(); chosen.IsOne() {
			return askMsg{pick: p, chosen: chosen, pair: listing.Pair(chosen)}
		}
		return keepIn(themes, listing, p)
	}
}

// answer takes in the answer to the picker's question: y clears the one
// theme and sets the half asked of, while n or esc keeps it.
func (m Model) answer(key string) (Model, tea.Cmd) {
	p := m.picker.asking
	switch key {
	case "y", "Y":
		m.picker.asking = pick{}
		return m, m.keep(p)
	case "n", "N", "esc":
		m.picker.asking = pick{}
	}
	return m, nil
}

// keep has the pick kept, set in the choice as it stands now.
func (m Model) keep(p pick) tea.Cmd {
	themes, listing := m.cfg.Themes, m.picker.listing
	return func() tea.Msg {
		return keepIn(themes, listing, p)
	}
}

// keepIn has themes keep the pick, set in the choice as it stands now, and
// pairs what it keeps from listing.
func keepIn(themes Themes, listing theme.Listing, p pick) keptMsg {
	choice, err := themes.Keep(p.of)
	return keptMsg{pick: p, choice: choice, pair: listing.Pair(choice), err: err}
}

// asked asks, in the picker, whether to clear the one theme the choice kept
// holds, as it stands now, to set the half of the pair asked of, the
// dashboard drawn by it from then on.
func (m Model) asked(msg askMsg) Model {
	if !m.picker.open {
		return m
	}
	m = m.chosen(msg.chosen, msg.pair)
	m.picker.asking = msg.pick
	return m
}

// kept takes in how keeping a pick went: the dashboard drawn by the choice
// as kept from now on; or, should it not be kept, the choice standing as it
// was, and the picker saying so.
func (m Model) kept(msg keptMsg) Model {
	if msg.err != nil {
		logger.Warn("couldn't keep the theme chosen", "theme", msg.pick.slug, "half", msg.pick.half, "error", msg.err)
		if m.picker.open {
			m.picker.note = "not kept: see the log"
		}
		return m
	}
	logger.Info("theme chosen", "theme", msg.choice.Theme, "light", msg.choice.Light, "dark", msg.choice.Dark)
	return m.chosen(msg.choice, msg.pair)
}

// chosen has the dashboard drawn by choice, in pair, from now on: while the
// picker is open, which shows the theme the cursor is on, its rows badged by
// it, the cursor where it was; else in the theme in force by it.
func (m Model) chosen(choice theme.Choice, pair theme.Pair) Model {
	m.choice, m.pair = choice, pair
	if !m.picker.open {
		return m.drawIn(m.inForce())
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
	return m.drawIn(m.inForce())
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
		r := dashboard.PickerRow{Name: status.Clean(row.Name)}
		if row.Slug != "" {
			r.Badge = choice.Badge(row.Slug)
		}
		if row.Problem != nil {
			r.Problem = row.Problem.Reason
		}
		d.Rows = append(d.Rows, r)
	}
	if p.asking != (pick{}) {
		d.Clearing = status.Clean(choice.Theme)
	}
	return d
}

// keys are the keys there are while the picker is open, which takes every
// key but q: while it asks its question, y and n; else the arrows, through
// the themes, enter, d and l, to set one, and esc, to close it; then q.
func (p picker) keys() []keyListing {
	keys := []keyListing{
		{key: "↑↓", footer: "preview", help: "move through the themes, the dashboard drawn in each"},
		{key: "⏎", footer: "set theme", help: "set the theme as the one theme"},
		{key: "d", footer: "set as dark", help: "set the theme as the dark half of the pair"},
		{key: "l", footer: "set as light", help: "set the theme as the light half of the pair"},
		{key: "esc", footer: "close", help: "close the picker, putting back the theme in force"},
	}
	if p.asking != (pick{}) {
		keys = []keyListing{
			{key: "y", footer: "confirm", help: "clear the one theme, and set the half"},
			{key: "n", footer: "cancel", help: "keep the one theme"},
		}
	}
	keys = append(keys, keyListing{key: "q", footer: "quit", help: "quit", always: true})
	for i := range keys {
		keys[i].works = true
	}
	return keys
}
