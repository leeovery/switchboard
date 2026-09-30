package dashboard

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// compactBar is how many cells a compact line's bars take.
	compactBar = 8
	// compactGap is the blank cells between the parts of a compact line.
	compactGap = 3
	// compactTitle is the fewest cells titles shrink to so that a line can
	// show more of its windows.
	compactTitle = 8
	// useWidth fits any percentage up to 999%, so they line up.
	useWidth = 4
)

// summary is an account as a compact line shows it, before it's fitted to a
// width.
type summary struct {
	title string
	// parts are its windows, and the windows that couldn't be read. Each
	// shows whole or not at all.
	parts []line
	// note says why the account couldn't be read, or that it hasn't been. It's
	// cut to fit.
	note span
	// extras are what the line shows, each whole, where there's room once
	// everything else is shown: how the account's reserve stands, once a
	// window has reached it, that it's under pressure, while it is, and how
	// many sessions it has, when it has any.
	extras []span
	marks  badges
}

// compact lays each account out on a line: its title, then each window's
// key, a short bar and how much is used, then how its reserve stands, whether
// it's under pressure, and its sessions, and last its marks. Titles line up.
// When room runs short, titles shrink as far as lets a line show another
// window, and windows that still don't fit give way from the right, after
// those notes. It reports how wide the widest line is.
func compact(doc status.Document, now time.Time, room int) ([]line, int) {
	summaries := make([]summary, len(doc.Accounts))
	longest := 0
	for i, a := range doc.Accounts {
		summaries[i] = summarize(doc, a, now)
		longest = max(longest, ansi.StringWidth(summaries[i].title))
	}
	narrowest := min(longest, compactTitle)
	rest := 0
	for _, s := range summaries {
		rest = max(rest, s.needs(s.fitting(room-narrowest)))
	}
	titleWidth := min(longest, max(room-rest, narrowest))
	lines := make([]line, len(summaries))
	width := 0
	for i, s := range summaries {
		lines[i] = s.layout(titleWidth, room)
		width = max(width, lines[i].width())
	}
	return lines, width
}

// summarize is how a compact line shows an account in doc, and the marks it
// carries.
func summarize(doc status.Document, a status.Account, now time.Time) summary {
	s := summary{title: a.Title(), marks: badgesOf(doc, a)}
	for _, note := range []string{doc.Reserved(a), doc.Pressed(a, now)} {
		if note != "" {
			s.extras = append(s.extras, span{note, warningInk})
		}
	}
	if a.Sessions > 0 {
		s.extras = append(s.extras, span{status.SessionCount(a.Sessions), dimInk})
	}
	for _, w := range a.Windows {
		s.parts = append(s.parts, compactWindow(a, w, now))
	}
	for _, f := range a.Failures {
		s.parts = append(s.parts, line{{status.Clean(f.Label) + " offline", offlineInk}})
	}
	switch {
	case a.Error != "":
		s.note = span{errorMark + status.Clean(a.Error), errorInk}
	case len(s.parts) == 0:
		s.note = span{"no usage yet", dimInk}
	}
	return s
}

// compactWindow shows the account's window w as its key, a short bar marking
// where the account's reserve starts, and how much of it is used.
func compactWindow(a status.Account, w quota.Window, now time.Time) line {
	pct := use(w, a.Project(w, now))
	l := line{{status.Clean(w.Key), dimInk}, spaces(1)}
	l = append(l, bar(w.Utilization, compactBar).mark(reserveCell(a.Reserve, compactBar), reserveMarker)...)
	return append(l, spaces(1+useWidth-ansi.StringWidth(pct.text)), pct)
}

// layout fits the summary to room cells, its title padded to titleWidth.
func (s summary) layout(titleWidth, room int) line {
	title := truncate(s.title, min(titleWidth, room))
	l := line{{title, titleInk}}
	lead := titleWidth - ansi.StringWidth(title) + compactGap
	room -= titleWidth
	shown := s.fitting(room)
	for _, part := range s.parts[:shown] {
		l = append(append(l, spaces(lead)), part...)
		lead = compactGap
	}
	free := room - s.needs(shown)
	switch {
	case shown < len(s.parts) && free >= 0:
		l = append(l, spaces(lead), span{ellipsis, dimInk})
	case shown == len(s.parts) && s.note.text != "":
		if free > compactGap {
			l = append(l, spaces(lead), span{truncate(s.note.text, free-compactGap), s.note.ink})
		}
	case shown == len(s.parts):
		l = append(l, s.extrasIn(lead, free)...)
	}
	return append(l, s.tail()...)
}

// extrasIn is those of the summary's extras that fit whole in free cells, in
// order, lead cells before the first and a gap before each other.
func (s summary) extrasIn(lead, free int) line {
	var l line
	for _, extra := range s.extras {
		needs := compactGap + ansi.StringWidth(extra.text)
		if needs > free {
			continue
		}
		l = append(l, spaces(lead), extra)
		free -= needs
		lead = compactGap
	}
	return l
}

// fitting is how many of its parts the summary can show in room cells after
// its title.
func (s summary) fitting(room int) int {
	n := len(s.parts)
	for n > 0 && s.needs(n) > room {
		n--
	}
	return n
}

// needs is how many cells the summary takes after its title to show its
// first n parts: those parts, an ellipsis standing for the rest, and the
// badge. Its note takes whatever is left.
func (s summary) needs(n int) int {
	cells := s.tail().width()
	for _, part := range s.parts[:n] {
		cells += compactGap + part.width()
	}
	if n < len(s.parts) {
		cells += compactGap + ansi.StringWidth(ellipsis)
	}
	return cells
}

// tail ends the line with the account's marks: that it's the primary, that
// the router pins new sessions to it, and that it's the best.
func (s summary) tail() line {
	var l line
	gap := 2
	for _, mark := range s.marks.shown(primaryMark, pinMark, bestMark) {
		l = append(l, spaces(gap), mark)
		gap = 1
	}
	return l
}
