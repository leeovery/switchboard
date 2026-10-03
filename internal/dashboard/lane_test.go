package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// clockAt is a time on the day the frames are drawn.
func clockAt(hour, minute int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
}

// weekRunningOut is a week resetting in resets, used so that at the pace its
// use since it started sets, it runs out in out.
func weekRunningOut(out, resets time.Duration) quota.Window {
	passed := 7*day - resets
	return weekOf(passed.Hours()/(passed+out).Hours(), resets)
}

// limitedSince is an account whose session reached its limit at reached, as
// the router told of it, which holds until it resets in an hour, and whose
// week, resetting in three days, runs out in twelve hours.
func limitedSince(id string, reached time.Time) (status.Account, status.Event) {
	session := sessionOf(1, time.Hour)
	session.Status = quota.StatusRejected
	a := readAccount(id, session, weekRunningOut(12*time.Hour, 3*day))
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: session.ResetsAt}
	return a, status.Event{ID: 1, At: reached.UTC(), Kind: status.EventLimit, Account: id, Windows: []string{"5h"}, Until: session.ResetsAt}
}

func TestTheTimelineOfEachSpan(t *testing.T) {
	tests := []struct {
		name    string
		span    Span
		columns int
		start   time.Time
		step    time.Duration
		// wantNow is the column now falls in.
		wantNow int
	}{
		{name: "the day at 160 columns: from the hour before now, on the ten minutes, a column every 10", span: Day, columns: 136, start: clockAt(12, 10), step: 10 * time.Minute, wantNow: 6},
		{name: "the day narrower: more minutes a column, spanning as long", span: Day, columns: 68, start: clockAt(12, 10), step: 20 * time.Minute, wantNow: 3},
		{name: "the day wider: fewer", span: Day, columns: 170, start: clockAt(12, 10), step: 8 * time.Minute, wantNow: 7},
		{name: "the week: from a day before now, a column about every 75 minutes", span: Week, columns: 136, start: now.Add(-24 * time.Hour), step: weekSpan / 136, wantNow: 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := timelineOf(tt.span, now, tt.columns)
			if !tl.start.Equal(tt.start) || tl.step != tt.step || tl.columns != tt.columns {
				t.Errorf("timeline from %s, a column every %s across %d, want from %s, every %s", tl.start.Format(time.DateTime), tl.step, tl.columns, tt.start.Format(time.DateTime), tt.step)
			}
			if got := tl.nowColumn(); got != tt.wantNow {
				t.Errorf("now falls in column %d, want %d", got, tt.wantNow)
			}
		})
	}
}

func TestATimelinesColumnsStandForTheirMiddlesButNowsForNow(t *testing.T) {
	tl := timelineOf(Day, now, 136)
	for _, tt := range []struct {
		col  int
		want time.Time
	}{
		{col: 0, want: clockAt(12, 15)},
		{col: 5, want: clockAt(13, 5)},
		{col: 6, want: now},
		{col: 7, want: clockAt(13, 25)},
	} {
		if got := tl.moment(tt.col); !got.Equal(tt.want) {
			t.Errorf("column %d stands for %s, want %s", tt.col, got.Format(time.TimeOnly), tt.want.Format(time.TimeOnly))
		}
	}
	for _, tt := range []struct {
		at   time.Time
		want int
	}{
		{at: clockAt(12, 10), want: 0},
		{at: clockAt(12, 9), want: -1},
		{at: clockAt(14, 42), want: 15},
	} {
		if got := tl.column(tt.at); got != tt.want {
			t.Errorf("%s falls in column %d, want %d", tt.at.Format(time.TimeOnly), got, tt.want)
		}
	}
}

func TestALanesRoomFromEachCause(t *testing.T) {
	limited, reached := limitedSince("personal", clockAt(12, 50))
	probedLimit, _ := limitedSince("personal", clockAt(12, 50))
	noneNamed := probedLimit
	noneNamed.Limit.Windows = nil
	atReserve := reserving(readAccount("work", sessionOf(0.9, 3*time.Hour), weekOf(0.34, 4*day)), 0.1)
	atReserve.AtReserve = []string{"5h"}
	headingThere := reserving(pressedAccount("work"), 0.1)
	lapsed := readAccount("spare", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.05, 6*day))
	lapsed.Lapsed = []string{"5h"}
	refused := readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day))
	refused.Refused = status.Refusal{Until: now.Add(48 * time.Minute).UTC(), Status: 401}
	fable := readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day), windowOf("7d_oi", "Fable week", 1, 5*day))
	fable.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: now.Add(5 * day).UTC()}
	tokenless := status.Account{ID: "work", Label: "work", Error: "no token file"}
	unread := status.Account{ID: "work", Label: "work", TokenSet: true}
	weekly := readAccount("work", sessionOf(0.1, 4*time.Hour), weekRunningOut(5*time.Hour, 3*day))

	type moment struct {
		in   time.Duration
		want room
	}
	tests := []struct {
		name    string
		span    Span
		doc     status.Document
		moments []moment
		// wantSays is what's said where each stretch without room starts.
		wantSays []string
	}{
		{
			name: "its 5-hour window running out at its pace: back as it resets", doc: routerDoc("", 1, pressedAccount("work")),
			moments:  []moment{{-30 * time.Minute, roomOpen}, {10 * time.Minute, roomDraining}, {89 * time.Minute, roomDraining}, {91 * time.Minute, roomNone}, {179 * time.Minute, roomNone}, {181 * time.Minute, roomOpen}},
			wantSays: []string{"runs out ~14:42  ·  back 16:12, as it resets"},
		},
		{
			name: "its 5-hour window reaching its limit just as it resets: room all along", doc: routerDoc("", 1, readAccount("work", sessionOf(0.2, 4*time.Hour), weekOf(0.1, 5*day))),
			moments: []moment{{10 * time.Minute, roomOpen}, {3*time.Hour + 59*time.Minute, roomOpen}, {4*time.Hour + time.Minute, roomOpen}},
		},
		{
			name: "its 5-hour window reaching its reserve, which holds it back", doc: routerDoc("", 1, headingThere),
			moments:  []moment{{60 * time.Minute, roomDraining}, {70 * time.Minute, roomNone}},
			wantSays: []string{"reaches its reserve ~14:20  ·  back 16:12, as it resets"},
		},
		{
			name: "a limit, from when it was reached, as the router told of it; then its week running out", doc: withEvents(routerDoc("", 1, limited), reached),
			moments:  []moment{{-40 * time.Minute, roomOpen}, {-10 * time.Minute, roomNone}, {30 * time.Minute, roomNone}, {61 * time.Minute, roomDraining}, {11 * time.Hour, roomDraining}, {13 * time.Hour, roomNone}},
			wantSays: []string{"limit reached 12:50  ·  back 14:12", "week runs out ~Tue 01:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "a limit, probed, from before anything shown", doc: probed(probedLimit),
			moments:  []moment{{-50 * time.Minute, roomNone}, {30 * time.Minute, roomNone}, {61 * time.Minute, roomDraining}},
			wantSays: []string{"limit reached  ·  back 14:12", "week runs out ~Tue 01:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "its week running out, named over the day", doc: routerDoc("", 1, weekly),
			moments:  []moment{{4 * time.Hour, roomDraining}, {6 * time.Hour, roomNone}},
			wantSays: []string{"week runs out ~18:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "its week running out, over the week: back as it resets", span: Week, doc: routerDoc("", 1, weekly),
			moments:  []moment{{-20 * time.Hour, roomOpen}, {4 * time.Hour, roomDraining}, {6 * time.Hour, roomNone}, {3*day + time.Hour, roomOpen}},
			wantSays: []string{"runs out ~18:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "over the week, a limit in its 5-hour window alone holds no week", span: Week, doc: withEvents(routerDoc("", 1, limited), reached),
			moments:  []moment{{30 * time.Minute, roomDraining}, {13 * time.Hour, roomNone}},
			wantSays: []string{"runs out ~Tue 01:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "over the week, a limit naming no window holds every one", span: Week, doc: probed(noneNamed),
			moments:  []moment{{30 * time.Minute, roomNone}, {61 * time.Minute, roomDraining}},
			wantSays: []string{"limit reached  ·  back 14:12", "runs out ~Tue 01:12  ·  back Thu 13:12, as it resets"},
		},
		{
			name: "at its reserve, which holds it back till the window resets", doc: routerDoc("", 1, atReserve),
			moments:  []moment{{-50 * time.Minute, roomNone}, {2 * time.Hour, roomNone}, {3*time.Hour + time.Minute, roomOpen}},
			wantSays: []string{"at its reserve  ·  back 16:12, as it resets"},
		},
		{
			name: "at its reserve, pinned, which spends it", doc: pinning(routerDoc("", 1, atReserve), "work"),
			moments:  []moment{{5 * time.Minute, roomDraining}, {2 * time.Hour, roomNone}},
			wantSays: []string{"runs out ~13:25  ·  back 16:12, as it resets"},
		},
		{
			name: "its 5-hour window lapsed, which its next request starts", doc: routerDoc("", 1, lapsed),
			moments: []moment{{-50 * time.Minute, roomOpen}, {time.Hour, roomOpen}, {20 * time.Hour, roomOpen}},
		},
		{
			name: "its token refused, over the day", doc: routerDoc("", 1, refused),
			moments:  []moment{{-50 * time.Minute, roomNone}, {47 * time.Minute, roomNone}, {49 * time.Minute, roomOpen}},
			wantSays: []string{"refused (401)  ·  back 14:00"},
		},
		{
			name: "its token refused, over the week, which counts the weeks alone", span: Week, doc: routerDoc("", 1, refused),
			moments: []moment{{10 * time.Minute, roomOpen}},
		},
		{
			name: "a limit in a model's own week, other models still going to it", doc: routerDoc("", 1, fable),
			moments: []moment{{10 * time.Minute, roomOpen}},
		},
		{
			name: "without a token, none all along", doc: probed(tokenless),
			moments:  []moment{{-50 * time.Minute, roomNone}, {20 * time.Hour, roomNone}},
			wantSays: []string{"no token  ·  switchboard accounts token work"},
		},
		{
			name: "nothing read of it, nothing known", doc: probed(unread),
			moments: []moment{{-50 * time.Minute, roomUnknown}, {time.Hour, roomUnknown}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Policy: claudeLike, Span: tt.span}
			l := f.laneOf(tt.doc, tt.doc.Accounts[0], now)
			for _, m := range tt.moments {
				if got := l.at(now.Add(m.in), now); got != m.want {
					t.Errorf("at %s, the lane stands %v, want %v", now.Add(m.in).Format("Mon 15:04"), got, m.want)
				}
			}
			var says []string
			for _, s := range l.stretches {
				says = append(says, s.says(now).plain())
			}
			if !slices.Equal(says, tt.wantSays) {
				t.Errorf("the lane says %q, want %q", says, tt.wantSays)
			}
		})
	}
}

func TestTheRoomRunwayGoesByIsTheProjectionTheCardsDo(t *testing.T) {
	a := pressedAccount("work")
	doc := routerDoc("", 1, a)
	w, _ := a.Window("5h")
	out, ok := doc.RunsOut(a, w, now)
	if !ok {
		t.Fatal("work's session doesn't run out")
	}
	l := Frame{Policy: claudeLike}.laneOf(doc, a, now)
	if len(l.stretches) != 1 || !l.stretches[0].from.Equal(out.At) || !l.drains.Equal(out.At) {
		t.Errorf("the lane runs out at %v, draining till %v, want both at %s, where RunsOut has it", l.stretches, l.drains, out.At.Format(time.TimeOnly))
	}
}

func TestStretchesThatMeetRunTogether(t *testing.T) {
	at := func(minutes int) time.Time { return now.Add(time.Duration(minutes) * time.Minute) }
	tests := []struct {
		name string
		in   []stretch
		want []stretch
	}{
		{
			name: "apart, in order",
			in:   []stretch{{from: at(60), until: at(90), cause: "b"}, {from: at(10), until: at(20), cause: "a"}},
			want: []stretch{{from: at(10), until: at(20), cause: "a"}, {from: at(60), until: at(90), cause: "b"}},
		},
		{
			name: "overlapping, until the later end, said as the first, back as it resets no longer",
			in:   []stretch{{from: at(10), until: at(40), resets: at(40), cause: "a"}, {from: at(30), until: at(90), cause: "b"}},
			want: []stretch{{from: at(10), until: at(90), resets: at(40), cause: "a"}},
		},
		{
			name: "meeting end to start",
			in:   []stretch{{from: at(10), until: at(30), cause: "a"}, {from: at(30), until: at(50), cause: "b"}},
			want: []stretch{{from: at(10), until: at(50), cause: "a"}},
		},
		{
			name: "one within another",
			in:   []stretch{{from: at(10), until: at(90), cause: "a"}, {from: at(30), until: at(50), cause: "b"}},
			want: []stretch{{from: at(10), until: at(90), cause: "a"}},
		},
		{
			name: "one with no end known taking in what follows",
			in:   []stretch{{cause: "a"}, {from: at(30), until: at(50), cause: "b"}},
			want: []stretch{{cause: "a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := merged(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("merged() = %+v, want %+v", got, tt.want)
			}
		})
	}
	if s := (stretch{from: at(10), until: at(90), resets: at(40), cause: "runs out ~13:22"}); s.says(now).plain() != "runs out ~13:22  ·  back 14:42" {
		t.Errorf("run together, it says %q, want it back when they end, as no window resets then", s.says(now).plain())
	}
}

func TestAStretchTooShortForAnyColumnsMomentShowsInTheColumnItStartsIn(t *testing.T) {
	tl := timelineOf(Day, now, 136)
	short := lane{known: true, stretches: []stretch{{from: clockAt(14, 46), until: clockAt(14, 52), cause: "runs out ~14:46"}}}

	cells := short.cells(tl)
	if col, ok := short.stretches[0].column(tl); !ok || col != 15 {
		t.Errorf("it shows from column %d, %v, want 15, where it starts", col, ok)
	}
	for col, r := range cells {
		if want := roomOpen; col != 15 && col >= 6 && r != want {
			t.Errorf("column %d stands %v, want %v", col, r, want)
		}
	}
	if cells[15] != roomNone {
		t.Errorf("column 15 stands %v, want no room, the stretch shown where it starts", cells[15])
	}
}

// withEvents is doc telling of events.
func withEvents(doc status.Document, events ...status.Event) status.Document {
	doc.Events = events
	return doc
}

// pinning is doc with the global pin naming the accounts with the given ids.
func pinning(doc status.Document, ids ...string) status.Document {
	doc.Pin = status.Pin{Accounts: ids, Since: now.UTC()}
	return doc
}

// reserving is a, leaving reserve of each window unused.
func reserving(a status.Account, reserve float64) status.Account {
	a.Reserve = reserve
	return a
}
