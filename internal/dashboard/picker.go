package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

// Picker is the theme picker as it's drawn: a panel slid over the right of
// the screen, so the view beside it shows each theme as the cursor reaches
// it.
type Picker struct {
	// Rows are the themes it lists, in order.
	Rows []PickerRow
	// Cursor is the row the cursor is on.
	Cursor int
	// Clearing names the one theme chosen while the picker asks whether to
	// clear it, to set a half of the pair: "" while it asks nothing. Its
	// answers, y and n, take the place of the keys.
	Clearing string
	// Note says what went wrong, such as a choice that wasn't kept.
	Note string
}

// PickerRow is a theme as the picker lists it.
type PickerRow struct {
	Name string
	// Badge says what the theme fills in the choice: ●, ● light, ● dark or
	// ● both.
	Badge string
	// Problem is why it doesn't load, in a word or two: "" for one that does.
	Problem string
}

const (
	// pickerWide is how many cells wide the picker is, and pickerNarrow how
	// many on a screen too narrow to leave the view as many as it takes.
	pickerWide   = 30
	pickerNarrow = 24
	// pickerChrome is the cells of a row that aren't its content: the border
	// and a gap.
	pickerChrome = 2
	// cursorColumn is the cells the cursor's bar takes before each name.
	cursorColumn = 2
	// keyColumn is the cells a key takes before what it does.
	keyColumn = 5
	// nameFloor is the fewest cells a theme's name is cut to, however much
	// its row's badge and problem take.
	nameFloor = 4
)

// The keys the picker takes, and what each does: those it always takes, and
// those that answer its question.
var (
	pickerKeys = [][2]string{{"⏎", "set theme"}, {"d", "set as dark"}, {"l", "set as light"}, {"esc", "close"}}
	answerKeys = [][2]string{{"y", "confirm"}, {"n", "cancel"}}
)

// pickerHead is the picker's rows over the themes: a blank one, its name,
// level with the dashboard's, and another blank.
var pickerHead = []line{nil, {{"Themes", nameInk}}, nil}

// PickerWidth is how many cells wide the picker is on a screen width cells
// wide: 30, or 24 where 30 would leave the view fewer than it takes; zero
// where not even that fits.
func PickerWidth(width int) int {
	switch {
	case width >= 2*pickerWide:
		return pickerWide
	case width >= pickerNarrow:
		return pickerNarrow
	default:
		return 0
	}
}

// PickerFits reports whether the picker fits a screen width × height: its
// width, and the height of its head, a theme, and its foot.
func PickerFits(width, height int) bool {
	return PickerWidth(width) > 0 && height >= len(pickerHead)+1+len(Picker{}.foot(0))
}

// Over draws the picker over the right of screen, a row of it on each of the
// screen's lines, width cells wide, as look draws it.
func (p Picker) Over(screen []string, width int, look Look) []string {
	pw := PickerWidth(width)
	keep := width - pw
	panel := p.lines(pw, len(screen))
	drawn := make([]string, len(screen))
	for i, row := range screen {
		var b strings.Builder
		kept := ansi.Truncate(row, keep, "")
		b.WriteString(kept)
		if gap := keep - ansi.StringWidth(kept); gap > 0 {
			b.WriteString(look.render(strings.Repeat(" ", gap), ink{}))
		}
		if look.styled {
			// The screen's line may be cut in the middle of a span: nothing
			// of its style may run on into the picker.
			b.WriteString(ansi.ResetStyle)
		}
		panel[i].draw(&b, look)
		drawn[i] = b.String()
	}
	return drawn
}

// lines are the picker's rows, height of them, width cells wide: its head,
// as many of its themes as fit, scrolled to keep the cursor's in sight, and
// its foot.
func (p Picker) lines(width, height int) []line {
	inner := width - pickerChrome
	foot := p.foot(inner)
	shown := max(height-len(pickerHead)-len(foot), 0)
	top := min(max(p.Cursor-shown+1, 0), max(len(p.Rows)-shown, 0))
	lines := make([]line, 0, height)
	for _, l := range pickerHead {
		lines = append(lines, framed(l, inner, hue{}))
	}
	for i := top; i < top+shown; i++ {
		var on hue
		var content line
		if i < len(p.Rows) {
			if i == p.Cursor {
				on = hue{token: theme.BgSelection}
			}
			content = p.Rows[i].line(inner, on)
		}
		lines = append(lines, framed(content, inner, on))
	}
	for _, l := range foot {
		lines = append(lines, framed(l, inner, hue{}))
	}
	return lines[:min(len(lines), height)]
}

// foot is the picker's rows under its themes, inner cells wide: its question
// or its note, a blank line for neither, a rule, and the keys it takes now.
// A question cuts the theme it names short to stay whole.
func (p Picker) foot(inner int) []line {
	var message line
	keys := pickerKeys
	switch {
	case p.Clearing != "":
		const ask, answers = "clear ", "?  y / n"
		name := truncate(p.Clearing, max(inner-ansi.StringWidth(ask+answers), nameFloor))
		message = line{{ask + name + answers, ink{token: theme.TextSecondary}}}
		keys = answerKeys
	case p.Note != "":
		message = line{{"⚠ " + p.Note, warningInk}}
	}
	foot := []line{message, {{rule(inner), borderInk}}}
	for _, k := range keys {
		key := k[0] + strings.Repeat(" ", keyColumn-ansi.StringWidth(k[0]))
		foot = append(foot, line{{key, ink{token: theme.AccentKey}}, {k[1], ink{token: theme.TextMuted}}})
	}
	return foot
}

// line is a theme's row, inner cells wide, on the surface on, the zero hue
// for the canvas: the cursor's bar where the cursor is, the theme's
// name, and at the right, its badge, and that it doesn't load, with why
// where that fits beside no badge.
func (r PickerRow) line(inner int, on hue) line {
	bar, name := span{strings.Repeat(" ", cursorColumn), ink{on: on}}, ink{token: theme.TextPrimary, on: on}
	switch {
	case on != hue{}:
		bar, name = span{"▌ ", ink{token: theme.AccentPrimary, on: on}}, ink{token: theme.TextOnSelection, on: on, bold: true}
	case r.Problem != "":
		name = ink{token: theme.TextSubtle, on: on}
	}
	tail := r.tail(inner, on)
	room := inner - cursorColumn - tail.width()
	label := truncate(r.Name, max(room, nameFloor))
	gap := span{strings.Repeat(" ", max(room-ansi.StringWidth(label), 0)), ink{on: on}}
	return append(line{bar, {label, name}, gap}, tail...)
}

// tail is the end of a theme's row, inner cells wide, on the surface on: a
// warning that it doesn't load, saying why where that fits beside the whole
// of its name and there's no badge, then its badge.
func (r PickerRow) tail(inner int, on hue) line {
	var tail line
	if r.Problem != "" {
		warning := "⚠"
		room := inner - cursorColumn - max(ansi.StringWidth(r.Name), nameFloor)
		if why := warning + " " + r.Problem; r.Badge == "" && 1+ansi.StringWidth(why) <= room {
			warning = why
		}
		tail = line{{" ", ink{on: on}}, {warning, ink{token: theme.AccentAttention, on: on}}}
	}
	if r.Badge != "" {
		tail = append(tail, span{" ", ink{on: on}}, span{r.Badge, ink{token: theme.AccentPrimary, on: on}})
	}
	return tail
}

// framed is a row of the picker: its border, then a gap and content, cut or
// padded to inner cells, on the surface on.
func framed(content line, inner int, on hue) line {
	content = content.fit(inner)
	l := append(line{{"│", borderInk}, {" ", ink{on: on}}}, content...)
	return append(l, span{strings.Repeat(" ", max(inner-content.width(), 0)), ink{on: on}})
}
