package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// At every width from 100 to 200, the scene's Switchboard draws each part
// where the narrowing table puts it: each call's directory cut to its cells,
// what it's doing, where it came from, short where ▸ drops its from, and its
// plug; the jacks; and each line, its name, its window's label, its bar's
// cells, its percentage after them, and its words, in usage's short form
// where the table takes it.
func TestEveryWidthDrawsTheSwitchboardsPartsWhereTheNarrowingTableHasThem(t *testing.T) {
	doc := usageWorld()
	for width := 100; width <= 200; width++ {
		f := sceneFrame(width, 37, Look{})
		k := f.switchboardLaid(doc, routedAt).columns
		c := f.page(doc, routedAt)
		from, words, runsOut := "▸ from work", "at cap · back 17:10", "out ~Sat 06:00"
		if k.fromShort {
			from = "▸ work"
		}
		if k.shortWords {
			words, runsOut = "back 17:10", "out ~Sat"
		}
		bar := k.lines + 1 + firstLabelCells
		checks := []struct{ what, got, want string }{
			{"d28c's directory", textFrom(c, 5, 5+k.dir, 10), padded(status.DirName("~/Code/switchboard", k.dir), k.dir)},
			{"what d28c's doing", textFrom(c, k.doing, k.doing+8, 10), "↑ asking"},
			{"where it came from", textFrom(c, k.from, k.from+len([]rune(from)), 10), from},
			{"its plug", c.at(k.plug, 10).glyph, plugGlyph},
			{"41e0's jack", c.at(k.jacks, 14).glyph, pluggedJack},
			{"work's name", textFrom(c, k.lines, k.lines+6, 6), "1 work"},
			{"its 5-hour's label", textFrom(c, k.lines+1, k.lines+7, 7), "5-hour"},
			{"its percentage", textFrom(c, bar+k.bar+1, bar+k.bar+1+percentCells, 7), " 95%"},
			{"its words", strings.TrimRight(textFrom(c, bar+k.bar+afterBar, width, 7), " "), words},
			{"personal's week's words, which fit in full", strings.TrimRight(textFrom(c, bar+k.bar+afterBar, width, 17), " "), runsOut},
		}
		for _, check := range checks {
			if check.got != check.want {
				t.Errorf("at %d, %s reads %q, want %q", width, check.what, check.got, check.want)
			}
		}
	}
}

// The scene's lines compact as rows run short, each a step on: at 160 by 37,
// each line as usage draws it; by 29, no blank rows within a line; by 24,
// two rows a line, its 5-hour and week side by side; by 19, one row a line,
// its name, its 5-hour and how it stands; the key under each, two blank rows
// after the last line.
func TestTheLinesCompactAsRowsRunShort(t *testing.T) {
	tests := []struct {
		name   string
		height int
		step   int
		rows   map[int]string
	}{
		{name: "step 1", height: 37, step: 1, rows: map[int]string{
			6: "1 work          ● at its cap · back 17:10 · new sessions go elsewhere", 7: " 5-hour", 9: " Week", 11: " Fable wk",
			14: "2 personal ◆    ● open", 22: "3 side ▲        ● open · new sessions come here", 30: " ┃ even pace",
		}},
		{name: "step 2", height: 29, step: 2, rows: map[int]string{
			6: "1 work ", 7: " 5-hour", 8: " Week", 9: " Fable wk", 11: "2 personal ◆", 16: "3 side ▲", 22: " ┃ even pace",
		}},
		{name: "step 3", height: 24, step: 3, rows: map[int]string{
			6: "1 work ", 7: " 5-hour", 8: "2 personal ◆", 9: " 5-hour", 10: "3 side ▲", 11: " 5-hour", 16: " ┃ even pace",
		}},
		{name: "step 4", height: 19, step: 4, rows: map[int]string{
			6: "1 work      ", 7: "2 personal ◆", 9: "3 side ▲    ", 15: " ┃ even pace",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := sceneFrame(160, tt.height, Look{})
			if got := f.switchboardLaid(usageWorld(), routedAt).step; got != tt.step {
				t.Fatalf("the lines take step %d, want %d", got, tt.step)
			}
			c := f.page(usageWorld(), routedAt)
			for row, want := range tt.rows {
				if got := textFrom(c, 86, 86+len([]rune(want)), row); got != want {
					t.Errorf("row %d reads %q from the lines' column, want %q", row+1, got, want)
				}
			}
		})
	}
}

// At step 3, a line's 5-hour and week are side by side on the row under its
// name: each its label, its bar, a cell and its percentage, the two bars
// sharing the lines' width, ⌊(width − 30) ÷ 2⌋ cells each, ending at the last
// column, no words; at step 4, its 5-hour bar is 14 cells from the name
// column, and how it stands 21 cells on, short where the lines are narrower
// than 74 columns.
func TestTheCompactLinesSitSideBySideThenOnOneRow(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		row           int
		want          map[int]string
	}{
		{name: "step 3, 74 columns", width: 160, height: 24, row: 7, want: map[int]string{87: "5-hour", 121: " 95%", 128: "Week ", 156: " 34%"}},
		{name: "step 3, 59 columns", width: 130, height: 24, row: 7, want: map[int]string{72: "5-hour", 98: " 95%", 105: "Week ", 125: " 34%"}},
		{name: "step 4, 74 columns", width: 160, height: 19, row: 6, want: map[int]string{86: "1 work", 117: " 95%", 123: "● at its cap · back 17:10 "}},
		{name: "step 4, 59 columns, short", width: 130, height: 19, row: 6, want: map[int]string{71: "1 work", 102: " 95%", 108: "● at its cap · 17:10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sceneFrame(tt.width, tt.height, Look{}).page(usageWorld(), routedAt)
			for x, want := range tt.want {
				if got := textFrom(c, x, x+len([]rune(want)), tt.row); got != want {
					t.Errorf("row %d reads %q from column %d, want %q", tt.row+1, got, x, want)
				}
			}
			if got := rowOf(c, tt.row); len([]rune(got)) > tt.width {
				t.Errorf("row %d runs past the last column: %q", tt.row+1, got)
			}
		})
	}
}

// The bars the lines draw are those a watch eases, each as full as it's
// used: every window at steps 1 and 2, the 5-hour and the week at step 3,
// and the 5-hour alone from step 4.
func TestTheBarsEasedAreTheLinesBarsAtTheirStep(t *testing.T) {
	tests := []struct {
		height int
		keys   []string
	}{
		{height: 37, keys: []string{"5h", "7d", "7d_oi"}},
		{height: 29, keys: []string{"5h", "7d", "7d_oi"}},
		{height: 24, keys: []string{"5h", "7d"}},
		{height: 19, keys: []string{"5h"}},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.height), func(t *testing.T) {
			doc := usageWorld()
			got := sceneFrame(160, tt.height, Look{}).Filled(doc, routedAt)
			want := windowsFilled(doc.At(routedAt, claudeLike), func(w status.WindowNow) bool { return slices.Contains(tt.keys, w.Key) })
			if !maps.Equal(got, want) {
				t.Errorf("Filled() = %v, want %v", got, want)
			}
		})
	}
}

// On a line of one row, the pin's mark ends the row where it clears the
// percentage before it and leaves the account's words their first clause
// whole, and is left out where it doesn't: the meter is never drawn over,
// and how the account stands is never given up for the mark. At 120
// columns, side's ● open leaves its mark room, and personal's ■ limit
// reached and work's ● at its cap leave theirs none.
func TestThePinsMarkNeverDrawsOverALinesMeterOrItsFirstWords(t *testing.T) {
	doc := pinnedWorld()
	marks := map[string]string{"personal": "● pinned", "side": "● pinned", "work": "✕ out of the pool"}
	leads := map[string]string{"personal": "■ limit reached", "side": "● open", "work": "● at its cap"}
	for width := 100; width <= 120; width++ {
		for height := 15; height <= 20; height++ {
			f := boardFrame(width, height, Look{}, boardOf(boardLook{calls: pinnedCalls()}), Traffic{})
			laid := f.switchboardLaid(doc, routedAt)
			if laid.lanes || laid.step < 4 {
				continue
			}
			d := f.linesDrawn(f.Moment(doc, routedAt), laid)
			if f.sessionsForm(doc, routedAt).byRows {
				t.Fatalf("at %d×%d, the Switchboard scrolls a row at a time, want an account at a time", width, height)
			}
			for i, ln := range laid.lines {
				y := f.top() + ln.top
				if laid.step == compactedStep {
					f.Scroll = min(i, f.Scrolling(doc, routedAt).Most)
					y += min(f.Scroll, 1) - laid.lines[f.Scroll].first
				}
				c := f.page(doc, routedAt)
				at := d.x() + d.column + oneRowBar + 1
				if got := textFrom(c, at, at+percentCells, y); !strings.HasSuffix(got, "%") {
					t.Errorf("at %d×%d, %s's percentage reads %q, want it whole", width, height, ln.account, got)
				}
				row := rowOf(c, y)
				shown := strings.HasSuffix(row, marks[ln.account])
				if !shown && (strings.Contains(row, "pinned") || strings.Contains(row, "✕")) {
					t.Errorf("at %d×%d, %s's row reads %q, want its mark whole at its end, or none", width, height, ln.account, row)
				} else if width == 120 && shown != (ln.account == "side") {
					t.Errorf("at %d×%d, %s's row reads %q, its mark shown %v, want it shown on side's alone", width, height, ln.account, row, shown)
				}
				if shown && !strings.Contains(row, leads[ln.account]) {
					t.Errorf("at %d×%d, %s's row reads %q, want %q whole before its mark", width, height, ln.account, row, leads[ln.account])
				}
			}
		}
	}
}

// moreAccounts counts n more accounts, as a line saying what's out of view
// counts them.
func moreAccounts(n int) string {
	if n == 1 {
		return "1 more account"
	}
	return fmt.Sprintf("%d more accounts", n)
}

// tenOnBoard is the Switchboard of ten accounts, height rows tall, scrolled
// as given.
func tenOnBoard(height, scroll int) Frame {
	f := boardFrame(160, height, Look{}, boardOf(boardLook{calls: tenCalls()}), Traffic{})
	f.Scroll = scroll
	return f
}

// At step 5, the lines scroll an account at a time, each with its calls, the
// key giving way: how many accounts are out of view above is said on the
// view's first row, and how many below on the row over the key line, each
// right-aligned; scrolled as far as it goes, the last line shows whole.
func TestAtStep5TheLinesScrollAnAccountAtATimeWithTheirCalls(t *testing.T) {
	doc := boardOfTen()
	f := tenOnBoard(24, 0)
	laid := f.switchboardLaid(doc, routedAt)
	if laid.step != compactedStep || laid.keyAt >= 0 {
		t.Fatalf("ten accounts at 160 by 24 take step %d, the key at %d, want step 5 with no key", laid.step, laid.keyAt)
	}
	s := f.Scrolling(doc, routedAt)
	if s.Most == 0 || s.Page == 0 || s.Page >= 10 {
		t.Fatalf("Scrolling() = %+v, want it to scroll, some accounts at a time", s)
	}
	for _, offset := range []int{0, 1, s.Most, s.Most + 3} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) {
			f := tenOnBoard(24, offset)
			shown := min(offset, s.Most)
			rows := textOf(f.Draw(doc, routedAt))
			top, over := f.top(), f.Height-underView
			above, below := "", ""
			if shown > 0 {
				above = "▲ " + moreAccounts(shown)
			}
			page := f.Scrolling(doc, routedAt).Page
			if left := 10 - shown - page; left > 0 {
				below = "▼ " + moreAccounts(left)
			}
			if !strings.HasSuffix(rows[top], above) || above == "" && strings.Contains(rows[top], "▲") {
				t.Errorf("the view's first row reads %q, want it to end %q", rows[top], above)
			}
			if !strings.HasSuffix(rows[over], below) || below == "" && strings.Contains(rows[over], "▼") {
				t.Errorf("the row over the key line reads %q, want it to end %q", rows[over], below)
			}
			ln := laid.lines[shown]
			first := top + min(shown, 1)
			name := fmt.Sprintf("%2d %s", shown+1, ln.account)
			if got := rows[first+ln.top-ln.first]; !strings.Contains(got, name) {
				t.Errorf("row %d reads %q, want the first account shown, %q", first+ln.top-ln.first+1, got, name)
			}
			for _, call := range laid.calls {
				if call.account != ln.account {
					continue
				}
				if got, want := rows[first+call.cord.from-ln.first], status.ShownID(call.session); !strings.HasPrefix(got, want) {
					t.Errorf("row %d reads %q, want its call %s with it", first+call.cord.from-ln.first+1, got, want)
				}
			}
		})
	}
}

// Scrolled at step 5, what of the Switchboard's cords is scrolled out of
// view moves no frame: what travels a call's cord moves frames only while
// it shows.
func TestACordScrolledOutOfViewMovesNoFrames(t *testing.T) {
	doc := boardOfTen()
	traffic := Traffic{Calls: map[Plug]Call{{Session: idF3B1, Account: "studio"}: askingOn(3, 40)}}
	shown := tenOnBoard(24, 0)
	shown.Traffic = traffic
	most := shown.Scrolling(doc, routedAt).Most
	scrolled := tenOnBoard(24, most)
	scrolled.Traffic = traffic
	if got := shown.Motion(doc, routedAt); got != (Motion{}) {
		t.Errorf("studio's call out of view below, Motion() = %+v, want none", got)
	}
	if got := scrolled.Motion(doc, routedAt); got != (Motion{Smooth: true}) {
		t.Errorf("scrolled to studio's call, Motion() = %+v, want it smooth", got)
	}
}
