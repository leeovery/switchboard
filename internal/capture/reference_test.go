package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// referenceDir is where milestone 7's signed-off frames are kept, each by its
// stem, as the text and ANSI of the generator that drew it and a PNG.
const referenceDir = "../../testdata/vhs/reference"

// owed are the frames no fixture draws yet. Each stage that builds a view
// adds its fixtures, named after the frames they draw, and takes their stems
// from here; by stage 14 none is owed.
var owed = []string{
	"accounts-compare-160", "accounts-compare-bad-hour-160", "accounts-compare-month-160", "accounts-compare-ten-160",
	"accounts-detail-all-160", "accounts-detail-limit-160", "accounts-detail-one-160", "accounts-detail-sessions-160",
	"accounts-detail-work-160", "accounts-phone-compare-52", "accounts-phone-detail-52",
	"history-days-160", "history-days-ten-160", "history-phone-year-52", "history-tokens-160", "history-tokens-models-160",
	"history-weeks-160", "history-weeks-columns-160", "history-weeks-ten-160", "history-year-160", "history-year-opus-160",
	"log-event-details-160", "log-events-160", "log-events-filter-160", "log-not-live-160", "log-request-details-160",
	"log-requests-160", "log-requests-filter-160", "log-requests-filtered-160", "log-requests-phone-52",
	"routing-session-160",
	"runway-5-hour-160", "runway-phone-52", "runway-stepped-back-160", "runway-ten-accounts-160", "runway-week-160",
	"session-ended-160", "session-moved-160", "session-phone-52", "session-quiet-160", "session-resumed-160",
	"session-turn-opened-160",
	"sessions-list-160", "sessions-list-phone-52", "sessions-moved-160", "sessions-one-account-160", "sessions-phone-52",
	"sessions-pinned-160", "sessions-refused-160", "sessions-switchboard-160", "sessions-ten-accounts-160",
}

// passing is cells a frame drew otherwise than the design says, or than its
// world gives, which the test passes over: their span, and what the frame's
// .txt reads there, which a test checks.
type passing struct {
	span
	// reads is what the frame reads over the span, from its first glyph that
	// isn't blank; empty, it reads blank there.
	reads string
	// glyphs, where they're given, are what the frame draws over the span
	// instead of words, in place of reads: those glyphs alone, and blanks,
	// as a trace's.
	glyphs string
}

// holds reports whether text, what the frame reads over the passing's span,
// is what the passing says it reads.
func (p passing) holds(text string) bool {
	got := strings.TrimSpace(text)
	switch {
	case p.glyphs != "":
		return got != "" && !strings.ContainsFunc(got, func(r rune) bool { return r != ' ' && !strings.ContainsRune(p.glyphs, r) })
	case p.reads == "":
		return got == ""
	}
	return strings.HasPrefix(got, p.reads)
}

// want says what the passing says the frame reads, as a failure gives it.
func (p passing) want() string {
	switch {
	case p.glyphs != "":
		return fmt.Sprintf("only %q, and blanks", p.glyphs)
	case p.reads == "":
		return "blank"
	}
	return fmt.Sprintf("to start %q", p.reads)
}

// wholeRow passes over row n, which reads as given.
func wholeRow(n int, reads string) passing {
	return passing{row: n, reads: reads}
}

// cellsOf passes over row n's cells from and to, which read as given.
func cellsOf(n, from, to int, reads string) passing {
	return passing{row: n, from: from, to: to, reads: reads}
}

// blankCells passes over row n's cells from and to, where the frame draws
// nothing.
func blankCells(n, from, to int) passing {
	return cellsOf(n, from, to, "")
}

// eighths are the glyphs a trace, and a chart's fill, draw a cell's height
// in: from an eighth of a cell to a whole one.
const eighths = "▁▂▃▄▅▆▇█"

// traced passes over row n's cells from and to, where the frame draws a
// trace.
func traced(n, from, to int) passing {
	return passing{row: n, from: from, to: to, glyphs: eighths}
}

// charted passes over the cells from and to of rows top to bottom, where the
// frame draws a chart: its fill, and its lines, the cap's ╌, the even pace's
// · and now's │.
func charted(top, bottom, from, to int) []passing {
	var p []passing
	for n := top; n <= bottom; n++ {
		p = append(p, passing{row: n, from: from, to: to, glyphs: eighths + "╌·│"})
	}
	return p
}

// How the rows passed over whole start, in the frames that drew them before
// the keys were swept.
const (
	// overviewKeys starts an Overview's key line.
	overviewKeys = "tab views     u usage"
	// accountsKeys starts the key line of Accounts wider than a phone.
	accountsKeys = "tab views   [ ] view     p period"
	// historyKeys starts the key line of History wider than a phone.
	historyKeys = "tab views   [ ] view   p period"
	// pinnedRouting starts ROUTING's heading under a pin, which ends with
	// its key back to auto, 0.
	pinnedRouting = "ROUTING   pinned to"
)

// period passes over the period on a heading row 160 wide, its key, p, at
// column p: the period and its key, with the rule's last two cells, as the
// period between its keys, < and >, is two cells wider.
func period(p int, reads string) passing {
	return cellsOf(5, p-4, 160, "──  p "+reads)
}

// phonePeriod passes over a phone's period, the row under its heading.
func phonePeriod(reads string) passing {
	return wholeRow(7, "p "+reads)
}

// How usage's frames word side's week, at its allowance, where it runs out
// first, at its average pace: in full, and short.
const (
	sidesWeek      = "6%/day till Mon 10:00"
	sidesWeekShort = "6%/day · Mon"
)

// evenPace passes over row n's two cells from column c, one of them the
// frame's even pace on a window, which it places as if the pace were a whole
// percent, or otherwise than the design's rule places it for the reset the
// frame gives, and the other where that rule places it.
func evenPace(n, c int) passing {
	return cellsOf(n, c, c+1, "┃")
}

// outOfPool passes over the account named on row n from column c, out of the
// pool, which the frame draws in its colours, where the design has it faint
// wherever the page names it.
func outOfPool(n, c int, name string) passing {
	return cellsOf(n, c, c+utf8.RuneCountInString(name)-1, name)
}

// pastNow passes over column c of rows top to bottom, which the frame's chart
// fills a column past now's, as if the even pace were a whole percent: whole
// cells of fill, their glyphs blank.
func pastNow(top, bottom, c int) []passing {
	var p []passing
	for n := top; n <= bottom; n++ {
		p = append(p, blankCells(n, c, c))
	}
	return p
}

// rate passes over the rate on row n of a frame w columns wide, right-aligned
// in its last eight columns, which reads as given.
func rate(n, w int, reads string) passing {
	return cellsOf(n, w-7, w, reads)
}

// oneHigh passes over the traces a row high in a frame w columns wide,
// ROUTING's at size 1 or SESSIONS', from column 93 to the rate, of the rows
// from first, and their rates, given in order, a row each.
func oneHigh(first, w int, rates ...string) []passing {
	var p []passing
	for i, r := range rates {
		p = append(p, traced(first+i, 93, w-8), rate(first+i, w, r))
	}
	return p
}

// twoHigh passes over ROUTING's traces at size 2 in a frame w columns wide,
// from column from to the rate, of the auto world's three accounts from row
// first, and their rates, given in that order: work's and side's two rows
// each, and personal's lower row, the frames leaving its upper row blank,
// as its trace never rises a row high; and each one's rate, on its lower row.
func twoHigh(first, w, from int, work, personal, side string) []passing {
	upper := func(n int) passing { return traced(n, from, w-8) }
	lower := func(n int, reads string) []passing { return []passing{traced(n, from, w-8), rate(n, w, reads)} }
	return slices.Concat(
		[]passing{upper(first)}, lower(first+1, work),
		[]passing{blankCells(first+3, from, w-8)}, lower(first+4, personal),
		[]passing{upper(first + 6)}, lower(first+7, side),
	)
}

// besideBlocks passes over the traces beside USAGE's blocks, by account, on a
// frame 160 columns wide, from column 108 to the rate, a block every seven
// rows from row first, and their rates, given in order.
func besideBlocks(first int, rates ...string) []passing {
	var p []passing
	for i, r := range rates {
		p = append(p, traced(first+7*i, 108, 152), rate(first+7*i, 160, r))
	}
	return p
}

// onAPhone passes over ROUTING's traces on a phone, of the accounts from row
// first, each on the lower of its two rows, a blank row between accounts,
// from column 17 to 32, and their rates, given in order, at its right.
func onAPhone(first int, rates ...string) []passing {
	var p []passing
	for i, r := range rates {
		p = append(p, traced(first+3*i, 17, 32), cellsOf(first+3*i, 46, 52, r))
	}
	return p
}

// comingLines passes over COMING UP's lines, from column 1 to to, of the rows
// from first, which read as given, a row each, "" where the frame's is blank.
func comingLines(first, to int, reads ...string) []passing {
	var p []passing
	for i, r := range reads {
		p = append(p, cellsOf(first+i, 1, to, r))
	}
	return p
}

// latelyLines passes over LATELY's lines in a frame 216 columns wide, from
// column 111, of the rows from first, which read as given, a row each.
func latelyLines(first int, reads ...string) []passing {
	var p []passing
	for i, r := range reads {
		p = append(p, cellsOf(first+i, 111, 216, r))
	}
	return p
}

// figures passes over an account's two figures on row n of a routing frame,
// its 5-hour window's and its week's, which read as given: the frames colour
// them along a ramp, where the design colours them by level.
func figures(n int, session, week string) []passing {
	return []passing{cellsOf(n, 76, 78, session), cellsOf(n, 87, 89, week)}
}

// The routing frames' figures of work, personal and side, as the auto world
// has them, rows 15, 16 and 17.
var routingFigures = slices.Concat(figures(15, "95%", "34%"), figures(16, "39%", "66%"), figures(17, "13%", "78%"))

// cursorRow passes over row n of a routing frame, the card's row the cursor
// is on, which reads as given, from a cell before the card's content to a
// cell after it: the frames light no row, where the design lights the
// cursor's on bg.selection.
func cursorRow(n int, reads string) passing {
	return cellsOf(n, 42, 119, reads)
}

// behindTheCard passes over the page the routing frames draw behind the
// card's box, of three accounts: work's week's and Fable week's even paces,
// on row 9, above it; and ROUTING's traces beside and under it, and their
// rates, as given.
func behindTheCard(work, personal, side string) []passing {
	return slices.Concat(
		[]passing{evenPace(9, 73), evenPace(9, 121), traced(28, 93, 121)},
		twoHigh(21, 160, 122, work, personal, side),
	)
}

// The Overview's USAGE of the auto world's three accounts: in the compact
// matrix at 160 columns, as usage-160 has them, its row 5 the matrix's row 9;
// and in the big bars at 216, work's 5-hour window's, week's and Fable
// week's even paces, and side's week's words.
var (
	matrix160 = []passing{evenPace(9, 73), evenPace(9, 121), cellsOf(13, 88, 108, sidesWeek)}
	bigBars   = []passing{evenPace(9, 103), evenPace(20, 107), evenPace(31, 107), cellsOf(24, 194, 214, sidesWeek)}
)

// The Overview's COMING UP and LATELY at 216 columns, from their fourth line
// and their eighth, by the auto world: COMING UP's lines after side's week's
// run-out, which its world adds, and LATELY's after d28c's start, which its
// world adds.
var (
	comingUp216 = []string{"~Sat 06:00", "Sun 02:00", "Sun 21:00", "Mon 10:00", ""}
	lately216   = []string{"11:55", "11:30", "10:02", "09:40", "09:12"}
)

// extrasPaces passes over the even paces of the six and ten accounts'
// extras, client, spare and lab, in the compact matrix at 160 columns, its
// bars from column c, 17 or, of ten, 18, which the frames place otherwise
// than the design's rule for the resets they give: client's 5-hour window's,
// week's and Fable week's; spare's 5-hour window's, drawn where the frame has
// its bar; and lab's week's and Fable week's.
func extrasPaces(c int) []passing {
	return []passing{
		evenPace(15, c+6), evenPace(15, c+56), evenPace(15, c+104),
		cellsOf(17, c, c+1, "▋┃"),
		evenPace(19, c+55), evenPace(19, c+103),
	}
}

// extrasFigures passes over the figures of the six and ten accounts'
// extras, client, spare and lab, in the compact matrix at 160 columns, its
// bars from column c, 17 or, of ten, 18, which the frames typed and their
// world gives otherwise: client's 5-hour window's words and its weeks'
// allowances; spare's 5-hour window's rate, and its weeks' even paces and
// words, as its world has them reset today at 18:00; and lab's 5-hour
// window's even pace, its window starting later in its world, its week's
// words and its Fable week's allowance.
func extrasFigures(c int) []passing {
	return []passing{
		cellsOf(15, c+23, c+38, "16%/h till 17:40"), cellsOf(15, c+71, c+77, "14%/day"), cellsOf(15, c+119, c+125, "25%/day"),
		cellsOf(17, c+23, c+27, "19%/h"), cellsOf(17, c+53, c+63, "┃"), cellsOf(17, c+71, c+92, "20%/day till Thu 18:00"),
		cellsOf(17, c+101, c+111, "┃"), cellsOf(17, c+119, c+140, "20%/day till Thu 18:00"),
		cellsOf(19, c+9, c+10, "┃"), cellsOf(19, c+71, c+92, "11%/day till Mon 08:00"), cellsOf(19, c+119, c+125, "20%/day"),
	}
}

// passedOver is, by stem, the rows and cells each frame drew otherwise than
// the design says, or than its world gives, which testdata/vhs/README.md
// lists, with what the dashboard draws in their place.
var passedOver = map[string][]passing{
	"usage-160": {evenPace(5, 73), evenPace(5, 121), cellsOf(9, 88, 108, sidesWeek)},
	"usage-120": {evenPace(16, 56), cellsOf(20, 98, 118, sidesWeek), evenPace(27, 56)},
	"usage-80":  {cellsOf(22, 58, 78, sidesWeek)},
	"usage-52":  {cellsOf(22, 38, 49, sidesWeekShort)},
	"usage-by-account-160": {
		evenPace(6, 82), evenPace(8, 82), evenPace(13, 94), evenPace(15, 94),
		cellsOf(20, 138, 158, sidesWeek), evenPace(20, 74), evenPace(22, 74),
		wholeRow(31, "┃ even pace"),
	},
	"usage-by-account-52":   {cellsOf(25, 38, 49, sidesWeekShort)},
	"usage-one-account-160": {cellsOf(5, 8, 10, "▲")},

	"overview-default-160": slices.Concat(matrix160, twoHigh(21, 160, 93, "0/min", "2/min", "15/min"), []passing{wholeRow(37, overviewKeys)}),
	"overview-pinned-160": slices.Concat(
		matrix160, []passing{wholeRow(19, pinnedRouting)}, twoHigh(21, 160, 93, "0/min", "2/min", "15/min"),
		[]passing{outOfPool(34, 8, "work"), outOfPool(34, 92, "work"), outOfPool(34, 134, "side"), outOfPool(35, 8, "side")},
		[]passing{wholeRow(37, overviewKeys)},
	),
	"overview-bad-hour-160": slices.Concat(
		matrix160, []passing{cellsOf(11, 27, 29, "┃"), cellsOf(15, 23, 24, "┃"), wholeRow(19, pinnedRouting)},
		twoHigh(21, 160, 93, "0/min", "0/min", "15/min"),
		comingLines(33, 72, "15:54", "18:50", "~Sat 06:00"),
		[]passing{cellsOf(33, 112, 137, ": personal is at its limit"), outOfPool(34, 92, "work"), wholeRow(37, overviewKeys)},
	),
	"overview-by-account-160": slices.Concat(
		[]passing{cellsOf(7, 133, 160, "· new sessions go elsewhere"), evenPace(9, 52), evenPace(11, 52), cellsOf(23, 82, 102, sidesWeek)},
		besideBlocks(11, "0/min", "2/min", "15/min", "16/min"), []passing{wholeRow(42, overviewKeys)},
	),
	"overview-usage-alone-160": slices.Concat(matrix160, []passing{wholeRow(37, overviewKeys)}),
	"overview-everything-160x39": slices.Concat(
		matrix160, charted(22, 26, 17, 37), charted(25, 26, 68, 87), charted(26, 26, 118, 136),
		oneHigh(31, 160, "0/min", "2/min", "15/min"), []passing{cellsOf(37, 117, 120, "side"), wholeRow(39, overviewKeys)},
	),
	"overview-six-accounts-160": slices.Concat(
		matrix160, extrasPaces(17), extrasFigures(17), []passing{cellsOf(21, 73, 75, "┃"), cellsOf(21, 121, 123, "┃")},
		oneHigh(27, 160, "0/min", "2/min", "15/min", "2/min", "0/min", "4/min"), []passing{cellsOf(30, 17, 45, "● open")},
		comingLines(37, 72, "16:30", "17:10", "18:50"), []passing{wholeRow(41, overviewKeys)},
	),
	"overview-ten-accounts-160": slices.Concat(
		[]passing{evenPace(9, 74), evenPace(9, 122), cellsOf(13, 89, 109, sidesWeek)}, extrasPaces(18), extrasFigures(18),
		[]passing{
			cellsOf(21, 29, 31, "┃"), evenPace(21, 76), evenPace(21, 124),
			cellsOf(27, 27, 29, "┃"), cellsOf(27, 72, 76, "┃"), cellsOf(27, 120, 124, "┃"),
		},
		[]passing{
			cellsOf(21, 89, 109, "4%/day till Sat 22:00"), cellsOf(21, 137, 143, "17%/day"),
			cellsOf(23, 41, 45, "26%/h"), cellsOf(23, 89, 95, "17%/day"), cellsOf(23, 137, 143, "19%/day"),
			cellsOf(25, 41, 45, "23%/h"), cellsOf(25, 89, 95, "15%/day"), cellsOf(25, 137, 143, "21%/day"),
			cellsOf(27, 41, 45, "30%/h"), cellsOf(27, 89, 95, "13%/day"), cellsOf(27, 137, 143, "22%/day"),
			cellsOf(29, 73, 75, "┃"), cellsOf(29, 121, 123, "┃"),
		},
		oneHigh(35, 160, "0/min", "2/min", "15/min", "2/min", "0/min", "4/min", "0/min", "2/min", "0/min", "3/min"),
		[]passing{cellsOf(38, 18, 46, "● open")}, comingLines(49, 72, "16:30", "17:10", "18:50"), []passing{wholeRow(53, overviewKeys)},
	),
	"overview-default-216": slices.Concat(
		bigBars, twoHigh(43, 216, 93, "0/min", "2/min", "15/min"), comingLines(58, 97, comingUp216...), latelyLines(62, lately216...),
		[]passing{wholeRow(70, overviewKeys)},
	),
	"overview-sessions-216": slices.Concat(
		bigBars, twoHigh(43, 216, 93, "0/min", "2/min", "15/min"), oneHigh(55, 216, "1/min", "0/min", "9/min", "5/min", "0/min", "0/min"),
		comingLines(67, 97, comingUp216[:2]...), []passing{wholeRow(70, overviewKeys)},
	),
	"overview-sessions-graphs-216": slices.Concat(
		[]passing{cellsOf(13, 126, 146, sidesWeek)},
		charted(23, 40, 17, 47), pastNow(23, 40, 50), []passing{cellsOf(41, 48, 51, "now")}, charted(34, 40, 87, 117), charted(38, 40, 158, 184),
		twoHigh(45, 216, 93, "0/min", "2/min", "15/min"), oneHigh(57, 216, "1/min", "0/min", "9/min", "5/min", "0/min", "0/min"),
		[]passing{wholeRow(70, overviewKeys)},
	),
	"overview-everything-216x106": slices.Concat(
		bigBars,
		charted(45, 62, 17, 47), pastNow(45, 62, 50), []passing{cellsOf(63, 48, 51, "now")}, charted(56, 62, 87, 117), charted(60, 62, 158, 184),
		twoHigh(67, 216, 93, "0/min", "2/min", "15/min"), oneHigh(79, 216, "1/min", "0/min", "9/min", "5/min", "0/min", "0/min"),
		comingLines(91, 97, comingUp216...), latelyLines(95, lately216...), []passing{wholeRow(106, overviewKeys)},
	),
	"overview-phone-52": slices.Concat([]passing{cellsOf(25, 38, 49, sidesWeekShort)}, onAPhone(46, "0/min", "2/min", "15/min")),

	"accounts-compare-160":          {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-compare-bad-hour-160": {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-compare-month-160":    {period(152, "30 days"), wholeRow(37, accountsKeys)},
	"accounts-compare-ten-160":      {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-all-160":       {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-limit-160":     {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-one-160":       {period(150, "this week"), cellsOf(28, 20, 20, "●"), wholeRow(37, accountsKeys)},
	"accounts-detail-sessions-160":  {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-work-160":      {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-phone-compare-52":     {phonePeriod("this week")},
	"accounts-phone-detail-52":      {phonePeriod("this week")},

	"history-days-160":          {period(148, "eight weeks"), wholeRow(37, historyKeys)},
	"history-days-ten-160":      {period(148, "eight weeks"), wholeRow(37, historyKeys)},
	"history-phone-year-52":     {phonePeriod("last 12 months")},
	"history-tokens-160":        {period(152, "by week"), wholeRow(37, historyKeys)},
	"history-tokens-models-160": {period(152, "by week"), wholeRow(37, historyKeys)},
	"history-weeks-160": {
		period(148, "eight weeks"),
		cellsOf(15, 137, 137, "6"), cellsOf(16, 138, 138, "5"), cellsOf(17, 139, 139, "3"),
		wholeRow(37, historyKeys),
	},
	"history-weeks-columns-160": {
		period(148, "eight weeks"),
		cellsOf(15, 114, 114, "6"), cellsOf(21, 114, 114, "5"), cellsOf(27, 114, 114, "3"),
		wholeRow(37, historyKeys),
	},
	"history-weeks-ten-160": {
		period(148, "eight weeks"),
		cellsOf(8, 18, 18, "4"), cellsOf(8, 96, 96, "4"), cellsOf(8, 150, 150, "6"),
		cellsOf(9, 36, 42, "5% used"),
		cellsOf(13, 92, 92, "4"),
		cellsOf(19, 93, 99, "5% used"),
		cellsOf(22, 96, 114, "80% left at a reset"),
		wholeRow(37, historyKeys),
	},
	"history-year-160":      {period(145, "last 12 months"), wholeRow(37, historyKeys)},
	"history-year-opus-160": {period(145, "last 12 months"), wholeRow(37, historyKeys)},

	"routing-auto-160": slices.Concat(routingFigures, []passing{cursorRow(14, "■ 0 auto")}, behindTheCard("0/min", "2/min", "15/min")),
	"routing-two-160":  slices.Concat(routingFigures, []passing{cursorRow(17, "■ 3 side")}, behindTheCard("0/min", "2/min", "15/min")),
	"routing-move-160": slices.Concat(
		routingFigures, []passing{cursorRow(16, "■ 2 personal"), cellsOf(23, 76, 79, "6.20")}, behindTheCard("0/min", "2/min", "15/min"),
	),
	"routing-limit-160": slices.Concat(
		[]passing{cellsOf(11, 27, 29, "┃"), cellsOf(15, 24, 25, "┃"), cursorRow(17, "■ 3 side")},
		figures(15, "95%", "34%"), []passing{cellsOf(16, 87, 89, "64%")}, figures(17, "13%", "78%"),
		behindTheCard("0/min", "0/min", "15/min"), comingLines(33, 72, "16:30"),
	),

	"session-phone-52":         {wholeRow(32, "esc back   ↑↓ turn   enter open   ? keys")},
	"session-turn-opened-160":  {wholeRow(37, "esc close    ↑↓ request  p pin")},
	"sessions-one-account-160": {wholeRow(37, "tab views   [ ] view    ↑↓ session  enter its page    p pin")},
	"runway-phone-52":          {wholeRow(33, "< > window   ←→ day   ? keys")},
}

func TestReferenceKeepsEachFrameAsTextANSIAndPNG(t *testing.T) {
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		stem := strings.TrimSuffix(e.Name(), ext)
		kinds[stem] = append(kinds[stem], ext)
	}
	for stem, exts := range kinds {
		slices.Sort(exts)
		if want := []string{".ansi", ".png", ".txt"}; !slices.Equal(exts, want) {
			t.Errorf("reference/ keeps %s as %v, want %v", stem, exts, want)
		}
	}
}

func TestEveryFrameIsDrawnByAFixtureOrOwed(t *testing.T) {
	stems, names := stems(t), Names()
	for _, stem := range stems {
		drawn, isOwed := slices.Contains(names, stem), slices.Contains(owed, stem)
		switch {
		case drawn && isOwed:
			t.Errorf("%s is drawn by a fixture, and still owed one: take it from owed", stem)
		case !drawn && !isOwed:
			t.Errorf("no fixture draws %s: name one after it, or owe it", stem)
		}
	}
	for _, stem := range owed {
		if !slices.Contains(stems, stem) {
			t.Errorf("owed %s, which reference/ doesn't keep", stem)
		}
	}
	if compacted := slices.Compact(slices.Sorted(slices.Values(owed))); len(compacted) != len(owed) {
		t.Errorf("owed names a frame twice")
	}
}

func TestEveryFramesANSIParsesToItsText(t *testing.T) {
	for _, stem := range stems(t) {
		t.Run(stem, func(t *testing.T) {
			g, text := reference(t, stem)
			if got := g.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			width, height := sizeOf(t, stem)
			if height != 0 && len(g) != height {
				t.Errorf("it's %d rows, want %d", len(g), height)
			}
			for r, row := range g {
				if len(row) != width {
					t.Errorf("row %d is %d columns, want %d", r+1, len(row), width)
				}
				for c, cell := range row {
					if !cell.fg.painted || !cell.bg.painted {
						t.Errorf("row %d, column %d isn't painted: %s", r+1, c+1, cell)
					}
				}
			}
		})
	}
}

func TestEveryFixtureNamedAfterAFrameDrawsItCellForCell(t *testing.T) {
	utc := fixturesIn(time.UTC)
	canvas := rgb(terminalBackground.R, terminalBackground.G, terminalBackground.B)
	names := Names()
	for _, stem := range stems(t) {
		if !slices.Contains(names, stem) {
			continue
		}
		t.Run(stem, func(t *testing.T) {
			frame, _ := reference(t, stem)
			// A fixture over records is drawn in time.Local whatever the zone
			// asked, as fixtures builds it.
			drawn, err := parse(frameOf(t, utc, stem))
			if err != nil {
				t.Fatalf("parsing what %s draws: %v", stem, err)
			}
			diffs, err := compare(frame, drawn, canvas, spans(passedOver[stem]))
			if err != nil {
				t.Fatalf("%s: %v", stem, err)
			}
			if len(diffs) > 0 {
				t.Error(describe(stem, diffs))
			}
		})
	}
}

// partial is part of a frame a page draws before any fixture is named after
// it, held to the frame cell for cell: the page, over the frame's world with
// its keys pressed, and the area of the frame it draws as the frame does.
type partial struct {
	page page
	area area
}

// The areas the page is held to: the chrome, the title row and the status
// line, the design's rows 0 to 3, or 0 to 4 on a phone, whose tabs take a
// row of their own.
var (
	chrome      = area{top: 1, bottom: 4}
	phoneChrome = area{top: 1, bottom: 5}
)

// heldTabbed is the page over the auto world, tab pressed the times given,
// held over the area given.
func heldTabbed(tabs int, within area) partial {
	keys := slices.Repeat([]tea.KeyPressMsg{{Code: tea.KeyTab}}, tabs)
	return partial{page: page{world: accountWorlds.auto, keys: keys}, area: within}
}

// holds are, by stem, the frames the page is held to in part until their
// fixtures draw them whole: the chrome of a frame of each tab but the
// Overview's, and of Sessions on a phone.
var holds = map[string]partial{
	"accounts-compare-160":     heldTabbed(1, chrome),
	"history-year-160":         heldTabbed(2, chrome),
	"sessions-switchboard-160": heldTabbed(3, chrome),
	"runway-5-hour-160":        heldTabbed(4, chrome),
	"log-requests-160":         heldTabbed(5, chrome),
	"sessions-phone-52":        heldTabbed(3, phoneChrome),
}

func TestThePageDrawsWhatItHoldsOfAFrameCellForCell(t *testing.T) {
	worlds := worldsNow()
	canvas := rgb(terminalBackground.R, terminalBackground.G, terminalBackground.B)
	names := Names()
	for stem, h := range holds {
		t.Run(stem, func(t *testing.T) {
			if slices.Contains(names, stem) {
				t.Fatalf("a fixture draws %s whole: hold it so, and take it from holds", stem)
			}
			frame, _ := reference(t, stem)
			width, _ := sizeOf(t, stem)
			p := h.page
			p.name, p.size = stem, watch.Size{Width: width, Height: len(frame)}
			f := p.fixture(worlds)
			drawn, err := parse(f.Frame(f.Size))
			if err != nil {
				t.Fatalf("parsing what the page draws: %v", err)
			}
			diffs, err := compareWithin(frame, drawn, canvas, h.area, spans(passedOver[stem]))
			if err != nil {
				t.Fatalf("%s: %v", stem, err)
			}
			if len(diffs) > 0 {
				t.Error(describe(stem, diffs))
			}
		})
	}
}

func TestEachCellPassedOverIsInItsFrame(t *testing.T) {
	stems := stems(t)
	for stem, passings := range passedOver {
		if !slices.Contains(stems, stem) {
			t.Errorf("passes over cells of %s, which reference/ doesn't keep", stem)
			continue
		}
		g, _ := reference(t, stem)
		for _, p := range passings {
			if !within(p.span, g) {
				t.Errorf("%s: %v is out of its frame, %d rows by %d columns", stem, p.span, len(g), len(g[0]))
				continue
			}
			if got := textOf(g, p.span); !p.holds(got) {
				t.Errorf("%s: %v reads %q, want %s", stem, p.span, strings.TrimSpace(got), p.want())
			}
		}
	}
}

func TestAPassingHoldsTheFrameToWhatItSaysItReads(t *testing.T) {
	tests := []struct {
		name    string
		passing passing
		text    string
		want    bool
	}{
		{name: "the words it starts with", passing: cellsOf(1, 1, 9, "out ~"), text: "  out ~Fri", want: true},
		{name: "other words", passing: cellsOf(1, 1, 9, "out ~"), text: "  6%/day"},
		{name: "words where it says blank", passing: blankCells(1, 1, 9), text: "  ▁  "},
		{name: "blank where it says blank", passing: blankCells(1, 1, 9), text: "     ", want: true},
		{name: "a trace", passing: traced(1, 1, 9), text: " ▁▂ ▇█ ", want: true},
		{name: "words where it says a trace", passing: traced(1, 1, 9), text: "▁▂ 2/min"},
		{name: "blank where it says a trace", passing: traced(1, 1, 9), text: "      "},
		{name: "a chart's fill and lines", passing: charted(1, 1, 1, 9)[0], text: "··▂▅╌╌ │", want: true},
		{name: "a trace's glyphs and an axis's words", passing: charted(1, 1, 1, 9)[0], text: "▂▅ now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.passing.holds(tt.text); got != tt.want {
				t.Errorf("a passing that wants it %s holds %q: %v, want %v", tt.passing.want(), tt.text, got, tt.want)
			}
		})
	}
}

func TestTheReadmeListsEveryFramePassedOver(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(referenceDir, "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for stem := range passedOver {
		if !strings.Contains(string(readme), "`"+stem+"`") {
			t.Errorf("testdata/vhs/README.md doesn't list %s, whose cells the test passes over", stem)
		}
	}
}

// stems are the frames reference/ keeps, by the stems of their .txt files,
// sorted.
func stems(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(referenceDir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("reference/ keeps no frames")
	}
	var stems []string
	for _, p := range paths {
		stems = append(stems, strings.TrimSuffix(filepath.Base(p), ".txt"))
	}
	return stems
}

// reference is the frame kept by the stem given, as its ANSI's cells, and its
// .txt's lines.
func reference(t *testing.T, stem string) (grid, []string) {
	t.Helper()
	ansi, err := os.ReadFile(filepath.Join(referenceDir, stem+".ansi"))
	if err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(filepath.Join(referenceDir, stem+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := parse(string(ansi))
	if err != nil {
		t.Fatalf("parsing %s.ansi: %v", stem, err)
	}
	return g, strings.Split(strings.TrimSuffix(string(txt), "\n"), "\n")
}

// sizeOf is the size a frame's stem ends with: its width, as in -160, and its
// height where it's given too, as in -160x39, else 0.
func sizeOf(t *testing.T, stem string) (width, height int) {
	t.Helper()
	size := stem[strings.LastIndex(stem, "-")+1:]
	if _, err := fmt.Sscanf(size, "%dx%d", &width, &height); err == nil {
		return width, height
	}
	if _, err := fmt.Sscanf(size, "%d", &width); err != nil {
		t.Fatalf("%s ends with no size: %v", stem, err)
	}
	return width, 0
}

// spans are the spans the passings given pass over.
func spans(passings []passing) []span {
	var s []span
	for _, p := range passings {
		s = append(s, p.span)
	}
	return s
}

// within reports whether s is a span of g's cells: its row one of g's, and
// its columns, where it has them, in order within the row.
func within(s span, g grid) bool {
	if s.row < 1 || s.row > len(g) {
		return false
	}
	return s.from == 0 || 1 <= s.from && s.from <= s.to && s.to <= len(g[s.row-1])
}

// textOf is the glyphs of g the span covers.
func textOf(g grid, s span) string {
	var b strings.Builder
	for c, cell := range g[s.row-1] {
		if s.covers(s.row, c+1) {
			b.WriteRune(cell.glyph)
		}
	}
	return b.String()
}
