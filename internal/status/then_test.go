package status_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// readOf is work's window with the given key as the readings history read it
// at when: used so far, resetting at resets, zero where it didn't say.
func readOf(when time.Time, key string, used float64, resets time.Time) readings.Reading {
	return readings.Reading{At: when, Account: "work", Key: key, Utilization: used, ResetsAt: resets, Source: readings.FromAnswer}
}

// rejected is the reading r, its window read rejected.
func rejected(r readings.Reading) readings.Reading {
	r.Status = quota.StatusRejected
	return r
}

// workAt is the moment, at stateNow, of the router's document of work alone,
// read just now with room, a tenth of each window its reserve, as change
// leaves the document: its policy is what Then judges windows by.
func workAt(change func(*status.Document)) status.Moment {
	doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Accounts: []status.Account{with(readNow(0.2, 0.4, 0.2), reserving)}}
	change(&doc)
	return doc.At(stateNow, policy)
}

// sameWas reports whether a and b say the same, their times the same
// instants.
func sameWas(a, b status.Was) bool {
	return a.Kind == b.Kind && a.Since.Equal(b.Since) && a.Until.Equal(b.Until)
}

// wasString shows w, its times on their days as stateNow's zone has them.
func wasString(w status.Was) string {
	return fmt.Sprintf("{Kind:%d Since:%s Until:%s}", w.Kind, onDay(w.Since), onDay(w.Until))
}

// onDay shows t on its day, as stateNow's zone has it: "none" where it's
// zero.
func onDay(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.In(stateNow.Location()).Format("Mon 2 Jan 15:04:05")
}

// Then is how an account stood at a moment gone, by the last reading of each
// window every model shares by then, a reset passed counting as reset, and
// by the holds the router told of that the readings can't show: the first
// that holds of no token now, a limit, its token refused, nothing read, its
// cap by its reserve as it holds now, and room.
func TestHowAnAccountStoodThen(t *testing.T) {
	// Work's week, and its 5-hour window on Thursday: at its limit in the
	// morning; afresh from 12:10; at its cap from 14:31:03, as the auto
	// world has it; and at its limit from 15:02, till 17:10.
	week := readOf(at(1, 8, 0), "7d", 0.4, at(4, 21, 0))
	morningLimit := rejected(readOf(at(1, 11, 50), "5h", 1, at(1, 12, 10)))
	afresh := readOf(at(1, 12, 20), "5h", 0.05, at(1, 17, 10))
	capped := readOf(at(1, 14, 31).Add(3*time.Second), "5h", 0.95, at(1, 17, 10))
	stillCapped := readOf(at(1, 14, 40), "5h", 0.97, at(1, 17, 10))
	limited := rejected(readOf(at(1, 15, 2), "5h", 1, at(1, 17, 10)))
	// weekOut is the v0.1.6 case: the week at its limit from Monday till it
	// reset at Thursday's 21:00, read again 30 seconds after.
	weekOut := rejected(readOf(at(1, 10, 0).AddDate(0, 0, -3), "7d", 1, at(1, 21, 0)))
	weekReread := readOf(at(1, 21, 0).Add(30*time.Second), "7d", 0, at(8, 21, 0))
	// yesterday is Wednesday's 5-hour window, read at 93% at 15:00.
	yesterday := at(1, 0, 0).AddDate(0, 0, -1)
	nearlySpent := readOf(yesterday.Add(15*time.Hour), "5h", 0.93, yesterday.Add(17*time.Hour))
	refusal := status.Span{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 15, 0)}
	untoldLimit := status.Span{Kind: status.WasLimited, Until: at(1, 15, 30)}

	capping := workAt(func(*status.Document) {})
	reservingLess := workAt(func(d *status.Document) { d.Accounts[0].Reserve = 0.02 })
	pinned := workAt(func(d *status.Document) { d.Pin = status.Pin{Accounts: []string{"work"}} })
	tokenless := workAt(func(d *status.Document) { d.Accounts[0] = status.Account{ID: "work", Label: "work"} })
	tests := []struct {
		name string
		// moment is of work as it's configured and read now: its reserve,
		// the global pin, and whether its token is usable.
		moment status.Moment
		read   []readings.Reading
		told   []status.Span
		t      time.Time
		want   status.Was
	}{
		{
			name: "room, each window read with room", moment: capping, read: []readings.Reading{week, afresh}, t: at(1, 13, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "nothing read by then", moment: capping, read: []readings.Reading{week, afresh}, t: at(1, 7, 0),
			want: status.Was{Kind: status.WasUnread},
		},
		{
			name: "nothing read of a window every model shares, a model's own read", moment: capping,
			read: []readings.Reading{readOf(at(1, 8, 0), "7d_oi", 0.3, at(4, 21, 0))}, t: at(1, 13, 0),
			want: status.Was{Kind: status.WasUnread},
		},
		{
			name: "another account's windows read", moment: capping,
			read: []readings.Reading{{At: at(1, 11, 50), Account: "side", Key: "5h", Utilization: 1, ResetsAt: at(1, 17, 10), Status: quota.StatusRejected}},
			t:    at(1, 13, 0), want: status.Was{Kind: status.WasUnread},
		},
		{
			name: "a model's own window at its limit, which holds back that model's requests alone", moment: capping,
			read: []readings.Reading{week, afresh, rejected(readOf(at(1, 10, 0), "7d_oi", 1, at(4, 21, 0)))}, t: at(1, 13, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "at its limit, a window read rejected", moment: capping, read: []readings.Reading{week, morningLimit}, t: at(1, 12, 0),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 11, 50), Until: at(1, 12, 10)},
		},
		{
			name: "at its limit, a window read rejected with no reset", moment: capping,
			read: []readings.Reading{week, rejected(readOf(at(1, 11, 50), "5h", 1, time.Time{}))}, t: at(1, 16, 0),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 11, 50)},
		},
		{
			name: "just after its limit's window reset, before the reread", moment: capping, read: []readings.Reading{week, morningLimit},
			t: at(1, 12, 10).Add(5 * time.Second), want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "as its limit's window resets", moment: capping, read: []readings.Reading{week, morningLimit}, t: at(1, 12, 10),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "a week at its limit before it reset, as a reading carried in from Monday has it", moment: capping,
			read: []readings.Reading{weekOut, afresh, weekReread}, t: at(1, 20, 50),
			want: status.Was{Kind: status.WasLimited, Since: weekOut.At, Until: at(1, 21, 0)},
		},
		{
			name: "a week at its limit, reset since, before the reread", moment: capping,
			read: []readings.Reading{weekOut, afresh, weekReread}, t: at(1, 21, 0).Add(10 * time.Second),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "a week at its limit, reset and read again", moment: capping,
			read: []readings.Reading{weekOut, afresh, weekReread}, t: at(1, 22, 50),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "a window read rejected again in its next period, its limit from then", moment: capping,
			read: []readings.Reading{morningLimit, rejected(readOf(at(1, 12, 15), "5h", 1, at(1, 17, 10)))}, t: at(1, 13, 0),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 12, 15), Until: at(1, 17, 10)},
		},
		{
			name: "two windows at their limits, from the first till the last lifts", moment: capping,
			read: []readings.Reading{rejected(readOf(at(1, 10, 0), "7d", 1, at(4, 21, 0))), afresh, limited}, t: at(1, 15, 30),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 10, 0), Until: at(4, 21, 0)},
		},
		{
			name: "two windows at their limits, one's reset unknown", moment: capping,
			read: []readings.Reading{rejected(readOf(at(1, 10, 0), "7d", 1, time.Time{})), afresh, limited}, t: at(1, 15, 30),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 10, 0)},
		},
		{
			name: "at its cap, by the reserve configured now", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasCapped, Since: capped.At, Until: at(1, 17, 10)},
		},
		{
			name: "short of its cap, by the reserve configured now", moment: reservingLess,
			read: []readings.Reading{week, afresh, capped, stillCapped}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "at its cap on a day gone, by the reserve configured now", moment: capping,
			read: []readings.Reading{nearlySpent}, t: yesterday.Add(16 * time.Hour),
			want: status.Was{Kind: status.WasCapped, Since: nearlySpent.At, Until: nearlySpent.ResetsAt},
		},
		{
			name: "at its cap, the global pin spending it now", moment: pinned,
			read: []readings.Reading{week, afresh, capped, stillCapped}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "its cap turned to its limit", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped, limited}, t: at(1, 15, 5),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 15, 2), Until: at(1, 17, 10)},
		},
		{
			name: "without a usable token now, whatever was read", moment: tokenless,
			read: []readings.Reading{week, afresh}, t: at(1, 13, 0),
			want: status.Was{Kind: status.WasTokenless},
		},
		{
			name: "its token refused, as told", moment: capping, read: []readings.Reading{week, afresh}, told: []status.Span{refusal}, t: at(1, 14, 30),
			want: status.Was{Kind: status.WasRefused, Since: at(1, 14, 12), Until: at(1, 15, 0)},
		},
		{
			name: "its token refused, the refusal lifted early", moment: capping, read: []readings.Reading{week, afresh},
			told: []status.Span{{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 14, 20)}}, t: at(1, 14, 30),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "its token refused from after then", moment: capping, read: []readings.Reading{week, afresh}, told: []status.Span{refusal}, t: at(1, 14, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "its token refused, nothing read", moment: capping, told: []status.Span{refusal}, t: at(1, 14, 30),
			want: status.Was{Kind: status.WasRefused, Since: at(1, 14, 12), Until: at(1, 15, 0)},
		},
		{
			name: "its token refused at its cap", moment: capping, read: []readings.Reading{week, afresh, capped}, told: []status.Span{refusal}, t: at(1, 14, 45),
			want: status.Was{Kind: status.WasRefused, Since: at(1, 14, 12), Until: at(1, 15, 0)},
		},
		{
			name: "a limit naming no window, as told, from a start untold", moment: capping, read: []readings.Reading{week, afresh},
			told: []status.Span{untoldLimit}, t: at(1, 14, 0),
			want: status.Was{Kind: status.WasLimited, Until: at(1, 15, 30)},
		},
		{
			name: "a limit naming no window, as told, its token refused", moment: capping, read: []readings.Reading{week, afresh},
			told: []status.Span{refusal, untoldLimit}, t: at(1, 14, 30),
			want: status.Was{Kind: status.WasLimited, Until: at(1, 15, 30)},
		},
		{
			name: "a limit naming no window, as told, then a window read rejected", moment: capping, read: []readings.Reading{week, afresh, limited},
			told: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 0), Until: at(1, 15, 30)}}, t: at(1, 15, 10),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 14, 0), Until: at(1, 17, 10)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.moment.Then(tt.moment.Accounts[0], tt.read, tt.told, tt.t); !sameWas(got, tt.want) {
				t.Errorf("Then(work, %s) = %s, want %s", onDay(tt.t), wasString(got), wasString(tt.want))
			}
		})
	}
}

// Then at a moment's Now is how the moment has each account stand, at every
// hard moment, given the readings its document read and the holds the
// router told of holding then that they can't show: its kind, as ROUTING's
// has it, and when its hold lifts. When a hold began is told by the readings
// and the router's events each as they can, so the two may differ.
func TestThenAtNowIsHowTheMomentHasEachAccountStand(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for i, a := range m.Accounts {
				read := hm.Doc.Accounts[i]
				if got, want := m.Then(a, readingsOf(read), toldOf(read, a, m.Now), m.Now), asTheMomentHasIt(a); got.Kind != want.Kind || !got.Until.Equal(want.Until) {
					t.Errorf("Then(%s, Now) = %s, want %s, as the moment has it", a.ID, wasString(got), wasString(want))
				}
			}
		})
	}
}

// readingsOf are the readings of the account a as its document read it: a
// reading each of its windows, when it was read, none where it wasn't.
func readingsOf(a status.Account) []readings.Reading {
	if a.FetchedAt.IsZero() {
		return nil
	}
	return readings.Of(a.ID, a.Windows, a.FetchedAt, readings.FromAnswer)
}

// toldOf are the holds of the account a, read as its document has it, at
// now as an, that the router told of and the readings can't show: its limit
// naming no window, and the refusal of its token, each holding at now, from
// when it began, as the moment has it.
func toldOf(a status.Account, an status.AccountNow, now time.Time) []status.Span {
	var told []status.Span
	if a.Limit.Holds(now) && len(a.Limit.Windows) == 0 {
		told = append(told, status.Span{Kind: status.WasLimited, From: an.LimitSince, Until: a.Limit.Until})
	}
	if a.Refused.Holds(now) && a.Refused.Family == "" {
		told = append(told, status.Span{Kind: status.WasRefused, From: an.RefusedSince, Until: a.Refused.Until})
	}
	return told
}

// When a hold began is told by the router's event of it, as the moment has
// it, and by the first reading of the unbroken run that shows it, as Then has
// it: where both tell, they agree; where the event is missing, the readings
// tell; and a hold neither tells of, as a limit naming no window untold,
// began before anything given.
func TestWhenAHoldBeganByItsEventAndByItsReadings(t *testing.T) {
	// The auto world's cap: work's 5-hour window at 95% at 14:31:03, as
	// the router's event of it tells, and 97% at 14:40.
	capping := with(readNow(0.97, 0.4, 0.2), reserving)
	capReached := status.Event{At: at(1, 14, 31).Add(3 * time.Second).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Reserve: 0.1}
	capRead := []readings.Reading{
		readOf(at(1, 12, 20), "5h", 0.05, at(1, 17, 10)),
		readOf(at(1, 14, 31).Add(3*time.Second), "5h", 0.95, at(1, 17, 10)),
		readOf(at(1, 14, 40), "5h", 0.97, at(1, 17, 10)),
	}
	// spent is work's 5-hour window read rejected at 14:20, the router's
	// limit of it holding till it resets.
	spent := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) {
		a.Windows[0].Utilization, a.Windows[0].Status = 1, quota.StatusRejected
		a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: at(1, 17, 10).UTC()}
	})
	unnamed := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) { a.Limit = status.Limit{ID: 3, Until: at(1, 15, 30).UTC()} })
	refused := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) { a.Refused = status.Refusal{Until: at(1, 15, 0).UTC(), Status: 401} })
	tests := []struct {
		name    string
		account status.Account
		events  []status.Event
		read    []readings.Reading
		kind    status.WasKind
		// event is when the hold began as the router's event tells, and
		// reading as the readings tell: zero where they don't.
		event, reading time.Time
	}{
		{name: "a cap, its event and its first reading agreeing", account: capping, events: []status.Event{capReached}, read: capRead, kind: status.WasCapped, event: capReached.At, reading: capReached.At},
		{name: "a cap, its event missing", account: capping, read: capRead, kind: status.WasCapped, reading: capReached.At},
		{
			name: "a window read spent, its limit's event and its reading agreeing", account: spent,
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Limit: 4}},
			read:   []readings.Reading{rejected(readOf(at(1, 14, 20), "5h", 1, at(1, 17, 10)))}, kind: status.WasLimited, event: at(1, 14, 20), reading: at(1, 14, 20),
		},
		{
			name: "a limit naming no window, as its event tells", account: unnamed,
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 15, 30).UTC(), Limit: 3}},
			read:   []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasLimited, event: at(1, 14, 20), reading: at(1, 14, 20),
		},
		{
			name: "a limit naming no window, neither its event nor a reading telling", account: unnamed,
			read: []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasLimited,
		},
		{
			name: "its token refused, as its event tells", account: refused,
			events: []status.Event{{At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "work", Until: at(1, 15, 0).UTC(), Status: 401}},
			read:   []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasRefused, event: at(1, 14, 12), reading: at(1, 14, 12),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Accounts: []status.Account{tt.account}, Events: tt.events}
			m := doc.At(stateNow, policy)
			a := m.Accounts[0]
			was := m.Then(a, tt.read, toldOf(tt.account, a, m.Now), m.Now)
			if was.Kind != tt.kind || !eventSince(a, tt.kind).Equal(tt.event) || !was.Since.Equal(tt.reading) {
				t.Errorf("Then(work, Now) = %s, its event telling it began at %s; want of kind %d, as its event tells at %s, as its readings tell at %s",
					wasString(was), onDay(eventSince(a, tt.kind)), tt.kind, onDay(tt.event), onDay(tt.reading))
			}
		})
	}
}

// eventSince is when the hold of the given kind holding the account a at
// its moment began, as the router's event of it tells, as the moment has it.
func eventSince(a status.AccountNow, kind status.WasKind) time.Time {
	switch kind {
	case status.WasLimited:
		return a.LimitSince
	case status.WasCapped:
		return a.CapSince
	case status.WasRefused:
		return a.RefusedSince
	}
	return time.Time{}
}

// asTheMomentHasIt is how the moment has the account a stand, as Then says
// it, by ROUTING's kind: held back from every request by a limit, back when
// ROUTING says; its token refused, till the refusal ends; nothing read of
// it; at its cap, which holds it back, back when ROUTING says; and room, as
// a hold of some models' requests alone, a cap the global pin spends,
// pressure and an idle window all leave it.
func asTheMomentHasIt(a status.AccountNow) status.Was {
	switch a.Routing.Kind {
	case status.RoutingTokenless:
		return status.Was{Kind: status.WasTokenless}
	case status.RoutingLimited:
		return status.Was{Kind: status.WasLimited, Until: a.Routing.Back}
	case status.RoutingRefused:
		return status.Was{Kind: status.WasRefused, Until: a.Refusal.Until}
	case status.RoutingUnreadable, status.RoutingUnread:
		return status.Was{Kind: status.WasUnread}
	case status.RoutingCapped:
		return status.Was{Kind: status.WasCapped, Until: a.Routing.Back}
	}
	return status.Was{Kind: status.WasRoom}
}
