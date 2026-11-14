package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionLine is a line of sessions by day: a session's row; or the rule
// over the sessions that ended on a day, by the session first under it.
type sessionLine struct {
	session *views.PeriodSession
	ruled   bool
}

// sessionLines are the lines of the sessions given, in their order: those
// running first, with no rule; then, for each day the rest ended on, its
// rule and the sessions that ended on it.
func sessionLines(sessions []views.PeriodSession) []sessionLine {
	lines := make([]sessionLine, 0, len(sessions))
	for i := range sessions {
		s := &sessions[i]
		if s.EndedOn != "" && (i == 0 || sessions[i-1].EndedOn != s.EndedOn) {
			lines = append(lines, sessionLine{session: s, ruled: true})
		}
		lines = append(lines, sessionLine{session: s})
	}
	return lines
}

// sessionsByDay are sessions grouped by the day each ended, as the List and
// a project's SESSIONS draw them: their words, as the List's say them
// grouped by day; their columns; their lines; the width they're drawn
// across; and the session lit, by id, "" where none is.
type sessionsByDay struct {
	words   sessionWords
	columns []listColumn
	lines   []sessionLine
	width   int
	lit     string
}

// within is the sessions shown a page rows tall: where that leaves a line
// alone, without their day rules, as a rule shows with the first session
// under it or not at all, and there can't.
func (s sessionsByDay) within(page int) sessionsByDay {
	if _, fits := flex.Lay(page, flex.Part{Least: 1}, flex.Part{Least: 1}); !fits {
		s.lines = slices.DeleteFunc(slices.Clone(s.lines), func(l sessionLine) bool { return l.ruled })
	}
	return s
}

// heads are the day rules, each with the first session under it, which a
// page shows together, or the rule not at all.
func (s sessionsByDay) heads() []headed {
	var heads []headed
	for i, l := range s.lines {
		if l.ruled {
			heads = append(heads, headed{at: i, first: i + 1})
		}
	}
	return heads
}

// kept are the day rules, each to the line after the last session under
// it, as a page scrolled past one into its sessions keeps it on its first
// row.
func (s sessionsByDay) kept() []keptHeading {
	var kept []keptHeading
	for i, l := range s.lines {
		if !l.ruled {
			continue
		}
		if n := len(kept); n > 0 {
			kept[n-1].end = i
		}
		kept = append(kept, keptHeading{at: i, end: len(s.lines)})
	}
	return kept
}

// drawPage draws the lines paged shows, scrolled as given, from row y: a
// rule kept in place of the first, where paged keeps one, then the rest.
func (s sessionsByDay) drawPage(c *canvas, y int, paged rowsPaged, scroll int) {
	from, to := paged.shown(scroll)
	if at, ok := paged.keeps(from, to); ok {
		s.drawLine(c, y, s.lines[at])
	}
	for i := paged.top(from, to); i < to; i++ {
		s.drawLine(c, y+i-from, s.lines[i])
	}
}

// drawLine draws the line l along row y: a day's rule, as the List's ended
// today is drawn, naming the day as prose.DayGone does; or the session's
// row, as the List draws one, lit where it's the one lit.
func (s sessionsByDay) drawLine(c *canvas, y int, l sessionLine) {
	if l.ruled {
		endedRule(c, y, s.width, line{{"ended " + prose.DayGone(s.words.now, l.session.Ended), dimInk}})
		return
	}
	s.words.sessionRow(c, y, s.columns, l.session.ListedSession, s.width, l.session.Session == s.lit)
}

// lineOf is the index of the line of the session with the given id: -1
// where none is.
func (s sessionsByDay) lineOf(id string) int {
	for i, l := range s.lines {
		if !l.ruled && l.session.Session == id {
			return i
		}
	}
	return -1
}

// targets are the sessions' rows paged shows, scrolled as given, from row
// y, a click on which selects the session, and opens the one selected, each
// across the width: none of a row a kept rule takes.
func (s sessionsByDay) targets(y int, paged rowsPaged, scroll int) []Target {
	var targets []Target
	from, to := paged.shown(scroll)
	for i := paged.top(from, to); i < to; i++ {
		if l := s.lines[i]; !l.ruled {
			targets = append(targets, Target{Y: y + i - from, Width: s.width, On: OnSession, Session: l.session.Session})
		}
	}
	return targets
}

// outOfView are how many sessions are out of view of a page showing the
// lines from top up to to, as paged's top has it, a rule kept on its first
// row: those above, the one the rule takes the place of among them, and
// those below.
func (s sessionsByDay) outOfView(top, to int) (above, below int) {
	return sessionsAmong(s.lines[:top]), sessionsAmong(s.lines[to:])
}

// sessionsAmong counts the sessions' rows among the lines given.
func sessionsAmong(lines []sessionLine) int {
	n := 0
	for _, l := range lines {
		if !l.ruled {
			n++
		}
	}
	return n
}
