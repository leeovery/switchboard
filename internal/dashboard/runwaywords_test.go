package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayLive is Runway's live view of the window given at routedAt, 136
// columns from 13:40 of the 5-hour, ten minutes each, now's the sixth; or
// from Wednesday 14:40 of the week, about 74 minutes each, now's the 19th.
func runwayLive(w views.RunwayWindow) RunwayShown {
	return RunwayShown{Window: w, Timeline: views.LiveTimeline(w, routedAt, 136)}
}

// runwayBack is Runway's view of the window given stepped back to the day
// given, as dayfile.Civil gives one, at routedAt, 136 columns, weeks
// starting on Monday.
func runwayBack(w views.RunwayWindow, day time.Time) RunwayShown {
	return RunwayShown{Window: w, Back: day, Timeline: views.Stepped(w, day, routedAt, time.Monday, 136)}
}

// runwayShowing is a frame 160 wide showing Runway as r.
func runwayShowing(r RunwayShown) Frame {
	return Frame{Width: 160, Height: 37, View: Runway, Policy: claudeLike, Runway: r}
}

// stretchOf is a stretch held as cell over the columns from first to last of
// the timeline tl, from start, zero for one from before the timeline, back at
// back, its words from the column its start falls in, not over.
func stretchOf(tl views.Timeline, cell views.Cell, start, back time.Time, first, last int) views.LaneStretch {
	s := views.LaneStretch{Cell: cell, Start: start, Back: back, Column: first, First: first, Last: last}
	if !start.IsZero() {
		s.Column = max(tl.Column(start), 0)
	}
	return s
}

// heldOn is the stretch s held on by the stretch after it: lifting as it
// was back, the run back at back.
func heldOn(s views.LaneStretch, back time.Time) views.LaneStretch {
	s.Lifts, s.Back = s.Back, back
	return s
}

// laneWith is work's lane, read, with room but for the stretches given.
func laneWith(stretches ...views.LaneStretch) laidLane {
	return laidLane{ID: "work", Cells: make([]views.Cell, 136), Stretches: stretches, account: status.AccountNow{ID: "work", Read: true}, row: 10}
}

// wordsRead is what the words placed say, read off a row of the lanes' columns,
// as a terminal shows them.
func wordsRead(words []placed) string {
	var row []rune
	for _, w := range words {
		for len(row) < w.x {
			row = append(row, ' ')
		}
		row = append(row, []rune(w.said.plain())...)
	}
	return string(row)
}

func TestTheWordsUnderALaneSayEachStretchFromItsStart(t *testing.T) {
	five, week := runwayLive(views.RunwayFiveHour), runwayLive(views.RunwayWeek)
	tl := five.Timeline
	capped := stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20)
	capped.AsItResets = true
	ahead := stretchOf(tl, views.CellCapped, routedClock(1, 15, 45), routedClock(1, 17, 10), 12, 20)
	ahead.Projected, ahead.AsItResets = true, true
	out := stretchOf(tl, views.CellLimited, routedClock(1, 16, 5), routedClock(1, 16, 55), 14, 18)
	out.Projected, out.AsItResets = true, true
	limited := stretchOf(tl, views.CellLimited, routedClock(1, 13, 58), routedClock(1, 15, 30), 2, 10)
	limited.AsItResets = true
	refused := stretchOf(tl, views.CellRefused, routedClock(1, 14, 12), routedClock(1, 15, 0), 3, 7)
	refusing := status.AccountNow{ID: "work", Read: true, TokenRefused: true, Refusal: status.Refusal{Until: routedClock(1, 15, 0), Status: 401}}
	foreign := status.AccountNow{ID: "work", TokenForeign: true}
	weekOut := stretchOf(week.Timeline, views.CellLimited, routedClock(3, 6, 0), routedClock(4, 2, 0), 51, 66)
	weekOut.Projected, weekOut.AsItResets = true, true
	weekSoon := stretchOf(week.Timeline, views.CellLimited, routedClock(2, 6, 0), routedClock(4, 2, 0), 31, 66)
	weekSoon.Projected, weekSoon.AsItResets = true, true
	tests := []struct {
		name    string
		shown   RunwayShown
		stretch views.LaneStretch
		account *status.AccountNow
		want    string
		inks    []ink
	}{
		{
			name: "at its cap, back as its 5-hour resets, said as it resets", shown: five, stretch: capped,
			want: "capped 14:31  ·  resets 17:10", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "at its cap, back otherwise than as its window resets", shown: five, stretch: stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20),
			want: "capped 14:31  ·  back 17:10", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "a run-out reaching its cap first", shown: five, stretch: ahead,
			want: "caps ~15:45  ·  resets 17:10", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "a run-out reaching its limit", shown: five, stretch: out,
			want: "out ~16:05  ·  resets 16:55", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "at its limit, its back never as its window resets", shown: five, stretch: limited,
			want: "out 13:58  ·  back 15:30", inks: []ink{exhaustedInk, mutedInk},
		},
		{
			name: "at its limit, held on by the stretch after it, lifting", shown: five, stretch: heldOn(limited, routedClock(4, 21, 0)),
			want: "out 13:58  ·  lifts 15:30", inks: []ink{exhaustedInk, mutedInk},
		},
		{
			name: "at its cap, held on by the stretch after it, lifting as its 5-hour resets, said as it lifts", shown: five, stretch: heldOn(capped, routedClock(4, 21, 0)),
			want: "capped 14:31  ·  lifts 17:10", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "at its limit from before the timeline", shown: five, stretch: stretchOf(tl, views.CellLimited, time.Time{}, routedClock(1, 15, 30), 0, 10),
			want: "out  ·  back 15:30", inks: []ink{exhaustedInk, mutedInk},
		},
		{
			name: "at its limit, back at no time known", shown: five, stretch: stretchOf(tl, views.CellLimited, routedClock(1, 13, 58), time.Time{}, 2, 135),
			want: "out 13:58", inks: []ink{exhaustedInk},
		},
		{
			name: "its token refused, the refusal holding it", shown: five, stretch: refused, account: &refusing,
			want: "token refused (401) 14:12  ·  back 15:00", inks: []ink{exhaustedInk, mutedInk},
		},
		{
			name: "its token refused, a refusal since gone", shown: five, stretch: refused,
			want: "token refused 14:12  ·  back 15:00", inks: []ink{exhaustedInk, mutedInk},
		},
		{
			name: "without a token", shown: five, stretch: stretchOf(tl, views.CellTokenless, time.Time{}, time.Time{}, 0, 135),
			want: "no token", inks: []ink{exhaustedInk},
		},
		{
			name: "its token another subscription's", shown: five, stretch: stretchOf(tl, views.CellTokenless, time.Time{}, time.Time{}, 0, 135), account: &foreign,
			want: "wrong token", inks: []ink{exhaustedInk},
		},
		{
			name: "the week's run-out more than a day ahead, by its day alone", shown: week, stretch: weekOut,
			want: "out ~Sat  ·  resets Sun 02:00", inks: []ink{alertInk, mutedInk},
		},
		{
			name: "the week's run-out within a day, by its time", shown: week, stretch: weekSoon,
			want: "out ~Fri 06:00  ·  resets Sun 02:00", inks: []ink{alertInk, mutedInk},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln := laneWith(tt.stretch)
			if tt.account != nil {
				ln.account = *tt.account
			}
			words := tt.shown.view().wordsUnder(ln, routedAt, 0)
			if len(words) != 1 || words[0].said.plain() != tt.want || words[0].x != tt.stretch.Column {
				t.Fatalf("the words say %q, want %q from column %d", wordsRead(words), tt.want, tt.stretch.Column)
			}
			var inks []ink
			for _, s := range words[0].said {
				inks = append(inks, s.ink)
			}
			if !slices.Equal(inks, tt.inks) {
				t.Errorf("the words' parts are in %v, want %v", inks, tt.inks)
			}
		})
	}
}

// Today stepped back to is drawn as a day stepped back to, its lanes as views
// build them from the readings and the router's events: the words under a
// lane say each stretch it held, those over before now's column among them,
// the status its token was refused with only of the refusal holding it now.
func TestTodaySteppedBackToSaysEachStretchOverBeforeNow(t *testing.T) {
	at := func(hour, minute int) time.Time { return routedClock(1, hour, minute) }
	week := routedClock(4, 21, 0)
	fiveHour := func(used float64, resets time.Time, st quota.Status) quota.Window {
		return quota.Window{Key: "5h", Utilization: used, ResetsAt: resets, Status: st}
	}
	read := func(id string, when time.Time, windows ...quota.Window) []readings.Reading {
		return readings.Of(id, windows, when, readings.FromAnswer)
	}
	weekOf := quota.Window{Key: "7d", Utilization: 0.3, ResetsAt: week}
	open := []quota.Window{fiveHour(0.2, at(15, 0), ""), weekOf}
	doc := func(work status.Account, events ...status.Event) status.Document {
		work.ID, work.TokenSet, work.FetchedAt, work.Windows = "work", true, routedAt.Add(-time.Minute), open
		return status.Document{GeneratedAt: routedAt, Source: status.SourceRouter, Events: events, Accounts: []status.Account{
			work, {ID: "side", TokenSet: true, FetchedAt: routedAt.Add(-time.Minute), Windows: open},
		}}.WorkedOut(claudeLike)
	}
	both := slices.Concat(read("work", at(8, 0), fiveHour(0.5, at(10, 0), ""), weekOf), read("side", at(8, 0), open...))
	refused := func(id int, from, until time.Time) status.Event {
		return status.Event{ID: id, At: from, Kind: status.EventRefused, Account: "work", Until: until, Status: 401}
	}
	tests := []struct {
		name  string
		doc   status.Document
		read  []readings.Reading
		words string
	}{
		{
			name:  "at its limit this morning, room since",
			doc:   doc(status.Account{}),
			read:  slices.Concat(both, read("work", at(9, 0), fiveHour(1, at(10, 0), quota.StatusRejected)), read("work", at(10, 5), open...)),
			words: strings.Repeat(" ", 51) + "out 09:00  ·  back 10:00",
		},
		{
			name:  "its token refused this morning, and again now",
			doc:   doc(status.Account{Refused: status.Refusal{Until: at(15, 0), Status: 401}}, refused(2, at(14, 12), at(15, 0)), refused(1, at(9, 0), at(9, 30))),
			read:  both,
			words: strings.Repeat(" ", 51) + "token refused 09:00  ·  back 09:30  token refused (401) 14:12  ·  back 15:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := runwayBack(views.RunwayFiveHour, civilDay(10, 1))
			m := tt.doc.At(routedAt, claudeLike)
			l := views.Lanes{}.Look(views.LanesFrom{Moment: m, Readings: tt.read, Timeline: shown.Timeline, Window: shown.Window})
			shown.Lanes = &l
			ln := laidLane{Lane: l.Accounts[0], account: m.Accounts[0], row: 10}
			if got := wordsRead(shown.view().wordsUnder(ln, routedAt, 0)); got != tt.words {
				t.Errorf("the words under work's lane say\n%q, want\n%q", got, tt.words)
			}
		})
	}
}

// A cap outlasting what holds its account back first has words of its own,
// after that hold's: each stretch said from its own start; the hold it
// outlasts said to lift as it does, and the cap alone, the run's last, back
// as the account next has room, as ROUTING says it; the status the token's
// refused with said of the refusal holding it now.
func TestAHoldOutlastedIsSaidFromItsStartAndOnlyTheLastIsBack(t *testing.T) {
	tests := []struct {
		moment string
		want   string
	}{
		{moment: "a cap outlasting a limit", want: "  out 14:00  ·  lifts 15:30  capped 14:00  ·  back Mon 21:00"},
		{moment: "a cap outlasting a refusal", want: "token refused (401)  ·  lifts 14:52  capped 14:00  ·  back Mon 21:00"},
		{
			moment: "stepped back to a cap lifting as its week resets, a limit holding it on",
			want:   strings.Repeat(" ", 93) + "capped Fri 12:00  ·  lifts Sat 21:00  out Sat 20:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.moment, func(t *testing.T) {
			f, doc, now := runwayFramed(t, tt.moment)
			m := doc.At(now, claudeLike)
			ln := laidLane{Lane: f.Runway.Lanes.Accounts[0], account: m.Accounts[0], row: 10}
			if got := wordsRead(f.Runway.view().wordsUnder(ln, now, 0)); got != tt.want {
				t.Errorf("the words under work's lane say\n%q, want\n%q", got, tt.want)
			}
		})
	}
}

func TestTheWordsUnderALaneArePlacedFromLeftToRightShiftingLeftToFitWhole(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	tl := five.Timeline
	at := func(cell views.Cell, start, back time.Time, first, last int) views.LaneStretch {
		s := stretchOf(tl, cell, start, back, first, last)
		s.AsItResets = cell == views.CellCapped
		return s
	}
	over := at(views.CellLimited, routedClock(1, 13, 40), routedClock(1, 14, 0), 0, 1)
	over.Over = true
	tests := []struct {
		name      string
		stretches []views.LaneStretch
		want      string
	}{
		{
			name:      "one over, before now's column, not said",
			stretches: []views.LaneStretch{over, at(views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20)},
			want:      "     capped 14:31  ·  resets 17:10",
		},
		{
			name:      "a start in a column held otherwise, said from it",
			stretches: []views.LaneStretch{at(views.CellLimited, routedClock(1, 13, 58), routedClock(1, 15, 30), 2, 10)},
			want:      " out 13:58  ·  back 15:30",
		},
		{
			name: "one too close to the words before, two cells after them",
			stretches: []views.LaneStretch{
				at(views.CellLimited, routedClock(1, 14, 42), routedClock(1, 15, 0), 6, 7),
				at(views.CellCapped, routedClock(1, 15, 0), routedClock(1, 17, 10), 8, 20),
			},
			want: "      out 14:42  ·  back 15:00  capped 15:00  ·  resets 17:10",
		},
		{
			name: "one ending at the row's last column, unmoved",
			stretches: []views.LaneStretch{
				at(views.CellLimited, routedClock(2, 7, 0), routedClock(2, 8, 50), 104, 114),
			},
			want: strings.Repeat(" ", 104) + "out Fri 07:00  ·  back Fri 08:50",
		},
		{
			name: "one near the end, shifted left to fit whole",
			stretches: []views.LaneStretch{
				at(views.CellCapped, routedClock(2, 10, 30), routedClock(2, 11, 50), 125, 132),
			},
			want: strings.Repeat(" ", 99) + "capped Fri 10:30  ·  resets Fri 11:50",
		},
		{
			name: "one with room from its column for its first part alone, shifted left to fit whole",
			stretches: []views.LaneStretch{
				at(views.CellCapped, routedClock(2, 8, 30), routedClock(2, 10, 50), 113, 128),
			},
			want: strings.Repeat(" ", 99) + "capped Fri 08:30  ·  resets Fri 10:50",
		},
		{
			name: "one shifted left to fit whole, leaving none after it room",
			stretches: []views.LaneStretch{
				at(views.CellCapped, routedClock(2, 11, 0), routedClock(2, 11, 50), 128, 132),
				at(views.CellLimited, routedClock(2, 12, 0), time.Time{}, 134, 135),
			},
			want: strings.Repeat(" ", 99) + "capped Fri 11:00  ·  resets Fri 11:50",
		},
		{
			name: "one not fitting whole even two cells after the words before, from there, its last parts left off",
			stretches: []views.LaneStretch{
				at(views.CellLimited, routedClock(2, 5, 0), time.Time{}, 92, 100),
				at(views.CellCapped, routedClock(2, 11, 0), routedClock(2, 11, 50), 128, 132),
			},
			want: strings.Repeat(" ", 92) + "out Fri 05:00  capped Fri 11:00",
		},
		{
			name: "one whose first part doesn't fit even two cells after the words before, unsaid, nor any after it",
			stretches: []views.LaneStretch{
				at(views.CellLimited, routedClock(2, 7, 40), time.Time{}, 108, 110),
				at(views.CellCapped, routedClock(2, 11, 0), routedClock(2, 11, 50), 128, 132),
				at(views.CellTokenless, time.Time{}, time.Time{}, 130, 135),
			},
			want: strings.Repeat(" ", 108) + "out Fri 07:40",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordsRead(five.view().wordsUnder(laneWith(tt.stretches...), routedAt, 0)); got != tt.want {
				t.Errorf("the words say\n%q, want\n%q", got, tt.want)
			}
		})
	}
}

func TestTheWeekViewSaysWhenAWeekResetsEndingAtItsMarkWhereThatClearsTheWordsBefore(t *testing.T) {
	week := runwayLive(views.RunwayWeek)
	tl := week.Timeline
	sunday := views.Reset{Column: 82, At: routedClock(4, 21, 0)}
	capped := stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 19, 20)
	tests := []struct {
		name  string
		shown RunwayShown
		lane  laidLane
		want  string
	}{
		{
			name: "after a stretch it clears", shown: week, lane: withResets(laneWith(capped), sunday),
			want: strings.Repeat(" ", 19) + "capped 14:31  ·  back 17:10" + strings.Repeat(" ", 21) + "resets Sun 21:00",
		},
		{
			name: "where it doesn't clear the words before, unsaid", shown: week,
			lane: withResets(laneWith(stretchOf(tl, views.CellCapped, routedClock(4, 0, 0), routedClock(4, 9, 0), 66, 73)), sunday),
			want: strings.Repeat(" ", 65) + "capped Sun 00:00  ·  back Sun 09:00",
		},
		{
			name: "a reset a stretch ends at, unsaid, as the stretch's words say it", shown: week,
			lane: withResets(laneWith(stretchOf(tl, views.CellLimited, time.Time{}, routedClock(4, 21, 0), 0, 81)), views.Reset{Column: 82, At: routedClock(4, 21, 0), Back: true}),
			want: "out  ·  back Sun 21:00",
		},
		{
			name: "in the 5-hour view, never", shown: runwayLive(views.RunwayFiveHour),
			lane: withResets(laneWith(stretchOf(runwayLive(views.RunwayFiveHour).Timeline, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20)), views.Reset{Column: 21, At: routedClock(1, 17, 10)}),
			want: "     capped 14:31  ·  back 17:10",
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

// withResets is the lane ln marking the resets given.
func withResets(ln laidLane, resets ...views.Reset) laidLane {
	ln.Resets = resets
	return ln
}

func TestALaneWithNothingElseToSaySaysWhatItHas(t *testing.T) {
	wednesday := civilDay(9, 30)
	roomy := laneWith()
	roomy.Room = true
	unreadable := laidLane{ID: "work", Cells: make([]views.Cell, 136), account: status.AccountNow{
		ID: "work", Routing: status.Routing{Kind: status.RoutingUnreadable, Says: "can't read it", Clauses: []string{"HTTP 529 · Overloaded"}},
	}}
	unknown := laidLane{ID: "work", Cells: make([]views.Cell, 136)}
	tests := []struct {
		name  string
		shown RunwayShown
		lane  laidLane
		want  string
		ink   ink
	}{
		{name: "room all day, from the cell after now's column", shown: runwayLive(views.RunwayFiveHour), lane: withRoom(roomy), want: "       room all day", ink: dimInk},
		{name: "room all week", shown: runwayLive(views.RunwayWeek), lane: withRoom(roomy), want: strings.Repeat(" ", 20) + "room all week", ink: dimInk},
		{name: "room all day, stepped back, from the first column", shown: runwayBack(views.RunwayFiveHour, wednesday), lane: withRoom(roomy), want: "room all day", ink: dimInk},
		{name: "room all day, stepped back to today, from the first column", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), lane: withRoom(roomy), want: "room all day", ink: dimInk},
		{name: "a lane without room throughout, nothing", shown: runwayLive(views.RunwayFiveHour), lane: unknown, want: ""},
		{
			name: "a first read failed, that it can't be read, and why", shown: runwayLive(views.RunwayFiveHour), lane: unreadable,
			want: "can't read it: HTTP 529 · Overloaded", ink: dimInk,
		},
		{name: "a day gone that read nothing of it", shown: runwayBack(views.RunwayFiveHour, wednesday), lane: unknown, want: "nothing read that day", ink: faintInk},
		{
			name: "a day gone that read nothing of it, not why it can't be read now", shown: runwayBack(views.RunwayFiveHour, wednesday), lane: unreadable,
			want: "nothing read that day", ink: faintInk,
		},
		{name: "today stepped back to, not read yet, nothing", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), lane: unknown, want: ""},
		{name: "a week gone that read nothing of it", shown: runwayBack(views.RunwayWeek, civilDay(9, 21)), lane: unknown, want: "nothing read that week", ink: faintInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := tt.shown.view().wordsUnder(tt.lane, routedAt, 0)
			if got := wordsRead(words); got != tt.want {
				t.Fatalf("the words say %q, want %q", got, tt.want)
			}
			if len(words) > 0 && words[0].said[0].ink != tt.ink {
				t.Errorf("the words are in %v, want %v", words[0].said[0].ink, tt.ink)
			}
		})
	}
}

// withRoom is the lane ln with room in every column.
func withRoom(ln laidLane) laidLane {
	ln.Cells = slices.Repeat([]views.Cell{views.CellRoom}, len(ln.Cells))
	return ln
}

// A lane's times are told against the day shown: live, today's times of
// day, any other day's with its weekday; a day gone, its own times of day,
// and another day's with its weekday.
func TestALanesTimesAreToldAgainstTheDayShown(t *testing.T) {
	wednesday := runwayBack(views.RunwayFiveHour, civilDay(9, 30))
	overMidnight := stretchOf(wednesday.Timeline, views.CellLimited, time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC), routedClock(1, 1, 10), 85, 135)
	live := runwayLive(views.RunwayFiveHour)
	tests := []struct {
		name    string
		shown   RunwayShown
		stretch views.LaneStretch
		want    string
	}{
		{name: "a day gone, its own times of day, the next day's with its weekday", shown: wednesday, stretch: overMidnight, want: "out 15:00  ·  back Thu 01:10"},
		{name: "live, today's times of day", shown: live, stretch: stretchOf(live.Timeline, views.CellLimited, routedClock(1, 13, 58), routedClock(1, 15, 30), 2, 10), want: "out 13:58  ·  back 15:30"},
		{name: "live, tomorrow's with its weekday", shown: live, stretch: stretchOf(live.Timeline, views.CellLimited, routedClock(2, 1, 0), routedClock(2, 3, 0), 68, 79), want: "out Fri 01:00  ·  back Fri 03:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.TrimSpace(wordsRead(tt.shown.view().wordsUnder(withRoom(laneWith(tt.stretch)), routedAt, 0))); got != tt.want {
				t.Errorf("the words say %q, want %q", got, tt.want)
			}
		})
	}
}

func TestALanesWordsAreCuedAsItsAccountNewlyStandsSo(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	tl := five.Timeline
	capped := laneWith(stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20))
	capped.Cells = slices.Repeat([]views.Cell{views.CellCapped}, 136)
	out := stretchOf(tl, views.CellLimited, routedClock(1, 16, 5), routedClock(1, 16, 55), 14, 18)
	out.Projected = true
	runsOut := laneWith(out)
	runsOut.Cells = slices.Repeat([]views.Cell{views.CellRunsOut}, 136)
	runsOut.account.Windows = []status.WindowNow{{Key: "5h", Shared: true}, {Key: "7d_oi"}}
	cappedSharing := capped
	cappedSharing.account.Windows = runsOut.account.Windows
	tests := []struct {
		name      string
		lane      laidLane
		changed   map[string]float64
		turnedOut map[Ref]float64
		want      float64
		cued      bool
	}{
		{name: "newly at its cap", lane: capped, changed: map[string]float64{"work": 0.25}, want: 0.25, cued: true},
		{name: "newly changed, with room at now", lane: laneWith(), changed: map[string]float64{"work": 0.25}},
		{name: "its window newly running out", lane: runsOut, turnedOut: map[Ref]float64{{Account: "work", Window: "5h"}: 0.5}, want: 0.5, cued: true},
		{name: "a model's own window newly running out", lane: runsOut, turnedOut: map[Ref]float64{{Account: "work", Window: "7d_oi"}: 0.5}},
		{name: "its window newly running out, where the lane draws none", lane: cappedSharing, turnedOut: map[Ref]float64{{Account: "work", Window: "5h"}: 0.5}},
		{name: "another account newly at its cap", lane: capped, changed: map[string]float64{"side": 0.25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(five)
			f.Changed, f.TurnedOut = tt.changed, tt.turnedOut
			if got, cued := f.wordsCue(tt.lane, tl.NowColumn()); cued != tt.cued || cued && got != tt.want {
				t.Errorf("wordsCue = %v, %t, want %v, %t", got, cued, tt.want, tt.cued)
			}
		})
	}
}

// The words under a lane are drawn on the row under it, from the lanes'
// first column, and cued, every cell from the first word to the last, on
// bg.attention as far faded as the cue has.
func TestTheWordsUnderALaneAreDrawnOnTheRowUnderItCuedWhereTheyAre(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	tl := five.Timeline
	ln := laneWith(
		stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 15, 0), 5, 7),
		stretchOf(tl, views.CellLimited, routedClock(1, 16, 0), routedClock(1, 17, 0), 14, 19),
	)
	ln.Cells = slices.Repeat([]views.Cell{views.CellCapped}, 136)
	f := runwayShowing(five)
	f.Look, f.Changed = Screen(builtin(t, "nord")), map[string]float64{"work": 0.25}
	c := newCanvas(160, 20)
	f.runwayWords(c, runwayLaid{m: status.Moment{Now: routedAt}, x: framesLanesFrom, lanes: []laidLane{ln}})
	words := "capped 14:31  ·  back 15:00  out 16:00  ·  back 17:00"
	if got := textAt(c, framesLanesFrom+5, 11, 136); got != words {
		t.Fatalf("the row under the lane reads %q, want %q", got, words)
	}
	cued := hue{token: theme.BgAttention, fade: 0.25}
	for x := range 160 {
		inWords := x >= framesLanesFrom+5 && x < framesLanesFrom+5+utf8.RuneCountInString(words)
		if got := c.at(x, 11).ink.on; inWords != (got == cued) {
			t.Errorf("column %d is on %v; cued %t, want %t", x, got, got == cued, inWords)
		}
	}
}

// With the account column collapsed, the words under a lane share their row
// with its use, from column 0: each still starts under its stretch, but where
// that falls in the use or within phraseGap cells of its end, phraseGap after
// it instead, as after the words before; words starting further on keep
// their column. Words that don't fit from where they start drop whole parts
// from the end, and a reset that can't end at its mark clear of the use goes
// unsaid, as one that can't clear the words before.
func TestCollapsedTheWordsUnderALaneStartTwoCellsAfterItsUseOnlyWhereTheyWouldMeetIt(t *testing.T) {
	five, week := runwayLive(views.RunwayFiveHour), runwayLive(views.RunwayWeek)
	read := func(used float64) views.Use { return views.Use{Key: "5h", Used: used, Read: true} }
	limited := func(r RunwayShown, start time.Time, first int) views.LaneStretch {
		return stretchOf(r.Timeline, views.CellLimited, start, routedClock(1, 15, 30), first, first+10)
	}
	narrow := func(cols int) RunwayShown {
		r := five
		r.Timeline = views.LiveTimeline(views.RunwayFiveHour, routedAt, cols)
		return r
	}
	roomy := withRoom(laneWith())
	roomy.Room = true
	tests := []struct {
		name  string
		shown RunwayShown
		lane  laidLane
		use   views.Use
		want  string
	}{
		{name: "out from the left edge, two cells after the use", shown: five, lane: laneWith(limited(five, routedClock(1, 13, 40), 0)), use: read(1), want: "100%  out 13:40  ·  back 15:30"},
		{name: "starting inside the use", shown: five, lane: laneWith(limited(five, routedClock(1, 14, 0), 2)), use: read(0.34), want: "34%  out 14:00  ·  back 15:30"},
		{name: "starting a cell after the use", shown: five, lane: laneWith(limited(five, routedClock(1, 14, 20), 4)), use: read(0.34), want: "34%  out 14:20  ·  back 15:30"},
		{name: "starting two cells after the use, unmoved", shown: five, lane: laneWith(limited(five, routedClock(1, 14, 31), 5)), use: read(0.34), want: "34%  out 14:31  ·  back 15:30"},
		{name: "starting further on, unmoved", shown: five, lane: laneWith(limited(five, routedClock(1, 15, 0), 8)), use: read(0.34), want: "34%" + strings.Repeat(" ", 5) + "out 15:00  ·  back 15:30"},
		{name: "starting far to the right, unmoved", shown: five, lane: laneWith(limited(five, routedClock(1, 23, 40), 60)), use: read(0.34), want: "34%" + strings.Repeat(" ", 57) + "out 23:40  ·  back 15:30"},
		{name: "no use to show, two cells after its —", shown: five, lane: laneWith(limited(five, routedClock(1, 13, 40), 0)), use: views.Use{Key: "5h", Unread: true}, want: "—  out 13:40  ·  back 15:30"},
		{name: "room all day from after now, unmoved", shown: five, lane: roomy, use: read(0.34), want: "34%    room all day"},
		{name: "room all day stepped back, from the first column, after the use", shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), lane: roomy, use: read(0.34), want: "34%  room all day"},
		{name: "narrow, every part fitting after the use", shown: narrow(30), lane: laneWith(limited(five, routedClock(1, 13, 40), 0)), use: read(1), want: "100%  out 13:40  ·  back 15:30"},
		{name: "narrower, its last part left off", shown: narrow(29), lane: laneWith(limited(five, routedClock(1, 13, 40), 0)), use: read(1), want: "100%  out 13:40"},
		{name: "a reset that can't end at its mark clear of the use, unsaid", shown: week, lane: withResets(laneWith(), views.Reset{Column: 18, At: routedClock(4, 21, 0)}), use: read(0.34), want: "34%"},
		{name: "a reset ending clear of it, said", shown: week, lane: withResets(laneWith(), views.Reset{Column: 30, At: routedClock(4, 21, 0)}), use: read(0.34), want: "34%" + strings.Repeat(" ", 12) + "resets Sun 21:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.shown
			r.Collapsed = true
			f := runwayShowing(r)
			ln := tt.lane
			ln.Use = tt.use
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

// Expanded, the words under a lane start under their stretch, from the
// lanes' first column, as its use is under its name.
func TestExpandedTheWordsUnderALaneStartUnderTheirStretch(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	ln := laneWith(stretchOf(five.Timeline, views.CellLimited, routedClock(1, 13, 40), routedClock(1, 15, 30), 0, 10))
	ln.Use = views.Use{Key: "5h", Used: 1, Read: true}
	f := runwayShowing(five)
	c := newCanvas(f.Width, 20)
	f.runwayWords(c, runwayLaid{m: status.Moment{Now: routedAt}, x: framesLanesFrom, lanes: []laidLane{ln}})
	if got, want := rowOf(c, 11), strings.Repeat(" ", framesLanesFrom)+"out 13:40  ·  back 15:30"; got != want {
		t.Errorf("the row under the lane reads\n%q, want\n%q", got, want)
	}
}
