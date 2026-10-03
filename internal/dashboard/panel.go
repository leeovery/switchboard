package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The parts of a window's row on a line's panel, from its jack: its label,
// jackGap cells in, in labelColumn cells; its bar, where the panel draws
// one; its use, right-aligned in useColumn cells; and after whitherGap
// blanks, where it's heading, to the panel's side.
const (
	jackGap    = 3
	whitherGap = 2
)

var (
	// panelLabelInk is a window's label on a line's panel.
	panelLabelInk = ink{token: theme.TextMuted}
	// freeJackInk is a jack nothing is plugged into.
	freeJackInk = ink{token: theme.TextFaint}
)

// freeJack is a jack nothing is plugged into, and pluggedJack one a cord
// ends at.
const (
	freeJack    = "○"
	pluggedJack = "◉"
)

// linePanel is an account's panel among the lines: the account and its
// place, its state, whether new sessions go to it and whether the global
// pin names it, when the router next primes it, zero where unknown; the
// windows it shows, and the key of the one a request starts among them; its
// top edge's row, and how many rows it has between its edges, a jack each:
// one for each window shown, and one more for each of its calls past those.
// A boxed panel, as the plain list has, has no jacks, its left side a side
// like its right.
type linePanel struct {
	account      status.Account
	place        int
	state        status.State
	next, pinned bool
	primed       time.Time
	windows      []string
	started      string
	top, rows    int
	boxed        bool
}

// linePanelOf is the panel of doc's account a at now, showing the windows
// shown.
func (f Frame) linePanelOf(doc status.Document, a status.Account, now time.Time, shown []string) linePanel {
	return linePanel{
		account: a, place: place(doc, a.ID), state: doc.StateOf(a, now, f.Policy),
		next: len(doc.Accounts) > 1 && doc.Best == a.ID, pinned: doc.Pin.Has(a.ID), primed: primedNext(doc, a.ID),
		windows: shown, started: f.Policy.Started,
	}
}

// bottom is the panel's bottom edge's row.
func (p linePanel) bottom() int {
	return p.top + p.rows + 1
}

// edges are how the panel's edges are drawn: in accent.mode on the account
// new sessions go to, of several, else in the border's colour.
func (p linePanel) edges() ink {
	if p.next {
		return bestBorderInk
	}
	return borderInk
}

// drawPanel draws the panel p of doc at now from column x, width cells wide,
// with its windows' bars where bars is set: its top edge; its rows, each a
// jack, free until a cord ends at it, or, boxed, a side, and a window shown;
// and its bottom edge.
func (f Frame) drawPanel(c *canvas, doc status.Document, p linePanel, x, width int, bars bool, now time.Time) {
	edge := p.edges()
	panelTop(c, p, x, width)
	for i := range p.rows {
		y := p.top + 1 + i
		if p.boxed {
			c.text(x, y, "│", edge)
		} else {
			c.text(x, y, freeJack, freeJackInk)
		}
		c.text(x+width-1, y, "│", edge)
		if i < len(p.windows) {
			f.panelRow(c, doc, p, p.windows[i], now, x+jackGap, y, width-jackGap-1, bars)
		}
	}
	panelBottom(c, p, x, p.bottom(), width, now)
}

// panelTop draws the panel's top edge from x along its top row, width cells
// wide: its place and its account's name, in capitals; its state, its mark
// and its words, as its card leads with them, but idle's in text.tertiary,
// and nothing read yet's dim; and at its right its badges, those leavingOff
// leaves where they don't all fit, the name and the state cut short where
// even they don't.
func panelTop(c *canvas, p linePanel, x, width int) {
	edge := p.edges()
	mark, tone := marked(p.state.Condition)
	says := ink{token: tone, bold: true}
	switch p.state.Condition {
	case status.Idle:
		says.token = theme.TextTertiary
	case status.Unread:
		says = dimInk
	}
	lead := line{{"┌─", edge}, {" " + strconv.Itoa(p.place) + " · ", dimInk}, {strings.ToUpper(name(p.account)), titleInk}, {" ─ ", edge}, {mark + " ", ink{token: tone}}, {p.state.Says, says}, {" ", edge}}
	badges := p.badges()
	tail := func(badges []span) line {
		if len(badges) == 0 {
			return line{{"─┐", edge}}
		}
		return slices.Concat(line{{" ", edge}}, badgeLine(badges, edge), line{{" ─┐", edge}})
	}
	badges = leavingOff(badges, func(badges []span) bool { return lead.width()+1+tail(badges).width() <= width })
	end := tail(badges)
	start := c.line(x, p.top, lead.fit(width-end.width()-1))
	c.text(start, p.top, strings.Repeat("─", max(x+width-end.width()-start, 0)), edge)
	c.right(x+width, p.top, end)
}

// badges are the badges on the panel's top edge, in order, as a card's are:
// the primary's, the pin's, and the next account's.
func (p linePanel) badges() []span {
	var badges []span
	if p.account.Primary {
		badges = append(badges, span{primaryBadge, primaryInk})
	}
	if p.pinned {
		badges = append(badges, span{pinBadge, pinBadgeInk})
	}
	if p.next {
		badges = append(badges, span{nextBadge, badgeInk})
	}
	return badges
}

// panelRow draws the row of a panel's window with the given key from x along
// row y, width cells to the panel's side: its label; its bar, where bars is
// set, as a card's bar is drawn; its use, toned as panelUse says; and where
// it's heading, in full. A window that has lapsed says it hasn't started
// where its bar would be, and one the account hasn't read leaves its row
// blank.
func (f Frame) panelRow(c *canvas, doc status.Document, p linePanel, key string, now time.Time, x, y, width int, bars bool) {
	w, ok := p.account.Window(key)
	if !ok {
		return
	}
	s := standingOf(doc, p.account, w, now, f.Policy)
	c.line(x, y, line{{truncate(status.Short(w.Label), labelColumn-1), panelLabelInk}})
	at := x + labelColumn
	if s.lapsed {
		c.line(at, y, line{{"not started", dimInk}})
		return
	}
	if bars {
		f.bar(c, s, p.account.Reserve, now, at, y, panelBar)
		at += panelBar
	}
	c.right(at+useColumn, y, line{panelUse(s)})
	whither := x + width - (at + useColumn + whitherGap)
	c.line(at+useColumn+whitherGap, y, line{s.headed(now, "~", status.Dated)}.fit(whither))
}

// panelUse is how much of a window standing as s is used, bold, as a panel
// tones it: from 90%, in the ramp's last colour, from 70%, in its second,
// else in text.primary.
func panelUse(s standing) span {
	k := ink{token: theme.TextPrimary, bold: true}
	switch used := s.window.Utilization; {
	case used >= 0.9:
		k.token = theme.VizRamp4
	case used >= 0.7:
		k.token = theme.VizRamp2
	}
	return span{status.Percent(s.window.Utilization), k}
}

// panelBottom draws the panel's bottom edge from x along row y, width cells
// wide: when its windows reset, as resetsSays has it, in full, or briefly
// where that doesn't fit; or, where its session has lapsed, when the router
// primes it, as in "primed at 16:20".
func panelBottom(c *canvas, p linePanel, x, y, width int, now time.Time) {
	edge := p.edges()
	says := resetsSays(p, now, false)
	if ansi.StringWidth("└─┘")+says.width() > width {
		says = resetsSays(p, now, true)
	}
	if primed, ok := primedAt(p, now); ok {
		says = primed
	}
	start := c.line(x, y, slices.Concat(line{{"└─", edge}}, says).fit(width-1))
	c.text(start, y, strings.Repeat("─", max(x+width-1-start, 0)), edge)
	c.text(x+width-1, y, "┘", edge)
}

// primedAt says when the router next primes the panel's account, where its
// window a request starts has lapsed and the router says, as in " primed at
// 16:20 ", reporting false otherwise.
func primedAt(p linePanel, now time.Time) (line, bool) {
	if p.state.Condition != status.Idle || p.primed.IsZero() {
		return nil, false
	}
	return line{{" primed at ", dimInk}, {status.Dated(now, p.primed), mutedInk}, {" ", dimInk}}, true
}

// resetsSays says when the panel's windows reset, as resets has them: in
// full, as in " resets  session 17:10  ·  weeks Mon 21:00 ", or briefly, as
// a narrow panel has room for, as in " session 17:10 · weeks Mon 21:00 ";
// nothing where none does, so its edge runs whole.
func resetsSays(p linePanel, now time.Time, brief bool) line {
	said := p.resets(now)
	if len(said) == 0 {
		return nil
	}
	lead, sep := " resets  ", status.Separator
	if brief {
		lead, sep = " ", " · "
	}
	l := line{{lead, dimInk}, {said[0], mutedInk}}
	for _, s := range said[1:] {
		l = append(l, span{sep + s, dimInk})
	}
	return append(l, span{" ", dimInk})
}

// resets says when the panel's windows reset, of those it shows: the window
// a request starts, while it runs, as in "session 17:10"; then the rest,
// together where they reset at once, as weeksReset has them.
func (p linePanel) resets(now time.Time) []string {
	var said []string
	var rest []quota.Window
	for _, key := range p.windows {
		w, ok := p.account.Window(key)
		switch {
		case !ok || w.ResetsAt.IsZero() || p.account.HasLapsed(w):
		case key == p.started:
			said = append(said, status.InProse(w.Label)+" "+status.Dated(now, w.ResetsAt))
		default:
			rest = append(rest, w)
		}
	}
	return append(said, weeksReset(rest, now)...)
}

// weeksReset says when the windows given reset, as a panel's bottom edge
// does: all at once, as in "weeks Mon 21:00", or "week Mon 21:00" for one;
// else each, by its label, as in "Week Mon 21:00".
func weeksReset(windows []quota.Window, now time.Time) []string {
	if len(windows) == 0 {
		return nil
	}
	same := !slices.ContainsFunc(windows, func(w quota.Window) bool { return !w.ResetsAt.Equal(windows[0].ResetsAt) })
	switch {
	case same && len(windows) > 1:
		return []string{"weeks " + status.Dated(now, windows[0].ResetsAt)}
	case same:
		return []string{status.InProse(windows[0].Label) + " " + status.Dated(now, windows[0].ResetsAt)}
	}
	said := make([]string, len(windows))
	for i, w := range windows {
		said[i] = status.Short(w.Label) + " " + status.Dated(now, w.ResetsAt)
	}
	return said
}
