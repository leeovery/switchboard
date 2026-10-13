package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// toldEvent is an event as the Log's Events tells it: its time, its kind's
// word, the account it befell and what happened.
type toldEvent struct {
	at, kind, account, what string
}

// toldBetween is what the auto world's events reader gives from from up to
// to, as the Log's Events tells each, at the frames' moment.
func toldBetween(from, to time.Time) []toldEvent {
	now := worldsNow().now
	_, _, happened := autoWorld().readers()
	telling := views.NewTelling(status.NameOfKey)
	var told []toldEvent
	for e := range happened.Between(from, to) {
		line := telling.Line(e.Event, now)
		told = append(told, toldEvent{at: e.At.In(now.Location()).Format(time.TimeOnly), kind: line.Kind, account: line.Account.String(), what: line.What.String()})
	}
	return told
}

func TestTheRoutersEventsAreTheLogsAsItTellsThem(t *testing.T) {
	now := localMoment()
	today := dayfile.DayStart(now, 0)
	told := toldBetween(clockAt(now, -1, 16, 55, 0), now)
	// The Log's Events (log-events-160), yesterday's from 16:55 and today's
	// 20, but db8a's move under pressure, which the frame says ran out at
	// 15:10: no pressure event is told today to say when, and the frame's
	// counts hold none.
	want := []toldEvent{
		{at: "16:55:20", kind: "started", account: "personal"},
		{at: "17:10:00", kind: "open", account: "work", what: "its 5-hour window reset"},
		{at: "17:20:15", kind: "pin", account: "personal", what: "new sessions go there, set from the command line"},
		{at: "18:02:40", kind: "auto", what: "new sessions back from personal to the router's choice, set from the dashboard"},
		{at: "19:12:40", kind: "limit", account: "personal", what: "its 5-hour window, till 23:58: its session moves to side"},
		{at: "19:12:52", kind: "moved", account: "personal → side", what: "personal reached its limit"},
		{at: "23:47:30", kind: "started", account: "side"},
		{at: "23:58:40", kind: "open", account: "personal", what: "its 5-hour window reset"},
		{at: "07:05:12", kind: "primed", account: "work", what: "its 5-hour window started, resets at 12:05"},
		{at: "07:58:30", kind: "started", account: "work"},
		{at: "08:31:02", kind: "started", account: "side"},
		{at: "08:47:00", kind: "primed", account: "side", what: "its 5-hour window started, resets at 13:47"},
		{at: "09:12:40", kind: "started", account: "work"},
		{at: "09:40:05", kind: "started", account: "work"},
		{at: "10:02:18", kind: "started", account: "personal"},
		{at: "11:30:02", kind: "primed", account: "personal", what: "its 5-hour window started, resets at 16:30"},
		{at: "11:55:31", kind: "started", account: "personal"},
		{at: "12:02:10", kind: "started", account: "work"},
		{at: "12:10:00", kind: "primed", account: "work", what: "its 5-hour window started, resets at 17:10"},
		{at: "12:47:12", kind: "started", account: "personal"},
		{at: "13:22:40", kind: "moved", account: "personal → work", what: "personal came under pressure"},
		{at: "13:50:00", kind: "primed", account: "side", what: "its 5-hour window started, resets at 18:50"},
		{at: "14:12:30", kind: "started", account: "work"},
		{at: "14:31:03", kind: "cap", account: "work", what: "95% of its 5-hour window: its 3 sessions move to side as their next requests come"},
		{at: "14:31:13", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:31:20", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:31:40", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:41:11", kind: "started", account: "side"},
	}
	if len(told) != len(want) {
		t.Fatalf("the events reader gives %d events from yesterday's 16:55, want %d: %+v", len(told), len(want), told)
	}
	for i := range want {
		if told[i] != want[i] {
			t.Errorf("event %d is told %+v, want %+v", i+1, told[i], want[i])
		}
	}

	kinds := make(map[string]int)
	for _, e := range toldBetween(today, now) {
		kinds[e.kind]++
	}
	wantKinds := map[string]int{"started": 10, "moved": 4, "cap": 1, "primed": 5}
	if len(kinds) != len(wantKinds) {
		t.Errorf("today's events by kind = %v, want %v", kinds, wantKinds)
	}
	for kind, n := range wantKinds {
		if kinds[kind] != n {
			t.Errorf("today's %s events = %d, want %d, as the Log's panel counts them", kind, kinds[kind], n)
		}
	}
}

// sentence is an event as a test reads it: when it happened, its kind, the
// session it's of, the account it befell, a pin's accounts, or a move's from
// and to, why, and the sessions it moved, where it says.
func sentence(e status.Event, now time.Time) string {
	s := e.At.In(now.Location()).Format(time.TimeOnly) + " " + e.Kind
	if e.Session != "" {
		s += " " + e.Session[:4]
	}
	switch {
	case e.From != "":
		s += " " + e.From + " → " + e.To
	case len(e.Accounts) > 0:
		s += " " + strings.Join(e.Accounts, ", ")
	default:
		s += " " + e.Account
	}
	if e.Reason != "" {
		s += ": " + e.Reason
	}
	if e.Count > 0 {
		s += fmt.Sprintf(", %d moved to %s", e.Count, e.To)
	}
	return s
}

// Each world's router tells of its day in its status document, the newest
// events first, those the Overview's LATELY draws as its frames have them:
// the pinned world's c61b started on personal under the pin, after work's cap
// and the pin; the bad hour's c61b on side under the pin, after work's
// refusal and personal's limit; and the auto world's, which the six and ten
// accounts' worlds and routing's world of a limit share, but for that limit,
// told before 7f3a's start.
func TestEachWorldsDocumentTellsTheNewestOfItsEventsAsItsFramesHaveThem(t *testing.T) {
	worlds := worldsNow()
	autos := []string{
		"14:41:11 started c61b side: new",
		"14:31:40 moved db8a work → side: " + reasonCapped,
		"14:31:20 moved 7f3a work → side: " + reasonCapped,
		"14:31:13 moved d28c work → side: " + reasonCapped,
		"14:31:03 cap work, 3 moved to side",
		"14:12:30 started 7f3a work: new",
	}
	tests := []struct {
		name  string
		world accountWorld
		want  []string
	}{
		{name: "auto", world: worlds.auto(), want: autos},
		{name: "six accounts", world: worlds.many(6), want: autos},
		{name: "ten accounts", world: worlds.many(10), want: autos},
		{name: "routing's limit", world: worlds.limit(), want: append(slices.Clone(autos),
			"14:12:00 moved 9c2e personal → side: "+reasonLimited,
			"14:12:00 moved 41e0 personal → side: "+reasonLimited,
			"14:12:00 limit personal, 2 moved to side",
		)},
		{name: "pinned to personal", world: worlds.pinned(), want: []string{
			"14:41:11 started c61b personal: " + status.ReasonGlobalPin,
			"14:31:40 moved db8a work → side: " + reasonCapped,
			"14:31:20 moved 7f3a work → side: " + reasonCapped,
			"14:31:13 moved d28c work → side: " + reasonCapped,
			"14:31:03 cap work, 3 moved to side",
			"14:02:00 pin personal",
			"13:55:30 started 7f3a work: new",
		}},
		{name: "the bad hour", world: worlds.badHour(), want: []string{
			"14:41:11 started c61b side: " + status.ReasonGlobalPin,
			"14:20:41 moved db8a work → side: " + reasonRefused,
			"14:20:00 moved d28c work → side: " + reasonRefused,
			"14:20:00 refused work, 2 moved to side",
			"14:12:00 moved 9c2e personal → side: " + reasonLimited,
			"14:12:00 moved 41e0 personal → side: " + reasonLimited,
			"14:12:00 limit personal, 2 moved to side",
			"13:55:30 started 7f3a side: " + status.ReasonGlobalPin,
			"13:50:00 primed side",
			"13:40:00 pin personal, side",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.world.document(worlds.now)
			if len(doc.Events) < len(tt.want) {
				t.Fatalf("the document tells of %d events, want at least %d", len(doc.Events), len(tt.want))
			}
			var got []string
			for _, e := range doc.Events[:len(tt.want)] {
				got = append(got, sentence(e, worlds.now))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the document's newest events are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// A world's router numbers its events as it tells them, from 1 as it
// started, and its document gives the newest it keeps, newest first, none
// after it was built; each move a cap, a limit or a refusal forced is
// counted in it, as many as it says it moved, to where it says, a limit's in
// the limit too; and a limit holding an account is told of, by its identity.
func TestEachWorldsRouterNumbersAndCountsItsEventsAsTheRouterDoes(t *testing.T) {
	worlds := worldsNow()
	tests := []struct {
		name  string
		world accountWorld
		// untold are the accounts whose limit no event tells of.
		untold []string
	}{
		{name: "auto", world: worlds.auto()},
		{name: "pinned", world: worlds.pinned()},
		{name: "routing's limit", world: worlds.limit()},
		{name: "the bad hour", world: worlds.badHour()},
		{name: "ten accounts, the auto world's events", world: worlds.many(10), untold: []string{"team"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			told := tt.world.events
			for i, e := range told {
				if e.ID != i+1 || i > 0 && e.At.Before(told[i-1].At) {
					t.Errorf("event %d is %+v, want it numbered %d, after the one before", i, e, i+1)
				}
			}
			doc := tt.world.document(worlds.now)
			if len(doc.Events) != min(len(told), eventsKept) || doc.Events[0].ID != told[len(told)-1].ID {
				t.Errorf("the document gives %d events, the newest %d, want the newest of %d, %d", len(doc.Events), doc.Events[0].ID, len(told), told[len(told)-1].ID)
			}
			for _, e := range doc.Events {
				if e.At.After(doc.GeneratedAt) {
					t.Errorf("the document, built at %s, tells of %+v", doc.GeneratedAt, e)
				}
			}
			for _, by := range told {
				if forces(by.Kind) {
					countsItsMoves(t, by, told)
				}
			}
			for _, a := range doc.Accounts {
				toldOf := slices.ContainsFunc(doc.Events, func(e status.Event) bool {
					return e.Kind == status.EventLimit && e.Account == a.ID && e.Limit == a.Limit.ID
				})
				if a.Limit.ID != 0 && toldOf == slices.Contains(tt.untold, a.ID) {
					t.Errorf("%s's limit %d told of: %v, want %v", a.ID, a.Limit.ID, toldOf, !toldOf)
				}
			}
		})
	}
}

// countsItsMoves checks the cap, limit or refusal by is counted in by as many
// moves as it says it moved, each off its account to where it says, a limit's
// counted in the limit too.
func countsItsMoves(t *testing.T, by status.Event, told []status.Event) {
	t.Helper()
	var moved int
	for _, m := range told {
		if m.ForcedBy != by.ID {
			continue
		}
		moved++
		if m.Kind != status.EventMoved || m.From != by.Account || m.To != by.To || (by.Kind == status.EventLimit) != (m.Limit == by.ID) {
			t.Errorf("%+v is counted in %+v, want a move off its account, to where it sent them, a limit's in the limit", m, by)
		}
	}
	if moved != by.Count {
		t.Errorf("%+v moved %d sessions, want the %d it counts", by, moved, by.Count)
	}
}

func TestTheAutoWorldsRouterNumbersItsDayAsTheLogHasIt(t *testing.T) {
	told := autoEvents(worldsNow().now)
	ids := func(kind string) []int {
		var got []int
		for _, e := range told {
			if e.Kind == kind {
				got = append(got, e.ID)
			}
		}
		return got
	}
	if got := ids(status.EventLimit); !slices.Equal(got, []int{5}) {
		t.Errorf("the limits are told as %v, want yesterday's personal's as 5", got)
	}
	if got := ids(status.EventCap); !slices.Equal(got, []int{24}) {
		t.Errorf("the caps are told as %v, want work's as 24", got)
	}
	var forcedBy []int
	for _, e := range told {
		if e.Kind == status.EventMoved {
			forcedBy = append(forcedBy, e.ForcedBy)
		}
	}
	if want := []int{5, 0, 24, 24, 24}; !slices.Equal(forcedBy, want) {
		t.Errorf("the moves are counted in %v, want %v: 8a2c's in personal's limit, db8a's afresh in none, and the cap's", forcedBy, want)
	}
}

func TestAMoveNothingOfItsAccountsForcedIsCountedInNothing(t *testing.T) {
	h := happenings{worldsNow().now}
	told := ofTheRun([]status.Event{
		{Kind: status.EventLimit, At: h.today(9, 0, 0), Account: "side"},
		h.forcedOff(h.today(10, 0, 0), id41E0, "work", "side", reasonCapped),
	})
	if told[1].ForcedBy != 0 || told[1].Limit != 0 {
		t.Errorf("a move off work, nothing of work's forcing it, is counted in %d and limit %d, want neither", told[1].ForcedBy, told[1].Limit)
	}
}
