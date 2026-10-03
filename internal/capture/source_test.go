package capture

import (
	"cmp"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestTheRouterGivesItsDocumentWhateverIsAsked(t *testing.T) {
	s := threeAccounts(moment(time.UTC))
	want := s.doc

	for _, r := range []watch.Read{{}, {Probe: true}, watch.Fresh()} {
		doc, health, err := s.Read(t.Context(), r)
		if err != nil || !reflect.DeepEqual(doc, want) {
			t.Errorf("Read(%+v) = %+v, %v, want the router's document", r, doc, err)
		}
		if !health.OK || health.PID != routerPID || !health.StartedAt.Equal(on(s.now, 1, 12, 0)) {
			t.Errorf("Read(%+v) says the router is %+v, want the one the fixtures have, healthy, started at noon", r, health)
		}
	}
	if err := s.Pin(t.Context(), []string{"work"}, true); err != nil {
		t.Errorf("Pin() error = %v, want the order taken", err)
	}
	if err := s.Unpin(t.Context()); err != nil {
		t.Errorf("Unpin() error = %v, want the order taken", err)
	}
	if err := s.PinSession(t.Context(), idD28C, "side"); err != nil {
		t.Errorf("PinSession() error = %v, want the order taken", err)
	}
	if err := s.UnpinSession(t.Context(), idD28C); err != nil {
		t.Errorf("UnpinSession() error = %v, want the order taken", err)
	}
	if doc, _, _ := s.Read(t.Context(), watch.Read{}); !reflect.DeepEqual(doc, want) {
		t.Errorf("after orders, Read() = %+v, want the document as it was", doc)
	}
	if !s.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = false, want the router answering")
	}
}

func TestSessionsListTheSampleSessionsSeenLastFirst(t *testing.T) {
	s := threeAccounts(moment(time.UTC))

	sessions, err := s.Sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	type placed struct{ session, family, account string }
	var got []placed
	for _, session := range sessions {
		for _, a := range session.Assignments {
			got = append(got, placed{session: session.ID[:4], family: a.Family, account: a.Account})
		}
	}
	want := []placed{
		{"c61b", "opus", "side"},
		{"db8a", "sonnet", "work"}, {"db8a", "opus", "side"},
		{"d28c", "opus", "work"},
		{"41e0", "sonnet", "side"},
		{"7f3a", "haiku", "work"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("sessions are %v, want %v", got, want)
	}
	if s.doc.Sessions != len(sessions) {
		t.Errorf("the document counts %d sessions, want the %d listed", s.doc.Sessions, len(sessions))
	}
}

func TestAPinnedSampleSessionHasItsOwnPin(t *testing.T) {
	sessions, err := fiveAccounts(moment(time.UTC)).Sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(sessions, func(s status.Session) bool { return s.ID == id9E21 })
	if i < 0 {
		t.Fatalf("no session %s among %+v", id9E21, sessions)
	}
	if got := sessions[i]; got.Pin != "client" || !got.Assignments[0].Pinned {
		t.Errorf("session 9e21 is %+v, want it pinned to client by its own pin", got)
	}
	if got, want := sessions[i].Assignments[0].PinnedAt, on(moment(time.UTC), 1, 14, 39); !got.Equal(want) {
		t.Errorf("session 9e21 was given its pin at %v, want %v, as it ran, as the pin's move tells", got, want)
	}
}

func TestHistoryGivesAPointEachStepFromEachWindowsStart(t *testing.T) {
	now := moment(time.UTC)
	h, err := fiveAccounts(now).History(t.Context(), "5h", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if h.Window != "5h" || h.Step != "30m0s" {
		t.Errorf("history of %s a step of %s, want of 5h a step of 30m0s", h.Window, h.Step)
	}
	var ids []string
	for _, a := range h.Accounts {
		ids = append(ids, a.ID)
	}
	if want := []string{"work", "personal", "side", "client", "spare"}; !slices.Equal(ids, want) {
		t.Errorf("history of accounts %v, want every account's, in order, %v", ids, want)
	}
	work := h.Accounts[0]
	if want := on(now, 1, 12, 10); !work.Start.Equal(want) {
		t.Errorf("work's session started at %s, want %s", work.Start, want)
	}
	var at []time.Time
	for _, p := range work.Points {
		at = append(at, p.At)
	}
	if want := stepsFrom(on(now, 1, 12, 10), on(now, 1, 14, 40), 30*time.Minute); !slices.EqualFunc(at, want, time.Time.Equal) {
		t.Errorf("work's points are at %v, want %v", at, want)
	}
	if spare := h.Accounts[4]; !spare.Start.IsZero() || len(spare.Points) > 0 {
		t.Errorf("spare's lapsed session has history %+v, want none", spare)
	}
}

func TestHistoryRisesAsTheFramesChartsDo(t *testing.T) {
	now := moment(time.UTC)
	h, err := threeAccounts(now).History(t.Context(), "5h", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	personal := h.Accounts[1]
	if !slices.IsSortedFunc(personal.Points, func(a, b router.HistoryPoint) int { return cmp.Compare(a.Utilization, b.Utilization) }) {
		t.Error("personal's session's use falls, want it rising")
	}
	for _, p := range personal.Points {
		if limited := on(now, 1, 14, 12); !p.At.Before(limited) && p.Utilization != 1 {
			t.Errorf("personal's session read %v at %s, after it reached its limit at %s, want it level at its limit", p.Utilization, p.At, limited)
		}
	}
}

func TestEveryWindowsHistoryEndsAtItsUse(t *testing.T) {
	for _, f := range fixtures(moment(time.UTC)) {
		for _, a := range f.router.doc.Accounts {
			for _, w := range a.Windows {
				if a.HasLapsed(w) {
					continue
				}
				h, err := f.router.History(t.Context(), w.Key, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				points := h.Accounts[slices.IndexFunc(h.Accounts, func(held router.AccountHistory) bool { return held.ID == a.ID })].Points
				if len(points) == 0 {
					t.Errorf("%s: %s's %s has no history", f.Name, a.ID, w.Key)
					continue
				}
				if last := points[len(points)-1].Utilization; last != w.Utilization {
					t.Errorf("%s: %s's %s history ends at %v, want its use, %v", f.Name, a.ID, w.Key, last, w.Utilization)
				}
			}
		}
	}
}

func TestHistoryRefusesAStepOfNothing(t *testing.T) {
	if _, err := threeAccounts(moment(time.UTC)).History(t.Context(), "5h", 0); err == nil {
		t.Error("History() with a step of 0 gave no error, want it refused")
	}
}

func TestEventsAreNumberedTheNewestFirst(t *testing.T) {
	for _, f := range fixtures(moment(time.UTC)) {
		events := f.router.doc.Events
		if len(events) == 0 {
			t.Errorf("%s tells of nothing lately", f.Name)
			continue
		}
		for i := 1; i < len(events); i++ {
			if events[i].ID != events[i-1].ID-1 || events[i].At.After(events[i-1].At) {
				t.Errorf("%s: event %+v follows %+v, want each older than the last, numbered one less", f.Name, events[i], events[i-1])
			}
		}
	}
}

func TestAMoveALimitForcedIsCountedInIt(t *testing.T) {
	events := threeAccounts(moment(time.UTC)).doc.Events
	limit := slices.IndexFunc(events, func(e status.Event) bool { return e.Kind == status.EventLimit })
	var counted []string
	for _, e := range events {
		if e.Kind == status.EventMoved && e.Limit == events[limit].ID {
			counted = append(counted, e.Session[:4])
		}
	}
	if want := []string{"41e0", "db8a"}; !slices.Equal(counted, want) {
		t.Errorf("the limit's event counts the moves of %q, want %q", counted, want)
	}
	moved := numbered([]status.Event{{Kind: status.EventMoved, From: "work", Limit: forced}})
	if moved[0].Limit != 0 {
		t.Errorf("a move forced by no limit told of is counted in event %d, want none", moved[0].Limit)
	}
}

// stepsFrom are the times from first to last, a step apart.
func stepsFrom(first, last time.Time, step time.Duration) []time.Time {
	var steps []time.Time
	for at := first; !at.After(last); at = at.Add(step) {
		steps = append(steps, at)
	}
	return steps
}
