package status_test

import (
	"fmt"
	"slices"
	"strings"
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
// by the holds the router told of, those of a window telling only when one
// the readings show began: the first that holds of no token now, a limit,
// its token refused, nothing read, its cap by its reserve as it holds now,
// and room. A window's hold lifts at its reset, or where a reading after,
// before then, no longer shows it, as a reset made by hand, at that reading.
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
	// handReset is the 5-hour reset by hand at 15:30, as claude.ai's banked
	// reset does, its use dropped and its reset kept.
	handReset := readOf(at(1, 15, 30), "5h", 0.05, at(1, 17, 10))
	// weekOut is the v0.1.6 case: the week at its limit from Monday till it
	// reset at Thursday's 21:00, read again 30 seconds after.
	weekOut := rejected(readOf(at(1, 10, 0).AddDate(0, 0, -3), "7d", 1, at(1, 21, 0)))
	weekReread := readOf(at(1, 21, 0).Add(30*time.Second), "7d", 0, at(8, 21, 0))
	// keptFrom is Tuesday's start, the first day the readings history keeps
	// in the moments that say so; weekKept is work's week as it read it first
	// that day, at its limit since before, as weekOut has it.
	keptFrom := at(1, 0, 0).AddDate(0, 0, -2)
	weekKept := rejected(readOf(keptFrom.Add(8*time.Hour), "7d", 1, at(1, 21, 0)))
	// yesterday is Wednesday's 5-hour window, read at 93% at 15:00.
	yesterday := at(1, 0, 0).AddDate(0, 0, -1)
	nearlySpent := readOf(yesterday.Add(15*time.Hour), "5h", 0.93, yesterday.Add(17*time.Hour))
	refusal := status.Span{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 15, 0)}
	untoldLimit := status.Span{Kind: status.WasLimited, Until: at(1, 15, 30)}
	// fiveHourLimit is the router's limit of work's 5-hour from 15:01:30, as
	// its event tells, a little before the reading shows it; capReached its
	// cap from 14:31, read at 14:31:03.
	fiveHourLimit := status.Span{Kind: status.WasLimited, From: at(1, 15, 1).Add(30 * time.Second), Until: at(1, 17, 10), Windows: []string{"5h"}}
	capReached := status.Span{Kind: status.WasCapped, From: at(1, 14, 31), Until: at(1, 17, 10), Windows: []string{"5h"}}

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
		// reach is how far back the readings reach: where the readings
		// history's keep begins, and the dashboard's own reads.
		reach status.Reach
		told  []status.Span
		t     time.Time
		want  status.Was
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
			name: "a week at its limit before the readings history's keep, read first from it", moment: capping,
			read: []readings.Reading{weekKept, afresh}, reach: status.Reach{Kept: keptFrom}, t: at(1, 20, 50),
			want: status.Was{Kind: status.WasLimited, Until: at(1, 21, 0)},
		},
		{
			name: "a week at its limit before the readings history's keep, as a reading carried in before it has it", moment: capping,
			read: []readings.Reading{weekOut, weekKept, afresh}, reach: status.Reach{Kept: keptFrom}, t: at(1, 20, 50),
			want: status.Was{Kind: status.WasLimited, Since: weekOut.At, Until: at(1, 21, 0)},
		},
		{
			name: "a week reaching its limit after the readings history's keep, read first from it with room", moment: capping,
			read: []readings.Reading{readOf(keptFrom.Add(6*time.Hour), "7d", 0.9, at(1, 21, 0)), weekKept, afresh}, reach: status.Reach{Kept: keptFrom}, t: at(1, 20, 50),
			want: status.Was{Kind: status.WasLimited, Since: weekKept.At, Until: at(1, 21, 0)},
		},
		{
			name: "at its cap, read first from the readings history's keep, its window begun since", moment: capping,
			read: []readings.Reading{capped, stillCapped}, reach: status.Reach{Kept: keptFrom}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasCapped, Since: capped.At, Until: at(1, 17, 10)},
		},
		{
			name: "at its limit as the dashboard first read it itself, its window begun before", moment: capping,
			read: []readings.Reading{nearlySpent, limited}, reach: status.Reach{Seen: limited.At}, t: at(1, 15, 5),
			want: status.Was{Kind: status.WasLimited, Until: at(1, 17, 10)},
		},
		{
			name: "at its limit since the dashboard read it itself short of it", moment: capping,
			read: []readings.Reading{nearlySpent, stillCapped, limited}, reach: status.Reach{Seen: stillCapped.At}, t: at(1, 15, 5),
			want: status.Was{Kind: status.WasLimited, Since: limited.At, Until: at(1, 17, 10)},
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
			name: "at its cap, till its limit, read after", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped, limited}, t: at(1, 14, 50),
			want: status.Was{Kind: status.WasCapped, Since: capped.At, Until: at(1, 17, 10)},
		},
		{
			name: "at its cap, its window reset by hand after, till the reading that shows it", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped, handReset}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasCapped, Since: capped.At, Until: handReset.At},
		},
		{
			name: "at its limit, its window reset by hand after, till the reading that shows it", moment: capping,
			read: []readings.Reading{week, afresh, limited, handReset}, t: at(1, 15, 10),
			want: status.Was{Kind: status.WasLimited, Since: limited.At, Until: handReset.At},
		},
		{
			name: "its window reset by hand, after the reading that shows it", moment: capping,
			read: []readings.Reading{week, afresh, limited, handReset}, t: handReset.At.Add(time.Minute),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "at its limit, read spent again after, till its reset", moment: capping,
			read: []readings.Reading{week, afresh, limited, rejected(readOf(at(1, 15, 20), "5h", 1, at(1, 17, 10)))}, t: at(1, 15, 10),
			want: status.Was{Kind: status.WasLimited, Since: limited.At, Until: at(1, 17, 10)},
		},
		{
			name: "at its limit, read afresh after its reset, till its reset", moment: capping,
			read: []readings.Reading{week, afresh, limited, readOf(at(1, 17, 20), "5h", 0.01, at(1, 22, 10))}, t: at(1, 15, 10),
			want: status.Was{Kind: status.WasLimited, Since: limited.At, Until: at(1, 17, 10)},
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
			name: "a window's limit as its event tells, read after", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped, limited}, told: []status.Span{fiveHourLimit}, t: at(1, 16, 0),
			want: status.Was{Kind: status.WasLimited, Since: fiveHourLimit.From, Until: at(1, 17, 10)},
		},
		{
			name: "a window's limit an event of the period before tells nothing of", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped, limited}, t: at(1, 16, 0),
			told: []status.Span{{Kind: status.WasLimited, From: at(1, 11, 45), Until: at(1, 12, 10), Windows: []string{"5h"}}},
			want: status.Was{Kind: status.WasLimited, Since: limited.At, Until: at(1, 17, 10)},
		},
		{
			name: "a cap as its event tells, read after", moment: capping,
			read: []readings.Reading{week, afresh, capped, stillCapped}, told: []status.Span{capReached}, t: at(1, 15, 0),
			want: status.Was{Kind: status.WasCapped, Since: capReached.From, Until: at(1, 17, 10)},
		},
		{
			name: "a window's limit told, the readings showing room", moment: capping,
			read: []readings.Reading{week, afresh}, told: []status.Span{fiveHourLimit}, t: at(1, 16, 0),
			want: status.Was{Kind: status.WasRoom},
		},
		{
			name: "a limit naming no window, as told, begun while another held on", moment: capping, read: []readings.Reading{week, afresh},
			told: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}, {Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 16, 0)}},
			t:    at(1, 14, 30), want: status.Was{Kind: status.WasLimited, Since: at(1, 12, 0), Until: at(1, 16, 0)},
		},
		{
			name: "a limit naming no window, as told, begun after another lifted early, its until brought forward", moment: capping, read: []readings.Reading{week, afresh},
			told: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}, {Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 10)}},
			t:    at(1, 14, 30), want: status.Was{Kind: status.WasLimited, Since: at(1, 14, 20), Until: at(1, 15, 30)},
		},
		{
			name: "a limit naming no window, as told, then a window read rejected", moment: capping, read: []readings.Reading{week, afresh, limited},
			told: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 0), Until: at(1, 15, 30)}}, t: at(1, 15, 10),
			want: status.Was{Kind: status.WasLimited, Since: at(1, 14, 0), Until: at(1, 17, 10)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.moment.Then(tt.moment.Accounts[0], tt.read, tt.reach, tt.told, tt.t); !sameWas(got, tt.want) {
				t.Errorf("Then(work, %s) = %s, want %s", onDay(tt.t), wasString(got), wasString(tt.want))
			}
		})
	}
}

// Then at a moment's Now is how the moment has each account stand, at every
// hard moment, given the readings its document read and the holds the
// router told of: its kind, as ROUTING's has it, and when its hold lifts.
// When a hold began is told by the readings and the router's events each as
// they can, so the two may differ where the event doesn't tell; where it
// does, Then, given it, says what the moment does, by the one rule.
func TestThenAtNowIsHowTheMomentHasEachAccountStand(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for i, a := range m.Accounts {
				read := hm.Doc.Accounts[i]
				got, want := m.Then(a, readingsOf(read), status.Reach{}, m.Told(a.ID), m.Now), asTheMomentHasIt(a)
				if got.Kind != want.Kind || !got.Until.Equal(want.Until) {
					t.Errorf("Then(%s, Now) = %s, want %s, as the moment has it", a.ID, wasString(got), wasString(want))
				}
				if since := eventSince(a, got.Kind); !since.IsZero() && !got.Since.Equal(since) {
					t.Errorf("Then(%s, Now) = %s, its hold begun as the moment's event of it has it at %s", a.ID, wasString(got), onDay(since))
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

// The resets a window's readings give are each reset as read, and each reset
// by hand, at the reading that shows it, its use fallen by a tenth of the
// window or more, its reset the same; a smaller dip is noise, and another
// window's readings give none of its.
func TestTheResetsAWindowsReadingsGiveIncludeThoseMadeByHand(t *testing.T) {
	reset, next := at(1, 17, 10), at(1, 22, 10)
	byHand := readOf(at(1, 15, 30), "5h", 0.05, reset)
	read := []readings.Reading{
		readOf(at(1, 12, 10), "5h", 0.4, reset),
		readOf(at(1, 13, 0), "7d", 0.9, at(5, 10, 0)),
		readOf(at(1, 14, 0), "5h", 0.95, reset),
		readOf(at(1, 14, 30), "5h", 0.9, reset),
		byHand,
		readOf(at(1, 17, 20), "5h", 0.1, next),
	}
	got := status.ResetsRead(read, "5h")
	slices.SortFunc(got, time.Time.Compare)
	if got, want := slices.Compact(got), []time.Time{byHand.At, reset, next}; !slices.Equal(got, want) {
		t.Errorf("the 5-hour's resets are %v, want %v", got, want)
	}
}

// The holds the router told of, as Then takes them: a limit, as its event
// tells, a span each window it names, from when the window joined it, till
// it lifts, as the router brings its event's until forward as it lifts
// early, or till the account has room again after that, as a router from
// before told of a limit lifting early without bringing its until forward;
// a cap alike; the refusal of its token, as its event tells, whose until the
// router brings forward too; and a limit naming no window, or a refusal,
// holding at Now that no event tells of, from before anything.
func TestTheHoldsTheRouterToldOf(t *testing.T) {
	unnamed := status.Event{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 15, 30).UTC(), Limit: 3}
	refused := status.Event{At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "work", Until: at(1, 15, 0).UTC(), Status: 401}
	// earlier is a limit naming no window from 12:00, held till 13:30.
	earlier := status.Event{At: at(1, 12, 0).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 13, 30).UTC(), Limit: 2}
	roomAt := func(h, m int) status.Event {
		return status.Event{At: at(1, h, m).UTC(), Kind: status.EventRoom, Account: "work"}
	}
	limitHolding := status.Limit{ID: 3, Until: at(1, 15, 30).UTC()}
	refusalHolding := status.Refusal{Until: at(1, 15, 0).UTC(), Status: 401}
	tests := []struct {
		name string
		// events are the router's, newest first; limit and refused are
		// work's, as its document has them.
		events  []status.Event
		limit   status.Limit
		refused status.Refusal
		want    []status.Span
	}{
		{name: "none"},
		{
			name: "a limit naming no window, as its event tells", events: []status.Event{unnamed}, limit: limitHolding,
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}},
		},
		{
			name:   "a limit naming a window, which the readings show",
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Limit: 4}},
			limit:  status.Limit{ID: 4, Windows: []string{"5h"}, Until: at(1, 17, 10).UTC()},
			want:   []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 17, 10), Windows: []string{"5h"}}},
		},
		{
			name: "a limit naming a window, lifted, the room after it",
			events: []status.Event{
				roomAt(14, 40), {At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Limit: 4},
			},
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 14, 40), Windows: []string{"5h"}}},
		},
		{
			name:   "a cap, which the readings show",
			events: []status.Event{{At: at(1, 14, 31).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Reserve: 0.1}},
			want:   []status.Span{{Kind: status.WasCapped, From: at(1, 14, 31), Until: at(1, 17, 10), Windows: []string{"5h"}}},
		},
		{
			name: "a cap a window joined after it began, a span each window, from when it joined",
			events: []status.Event{{
				At: at(0, 10, 0).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d_oi", "5h"},
				Joined: map[string]time.Time{"5h": at(1, 14, 31).UTC()}, Until: at(5, 21, 0).UTC(), Reserve: 0.1,
			}},
			want: []status.Span{
				{Kind: status.WasCapped, From: at(0, 10, 0), Until: at(5, 21, 0), Windows: []string{"7d_oi"}},
				{Kind: status.WasCapped, From: at(1, 14, 31), Until: at(5, 21, 0), Windows: []string{"5h"}},
			},
		},
		{
			name: "a cap a window joined after it began, the room after the cap began and before the window joined",
			events: []status.Event{roomAt(12, 0), {
				At: at(0, 10, 0).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d_oi", "5h"},
				Joined: map[string]time.Time{"5h": at(1, 14, 31).UTC()}, Until: at(5, 21, 0).UTC(), Reserve: 0.1,
			}},
			want: []status.Span{
				{Kind: status.WasCapped, From: at(0, 10, 0), Until: at(1, 12, 0), Windows: []string{"7d_oi"}},
				{Kind: status.WasCapped, From: at(1, 14, 31), Until: at(5, 21, 0), Windows: []string{"5h"}},
			},
		},
		{
			name: "a cap of two windows, as an event told before events kept when each joined, each from when it began",
			events: []status.Event{{
				At: at(1, 13, 0).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d_oi", "5h"}, Until: at(5, 21, 0).UTC(), Reserve: 0.1,
			}},
			want: []status.Span{
				{Kind: status.WasCapped, From: at(1, 13, 0), Until: at(5, 21, 0), Windows: []string{"7d_oi"}},
				{Kind: status.WasCapped, From: at(1, 13, 0), Until: at(5, 21, 0), Windows: []string{"5h"}},
			},
		},
		{
			name:   "a limit naming a window holding, and one naming none holding at Now untold",
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Limit: 4}},
			limit:  limitHolding,
			want: []status.Span{
				{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 17, 10), Windows: []string{"5h"}}, {Kind: status.WasLimited, Until: at(1, 15, 30)},
			},
		},
		{
			name: "a limit naming no window, lifted early, as the room after it tells", events: []status.Event{roomAt(13, 10), earlier},
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 10)}},
		},
		{
			name:   "a limit naming no window, lifted early while its cap held, the router bringing its until forward",
			events: []status.Event{{At: at(1, 12, 0).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 13, 10).UTC(), Limit: 2}},
			want:   []status.Span{{Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 10)}},
		},
		{
			name: "a limit naming no window, lifted on time, the room after it of its cap", events: []status.Event{roomAt(14, 0), earlier},
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 30)}},
		},
		{
			name: "a limit naming no window, the room before it of an older hold", events: []status.Event{unnamed, roomAt(14, 0)}, limit: limitHolding,
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}},
		},
		{
			name:   "a limit naming no window, the room after it another account's",
			events: []status.Event{{At: at(1, 13, 10).UTC(), Kind: status.EventRoom, Account: "side"}, earlier},
			want:   []status.Span{{Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 30)}},
		},
		{
			name: "its token refused, as its event tells", events: []status.Event{refused}, refused: refusalHolding,
			want: []status.Span{{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 15, 0)}},
		},
		{
			name:   "its token refused, lifted early, the router bringing its until forward, the room after it of its cap",
			events: []status.Event{roomAt(14, 20), {At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "work", Until: at(1, 14, 30).UTC(), Status: 401}},
			want:   []status.Span{{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 14, 30)}},
		},
		{
			name:    "a model family refused, which holds some models' requests alone",
			events:  []status.Event{{At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "work", Until: at(1, 15, 0).UTC(), Status: 429, Family: "fable"}},
			refused: status.Refusal{Until: at(1, 15, 0).UTC(), Status: 429, Family: "fable"},
		},
		{
			name: "another account's",
			events: []status.Event{
				{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "side", Until: at(1, 15, 30).UTC(), Limit: 5},
				{At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "side", Until: at(1, 15, 0).UTC(), Status: 401},
			},
		},
		{
			name: "a limit naming no window holding at Now, no event telling", limit: limitHolding,
			want: []status.Span{{Kind: status.WasLimited, Until: at(1, 15, 30)}},
		},
		{
			name: "its token refused at Now, no event telling", refused: refusalHolding,
			want: []status.Span{{Kind: status.WasRefused, Until: at(1, 15, 0)}},
		},
		{
			name: "a limit naming no window holding at Now, an older one's event told", events: []status.Event{earlier}, limit: limitHolding,
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 12, 0), Until: at(1, 13, 30)}, {Kind: status.WasLimited, Until: at(1, 15, 30)}},
		},
		{
			name:   "a limit naming no window holding at Now, as an event without its identity tells, as a router from before limits had them",
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 15, 30).UTC()}},
			limit:  status.Limit{Until: at(1, 15, 30).UTC()},
			want:   []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}},
		},
		{
			name: "a limit naming no window holding at Now untold, its token's refusal told", events: []status.Event{refused}, limit: limitHolding, refused: refusalHolding,
			want: []status.Span{{Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 15, 0)}, {Kind: status.WasLimited, Until: at(1, 15, 30)}},
		},
		{
			name: "both, each as its event tells", events: []status.Event{unnamed, refused}, limit: limitHolding, refused: refusalHolding,
			want: []status.Span{{Kind: status.WasLimited, From: at(1, 14, 20), Until: at(1, 15, 30)}, {Kind: status.WasRefused, From: at(1, 14, 12), Until: at(1, 15, 0)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := workAt(func(d *status.Document) {
				d.Events, d.Accounts[0].Limit, d.Accounts[0].Refused = tt.events, tt.limit, tt.refused
			})
			if got := m.Told("work"); !slices.EqualFunc(got, tt.want, sameSpan) {
				t.Errorf("Told(work) = %s, want %s", spansString(got), spansString(tt.want))
			}
		})
	}
}

// sameSpan reports whether a and b say the same, their times the same
// instants.
func sameSpan(a, b status.Span) bool {
	return a.Kind == b.Kind && a.From.Equal(b.From) && a.Until.Equal(b.Until) && slices.Equal(a.Windows, b.Windows)
}

// spansString shows the spans, their times on their days as stateNow's zone
// has them.
func spansString(spans []status.Span) string {
	shown := make([]string, len(spans))
	for i, s := range spans {
		shown[i] = fmt.Sprintf("{Kind:%d From:%s Until:%s Windows:%v}", s.Kind, onDay(s.From), onDay(s.Until), s.Windows)
	}
	return "[" + strings.Join(shown, " ") + "]"
}

// When a hold began is told by the router's event of it, as the moment has
// it, and by the first reading of the unbroken run that shows it, as Then has
// it of the readings alone: where both tell, they agree, and Then, given the
// holds told, says it too; where the event is missing, the readings tell;
// and a hold neither tells of, as a limit naming no window untold, began
// before anything given. A hold of a window every model shares that joined
// the event of a model's own window's hold began as it joined, not as that
// event began, however long before.
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
	// fableCapped is work's Fable week at its reserve too, its cap's event
	// begun as that week reached it, at fableAt, which holds back Fable's
	// requests alone, the 5-hour joining it at 14:31:03.
	fableCapped := with(readNow(0.97, 0.4, 0.95), reserving)
	fableCap := func(fableAt time.Time) status.Event {
		return status.Event{
			At: fableAt.UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d_oi", "5h"},
			Joined: map[string]time.Time{"5h": capReached.At}, Until: at(5, 21, 0).UTC(), Reserve: 0.1,
		}
	}
	// spent is work's 5-hour window read rejected at 14:20, the router's
	// limit of it holding till it resets.
	spent := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) {
		a.Windows[0].Utilization, a.Windows[0].Status = 1, quota.StatusRejected
		a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: at(1, 17, 10).UTC()}
	})
	spentRead := []readings.Reading{rejected(readOf(at(1, 14, 20), "5h", 1, at(1, 17, 10)))}
	// fableSpent is work's Fable week spent since the day before, the
	// router's limit of it holding till it resets, the 5-hour joining that
	// limit as an answer at 14:20 named both.
	fableSpent := with(spent, func(a *status.Account) {
		a.Windows[2].Utilization, a.Windows[2].Status = 1, quota.StatusRejected
		a.Limit = status.Limit{ID: 4, Windows: []string{"7d_oi", "5h"}, Until: at(5, 21, 0).UTC()}
	})
	fableLimit := status.Event{
		At: at(0, 10, 0).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"7d_oi", "5h"},
		Joined: map[string]time.Time{"5h": at(1, 14, 20).UTC()}, Until: at(5, 21, 0).UTC(), Limit: 4,
	}
	unnamed := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) { a.Limit = status.Limit{ID: 3, Until: at(1, 15, 30).UTC()} })
	refused := with(readNow(0.2, 0.4, 0.2), func(a *status.Account) { a.Refused = status.Refusal{Until: at(1, 15, 0).UTC(), Status: 401} })
	tests := []struct {
		name    string
		account status.Account
		events  []status.Event
		read    []readings.Reading
		kind    status.WasKind
		// event is when the hold began as the router's event tells, and
		// then as Then tells, given the holds told: zero where they don't.
		event, then time.Time
	}{
		{name: "a cap, its event and its first reading agreeing", account: capping, events: []status.Event{capReached}, read: capRead, kind: status.WasCapped, event: capReached.At, then: capReached.At},
		{name: "a cap, its event missing", account: capping, read: capRead, kind: status.WasCapped, then: capReached.At},
		{
			name: "a cap of every request joining the cap of a model's own week reached the day before", account: fableCapped,
			events: []status.Event{fableCap(at(0, 10, 0))}, read: capRead, kind: status.WasCapped, event: capReached.At, then: capReached.At,
		},
		{
			name: "a cap of every request joining the cap of a model's own week reached in the 5-hour's period", account: fableCapped,
			events: []status.Event{fableCap(at(1, 13, 0))}, read: capRead, kind: status.WasCapped, event: capReached.At, then: capReached.At,
		},
		{
			name: "a window read spent, its limit's event and its reading agreeing", account: spent,
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Limit: 4}},
			read:   spentRead, kind: status.WasLimited, event: at(1, 14, 20), then: at(1, 14, 20),
		},
		{
			name: "a window every model shares joining the limit of a model's own week reached the day before", account: fableSpent,
			events: []status.Event{fableLimit}, read: spentRead, kind: status.WasLimited, event: at(1, 14, 20), then: at(1, 14, 20),
		},
		{
			name: "a limit naming no window, as its event tells", account: unnamed,
			events: []status.Event{{At: at(1, 14, 20).UTC(), Kind: status.EventLimit, Account: "work", Until: at(1, 15, 30).UTC(), Limit: 3}},
			read:   []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasLimited, event: at(1, 14, 20), then: at(1, 14, 20),
		},
		{
			name: "a limit naming no window, neither its event nor a reading telling", account: unnamed,
			read: []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasLimited,
		},
		{
			name: "its token refused, as its event tells", account: refused,
			events: []status.Event{{At: at(1, 14, 12).UTC(), Kind: status.EventRefused, Account: "work", Until: at(1, 15, 0).UTC(), Status: 401}},
			read:   []readings.Reading{readOf(at(1, 12, 20), "5h", 0.2, at(1, 17, 10))}, kind: status.WasRefused, event: at(1, 14, 12), then: at(1, 14, 12),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Accounts: []status.Account{tt.account}, Events: tt.events}
			m := doc.At(stateNow, policy)
			a := m.Accounts[0]
			was := m.Then(a, tt.read, status.Reach{}, m.Told(a.ID), m.Now)
			if was.Kind != tt.kind || !eventSince(a, tt.kind).Equal(tt.event) || !was.Since.Equal(tt.then) {
				t.Errorf("Then(work, Now) = %s, its event telling it began at %s; want of kind %d, as its event tells at %s, as Then tells at %s",
					wasString(was), onDay(eventSince(a, tt.kind)), tt.kind, onDay(tt.event), onDay(tt.then))
			}
			event := eventSince(a, tt.kind)
			if read := m.Then(a, tt.read, status.Reach{}, nil, m.Now); read.Kind == tt.kind && !read.Since.IsZero() && !event.IsZero() && !read.Since.Equal(event) {
				t.Errorf("the readings alone tell the hold began at %s, its event at %s; want them to agree", onDay(read.Since), onDay(event))
			}
		})
	}
}

// A hold of every request one window every model shares began and another
// took over, with no room between, began as the first did, though that one
// has reset since: as the router's event tells it, as the moment has it, and
// as Then has it given the holds told. Where the account had room between,
// it began as the second did. The readings alone tell it too, with no event
// telling, as far as they reach: where the run of the window that began it
// is among them, as it began; where it isn't, as the window holding it now
// began its own run; and where that window's run began before what they
// reach, at no time they tell.
func TestAHoldOfEveryRequestBeganAsItsUnbrokenRunAcrossWindowsDid(t *testing.T) {
	// The 5-hour's period before this one, which reset at 12:10, and the
	// week's.
	earlierReset, weekReset := at(1, 12, 10).UTC(), at(5, 21, 0).UTC()
	capped := with(readNow(0.3, 0.95, 0.2), reserving)
	ownCapped := with(readNow(0.3, 0.95, 0.95), reserving)
	// spent is work's week read spent, the router's limit holding it alone.
	spent := with(readNow(0.3, 1, 0.2), func(a *status.Account) {
		a.Windows[1].Status = quota.StatusRejected
		a.Limit = status.Limit{ID: 4, Windows: []string{"7d"}, Until: weekReset}
	})
	// reachedAt has work's 5-hour reach its reserve at reached, before its
	// reset at 12:10, read afresh at 12:20, and the week reach it at joined.
	reachedAt := func(reached, joined time.Time) []readings.Reading {
		return []readings.Reading{
			readOf(at(1, 9, 0), "5h", 0.5, earlierReset), readOf(at(1, 9, 0), "7d", 0.85, weekReset),
			readOf(reached, "5h", 0.95, earlierReset),
			readOf(joined, "7d", 0.95, weekReset),
			readOf(at(1, 12, 20), "5h", 0.3, at(1, 17, 10)),
		}
	}
	spentRead := []readings.Reading{
		readOf(at(1, 9, 0), "5h", 0.5, earlierReset), readOf(at(1, 9, 0), "7d", 0.85, weekReset),
		rejected(readOf(at(1, 10, 0), "5h", 1, earlierReset)),
		rejected(readOf(at(1, 11, 0), "7d", 1, weekReset)),
		readOf(at(1, 12, 20), "5h", 0.3, at(1, 17, 10)),
	}
	capEvent := status.Event{
		At: at(1, 10, 0).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"5h", "7d"},
		Joined: map[string]time.Time{"7d": at(1, 11, 0).UTC()}, Until: weekReset, Reserve: 0.1,
	}
	tests := []struct {
		name    string
		account status.Account
		events  []status.Event
		read    []readings.Reading
		// reach is how far back the readings reach.
		reach status.Reach
		kind  status.WasKind
		// want is when the hold began, as the events tell, and ownRun as the
		// readings alone do.
		want, ownRun time.Time
	}{
		{
			name: "a cap the 5-hour began and the week took over", account: capped, events: []status.Event{capEvent},
			read: reachedAt(at(1, 10, 0), at(1, 11, 0)), kind: status.WasCapped, want: at(1, 10, 0), ownRun: at(1, 10, 0),
		},
		{
			name: "a cap the 5-hour began and the week took over, the 5-hour's run not among the readings", account: capped,
			events: []status.Event{capEvent}, read: reachedAt(at(1, 10, 0), at(1, 11, 0))[3:], kind: status.WasCapped,
			want: at(1, 10, 0), ownRun: at(1, 11, 0),
		},
		{
			name: "a cap the 5-hour began before the readings reach and the week took over", account: capped,
			events: []status.Event{capEvent},
			read: []readings.Reading{
				readOf(at(1, 10, 0), "5h", 0.95, earlierReset), readOf(at(1, 10, 5), "7d", 0.85, weekReset),
				readOf(at(1, 11, 0), "7d", 0.95, weekReset), readOf(at(1, 12, 20), "5h", 0.3, at(1, 17, 10)),
			},
			reach: status.Reach{Kept: at(1, 10, 0)}, kind: status.WasCapped, want: at(1, 10, 0),
		},
		{
			name: "a cap the 5-hour began and the week joined once the account had room again", account: ownCapped,
			events: []status.Event{
				{At: at(1, 12, 11).UTC(), Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
				{
					At: at(0, 10, 0).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d_oi", "5h", "7d"},
					Joined: map[string]time.Time{"5h": at(1, 10, 0).UTC(), "7d": at(1, 14, 0).UTC()}, Until: weekReset, Reserve: 0.1,
				},
			},
			read: reachedAt(at(1, 10, 0), at(1, 14, 0)), kind: status.WasCapped, want: at(1, 14, 0), ownRun: at(1, 14, 0),
		},
		{
			name: "a limit the 5-hour began and the week took over", account: spent,
			events: []status.Event{{
				At: at(1, 10, 0).UTC(), Kind: status.EventLimit, Account: "work", Windows: []string{"5h", "7d"},
				Joined: map[string]time.Time{"7d": at(1, 11, 0).UTC()}, Until: weekReset, Limit: 4,
			}},
			read: spentRead, kind: status.WasLimited, want: at(1, 10, 0), ownRun: at(1, 10, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Accounts: []status.Account{tt.account}, Events: tt.events}
			m := doc.At(stateNow, policy)
			a := m.Accounts[0]
			if since := eventSince(a, tt.kind); !since.Equal(tt.want) {
				t.Errorf("the moment has work held since %s; want %s", onDay(since), onDay(tt.want))
			}
			if was := m.Then(a, tt.read, tt.reach, m.Told(a.ID), m.Now); was.Kind != tt.kind || !was.Since.Equal(tt.want) {
				t.Errorf("Then(work, Now), given the holds told, = %s; want of kind %d, since %s", wasString(was), tt.kind, onDay(tt.want))
			}
			if was := m.Then(a, tt.read, tt.reach, nil, m.Now); was.Kind != tt.kind || !was.Since.Equal(tt.ownRun) {
				t.Errorf("Then(work, Now), of the readings alone, = %s; want of kind %d, since %s", wasString(was), tt.kind, onDay(tt.ownRun))
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
// it, by ROUTING's kind, each hold lifting as that hold alone does, as Then
// says a kind at a time, though another may hold the account back after it:
// held back from every request by a limit, till its hold lifts; its token
// refused, till the refusal ends; nothing read of it; at its cap, which
// holds it back, till its cap lets it go; and room, as a hold of some
// models' requests alone, a cap the global pin spends, pressure and an idle
// window all leave it.
func asTheMomentHasIt(a status.AccountNow) status.Was {
	switch a.Routing.Kind {
	case status.RoutingTokenless:
		return status.Was{Kind: status.WasTokenless}
	case status.RoutingLimited:
		return status.Was{Kind: status.WasLimited, Until: a.Hold.Until}
	case status.RoutingRefused:
		return status.Was{Kind: status.WasRefused, Until: a.Refusal.Until}
	case status.RoutingUnreadable, status.RoutingUnread:
		return status.Was{Kind: status.WasUnread}
	case status.RoutingCapped:
		return status.Was{Kind: status.WasCapped, Until: a.CapBack}
	}
	return status.Was{Kind: status.WasRoom}
}
