package watch

import (
	"fmt"
	"image/color"
	"io"
	"slices"
	"time"

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
	// Keep keeps the choice, for the dashboard to start in from now on.
	Keep(theme.Choice) error
}

// answerWithin is how long the terminal is given to say what its background
// is before it's taken for dark. Terminals answer in a few milliseconds, and
// the screen stays blank till then, so as not to flash one theme then the
// other.
const answerWithin = 100 * time.Millisecond

// unansweredMsg says the terminal has had its time to say what its
// background is.
type unansweredMsg struct{}

// backdrop is the terminal's background: whether it's dark, as the terminal
// said, what it was before the dashboard set its own, and whether it has.
type backdrop struct {
	// settled is set once the terminal has said what its background is, or
	// had its time to: the dashboard is drawn from then on.
	settled bool
	// dark is whether the terminal's background is dark: a terminal that
	// didn't say is taken for dark.
	dark bool
	// original is the terminal's background as it said before the dashboard
	// set its own: nil where it didn't say in time.
	original color.Color
	// set is set once the dashboard has set the terminal's background, which
	// is put back as the dashboard stops.
	set bool
}

// putBack puts back the terminal's background as the dashboard found it,
// once the dashboard has set its own: the colour the terminal said it was,
// or, where it didn't say before the dashboard painted, the terminal's own
// default, which OSC 111 restores.
func (b backdrop) putBack(w io.Writer) {
	switch {
	case !b.set:
		return
	case b.original == nil:
		_, _ = io.WriteString(w, ansi.ResetBackgroundColor)
	default:
		r, g, bl, _ := b.original.RGBA()
		_, _ = io.WriteString(w, ansi.SetBackgroundColor(fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, bl>>8)))
	}
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

// answered takes in the terminal's background, as it said, or nil where it
// didn't say in time: the dashboard is drawn from then on, in the theme for
// it. An answer once the dashboard is drawn changes nothing: by then it may
// be the dashboard's own canvas the terminal says.
func (m Model) answered(background color.Color) Model {
	if m.backdrop.settled {
		return m
	}
	m.backdrop.settled, m.backdrop.dark, m.backdrop.original = true, theme.Dark(background), background
	logger.Debug("terminal background", "answered", background != nil, "dark", m.backdrop.dark)
	return m.drawIn(m.inForce())
}

// picker is the theme picker, while it's open.
type picker struct {
	open bool
	// listing is the themes listed as it opened, and rows what it lists for
	// the user's choice.
	listing theme.Listing
	rows    []theme.Entry
	cursor  int
	// asking is the question it's asking, while it asks one.
	asking question
	// note says what went wrong, till the next key.
	note string
}

// question asks to clear the one theme chosen, to set a half of the pair: the
// dark half, or the light, to the theme slug names.
type question struct {
	slug string
	dark bool
}

// openPicker opens the theme picker, the themes listed afresh, the theme in
// force found again among them, and the cursor on it. Under NO_COLOR, or
// before the dashboard is drawn, it does nothing; on a terminal too small
// for it, it says so.
func (m Model) openPicker() (Model, tea.Cmd) {
	switch {
	case !m.coloured() || !m.backdrop.settled:
		return m, nil
	case !dashboard.PickerFits(m.size.Width, m.size.Height):
		return m.noting("the terminal's too small for the theme picker")
	}
	listing := m.cfg.Themes.List()
	m.pair = listing.Pair(m.choice)
	m = m.drawIn(m.inForce())
	m.picker = picker{open: true, listing: listing, rows: listing.Rows(m.choice)}
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
// half, asking first to clear the one theme; and esc closes the picker.
func (m Model) pickerKey(key string) (tea.Model, tea.Cmd) {
	m.picker.note = ""
	if m.picker.asking != (question{}) {
		return m.answer(key), nil
	}
	switch key {
	case "up":
		return m.moveCursor(-1), nil
	case "down":
		return m.moveCursor(1), nil
	case "enter":
		return m.choose(theme.One), nil
	case "d", "D", "l", "L":
		return m.chooseHalf(key == "d" || key == "D"), nil
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

// choose has the cursor's theme chosen as choice makes a choice of it.
func (m Model) choose(choice func(slug string) theme.Choice) Model {
	slug, ok := m.cursorTheme()
	if !ok {
		return m
	}
	return m.keep(choice(slug))
}

// chooseHalf sets the cursor's theme as the pair's dark half, or its light,
// or, over the one theme, asks first whether to clear it.
func (m Model) chooseHalf(dark bool) Model {
	slug, ok := m.cursorTheme()
	switch {
	case !ok:
		return m
	case m.choice.IsOne():
		m.picker.asking = question{slug: slug, dark: dark}
		return m
	default:
		return m.keep(m.choice.WithHalf(dark, slug))
	}
}

// answer takes in the answer to the picker's question: y clears the one
// theme and sets the half asked of, while n or esc keeps it.
func (m Model) answer(key string) Model {
	q := m.picker.asking
	switch key {
	case "y", "Y":
		m.picker.asking = question{}
		return m.keep(m.choice.WithHalf(q.dark, q.slug))
	case "n", "N", "esc":
		m.picker.asking = question{}
	}
	return m
}

// keep keeps the choice, and draws by it from now on, the screen still
// showing the theme the cursor is on: should it not be kept, the choice
// stands as it was, and the picker says so.
func (m Model) keep(choice theme.Choice) Model {
	if err := m.cfg.Themes.Keep(choice); err != nil {
		logger.Warn("couldn't keep the theme chosen", "theme", choice.Theme, "light", choice.Light, "dark", choice.Dark, "error", err)
		m.picker.note = "not kept: see the log"
		return m
	}
	logger.Info("theme chosen", "theme", choice.Theme, "light", choice.Light, "dark", choice.Dark)
	slug, _ := m.cursorTheme()
	m.choice, m.pair = choice, m.picker.listing.Pair(choice)
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
	return m.closePicker().noting("the terminal's too small for the theme picker")
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
	if p.asking != (question{}) {
		d.Clearing = status.Clean(choice.Theme)
	}
	return d
}
