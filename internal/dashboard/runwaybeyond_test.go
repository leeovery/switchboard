package dashboard

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// pastTheView is a stretch held as cell past the end of the timeline tl, as
// a lane's Beyond has one: a run-out from start, back as the week resets at
// back, its words from the column after the last.
func pastTheView(tl views.Timeline, cell views.Cell, start, back time.Time) views.LaneStretch {
	return views.LaneStretch{Cell: cell, Start: start, Projected: true, Back: back, AsItResets: true, Column: tl.Columns, First: tl.Columns, Last: tl.Columns}
}

// beyondLane is work's lane, read, over the timeline tl, with room through
// every column but for the stretches given, and the stretches past its end
// given, so without room all week.
func beyondLane(tl views.Timeline, stretches []views.LaneStretch, beyond ...views.LaneStretch) laidLane {
	ln := withRoom(laneWith(stretches...))
	ln.Cells = ln.Cells[:tl.Columns]
	ln.Beyond = beyond
	return ln
}

// A lane whose week runs out, or reaches its cap, past the view's end, before
// it resets, says so under it, as it would were it drawn on: ending at the
// lane's last column, shifted no further left than phraseGap after the words
// before, whole parts left off its end where it doesn't fit from there, and
// unsaid where its first part doesn't, nothing said in its place, as the
// lane hasn't room all week; at a phone's width, Runway's lanes 38 columns of
// 52, alike; and this week stepped back to, past its last day, alike.
func TestALaneSaysWhatItHasPastItsViewsEndEndingAtItsLastColumn(t *testing.T) {
	week, phone, narrow := runwayLive(), RunwayShown{Timeline: views.LiveTimeline(routedAt, 38)}, RunwayShown{Timeline: views.LiveTimeline(routedAt, 6)}
	back := runwayBack(civilDay(9, 28))
	tl := week.Timeline
	reset := routedClock(8, 2, 0)
	out := pastTheView(tl, views.CellLimited, routedClock(7, 20, 0), reset)
	caps := pastTheView(tl, views.CellCapped, routedClock(7, 20, 0), reset)
	near := stretchBetween(tl, views.CellCapped, routedClock(5, 8, 0), routedClock(5, 12, 0))
	nearer := stretchBetween(tl, views.CellCapped, routedClock(5, 10, 0), routedClock(5, 14, 0))
	tests := []struct {
		name  string
		shown RunwayShown
		lane  laidLane
		want  string
	}{
		{name: "running out", shown: week, lane: beyondLane(tl, nil, out), want: strings.Repeat(" ", 107) + "out ~Wed  ·  resets Thu 02:00"},
		{name: "reaching its cap", shown: week, lane: beyondLane(tl, nil, caps), want: strings.Repeat(" ", 106) + "caps ~Wed  ·  resets Thu 02:00"},
		{
			name: "after a stretch near the end, its last part left off", shown: week, lane: beyondLane(tl, []views.LaneStretch{near}, out),
			want: strings.Repeat(" ", 91) + "capped Mon 08:00  ·  back Mon 12:00  out ~Wed",
		},
		{
			name: "after a stretch too near the end for its first part, unsaid", shown: week, lane: beyondLane(tl, []views.LaneStretch{nearer}, out),
			want: strings.Repeat(" ", 93) + "capped Mon 10:00  ·  back Mon 14:00",
		},
		{
			name: "at a phone's width", shown: phone,
			lane: beyondLane(phone.Timeline, nil, pastTheView(phone.Timeline, views.CellLimited, routedClock(7, 20, 0), reset)),
			want: strings.Repeat(" ", 9) + "out ~Wed  ·  resets Thu 02:00",
		},
		{
			name: "too narrow for even its first part, nothing", shown: narrow,
			lane: beyondLane(narrow.Timeline, nil, pastTheView(narrow.Timeline, views.CellLimited, routedClock(7, 20, 0), reset)),
			want: "",
		},
		{
			name: "this week stepped back to, past its last day", shown: back,
			lane: beyondLane(back.Timeline, nil, pastTheView(back.Timeline, views.CellLimited, routedClock(6, 4, 0), routedClock(7, 20, 0))),
			want: strings.Repeat(" ", 107) + "out ~Tue  ·  resets Wed 20:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordsRead(tt.shown.view().wordsUnder(tt.lane, routedAt, 0)); got != tt.want {
				t.Errorf("the words say\n%q, want\n%q", got, tt.want)
			}
		})
	}
}

// The words under a lane of a run-out past the view's end are cued as one
// inside it is: while its week newly runs out.
func TestALanesWordsOfARunOutPastItsViewAreCuedAsItNewlyRunsOut(t *testing.T) {
	week := runwayLive()
	tl := week.Timeline
	ln := beyondLane(tl, nil, pastTheView(tl, views.CellLimited, routedClock(7, 20, 0), routedClock(8, 2, 0)))
	ln.account.Windows = []status.WindowNow{{Key: "7d", Shared: true}}
	f := runwayShowing(week)
	f.Look, f.TurnedOut = Screen(builtin(t, "nord")), map[Ref]float64{{Account: "work", Window: "7d"}: 0.5}
	c := newCanvas(160, 20)
	f.runwayWords(c, runwayLaid{m: status.Moment{Now: routedAt}, x: framesLanesFrom, lanes: []laidLane{ln}})
	words := "out ~Wed  ·  resets Thu 02:00"
	width := utf8.RuneCountInString(words)
	from := framesLanesFrom + tl.Columns - width
	if got := textAt(c, from, 11, width); got != words {
		t.Fatalf("the row under the lane reads %q, want %q", got, words)
	}
	cued := hue{token: theme.BgAttention, fade: 0.5}
	for x := range 160 {
		inWords := x >= from && x < from+width
		if got := c.at(x, 11).ink.on; inWords != (got == cued) {
			t.Errorf("column %d is on %v; cued %t, want %t", x, got, got == cued, inWords)
		}
	}
}

// Collapsed, the words of a lane past its view's end still end at its last
// column where they clear its use; where they'd meet it, they start
// phraseGap after it instead, whole parts left off their end.
func TestCollapsedALaneSaysWhatItHasPastItsViewsEndAfterItsUse(t *testing.T) {
	tests := []struct {
		name    string
		columns int
		want    string
	}{
		{name: "clear of its use, ending at its last column", columns: 38, want: "34%" + strings.Repeat(" ", 6) + "out ~Wed  ·  resets Thu 02:00"},
		{name: "meeting its use, after it, its last part left off", columns: 30, want: "34%  out ~Wed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RunwayShown{Timeline: views.LiveTimeline(routedAt, tt.columns), Collapsed: true}
			f := runwayShowing(r)
			ln := beyondLane(r.Timeline, nil, pastTheView(r.Timeline, views.CellLimited, routedClock(7, 20, 0), routedClock(8, 2, 0)))
			ln.Use = views.Use{Key: "7d", Used: 0.34, Read: true}
			c := newCanvas(f.Width, 20)
			laid := runwayLaid{m: status.Moment{Now: routedAt}, x: r.lanesFrom(f.Width), lanes: []laidLane{ln}}
			f.runwayLanes(c, laid)
			f.runwayWords(c, laid)
			if got := rowOf(c, 11); got != tt.want {
				t.Errorf("the row under the lane reads\n%q, want\n%q", got, tt.want)
			}
		})
	}
}
