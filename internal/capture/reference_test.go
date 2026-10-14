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
	"runway-5-hour-160", "runway-phone-52", "runway-stepped-back-160", "runway-ten-accounts-160", "runway-week-160",
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
	// as a trace's; or, orBlank, nothing at all, as a column of a story
	// that has none of its height in a row.
	glyphs  string
	orBlank bool
}

// holds reports whether text, what the frame reads over the passing's span,
// is what the passing says it reads.
func (p passing) holds(text string) bool {
	got := strings.TrimSpace(text)
	switch {
	case p.glyphs != "":
		return (got != "" || p.orBlank) && !strings.ContainsFunc(got, func(r rune) bool { return r != ' ' && !strings.ContainsRune(p.glyphs, r) })
	case p.reads == "":
		return got == ""
	}
	return strings.HasPrefix(got, p.reads)
}

// want says what the passing says the frame reads, as a failure gives it.
func (p passing) want() string {
	switch {
	case p.glyphs != "" && p.orBlank:
		return fmt.Sprintf("only %q, or blanks", p.glyphs)
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

// drawnIn passes over row n's cells from and to, where the frame draws the
// glyphs given, and blanks.
func drawnIn(n, from, to int, glyphs string) passing {
	return passing{row: n, from: from, to: to, glyphs: glyphs}
}

// traced passes over row n's cells from and to, where the frame draws a
// trace.
func traced(n, from, to int) passing {
	return drawnIn(n, from, to, eighths)
}

// charted passes over the cells from and to of rows top to bottom, where the
// frame draws a chart: its fill, and its lines, the cap's ╌, the even pace's
// · and now's │.
func charted(top, bottom, from, to int) []passing {
	var p []passing
	for n := top; n <= bottom; n++ {
		p = append(p, drawnIn(n, from, to, eighths+"╌·│"))
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
	// pinnedRouting starts ROUTING's heading under a pin, which ends with
	// its key back to auto, 0.
	pinnedRouting = "ROUTING   pinned to"
)

// historyKeys passes over History's key line wider than a phone, row 37, from
// column 24, where the frames' p period starts and the dashboard's keys
// leave a cell more after [ ] view, to column to, the last cell either
// draws otherwise: tab views and [ ] view, and the help and quit's keys
// where both end the line there, are held.
func historyKeys(to int) passing {
	return cellsOf(37, 24, to, "p period")
}

// currentMonth passes over where the frames light September, the month
// before the moment drawn, where the design lights today's, October: over
// the grid, September's name on row grid from column september, lit, and
// October's, eight columns on, left out; and in BY MONTH, September's name on
// row byMonth from column name, bold, and October's under it, not.
func currentMonth(grid, september, byMonth, name int) []passing {
	return []passing{
		cellsOf(grid, september, september+2, "Sep"), blankCells(grid, september+8, september+10),
		cellsOf(byMonth, name, name+2, "Sep"), cellsOf(byMonth+1, name, name+2, "Oct"),
	}
}

// unrouted passes over the ▲ after side, at column c of row n, where new
// sessions go, which the frames mark, where Accounts and History show no
// routing.
func unrouted(n, c int) passing {
	return cellsOf(n, c, c, "▲")
}

// inColumns passes over row n's values in Compare's columns of a frame 160
// wide, colw cells each from column 27, all accounts' last, as reads gives
// them in turn: each value's column but the blank before the next, and all
// accounts' to the frame's edge, as its values may run there, reading as
// given; "" passes over none of a column.
func inColumns(n, colw int, reads ...string) []passing {
	all := (160-26)/colw - 1
	var p []passing
	for i, r := range reads {
		if r == "" {
			continue
		}
		from, to := 27+i*colw, 25+(i+1)*colw
		if i == all {
			to = 160
		}
		p = append(p, cellsOf(n, from, to, r))
	}
	return p
}

// haikuInCompare passes over Haiku 4.5's cells in Compare's BY MODEL, which
// the frames draw in its own shade, where the version table holds Haiku 5.5,
// so the dashboard mixes 4.5's toward the canvas, as an older version's: its
// dot, row 18, column 3, and its cells of the bars on row 12 at the columns
// given.
func haikuInCompare(columns ...int) []passing {
	p := []passing{cellsOf(18, 3, 3, "●")}
	for _, c := range columns {
		p = append(p, blankCells(12, c, c))
	}
	return p
}

// byDays passes over the days of Detail's BY DAY on row n, each cells wide,
// from each of the columns given, which the frame shades from its
// generator's days, and marks ▪ where it has them hit a 5-hour limit, where
// the world's records shade and mark them otherwise: blank or ▪ in the frame.
func byDays(n, cells int, firsts ...int) []passing {
	p := make([]passing, len(firsts))
	for i, c := range firsts {
		p[i] = passing{row: n, from: c, to: c + cells - 1, glyphs: "▪", orBlank: true}
	}
	return p
}

// squares passes over the Year's squares, ■, on row n at the columns given,
// a cell each, which the frame shades from its generator's days, where the
// world's records shade them otherwise.
func squares(n int, columns ...int) []passing {
	var p []passing
	for _, c := range columns {
		p = append(p, drawnIn(n, c, c, "■"))
	}
	return p
}

// days passes over the days of Days' strip on row n from each of the columns
// given, 2 cells each, which the frame shades from its generator's days,
// where the world's records shade them otherwise: blank in the frame.
func days(n int, firsts ...int) []passing {
	var p []passing
	for _, c := range firsts {
		p = append(p, blankCells(n, c, c+1))
	}
	return p
}

// marked passes over days of Days' strip as days does, where the frame marks
// each ▪, a day it hit a 5-hour limit.
func marked(n int, firsts ...int) []passing {
	var p []passing
	for _, c := range firsts {
		p = append(p, drawnIn(n, c, c+1, "▪"))
	}
	return p
}

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
	return wordsAt(n, c, name)
}

// wordsAt passes over the words on row n from column c, as many cells as
// they take, which read as given.
func wordsAt(n, c int, reads string) passing {
	return cellsOf(n, c, c+utf8.RuneCountInString(reads)-1, reads)
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
// allowances; spare's 5-hour window's rate, and its weeks' words, as its
// world has them reset on Tuesdays at 14:00; and lab's 5-hour
// window's even pace, its window starting later in its world, its week's
// words and its Fable week's allowance.
func extrasFigures(c int) []passing {
	return []passing{
		cellsOf(15, c+23, c+38, "16%/h till 17:40"), cellsOf(15, c+71, c+77, "14%/day"), cellsOf(15, c+119, c+125, "25%/day"),
		cellsOf(17, c+23, c+27, "19%/h"), cellsOf(17, c+71, c+92, "20%/day till Thu 18:00"), cellsOf(17, c+119, c+140, "20%/day till Thu 18:00"),
		cellsOf(19, c+9, c+10, "┃"), cellsOf(19, c+71, c+92, "11%/day till Mon 08:00"), cellsOf(19, c+119, c+125, "20%/day"),
	}
}

// The Year's squares its frames shade otherwise than the world's records, of
// all use, weekdays alone, the frames' weekends shaded as the world's are;
// filtered to Opus 5.5; and the latest 22 weeks of all use on a phone.
var (
	yearSquares = slices.Concat(
		squares(8, 21, 29, 37, 41, 59, 81, 89, 111, 119),
		squares(9, 33, 39, 43, 45, 47, 51, 53, 59, 67),
		squares(10, 21, 39, 41, 75, 87, 103, 115),
		squares(11, 15, 35, 37, 41, 49, 73, 77, 83, 111),
		squares(12, 27, 29, 35, 51, 73, 95, 103, 105, 117),
	)
	opusSquares = slices.Concat(
		squares(8, 109, 111, 119), squares(9, 103, 117), squares(10, 103, 109, 115), squares(11, 109, 111, 115),
		squares(12, 91, 97, 103, 105, 109, 115),
	)
	phoneSquares = slices.Concat(squares(10, 9, 17, 39, 47), squares(12, 15, 31, 43), squares(13, 5, 11, 39), squares(14, 23, 31, 33, 45))
)

// The days of Days' strips its frames shade otherwise than the world's
// records, by the first of each day's 2 columns: BY MODEL's, rows 11 to 17, a
// family every other row, and BY ACCOUNT's, from row 22, of three accounts,
// and of ten, its first six.
var (
	daysStrips = slices.Concat(
		days(11, 15, 21, 25, 27, 30, 57, 66, 72, 87, 98, 100, 102, 113, 117, 120),
		days(13, 17, 25, 27, 47, 51, 57, 60, 64, 66, 72, 77, 94, 96, 98, 100, 105, 107, 117, 124, 126),
		days(15, 17, 25, 27, 32, 34, 42, 47, 49, 51, 57, 62, 72, 79, 83, 92, 98, 100, 105, 113, 117, 120, 122, 124, 126),
		days(17, 25, 27, 30, 32, 51, 60, 68, 72, 79, 92, 100, 107, 113, 117, 120, 122, 124),
		days(22, 21, 25, 27, 40, 55, 57, 72, 85, 87, 100, 115, 117, 120, 124), marked(22, 107, 126),
		days(24, 17, 21, 25, 27, 32, 34, 49, 51, 53, 64, 72, 75, 100, 115, 117, 120, 124), marked(24, 79),
		days(26, 15, 17, 19, 21, 23, 25, 27, 32, 34, 38, 42, 45, 49, 57, 64, 66, 68, 72, 83, 87, 96, 100, 102, 109, 117),
	)
	daysTenStrips = slices.Concat(
		days(11, 15, 21, 25, 27, 53, 57, 72, 87, 100, 102, 117, 126),
		days(13, 25, 27, 32, 36, 47, 49, 57, 60, 66, 72, 75, 77, 79, 94, 100, 105, 107, 117, 120, 122, 124, 126),
		days(15, 17, 25, 27, 32, 34, 47, 49, 72, 100, 107, 109, 117, 120, 122, 126),
		days(17, 25, 27, 62, 72, 77, 79, 100, 105, 107, 117, 122, 124, 126),
		days(22, 25, 27, 40, 55, 57, 70, 72, 85, 87, 100, 115, 117, 124), marked(22, 23, 30, 38, 122, 126),
		days(24, 25, 27, 45, 72, 81, 85, 96, 100, 113, 115, 117, 124, 126),
		days(26, 17, 23, 25, 27, 30, 42, 53, 57, 60, 62, 72, 81, 85, 87, 90, 98, 100, 102, 105, 113, 115, 117, 126),
		marked(26, 47, 79),
		days(28, 21, 23, 25, 27, 34, 36, 40, 42, 45, 53, 55, 57, 70, 72, 85, 100, 113, 117, 120, 122, 126), marked(28, 68),
		days(30, 17, 25, 27, 30, 32, 34, 42, 51, 55, 57, 60, 62, 64, 66, 72, 79, 81, 85, 87, 98, 100, 102, 105, 107, 109, 111,
			113, 115, 117),
		days(32, 19, 21, 25, 27, 40, 51, 53, 55, 70, 72, 85, 100, 117),
	)
)

// The figures of Tokens' periods its frames give otherwise than the world's
// records, each a figure or a run of them: by account, and by model.
var (
	byAccountFigures = []passing{
		cellsOf(9, 39, 66, "4.2M"), cellsOf(9, 77, 80, "615M"), cellsOf(9, 91, 94, "660M"),
		cellsOf(10, 39, 66, "1.2M"), cellsOf(10, 77, 80, "178M"), cellsOf(10, 91, 94, "191M"),
		cellsOf(11, 39, 42, "1.6M"), cellsOf(11, 62, 66, "14.2M"), cellsOf(11, 77, 80, "241M"), cellsOf(11, 91, 94, "258M"),
		cellsOf(12, 51, 80, "0.9M"), cellsOf(12, 91, 94, "211M"),
		cellsOf(14, 39, 110, "6.7M"),
		cellsOf(15, 39, 100, "3.5M"),
		cellsOf(16, 15, 15, "4"), cellsOf(16, 39, 100, "2.1M"),
		cellsOf(17, 63, 66, "9.8M"), cellsOf(17, 77, 80, "151M"), cellsOf(17, 91, 100, "162M"),
		cellsOf(19, 39, 80, "5.9M"), cellsOf(19, 91, 110, "934M"),
		cellsOf(20, 39, 100, "2.9M"),
		cellsOf(21, 15, 15, "4"), cellsOf(21, 39, 100, "2.2M"),
		cellsOf(22, 39, 42, "0.8M"), cellsOf(22, 63, 66, "7.7M"), cellsOf(22, 77, 80, "118M"), cellsOf(22, 91, 100, "127M"),
		cellsOf(24, 39, 110, "6.1M"),
		cellsOf(25, 39, 100, "3.5M"),
		cellsOf(26, 15, 15, "4"), cellsOf(26, 39, 66, "1.7M"), cellsOf(26, 77, 80, "252M"), cellsOf(26, 91, 100, "270M"),
		cellsOf(27, 39, 66, "0.9M"), cellsOf(27, 77, 80, "131M"), cellsOf(27, 91, 100, "141M"),
		cellsOf(29, 39, 110, "5.7M"),
		cellsOf(30, 39, 100, "3.3M"),
		cellsOf(31, 15, 15, "4"), cellsOf(31, 39, 66, "1.9M"), cellsOf(31, 77, 100, "281M"),
		cellsOf(32, 15, 15, "4"), cellsOf(32, 39, 42, "0.5M"), cellsOf(32, 63, 100, "4.9M"),
	}
	byModelFigures = []passing{
		cellsOf(9, 39, 66, "4.2M"), cellsOf(9, 77, 80, "615M"), cellsOf(9, 91, 94, "660M"),
		cellsOf(10, 15, 23, "work side"), cellsOf(10, 39, 100, "1.8M"),
		cellsOf(11, 39, 42, "0.2M"), cellsOf(11, 63, 99, "1.5M"),
		cellsOf(12, 39, 66, "1.7M"), cellsOf(12, 77, 80, "252M"), cellsOf(12, 91, 100, "271M"),
		cellsOf(13, 39, 42, "0.2M"), cellsOf(13, 63, 99, "1.9M"),
		cellsOf(14, 63, 99, "0.8M"),
		cellsOf(15, 39, 42, "0.2M"), cellsOf(15, 63, 99, "1.5M"),
		cellsOf(17, 39, 110, "6.7M"),
		cellsOf(18, 39, 100, "2.9M"),
		cellsOf(19, 39, 66, "0.3M"), cellsOf(19, 76, 99, "39.4M"),
		cellsOf(20, 39, 100, "2.7M"),
		cellsOf(21, 39, 99, "0.3M"),
		cellsOf(22, 20, 32, "side"), cellsOf(22, 63, 99, "1.2M"),
		cellsOf(23, 39, 99, "0.3M"),
		cellsOf(25, 39, 80, "5.9M"), cellsOf(25, 91, 110, "934M"),
		cellsOf(26, 39, 66, "2.6M"), cellsOf(26, 77, 100, "383M"),
		cellsOf(27, 39, 99, "0.2M"),
		cellsOf(28, 39, 100, "2.4M"),
		cellsOf(29, 39, 66, "0.3M"), cellsOf(29, 76, 99, "43.5M"),
		cellsOf(30, 20, 32, "side"), cellsOf(30, 90, 99, "18.7M"),
		cellsOf(31, 39, 66, "0.2M"), cellsOf(31, 76, 99, "34.8M"),
	}
)

// logTail are the times of the rows log-requests-160 draws, each after its
// mark, the newest 27 of the Log's tail: its generator's lines, where the
// world's d28c's are as its session's page draws them, and its requests in
// flight are the Overview's frames', so the world draws others in their
// place. A phone draws the newest 22.
var logTail = []string{
	"14:40:26", "14:40:26", "14:40:28", "14:40:37", "14:40:39", "14:40:40", "14:40:40", "14:40:41", "14:40:49",
	"14:40:59", "14:41:00", "14:41:00", "▲ 14:41:11", "14:41:14", "14:41:14", "14:41:16", "14:41:19", "14:41:22",
	"14:41:25", "14:41:29", "14:41:38", "14:41:39", "14:41:41", "14:41:45", "↓ 14:41:53", "↑ 14:42:00", "↓ 14:42:01",
}

// filteredTail are the times of the rows log-requests-filtered-160 draws,
// each after its mark, d28c's newest 27 lines: its generator's, where the
// world's are as d28c's page draws them, and its request in flight the
// Overview's frames'.
var filteredTail = []string{
	"14:36:27", "14:36:29", "14:36:48", "14:37:09", "14:37:25", "14:37:38", "14:37:54", "14:38:03", "14:38:17",
	"14:38:36", "14:38:44", "14:38:58", "14:39:15", "14:39:34", "14:39:48", "14:39:53", "14:40:03", "14:40:18",
	"14:40:37", "14:40:40", "14:40:41", "14:41:00", "14:41:14", "14:41:16", "14:41:22", "14:41:45", "↑ 14:42:00",
}

// aroundTheCap are the rows log-request-details-160 draws beside its card,
// from row 11 to 31, each after its mark, to its session's short id, as its
// generator drew them, where the world's d28c's and 9c2e's lines are as their
// sessions' pages draw them.
var aroundTheCap = []string{
	"14:31:20  9c2e", "▸ 14:31:20  7f3a", "14:31:21  41e0", "14:31:32  41e0", "14:31:40  9c2e", "▸ 14:31:40  db8a",
	"14:31:42  41e0", "14:31:44  d28c", "14:31:49  d28c", "14:31:51  db8a", "14:31:53  9c2e", "14:31:54  9c2e",
	"14:31:58  7f3a", "14:32:02  41e0", "14:32:04  9c2e", "14:32:14  9c2e", "14:32:16  9c2e", "14:32:19  db8a",
	"14:32:20  d28c", "14:32:22  7f3a", "14:32:23  41e0",
}

// besideTheCard passes over the row each of reads, from row first on, that a
// details card covers at 160 columns but its first 16 cells, which read as
// given.
func besideTheCard(first int, reads ...string) []passing {
	p := make([]passing, len(reads))
	for i, r := range reads {
		p[i] = cellsOf(first+i, 1, 16, r)
	}
	return p
}

// tableRows passes over a row each of reads, from row first on, each reading
// as given.
func tableRows(first int, reads ...string) []passing {
	p := make([]passing, len(reads))
	for i, r := range reads {
		p[i] = wholeRow(first+i, r)
	}
	return p
}

// behindThePanel passes over the Log's tail as log-requests-filter-160 draws
// it behind the f panel, faded, as log-requests-160 passes it over: left of
// the panel's card from row 8 to 29, and whole under it.
func behindThePanel() []passing {
	beside := len(logTail) - 5
	p := make([]passing, beside)
	for i, r := range logTail[:beside] {
		p[i] = cellsOf(8+i, 1, 18, r)
	}
	return append(p, tableRows(8+beside, logTail[beside:]...)...)
}

// panelSessions passes over the rows of today's sessions in a panel's frame,
// from column from to column to, each its session's row, reading as given,
// a row's from row first on, where the reads aren't blank.
func panelSessions(first, from, to int, reads ...string) []passing {
	var p []passing
	for i, r := range reads {
		if r != "" {
			p = append(p, cellsOf(first+i, from, to, r))
		}
	}
	return p
}

// fromTheLog passes over the tabs on the title row, row n, ` Overview ` and
// ` Log `, from the columns given: a session's page's frames light the
// Overview's, where its fixtures, which reach the page from the Log, light
// the Log's.
func fromTheLog(n, overview, log int) []passing {
	return []passing{cellsOf(n, overview, overview+9, "Overview"), cellsOf(n, log, log+4, "Log")}
}

// storyColumns passes over the columns given of a session's story, from row
// top to row bottom, which the frame draws from its generator's curves,
// where the world's requests fall in their minutes otherwise: each cell an
// eighth, ┊ where a move's marker runs down to its column, or blank. The
// story's other cells, its floor, markers, folds and ribbon among them, are
// held.
func storyColumns(top, bottom int, columns ...int) []passing {
	var p []passing
	for n := top; n <= bottom; n++ {
		for from := 0; from < len(columns); {
			to := from
			for to+1 < len(columns) && columns[to+1] == columns[to]+1 {
				to++
			}
			p = append(p, passing{row: n, from: columns[from], to: columns[to], glyphs: eighths + "┊", orBlank: true})
			from = to + 1
		}
	}
	return p
}

// The columns of the sessions' stories, wide, from row 12 to 16, and on a
// phone, from row 12 to 15, that their frames draw otherwise than the
// world's requests fill them: d28c's, wide and on a phone, 9c2e's, and that
// of 9c2e's frame behind the routing card, b9c4's, and f41a's but its fold.
var (
	d28cStory = storyColumns(12, 16,
		1, 2, 3, 4, 11, 12, 14, 15, 16, 18, 19, 27, 28, 29, 36, 37, 38, 39, 41, 42, 43, 44, 45, 53, 58, 65, 66, 71, 72, 73, 75, 77, 78,
		80, 82, 83, 85, 87, 88, 89, 97, 98, 99, 100, 101, 102, 109, 111, 113, 120, 121, 123, 126, 128, 129, 134, 135, 136, 138, 140,
		141, 142, 144, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 159, 160)
	d28cPhoneStory = storyColumns(12, 15,
		1, 2, 3, 4, 6, 7, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 21, 22, 27, 28, 29, 31, 32, 33, 34, 36, 37, 39, 40, 41, 42, 43, 44,
		45, 47, 48, 49, 50, 51, 52)
	quietStory = storyColumns(12, 16,
		1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 13, 15, 16, 17, 21, 22, 23, 24, 25, 26, 27, 28, 36, 37, 38, 39, 40, 41, 42, 44, 45, 59, 61, 76, 78,
		84, 85, 86, 87, 88, 89, 90, 91, 92, 102, 103, 104, 105, 106, 107, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 128,
		129, 130, 139, 140, 141, 142, 148, 149, 150, 151, 152)
	behindTheSessionsCard = storyColumns(12, 16,
		1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 13, 15, 16, 17, 21, 22, 23, 24, 25, 26, 27, 28, 36, 37, 38, 39, 122, 123, 124, 125, 126, 128, 129,
		130, 139, 140, 141, 142, 148, 149, 150, 151, 152)
	endedStory = storyColumns(12, 16,
		1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 13, 14, 15, 16, 22, 23, 24, 25, 26, 27, 28, 29, 34, 35, 41, 42, 43, 44, 46, 47, 48, 49, 50, 51,
		52, 53, 54, 55, 61, 62, 63, 70, 71, 78, 79, 80, 81, 82, 90, 91, 93, 94, 95, 99, 100, 101, 103, 104, 105, 113, 114, 116, 117, 118,
		123, 124, 125, 126, 128, 129, 130, 135, 136, 137, 138, 139, 140, 141, 142, 143, 155, 156, 157, 159, 160)
	resumedStory = storyColumns(12, 16,
		1, 2, 8, 9, 15, 16, 18, 19, 20, 23, 24, 26, 28, 29, 31, 38, 39, 40, 41, 43, 44, 45, 46, 47, 48, 50, 51, 52, 53, 54, 55, 56, 57, 60,
		61, 62, 63, 64, 65, 68, 69, 70, 73, 74, 80, 81, 82, 83, 87, 88, 90, 91, 92, 93, 94, 95, 98, 100, 101, 102, 103, 104, 107, 108,
		110, 111, 112, 116, 117, 118, 119, 120, 123, 125, 129, 131, 133, 134, 135, 136, 137, 138, 139, 140, 144, 145, 148, 150, 154, 155,
		157, 158)
)

// What d28c's frames, wide, draw otherwise than its world gives: how long
// it has run, from 12:02:00 where its first request came at 12:02:10; its
// stay on work ending 14:31 where its last request there came at 14:30; and
// each stay's points, where the world's lines climb no window, and worth.
var d28cPage = slices.Concat(fromTheLog(1, 16, 68), []passing{
	wordsAt(7, 155, "2h 40m"),
	wordsAt(20, 23, "14:31"), wordsAt(20, 57, "≈ 41 points of its 5-hour · 6 of its week"), wordsAt(20, 155, "$41.06"),
	wordsAt(21, 57, "≈ 9 points of its 5-hour · 1 of its week"), wordsAt(21, 156, "$7.14"),
}, d28cStory)

// What 9c2e's frames draw otherwise than its world gives above its story:
// sticky, where the router lists it as new; and how long it has run, from
// 11:55:00 where its first request came at 11:55:31.
var quietPage = slices.Concat(fromTheLog(1, 16, 68), []passing{wordsAt(7, 41, ": sticky"), wordsAt(7, 155, "2h 47m")})

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

	"accounts-compare-160": slices.Concat(
		[]passing{period(150, "this week"), unrouted(7, 100)}, haikuInCompare(83, 149),
		inColumns(13, 33, "", "", "", "57%"), inColumns(15, 33, "", "", "", "33%"), inColumns(18, 33, "", "", "", "3%"),
		inColumns(22, 33, "", "6", "11", "19"), inColumns(23, 33, "3 out", "1 out", "", "4 moves"),
		inColumns(26, 33, "", "none", "", "3"), inColumns(27, 33, "", "—", "", "4h 02m"),
		[]passing{wholeRow(37, accountsKeys)},
	),
	"accounts-compare-bad-hour-160": slices.Concat(
		[]passing{period(150, "this week"), unrouted(7, 100), cellsOf(10, 47, 49, "34%"), blankCells(12, 27, 160)}, haikuInCompare(),
		inColumns(13, 33, "55%", "29%", "82%", "57%"), inColumns(14, 33, "7%", "", "", "2%"),
		inColumns(15, 33, "28%", "60%", "16%", "33%"), inColumns(16, 33, "3%", "7%", "", "3%"),
		inColumns(17, 33, "5%"), inColumns(18, 33, "2%", "4%"),
		inColumns(21, 33, "1,204", "2,330", "1,876", "5,410"), inColumns(22, 33, "9", "6", "11", "19"),
		inColumns(23, 33, "3 out", "", "4 in", "4 moves"),
		inColumns(26, 33, "", "5-hour ×1", "", "3"), inColumns(27, 33, "3h 10m", "—", "", "4h 02m"),
		inColumns(33, 33, "$184", "$212", "$239", "$635"), inColumns(34, 33, "4.0×", "4.6×", "5.2×", "4.6×"),
		[]passing{wholeRow(37, accountsKeys)},
	),
	"accounts-compare-month-160": slices.Concat(
		[]passing{period(152, "30 days"), unrouted(7, 100), blankCells(12, 126, 160)}, haikuInCompare(83),
		inColumns(13, 33, "", "", "", "54%"), inColumns(15, 33, "", "", "", "35%"), inColumns(17, 33, "", "", "", "2%"),
		inColumns(21, 33, "5,180", "9,930", "8,050", "23,160"), inColumns(22, 33, "38", "27", "41", "71"),
		inColumns(23, 33, "12 out", "5 out", "14 in"),
		inColumns(26, 33, "5-hour ×9", "5-hour ×3", "5-hour ×4", "16"), inColumns(27, 33, "13h 40m", "2h 05m", "3h 10m", "18h 55m"),
		inColumns(33, 33, "$790", "$905", "$1,020", "$2,715"), inColumns(34, 33, "4.0×", "4.5×", "5.1×", "4.5×"),
		[]passing{wholeRow(37, accountsKeys)},
	),
	"accounts-compare-ten-160": slices.Concat(
		[]passing{period(150, "this week"), unrouted(7, 72), cellsOf(8, 84, 93, "● open")}, haikuInCompare(60, 98, 117),
		inColumns(13, 19, "", "", "", "", "", "", "53%"), inColumns(14, 19, "", "", "", "", "", "", "4%"),
		inColumns(16, 19, "", "", "", "", "", "", "4%"), inColumns(17, 19, "", "", "", "", "", "", "1%"),
		inColumns(21, 19, "", "", "", "", "", "", "12,598"), inColumns(22, 19, "", "6", "11", "", "", "", "51"),
		inColumns(23, 19, "3 out", "1 out", "", "", "", "1 out", "10 moves"),
		inColumns(26, 19, "", "none"), inColumns(27, 19, "", "—", "", "", "", "35m", "9h 57m"),
		inColumns(28, 19, "", "", "", "22%", "", "30%"),
		inColumns(33, 19, "", "", "", "", "", "", "$1,342"), inColumns(34, 19, "", "", "", "", "", "", "3.4×"),
		[]passing{wholeRow(37, accountsKeys)},
	),
	"accounts-detail-all-160": slices.Concat(
		[]passing{
			period(150, "this week"), wordsAt(11, 119, "3"), unrouted(13, 9), blankCells(16, 136, 140),
			wordsAt(17, 48, "57%"), wordsAt(17, 79, "33%"), cellsOf(17, 115, 115, "●"), wordsAt(17, 127, "3%"),
		},
		byDays(21, 2, 43, 45, 57, 67, 73, 77, 91, 101, 119, 121, 129, 131, 133),
		[]passing{cellsOf(24, 97, 97, "▋"), cellsOf(24, 132, 132, "▎"), wordsAt(26, 50, "30%"), wholeRow(37, accountsKeys)},
	),
	"accounts-detail-work-160": slices.Concat(
		[]passing{period(150, "this week"), unrouted(13, 9), blankCells(16, 139, 140), cellsOf(17, 115, 115, "●")},
		byDays(21, 2, 37, 47, 49, 55, 57, 61, 63, 67, 83, 85, 99, 101, 111, 119, 121, 129, 131, 133),
		[]passing{cellsOf(30, 37, 160, "12:10"), wholeRow(37, accountsKeys)},
	),
	"accounts-detail-limit-160": slices.Concat(
		[]passing{
			period(150, "this week"), cellsOf(10, 60, 64, "┃"), wordsAt(10, 119, "2,330"), wordsAt(11, 119, "5-hour ×1"),
			unrouted(13, 9), wordsAt(13, 119, "$212  4.6×"), blankCells(16, 37, 140), cellsOf(17, 37, 160, "● Opus 5.5 29%"),
		},
		byDays(21, 2, 67, 73, 101, 119, 121, 131, 133),
		[]passing{cellsOf(30, 37, 160, "11:30"), wholeRow(37, accountsKeys)},
	),
	"accounts-detail-sessions-160": slices.Concat(
		[]passing{
			period(150, "this week"), cellsOf(7, 32, 66, "▲"), cellsOf(12, 81, 102, sidesWeek), unrouted(13, 9),
			blankCells(16, 140, 140), cellsOf(17, 87, 87, "●"),
		},
		byDays(21, 2, 37, 41, 49, 59, 63, 67, 85, 99, 101, 103, 117, 119, 121, 127),
		[]passing{wholeRow(37, accountsKeys)},
	),
	"accounts-detail-one-160": slices.Concat(
		[]passing{
			period(150, "this week"), cellsOf(7, 8, 8, "▲"), evenPace(10, 35), wordsAt(10, 111, "1,204"), evenPace(12, 36),
			wordsAt(13, 111, "$184  4.0×"), evenPace(14, 36), blankCells(16, 13, 143), cellsOf(17, 13, 143, "● Opus 5.5 60%"),
		},
		byDays(21, 3, 13, 28, 31, 35, 38, 44, 47, 53, 72, 75, 91, 94, 104, 116, 119, 126, 129, 132),
		[]passing{cellsOf(28, 20, 20, "●"), blankCells(29, 39, 49), wholeRow(37, accountsKeys)},
	),
	"accounts-phone-compare-52": {
		phonePeriod("this week"), unrouted(15, 7), blankCells(23, 33, 33), unrouted(25, 7), blankCells(27, 32, 33), wordsAt(27, 47, "57%"),
		cellsOf(30, 36, 47, "3 out"), cellsOf(31, 24, 25, "6"), cellsOf(31, 36, 40, "1 out"), unrouted(32, 7), cellsOf(32, 24, 25, "11"),
		cellsOf(33, 24, 25, "19"), cellsOf(33, 36, 42, "4 moves"),
		cellsOf(37, 14, 18, "none"), cellsOf(37, 27, 32, "—"), unrouted(38, 7), wordsAt(39, 14, "3"), cellsOf(39, 27, 32, "4h 02m"),
		unrouted(44, 7),
	},
	"accounts-phone-detail-52": slices.Concat(
		[]passing{phonePeriod("this week"), blankCells(24, 50, 50), cellsOf(26, 32, 32, "●")},
		byDays(28, 2, 13, 15, 19, 21, 28, 34, 36, 38, 43, 45, 47),
		[]passing{wholeRow(38, "12:10")},
	),

	"history-year-160": slices.Concat(
		[]passing{period(145, "last 12 months")}, currentMonth(7, 111, 31, 3), yearSquares,
		[]passing{
			cellsOf(8, 140, 146, "236,410"), cellsOf(10, 140, 160, "5-hour ×402"), cellsOf(11, 140, 146, "$27,480"),
			cellsOf(13, 140, 143, "3.8×"), cellsOf(14, 140, 149, "Tue 22 Sep"), cellsOf(15, 140, 146, "58 days"),
			cellsOf(16, 32, 91, "450   ■ 450–750"),
			cellsOf(30, 49, 50, "55"),
			cellsOf(31, 49, 50, "60"), drawnIn(31, 77, 78, "▅"), drawnIn(31, 89, 89, "▅"),
			cellsOf(32, 15, 19, "1,548"), cellsOf(32, 27, 30, "$180"), cellsOf(32, 49, 49, "2"),
			cellsOf(32, 62, 70, "▅  so far"),
			historyKeys(93),
		},
	),
	"history-year-opus-160": slices.Concat(
		[]passing{period(145, "last 12 months")}, currentMonth(7, 111, 24, 3), opusSquares,
		[]passing{
			cellsOf(8, 140, 145, "48,880"), cellsOf(9, 140, 142, "352"), cellsOf(10, 140, 145, "$5,760"),
			cellsOf(11, 140, 142, "21%"), cellsOf(13, 140, 149, "Tue 22 Sep"),
			cellsOf(16, 32, 71, "400   ■ 400–600"),
			cellsOf(21, 15, 19, "3,120"), cellsOf(21, 27, 30, "$380"), cellsOf(21, 67, 82, "from the 22nd"),
			cellsOf(22, 15, 20, "12,940"), cellsOf(22, 27, 32, "$1,520"), cellsOf(22, 39, 41, "57%"), blankCells(22, 84, 85),
			cellsOf(23, 15, 20, "14,860"), cellsOf(23, 27, 32, "$1,730"), cellsOf(23, 39, 41, "60%"), blankCells(23, 87, 89),
			cellsOf(24, 15, 20, "17,320"), cellsOf(24, 27, 32, "$2,010"), cellsOf(24, 39, 41, "64%"),
			cellsOf(25, 15, 19, "640"), cellsOf(25, 27, 30, "$120"), cellsOf(25, 39, 42, "67%"),
			historyKeys(82),
		},
	),
	"history-phone-year-52": slices.Concat(
		[]passing{phonePeriod("last 12 months")}, currentMonth(9, 39, 30, 2), phoneSquares,
		[]passing{
			cellsOf(18, 8, 52, "450 ■ 450–750"),
			cellsOf(21, 11, 17, "236,410"), cellsOf(21, 37, 43, "$27,480"),
			cellsOf(23, 11, 25, "5-hour ×402"), cellsOf(23, 37, 40, "3.8×"),
			blankCells(29, 49, 49),
			drawnIn(30, 43, 43, "▅"), cellsOf(31, 13, 16, "$180"),
		},
	),
	"history-weeks-160": {
		period(148, "eight weeks"),
		cellsOf(15, 137, 137, "6"), cellsOf(16, 138, 138, "5"), unrouted(17, 6), cellsOf(17, 139, 139, "3"),
		cellsOf(35, 57, 58, "59"),
		historyKeys(92),
	},
	"history-weeks-columns-160": {
		period(148, "eight weeks"),
		drawnIn(8, 31, 35, "█"), cellsOf(8, 121, 142, ", each time work alone"),
		cellsOf(15, 114, 114, "6"), drawnIn(20, 71, 75, "█"), cellsOf(21, 114, 114, "5"), unrouted(25, 6), cellsOf(27, 114, 114, "3"),
		cellsOf(35, 48, 125, "100% it hit its limit"),
		historyKeys(94),
	},
	"history-weeks-ten-160": {
		period(148, "eight weeks"),
		cellsOf(8, 9, 38, "short on 4 of the last 7 weeks"), cellsOf(8, 87, 117, "short on 4 of 7, the same weeks"),
		cellsOf(8, 150, 150, "6"),
		cellsOf(9, 36, 42, "5% used"),
		cellsOf(13, 42, 42, "4"), cellsOf(13, 92, 92, "4"),
		unrouted(17, 6),
		cellsOf(19, 93, 99, "5% used"),
		cellsOf(22, 96, 114, "80% left at a reset"),
		historyKeys(92),
	},
	"history-days-160": slices.Concat(
		[]passing{period(148, "eight weeks"), cellsOf(11, 138, 139, "59"), cellsOf(13, 138, 139, "37")},
		daysStrips,
		[]passing{cellsOf(24, 140, 140, "1"), unrouted(26, 6), historyKeys(75)},
	),
	"history-days-ten-160": slices.Concat(
		[]passing{period(148, "eight weeks"), cellsOf(15, 139, 139, "1")},
		daysTenStrips,
		[]passing{cellsOf(24, 140, 140, "1"), unrouted(26, 6), historyKeys(75)},
	),
	"history-tokens-160": slices.Concat(
		[]passing{period(152, "by week")}, byAccountFigures, []passing{cellsOf(33, 3, 41, "3 more weeks below"), historyKeys(77)},
	),
	"history-tokens-models-160": slices.Concat(
		[]passing{period(152, "by week")}, byModelFigures, []passing{cellsOf(32, 3, 41, "5 more weeks below"), historyKeys(75)},
	),

	"log-event-details-160": {cellsOf(27, 145, 145, "8")},
	"log-events-160":        {cellsOf(27, 88, 160, ", its 5-hour to run out at 15:10   48k cached again, $0.48")},
	"log-requests-160":      append(tableRows(8, logTail...), cellsOf(36, 144, 148, "1,240")),
	"log-requests-phone-52": append(tableRows(9, logTail[len(logTail)-22:]...), cellsOf(32, 36, 40, "1,240")),
	"log-events-filter-160": slices.Concat(
		[]passing{cellsOf(13, 21, 31, "■ started")},
		panelSessions(15, 80, 138, "□ d28c", "", "□ 7f3a", "□ db8a", "□ 41e0"),
		[]passing{cellsOf(27, 143, 145, ".48")},
	),
	"log-requests-filter-160": slices.Concat(
		behindThePanel(),
		[]passing{cellsOf(10, 70, 74, "1,275"), blankCells(14, 98, 110), cellsOf(15, 83, 87, "1,240")},
		panelSessions(18, 73, 131, "□ d28c", "", "□ 7f3a", "□ db8a", "□ 41e0"),
		[]passing{cellsOf(26, 31, 45, "1,240 of 1,275"), cellsOf(36, 144, 148, "1,240")},
	),
	"log-request-details-160": slices.Concat(
		[]passing{wholeRow(10, "14:31:18  9c2e")}, besideTheCard(11, aroundTheCap...),
		[]passing{
			blankCells(11, 145, 160), cellsOf(12, 145, 160, "s cap  $0.98"),
			blankCells(15, 145, 160), cellsOf(16, 145, 160, "s cap  $0.66"),
		},
		tableRows(32, "14:32:29  d28c", "14:32:34  db8a", "14:32:36  9c2e"),
		[]passing{cellsOf(15, 35, 62, "req_011CQx…"), cellsOf(36, 26, 28, "214"), cellsOf(36, 144, 148, "1,240")},
	),
	"log-requests-filtered-160": slices.Concat(
		[]passing{cellsOf(5, 133, 135, "830")}, tableRows(8, filteredTail...), []passing{cellsOf(36, 142, 144, "445")},
	),

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

	"routing-session-160": slices.Concat(
		quietPage,
		[]passing{
			cellsOf(7, 23, 23, "◆"), wordsAt(8, 12, "personal"), wordsAt(8, 110, "personal"), wordsAt(8, 142, "66%"),
			wholeRow(10, "STORY   requests a minute, its whole life"), cellsOf(17, 77, 94, ": side for now"),
		},
		figures(18, "95%", "34%"), figures(19, "39%", "66%"),
		[]passing{
			cellsOf(20, 1, 39, "personal ◆"), cursorRow(20, "● 3 side"), cellsOf(22, 1, 39, "TURNS   newest first"),
			wordsAt(23, 73, "1.56"),
		},
		behindTheSessionsCard,
	),

	"session-moved-160": d28cPage,
	"session-quiet-160": slices.Concat(quietPage, quietStory, []passing{wordsAt(20, 57, "≈ 23 points of its 5-hour · 4 of its week")}),
	"session-ended-160": slices.Concat(
		fromTheLog(1, 16, 68),
		[]passing{
			wordsAt(7, 17, "18:47"), wordsAt(7, 155, "5h 27m"), cellsOf(8, 10, 40, "· haiku-4-5"),
			cellsOf(18, 49, 54, "15:00"),
			wordsAt(20, 33, "16:02"), wordsAt(20, 57, "≈ 52 points of its 5-hour · 8 of its week"),
			wordsAt(21, 33, "18:47"), wordsAt(21, 57, "≈ 38 points of its 5-hour · 6 of its week"),
			wordsAt(23, 124, "97%"), wordsAt(27, 86, "40k"), wordsAt(34, 33, "Edit ×2, Bash"),
		},
		endedStory,
	),
	"session-resumed-160": slices.Concat(
		fromTheLog(1, 16, 68),
		[]passing{
			wordsAt(7, 1, "↓ streaming"), cellsOf(7, 57, 95, "15h idle"), cellsOf(11, 36, 83, "┊ 09:12  rescored after 15h idle"),
			drawnIn(12, 32, 37, eighths+"╱┊"), drawnIn(13, 32, 37, eighths+"╱┊"), drawnIn(14, 32, 37, eighths+"╱┊"),
			drawnIn(15, 32, 37, eighths+"╱┊"), drawnIn(16, 32, 37, eighths+"╱┊"),
			cellsOf(17, 14, 20, "─ side"), cellsOf(17, 32, 37, "─┤"), cellsOf(18, 32, 42, "15h  09:12"),
			wordsAt(20, 33, "18:05"), wordsAt(20, 57, "≈ 18 points of its 5-hour · 3 of its week"),
			wordsAt(21, 57, "≈ 31 points of its 5-hour · 9 of its week"),
			wordsAt(23, 123, "97%"), wordsAt(27, 67, "16.1M"), wordsAt(27, 86, "76k"), wordsAt(33, 67, "14.2M"),
		},
		resumedStory,
	),
	"session-turn-opened-160": slices.Concat(d28cPage, []passing{wholeRow(29, "│  14:29:04"), wholeRow(37, "esc close    ↑↓ request  p pin")}),
	"session-phone-52": slices.Concat(
		fromTheLog(2, 1, 48),
		[]passing{
			wordsAt(9, 47, "2h 40m"),
			wordsAt(19, 12, "≈ 41 of 5-hour · 6 of week"), wordsAt(19, 47, "$41.06"),
			wordsAt(20, 12, "≈ 9 of 5-hour · 1 of week"), wordsAt(20, 48, "$7.14"),
			blankCells(31, 1, 34), wholeRow(32, "esc back   ↑↓ turn   enter open   ? keys"),
		},
		d28cPhoneStory,
	),
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
	"sessions-switchboard-160": heldTabbed(3, chrome),
	"runway-5-hour-160":        heldTabbed(4, chrome),
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
		{name: "a story's columns", passing: storyColumns(1, 1, 1, 2, 3)[0], text: "▂┊▅", want: true},
		{name: "a story's columns, none of their height in the row", passing: storyColumns(1, 1, 1, 2, 3)[0], text: "   ", want: true},
		{name: "words where it says a story's columns", passing: storyColumns(1, 1, 1, 2, 3)[0], text: "14:31"},
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
