package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The Log's tables' words with nothing to show today, where their rows
// would be.
var (
	noRequestsToday = line{{"no requests today", faintInk}}
	noEventsToday   = line{{"no events today", faintInk}}
)

// logCounted counts n of the thing a noun names, as views.CountedWords does,
// the count in the ink given and the words after it in text.muted: "1,240
// lines".
func logCounted(n int, noun string, figure ink) line {
	said := views.CountedWords(n, noun)
	l := make(line, len(said))
	for i, p := range said {
		l[i] = span{p.Text, mutedInk}
		if p.Kind == status.PartFigure {
			l[i].ink = figure
		}
	}
	return l
}

// muted is the line all in text.muted, as a phone's heading counts.
func muted(l line) line {
	m := slices.Clone(l)
	for i := range m {
		m[i].ink = mutedInk
	}
	return m
}

// logHeadingCount is what the Log's heading counts at its right, before its
// f: the count, and why, what alone hides the lines it counts, which the
// heading drops first.
type logHeadingCount struct {
	count, why line
}

// hiddenCount is Requests' heading's count of today's lines hidden, "35
// hidden", its figure in text.secondary; and why, where one thing alone
// hides them, by, as "checks and counts": ": checks and counts", in
// text.muted. by is "" where several things hide them, as with a filter on.
// With none hidden, there's nothing to count, and the heading has f alone.
func hiddenCount(hidden int, by string) logHeadingCount {
	if hidden == 0 {
		return logHeadingCount{}
	}
	c := logHeadingCount{count: line{{prose.Grouped(hidden), secondaryInk}, {" hidden", mutedInk}}}
	if by != "" {
		c.why = line{{": " + by, mutedInk}}
	}
	return c
}

// todayCount is Events' heading's count: today's events, "20 today", or,
// where some are hidden, "18 of 20 today", each figure in text.secondary
// and the words in text.muted.
func todayCount(shown, all int) logHeadingCount {
	count := line{{prose.Grouped(shown), secondaryInk}}
	if shown != all {
		count = append(count, span{" of ", mutedInk}, span{prose.Grouped(all), secondaryInk})
	}
	return logHeadingCount{count: append(count, span{" today", mutedInk})}
}

// phoneCountGap is the cells between a phone's heading's count and its f.
const phoneCountGap = 2

// The heading's f, and what it does, which the heading drops after why.
var (
	fKey     = span{"f", keyInk}
	fShowsIt = span{" what it shows", mutedInk}
)

// forms are the heading's right, the count and f, in the order the heading
// drops their parts as it narrows: "35 hidden: checks and counts   f what it
// shows"; then without why, "35 hidden   f what it shows"; then without
// what f does, "35 hidden   f". Each part is three cells after the one
// before.
func (c logHeadingCount) forms() []line {
	apart := func(parts ...line) line {
		var l line
		for _, p := range parts {
			if l != nil && p != nil {
				l = append(l, spaces(headingGap))
			}
			l = append(l, p...)
		}
		return l
	}
	whole := apart(slices.Concat(c.count, c.why), line{fKey, fShowsIt})
	forms := []line{whole, apart(c.count, line{fKey, fShowsIt}), apart(c.count, line{fKey})}
	return slices.CompactFunc(forms, func(a, b line) bool { return slices.Equal(a, b) })
}

// phone is the heading's right on a phone: the count, all in text.muted, as
// "35 hidden", and two cells on, f.
func (c logHeadingCount) phone() line {
	if c.count == nil {
		return line{fKey}
	}
	return slices.Concat(muted(c.count), line{spaces(phoneCountGap), fKey})
}

// logHeadingRight is the first of the wide heading's right's forms, in
// forms' order, that leaves the filter whole in room cells, the cells from
// the end of [ ] to the last column. Before the right, with a filter, are
// headingGap cells, the filter, a cell, the rule, as short as none, and
// ruleGap cells; without one, the rule with ruleGap cells either side.
// Where none does, it's the last, and the filter is to be cut.
func logHeadingRight(forms []line, filter line, room int) line {
	before := 2 * ruleGap
	if filter != nil {
		before = headingGap + filter.width() + 1 + ruleGap
	}
	for _, form := range forms {
		if before+form.width() <= room {
			return form
		}
	}
	return forms[len(forms)-1]
}

// logFilterNames are what the Log's filter keeps, each named as the table
// names it: sessions, as "d28c switchboard"; accounts; models; and whether
// it keeps those that failed alone.
type logFilterNames struct {
	sessions, accounts, models []string
	failed                     bool
}

// said is what the heading says the filter keeps, in text.faint: "only",
// then each it keeps, sessions, accounts, models, then failed, " · "
// between, as "only d28c switchboard · side · failed"; nothing where it
// keeps every line.
func (f logFilterNames) said() line {
	kept := slices.Concat(f.sessions, f.accounts, f.models)
	if f.failed {
		kept = append(kept, "failed")
	}
	if len(kept) == 0 {
		return nil
	}
	return line{{"only " + strings.Join(kept, " · "), faintInk}}
}

// logStanding is how the Log's table stands, as its state line says.
type logStanding int

const (
	// logFollowing is the table following, new lines joining at its foot.
	logFollowing logStanding = iota
	// logPaused is the table paused at the line chosen.
	logPaused
	// logNotLive is the table without the router, its lines the ledger's.
	logNotLive
)

// logState is what the Log's state line says: how the table stands; at,
// paused, the time of the line chosen, and not live, the latest end of the
// lines read, zero where none was read; fresh, paused, the lines come since;
// and at its right, how many of today's lines, or events, as noun names one,
// "line" or "event", the table shows, those of the session it's filtered to
// alone, named by its id as the table names it, where of names one.
type logState struct {
	standing logStanding
	at       time.Time
	fresh    int
	shown    int
	noun     string
	of       string
}

// line is the state line across width cells, as told at now: what it says
// of how the table stands at its left, and its count ending at the last
// column, at least headingGap cells apart. Where they don't fit, its last
// clause goes, "end to follow" paused, the one after ";" not live; then
// the count; then what's left is cut.
func (s logState) line(width int, now time.Time) line {
	whole, kept := s.said(now)
	count := s.count()
	for _, left := range []line{whole, kept} {
		if left.width()+headingGap+count.width() <= width {
			return spread(left, count, width)
		}
	}
	return kept.fit(width)
}

// said is what the state line says of how the table stands, whole, and with
// its last clause, which goes first as it narrows, left off:
//   - following: "● following";
//   - paused: "‖ paused at 14:31:13", "   ▼ 214 new" where lines have come
//     since, and "   end to follow", the clause that goes;
//   - not live: "○ not live", "   the ledger's lines to 14:40:52" where
//     lines were read, and "; those since come in as the router answers
//     again", the clause that goes.
//
// A time is to the second, on a day other than now's named before it as the
// day rule names a day: "Wed 30 Sep 23:47:30".
func (s logState) said(now time.Time) (whole, kept line) {
	switch s.standing {
	case logPaused:
		kept = line{{"‖", alertInk}, {" paused", secondaryInk}, {" at " + loggedAt(now, s.at), mutedInk}}
		if s.fresh > 0 {
			kept = append(kept, spaces(headingGap), span{"▼ ", keyMarkInk}, span{prose.Grouped(s.fresh) + " new", keyInk})
		}
		return slices.Concat(kept, line{spaces(headingGap), {"end", keyInk}, {" to follow", mutedInk}}), kept
	case logNotLive:
		kept = line{{"○", dimInk}, {" not live", secondaryInk}}
		if s.at.IsZero() {
			return kept, kept
		}
		kept = append(kept, spaces(headingGap), span{"the ledger's lines to " + loggedAt(now, s.at), mutedInk})
		return slices.Concat(kept, line{{"; those since come in as the router answers again", mutedInk}}), kept
	}
	kept = line{{"●", positiveInk}, {" following", secondaryInk}}
	return kept, kept
}

// count is the state line's count: "1,240 lines today", or, of a session
// alone, "445 of d28c's today", the figure in text.secondary.
func (s logState) count() line {
	if s.of != "" {
		return line{{prose.Grouped(s.shown), secondaryInk}, {" of " + s.of + "'s today", mutedInk}}
	}
	return append(logCounted(s.shown, s.noun, secondaryInk), span{" today", mutedInk})
}

// loggedAt is t as the Log's state line tells it, at now: to the second, as
// "14:31:13", on now's day; on another, after the day it's of, as the day
// rule names a day, "Wed 30 Sep 23:47:30".
func loggedAt(now, t time.Time) string {
	if prose.SameDay(now, t, now) {
		return prose.ToTheSecond(now, t)
	}
	return prose.Day(t.In(now.Location())) + " " + prose.ToTheSecond(now, t)
}

// The day rule's spacing: the cells after the day that ended, before its
// summary; around the rule; and the fewest cells of rule kept.
const (
	dayRuleGap   = 3
	dayRuleApart = 2
	dayRuleLeast = 3
)

// logDayRule is the day rule r across cells cells, from column 2 to the
// last: the day that ended, bold text.primary; three cells on, its summary,
// its parts " · " apart; two cells on, a rule, and two cells on, ending at
// the last, the day that began. Narrow, the summary's parts go from its end,
// each where it would leave fewer than dayRuleLeast cells of rule; then the
// two days stand alone, their rule as long as the room leaves; and where
// even they don't fit, they're cut.
func logDayRule(r views.DayRule, cells int) line {
	ended := line{{views.DateNamed(r.Ended.Date), titleInk}}
	began := line{{views.DateNamed(r.Began.Date), titleInk}}
	ruled := func(left line) line {
		n := cells - left.width() - 2*dayRuleApart - began.width()
		return slices.Concat(left, line{spaces(dayRuleApart), {rule(n), borderInk}, spaces(dayRuleApart)}, began)
	}
	parts := daySummary(r.Totals)
	for n := len(parts); n > 0; n-- {
		left := slices.Concat(ended, line{spaces(dayRuleGap)}, joinedSummary(parts[:n]))
		if left.width()+2*dayRuleApart+dayRuleLeast+began.width() <= cells {
			return ruled(left)
		}
	}
	return ruled(ended).fit(cells)
}

// daySummary are the parts of a day rule's summary of the day's totals, in
// order: its requests and sessions, each its figure in text.secondary; the
// limits its accounts reached, bold state.destructive; and its worth at API
// prices, bold text.secondary, as History writes it. A part that would say
// none is left out.
func daySummary(t views.DayTotals) []line {
	var parts []line
	if t.Requests > 0 {
		parts = append(parts, logCounted(t.Requests, "request", secondaryInk))
	}
	if t.Sessions > 0 {
		parts = append(parts, logCounted(t.Sessions, "session", secondaryInk))
	}
	if t.Limits > 0 {
		parts = append(parts, append(logCounted(t.Limits, "limit", exhaustedInk), span{" reached", mutedInk}))
	}
	switch said := t.Dollars(); {
	case said == "":
	case t.Amount == nil:
		parts = append(parts, worthSaid(t.Worth, strongInk))
	default:
		parts = append(parts, append(worthSaid(t.Worth, strongInk), span{" at API prices", mutedInk}))
	}
	return parts
}

// joinedSummary are a day rule's summary's parts, " · " between them.
func joinedSummary(parts []line) line {
	var l line
	for i, p := range parts {
		if i > 0 {
			l = append(l, span{" · ", mutedInk})
		}
		l = append(l, p...)
	}
	return l
}

// oldestDay is the line at the top of the oldest day the ledger keeps, cut
// to cells cells, in text.faint: how long it keeps its lines, "the oldest
// day kept: the ledger keeps its lines 400 days", as [ledger] keep says;
// or, where it keeps them for good, dayfile.Forever, that it's the first
// day, "the ledger's first day: it keeps its lines for good".
func oldestDay(keep time.Duration, cells int) line {
	said := "the ledger's first day: it keeps its lines for good"
	if keep != dayfile.Forever {
		said = "the oldest day kept: the ledger keeps its lines " + prose.Counted(int(keep/(24*time.Hour)), "day")
	}
	return line{{said, faintInk}}.fit(cells)
}
