package views_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// A look judges the timeline's past once, an account a column before now's,
// or every column of a day gone, and holds it while what it's judged from
// stands: the next look in the same column, the moment read a second on,
// judges none, and builds the lanes a first look would; a look with a
// reading gained, now's column moved on, the other view, the readings
// history's keep moved, past a day shown or not, or the readings the watch
// read itself begun, as each tells whether a hold read first from it began
// before, a reserve changed, or another hold told of, judges it all again.
// A hold at now whose start no event tells is judged at now, at each look,
// as its readings tell it; one whose event tells it, or on a day gone, isn't.
func TestALookJudgesThePastOnceWhileWhatItsJudgedFromStands(t *testing.T) {
	auto := runwayMoment(t, "the auto world's 5-hour view")
	team := runwayMoment(t, "a hold no event tells of, the router's")
	refused := runwayMoment(t, "its token refused")
	capped := runwayMoment(t, "a cap its event tells the start of, read later")
	kept := runwayMoment(t, "stepped back to a day older than the readings history's keep")
	today := auto
	today.Day = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	keptLater := kept
	keptLater.Kept = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	autoKept, autoSeen := auto, auto
	autoKept.Kept = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	autoSeen.Seen = auto.Now.Add(-time.Hour)
	untoldGone := runwayMoment(t, "a hold at now the readings tell otherwise")
	untoldGone.Day = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	gained := append(slices.Clip(auto.Readings), readings.Reading{
		At: auto.Now, Account: "side", Key: claude.FiveHourWindow, Utilization: 0.14, ResetsAt: time.Date(2026, 10, 1, 18, 50, 0, 0, time.UTC),
	})
	tests := []struct {
		name         string
		first, then  viewstest.RunwayMoment
		judged, more int
	}{
		{name: "a second on, in the same column", first: auto, then: later(auto, time.Second), judged: 3 * 6, more: 0},
		{name: "a reading gained", first: auto, then: withReadings(later(auto, time.Second), gained), judged: 3 * 6, more: 3 * 6},
		{name: "now's column moved on", first: auto, then: later(auto, 10*time.Minute), judged: 3 * 6, more: 3 * 6},
		{name: "the week view", first: auto, then: inWeek(auto), judged: 3 * 6, more: 3 * 19},
		{name: "a hold at now no event tells the start of", first: team, then: later(team, time.Second), judged: 2*6 + 1, more: 1},
		{name: "a refusal at now its event tells the start of", first: refused, then: later(refused, time.Second), judged: 2 * 6, more: 0},
		{name: "a cap at now its event tells the start of", first: capped, then: later(capped, time.Second), judged: 2 * 6, more: 0},
		{name: "today stepped back to, now's column moved on", first: today, then: later(today, 11*time.Minute), judged: 3 * 83, more: 3 * 84},
		{name: "a day gone, the keep moved past it", first: kept, then: keptLater, judged: 2 * 136, more: 2 * 136},
		{name: "the keep moved past no day shown", first: auto, then: autoKept, judged: 3 * 6, more: 3 * 6},
		{name: "the readings the watch read itself begun", first: auto, then: autoSeen, judged: 3 * 6, more: 3 * 6},
		{name: "a day gone, a second on", first: kept, then: later(kept, time.Second), judged: 2 * 136, more: 0},
		{name: "a day gone, a hold at now no event tells the start of", first: untoldGone, then: later(untoldGone, time.Second), judged: 2 * 136, more: 0},
		{name: "work's reserve changed", first: auto, then: withDoc(auto, reserveOf(auto.Doc, "work", 0.1)), judged: 3 * 6, more: 3 * 6},
		{name: "the router told of another hold", first: auto, then: withDoc(auto, withEvent(auto.Doc, status.Event{
			ID: 99, At: auto.Now.Add(-time.Hour), Kind: status.EventRefused, Account: "side", Until: auto.Now.Add(-50 * time.Minute), Status: 401,
		})), judged: 3 * 6, more: 3 * 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var judged int
			first := views.CountingLanes(&judged).Look(lanesFrom(tt.first))
			if judged != tt.judged {
				t.Errorf("the first look judged %d times, want %d", judged, tt.judged)
			}
			judged = 0
			from := lanesFrom(tt.then)
			then := first.Look(from)
			if judged != tt.more {
				t.Errorf("the next look judged %d times, want %d", judged, tt.more)
			}
			if fresh := (views.Lanes{}).Look(from); !reflect.DeepEqual(then.Accounts, fresh.Accounts) || !reflect.DeepEqual(then.Pool, fresh.Pool) || !reflect.DeepEqual(then.Drawn, fresh.Drawn) {
				t.Errorf("the next look's lanes aren't those a first look builds")
			}
		})
	}
}

// runwayMoment is Runway's moment with the given name.
func runwayMoment(t *testing.T, name string) viewstest.RunwayMoment {
	t.Helper()
	moments := viewstest.RunwayMoments()
	i := slices.IndexFunc(moments, func(rm viewstest.RunwayMoment) bool { return rm.Name == name })
	if i < 0 {
		t.Fatalf("no Runway moment is named %q", name)
	}
	return moments[i]
}

// later is the moment rm, d later.
func later(rm viewstest.RunwayMoment, d time.Duration) viewstest.RunwayMoment {
	rm.Now = rm.Now.Add(d)
	return rm
}

// withReadings is the moment rm, its readings those given.
func withReadings(rm viewstest.RunwayMoment, read []readings.Reading) viewstest.RunwayMoment {
	rm.Readings = read
	return rm
}

// withDoc is the moment rm, its document the one given.
func withDoc(rm viewstest.RunwayMoment, doc status.Document) viewstest.RunwayMoment {
	rm.Doc = doc
	return rm
}

// reserveOf is the document doc, the account with the given id keeping the
// reserve given, worked out again.
func reserveOf(doc status.Document, id string, reserve float64) status.Document {
	doc.Accounts = slices.Clone(doc.Accounts)
	for i := range doc.Accounts {
		if doc.Accounts[i].ID == id {
			doc.Accounts[i].Reserve = reserve
		}
	}
	return doc.WorkedOut(claude.Policy())
}

// withEvent is the document doc, the router telling of the event e too, newest.
func withEvent(doc status.Document, e status.Event) status.Document {
	doc.Events = append([]status.Event{e}, doc.Events...)
	return doc
}

// inWeek is the moment rm, in the week view.
func inWeek(rm viewstest.RunwayMoment) viewstest.RunwayMoment {
	rm.Window = views.RunwayWeek
	return rm
}

// A stretch is said to end as its own hold lifts where the stretch after it
// holds its account on, else as room returns, where either is known.
func TestAStretchEndsAsItLiftsOfARunElseAsItsBack(t *testing.T) {
	lifts, back := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 21, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		stretch   views.LaneStretch
		want      time.Time
		wantLifts bool
	}{
		{name: "the last of a run, or alone", stretch: views.LaneStretch{Back: back}, want: back},
		{name: "one of a run but the last", stretch: views.LaneStretch{Back: back, Lifts: lifts}, want: lifts, wantLifts: true},
		{name: "back at no time known", stretch: views.LaneStretch{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, lifts := tt.stretch.Ends(); !got.Equal(tt.want) || lifts != tt.wantLifts {
				t.Errorf("it ends %v, lifting %v; want %v, lifting %v", got, lifts, tt.want, tt.wantLifts)
			}
		})
	}
}
