package router

import (
	"context"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The holds a refused read starts, as the router tells of them.
var (
	workLapsed       = ReadHeld{Account: "work", Refused: quota.SubscriptionEnded, Status: http.StatusForbidden}
	workTokenRefused = ReadHeld{Account: "work", Refused: quota.TokenRefused, Status: http.StatusUnauthorized}
)

// A probe whose read the API refuses starts a hold, told of once, as it
// starts, naming the account, why the API refused it as the cards say it,
// and the status it answered with: a read refused again while it holds, for
// either cause, tells nothing new, and a read that fails otherwise neither
// starts one nor, after one, lifts it.
func TestAReadRefusedTellsOfTheHoldItStartsOnce(t *testing.T) {
	tests := []struct {
		name  string
		reads []error
		want  []ReadHeld
	}{
		{name: "its subscription ended", reads: []error{errSubscriptionEnded}, want: []ReadHeld{workLapsed}},
		{name: "its token refused", reads: []error{errTokenRefused}, want: []ReadHeld{workTokenRefused}},
		{name: "refused again", reads: []error{errSubscriptionEnded, errSubscriptionEnded}, want: []ReadHeld{workLapsed}},
		{name: "its token refused after", reads: []error{errSubscriptionEnded, errTokenRefused}, want: []ReadHeld{workLapsed}},
		{name: "timed out after", reads: []error{errSubscriptionEnded, errTimedOut, errSubscriptionEnded}, want: []ReadHeld{workLapsed}},
		{name: "the upstream overloaded after", reads: []error{errTokenRefused, errUnreadable, errTokenRefused}, want: []ReadHeld{workTokenRefused}},
		{name: "a read that came back clean between", reads: []error{errSubscriptionEnded, nil, errTokenRefused}, want: []ReadHeld{workLapsed, workTokenRefused}},
		{name: "timed out", reads: []error{errTimedOut}},
		{name: "the upstream overloaded", reads: []error{errUnreadable}},
		{name: "a proxy between refusing its token", reads: []error{errProxyRefused}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var heard []ReadHeld
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.Events = func(e Event) {
				if held, ok := e.(ReadHeld); ok {
					heard = append(heard, held)
				}
			}
			r := newTestRouterOf(t, cfg)
			defer r.probes.stop()
			readWorkFirst(r)
			for _, err := range tt.reads {
				probeEnding(r, "work", err)
			}

			if !reflect.DeepEqual(heard, tt.want) {
				t.Errorf("the router told of the holds %+v, want %+v", heard, tt.want)
			}
		})
	}
}

// A session that moves off a held account says why the API refused its
// read, and is counted in the hold's event, which a clean read lifts as room
// again, naming the cause that lifted.
func TestAHoldCountsTheSessionsItMovesAndRoomAgainTellsItsLift(t *testing.T) {
	tests := []struct {
		name   string
		read   error
		reason string
		cause  string
		status int
	}{
		{name: "its subscription ended", read: errSubscriptionEnded, reason: "moved: work had its read refused, its subscription ended", cause: "subscription lapsed", status: http.StatusForbidden},
		{name: "its token refused", read: errTokenRefused, reason: "moved: work had its read refused, its token refused", cause: "token refused", status: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newTestRouter(t, clock.read, &stubProber{})
			defer r.probes.stop()
			readWorkFirst(r)
			r.recent.look()
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			probeEnding(r, "work", tt.read)
			choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
			clock.now = start.Add(time.Minute)
			r.recent.look()
			probeEnding(r, "work", nil)
			r.recent.look()

			want := []status.Event{
				{ID: 3, At: clock.now, Kind: status.EventRoom, Account: "work", Reason: tt.cause},
				{ID: 2, At: start, Kind: status.EventMoved, Session: "one", Model: opus, From: "work", To: "side", Reason: tt.reason, ForcedBy: 1},
				{ID: 1, At: start, Kind: status.EventHeld, Account: "work", Reason: tt.cause, Status: tt.status, Count: 1, To: "side", Until: clock.now},
			}
			if got := r.recent.events(); !reflect.DeepEqual(got, want) {
				t.Errorf("events() =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

// A hold the state file kept, told of before, is told of again by nothing
// as the router starts once more, neither event nor notification, nor as its
// read is refused again: its lift is room again all the same, and posts as
// room again does.
func TestAHoldKeptAcrossARestartIsToldOfAgainByNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	clock := &testClock{now: start}
	before := newTestFile(clock.read, resolve(testConfigured, testTokens.Read, unrecorded))
	before.load(path)
	before.state.record("work", []quota.Window{session, soonWeek}, before.state.mark())
	probesEnd(before.state, "work", []error{errSubscriptionEnded})
	before.save()

	h := restarted(t, clock, path)
	probeEnding(h.r, "work", errSubscriptionEnded)
	h.looks(2)
	if h.heard != nil || h.r.recent.events() != nil || h.notifier.posted() != nil {
		t.Fatalf("after the restart, the router told of %+v, kept %+v and posted %q, want nothing: work's hold was told of before", h.heard, h.r.recent.events(), h.notifier.posted())
	}

	probesEnd(h.r.state, "work", []error{nil})
	h.looks(1)
	want := []status.Event{{ID: 1, At: start, Kind: status.EventRoom, Account: "work", Reason: "subscription lapsed"}}
	if got := h.r.recent.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("once work is read again, events() = %+v, want %+v", got, want)
	}
	if got := h.notifier.posted(); !reflect.DeepEqual(got, []string{"work · Work has room again"}) {
		t.Errorf("once work is read again, posted %q, want room again", got)
	}
}

// A hold the state file kept that was never told of, as a router from
// before kept it, is told of once as the router starts, from when it began,
// and posts nothing, as its news is old; a restart after tells it again by
// nothing.
func TestAHoldKeptButNeverToldIsToldOnceAsTheRouterStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	clock := &testClock{now: start}
	before := newTestFile(clock.read, resolve(testConfigured, testTokens.Read, unrecorded))
	before.load(path)
	before.state.record("work", []quota.Window{session, soonWeek}, before.state.mark())
	probesEnd(before.state, "work", []error{errSubscriptionEnded})
	before.state.usage["work"].heldTold = false
	before.save()
	clock.now = start.Add(time.Hour)

	h := restarted(t, clock, path)
	h.looks(2)
	if want := []Event{ReadHeld{Account: "work", Refused: quota.SubscriptionEnded, Late: true, Since: start}}; !reflect.DeepEqual(h.heard, want) {
		t.Errorf("as the router starts, it tells of %+v, want %+v", h.heard, want)
	}
	want := []status.Event{{ID: 1, At: start, Kind: status.EventHeld, Account: "work", Reason: "subscription lapsed", Since: start}}
	if got := h.r.recent.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("events() = %+v, want %+v", got, want)
	}
	if got := h.notifier.posted(); got != nil {
		t.Errorf("posted %q, want nothing of a hold told late", got)
	}

	h.r.file.save()
	again := restarted(t, clock, path)
	again.looks(1)
	if again.heard != nil {
		t.Errorf("after another restart, the router told of %+v, want nothing", again.heard)
	}
}

// restarting is a router started on a state file, with what it heard and
// what it posted.
type restarting struct {
	r        *Router
	heard    []Event
	notifier *noting
}

// restarted starts a router on the state file at path, on clock, its
// notifications of limits and room again on.
func restarted(t *testing.T, clock *testClock, path string) *restarting {
	t.Helper()
	h := &restarting{notifier: &noting{}}
	cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
	cfg.Events = func(e Event) { h.heard = append(h.heard, e) }
	cfg.Notifier, cfg.Notifications = h.notifier, config.Notifications{Limits: true, Room: true}
	h.r = newTestRouterOf(t, cfg)
	t.Cleanup(h.r.probes.stop)
	h.r.file.load(path)
	return h
}

// looks has the router look at the accounts n times, as recent and the
// notifications do.
func (h *restarting) looks(n int) {
	for range n {
		h.r.recent.look()
		h.r.notifications.lookAfresh(context.Background())
	}
}

// A move off an account both at its limit and held by its read refused
// counts in what its reason says held it there, one owner giving both: a
// request that went out on it, in what its attempt there met, its limit or
// its refusal, nothing where it met neither, as when it left for a new
// token; one
// that didn't, in the hold's, as the hold holds it back whatever else does.
func TestAMoveOffALimitedAndHeldAccountCountsInWhatItsReasonSays(t *testing.T) {
	tests := []struct {
		name   string
		tried  []Attempt
		reason string
		// held is what the move is held by, and counts the count of each
		// kind of event the account's hold and limit told.
		held   Hold
		counts map[string]int
	}{
		{
			name: "its limit hit there", tried: []Attempt{{Account: "work", Why: whyLimit}}, reason: "moved: work hit its limit",
			held: HeldByLimit, counts: map[string]int{status.EventLimit: 1, status.EventHeld: 0},
		},
		{
			name: "refused there", tried: []Attempt{{Account: "work", Why: whyRefused}}, reason: "moved: work was refused",
			held: HeldByRefusal, counts: map[string]int{status.EventLimit: 0, status.EventHeld: 0},
		},
		{
			name: "left for a new token there", tried: []Attempt{{Account: "work", Why: whyNewToken}}, reason: "moved: work has a new token",
			counts: map[string]int{status.EventLimit: 0, status.EventHeld: 0},
		},
		{
			name: "untried", reason: "moved: work had its read refused, its subscription ended",
			held: HeldByRead, counts: map[string]int{status.EventLimit: 0, status.EventHeld: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			var moves []Moved
			cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
			cfg.Events = func(e Event) {
				if m, ok := e.(Moved); ok {
					moves = append(moves, m)
				}
			}
			r := newTestRouterOf(t, cfg)
			defer r.probes.stop()
			readWorkFirst(r)
			r.recent.look()
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			probeEnding(r, "work", errSubscriptionEnded)
			reached, _ := r.state.limit("work", []string{session.Key}, start.Add(time.Hour), r.state.mark())
			r.recent.hear(reached)
			choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work", Tried: tt.tried})

			if len(moves) != 1 || moves[0].Reason != tt.reason || moves[0].Held != tt.held {
				t.Fatalf("the moves = %+v, want one, %q, held by %v", moves, tt.reason, tt.held)
			}
			counts, ids := make(map[string]int), make(map[string]int)
			var moved status.Event
			for _, e := range r.recent.events() {
				switch e.Kind {
				case status.EventMoved:
					moved = e
				case status.EventLimit, status.EventHeld:
					counts[e.Kind], ids[e.Kind] = e.Count, e.ID
				}
			}
			if !reflect.DeepEqual(counts, tt.counts) {
				t.Errorf("the account's events count %v, want %v", counts, tt.counts)
			}
			for kind, count := range tt.counts {
				if count == 1 && moved.ForcedBy != ids[kind] {
					t.Errorf("the move = %+v, want it forced by the %s's event, %d", moved, kind, ids[kind])
				}
			}
		})
	}
}

// A hold the look that tells its lift didn't read, told since the look read
// the accounts, holds on: the lift ends the hold before it alone.
func TestALiftEndsNoHoldToldAfterTheLookReadTheAccounts(t *testing.T) {
	k := newTelling(t)
	k.reads(0.5, 0.5)
	k.look()
	k.hear(workLapsed)
	k.r.change(func() { k.r.holdLifted("work", start.Add(-time.Second)) })
	if held := k.r.events()[0]; !held.Until.IsZero() {
		t.Errorf("the hold told at %v, after the look at %v, = %+v, want it holding on", held.At, start.Add(-time.Second), held)
	}
	k.r.change(func() { k.r.holdLifted("work", start) })
	if held := k.r.events()[0]; !held.Until.Equal(start) {
		t.Errorf("the hold told by the look = %+v, want it ended at %v", held, start)
	}
}

// A hold begun before the events files take an event back, told late as a
// router from before kept it, is filed from as far back as they take one,
// its true start in since, and reads back from the files.
func TestAHoldToldLateIsFiledWhereTheFilesReadItBack(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	files := fileEvents(t, r)
	began := start.Add(-10 * day)
	r.hear(ReadHeld{Account: "work", Refused: quota.SubscriptionEnded, Late: true, Since: began})

	want := status.Event{ID: 1, At: start.AddDate(0, 0, 1-events.ChangesFor), Kind: status.EventHeld, Account: "work", Reason: "subscription lapsed", Since: began}
	if got := r.events(); !reflect.DeepEqual(got, []status.Event{want}) {
		t.Errorf("events() = %+v, want %+v", got, want)
	}
	reader := events.NewReader(files.stateDir, at(start), logger)
	files.stop()
	var read []status.Event
	for line := range reader.Between(start.Add(-11*day), start.Add(day)) {
		read = append(read, line.Event)
	}
	if !reflect.DeepEqual(read, []status.Event{want}) {
		t.Errorf("the files read back %+v, want %+v", read, want)
	}
}

// A hold told late that doesn't say when it began is filed at now, as one
// told as it starts is.
func TestAHoldToldLateNotSayingWhenItBeganIsFiledAtNow(t *testing.T) {
	r := newTestRecent(&testClock{now: start})
	r.hear(ReadHeld{Account: "work", Refused: quota.SubscriptionEnded, Late: true})

	want := status.Event{ID: 1, At: start, Kind: status.EventHeld, Account: "work", Reason: "subscription lapsed"}
	if got := r.events(); !reflect.DeepEqual(got, []status.Event{want}) {
		t.Errorf("events() = %+v, want %+v", got, want)
	}
}

// A refused read that starts a hold is news, which the state file keeps at
// once, so a router stopped soon after doesn't tell it again; one refused
// again while it holds is routine.
func TestAHoldStartingIsSavedAtOnce(t *testing.T) {
	var changes, routine changeCount
	s := newState(testAccounts(), unpooled(), testPolicy, claude.Provider{}.Family, at(start), changes.hear, routine.hear)
	s.record("work", []quota.Window{session, soonWeek}, s.mark())
	changes, routine = 0, 0
	probesEnd(s, "work", []error{errSubscriptionEnded})
	if changes != 1 || routine != 0 {
		t.Errorf("a hold starting told of %d changes and %d routine ones, want 1 and 0", changes, routine)
	}
	changes, routine = 0, 0
	probesEnd(s, "work", []error{errSubscriptionEnded})
	if changes != 0 || routine != 1 {
		t.Errorf("a read refused again told of %d changes and %d routine ones, want 0 and 1", changes, routine)
	}
}

// Once room again tells a hold's lift, its event ends there, so a move held
// by the account's read refused after it waits for the hold that starts next
// rather than counting in the one lifted.
func TestAHoldLiftedEndsItsEventAndCountsNoMoveAfter(t *testing.T) {
	k := newTelling(t)
	k.reads(0.5, 0.5)
	k.look()
	k.hear(workLapsed)
	probesEnd(k.r.state, "work", []error{errSubscriptionEnded})
	k.look()
	probesEnd(k.r.state, "work", []error{nil})
	k.clock.now = start.Add(time.Minute)
	k.look()
	k.hear(heldOff("one", opus, "side"), workTokenRefused)

	var held []status.Event
	for _, e := range k.r.events() {
		if e.Kind == status.EventHeld {
			held = append(held, e)
		}
	}
	if len(held) != 2 || !held[1].Until.Equal(k.clock.now) || held[1].Count != 0 || !held[0].Until.IsZero() || held[0].Count != 1 {
		t.Errorf("the holds' events are %+v, want the first ending at %v counting none, and the second counting the move", held, k.clock.now)
	}
}

func TestEachHoldAsTheDocumentGivesIt(t *testing.T) {
	tests := []struct {
		name  string
		event ReadHeld
		want  status.Event
	}{
		{
			name:  "its subscription ended",
			event: workLapsed,
			want:  status.Event{Kind: status.EventHeld, Account: "work", Reason: "subscription lapsed", Status: http.StatusForbidden},
		},
		{
			name:  "its token refused",
			event: workTokenRefused,
			want:  status.Event{Kind: status.EventHeld, Account: "work", Reason: "token refused", Status: http.StatusUnauthorized},
		},
		{
			name:  "a status its failure didn't say",
			event: ReadHeld{Account: "work", Refused: quota.SubscriptionEnded},
			want:  status.Event{Kind: status.EventHeld, Account: "work", Reason: "subscription lapsed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecent(&testClock{now: start})
			r.hear(tt.event)
			want := tt.want
			want.ID, want.At = 1, start
			if got := r.events(); !reflect.DeepEqual(got, []status.Event{want}) {
				t.Errorf("events() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAHoldCountsTheSessionsItMoved(t *testing.T) {
	tests := []struct {
		name  string
		heard []Event
		// forcedBy is the id of the hold's event each move heard is counted
		// in, 0 for none, which counts them as wantCount and wantTo say.
		forcedBy  int
		wantCount int
		wantTo    string
	}{
		{
			name:     "each session once, of any model",
			heard:    []Event{workLapsed, heldOff("one", opus, "side"), heldOff("one", haiku, "side"), heldOff("two", haiku, "side")},
			forcedBy: 1, wantCount: 2, wantTo: "side",
		},
		{
			name:     "to several accounts, naming none",
			heard:    []Event{workLapsed, heldOff("one", opus, "side"), heldOff("two", opus, "personal")},
			forcedBy: 1, wantCount: 2,
		},
		{
			name:     "heard before it's told",
			heard:    []Event{heldOff("one", opus, "side"), workLapsed},
			forcedBy: 2, wantCount: 1, wantTo: "side",
		},
		{
			name:     "the newest hold's, once the one before it lifted",
			heard:    []Event{workLapsed, workTokenRefused, heldOff("one", opus, "side")},
			forcedBy: 2, wantCount: 1, wantTo: "side",
		},
		{
			name:  "but by choice",
			heard: []Event{workLapsed, Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved by pin"}},
		},
		{
			name:  "but off another account",
			heard: []Event{workLapsed, Moved{Session: "one", Model: opus, From: "side", To: "work", Reason: "moved: side had its read refused, its token refused", Held: HeldByRead}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			k.hear(tt.heard...)

			for _, e := range k.r.events() {
				switch {
				case e.Kind == status.EventMoved && e.ForcedBy != tt.forcedBy:
					t.Errorf("the move %+v is forced by %d, want %d", e, e.ForcedBy, tt.forcedBy)
				case e.Kind == status.EventHeld && e.ID == tt.forcedBy && (e.Count != tt.wantCount || e.To != tt.wantTo):
					t.Errorf("the hold's event = %+v, want it counting %d moved to %q", e, tt.wantCount, tt.wantTo)
				case e.Kind == status.EventHeld && e.ID != tt.forcedBy && e.Count != 0:
					t.Errorf("the hold's event = %+v, want it counting none", e)
				}
			}
		})
	}
}

// Room again tells a hold's lift as a read comes back clean, or the account
// goes out on another token, naming the cause of the last refusal while it
// held, beside the windows that held it back too; never before both lift,
// nor as a read fails otherwise, which leaves it held. An account never read
// before its first read was refused has room again as its hold lifts, too.
func TestRoomAgainTellsAHoldsLift(t *testing.T) {
	tests := []struct {
		name string
		// read has work read before its hold, unread where it's false; hold
		// holds work back, as it's looked at, and free has it room again,
		// as it's looked at once more.
		read        bool
		hold, free  func(k *telling)
		wantWindows []string
		wantReason  string
	}{
		{
			name:       "its subscription ended, then a clean read",
			read:       true,
			hold:       func(k *telling) { probesEnd(k.r.state, "work", []error{errSubscriptionEnded}) },
			free:       func(k *telling) { probesEnd(k.r.state, "work", []error{nil}) },
			wantReason: "subscription lapsed",
		},
		{
			name: "its subscription ended, then its token refused, then a clean read",
			read: true,
			hold: func(k *telling) {
				probesEnd(k.r.state, "work", []error{errSubscriptionEnded})
				k.look()
				probesEnd(k.r.state, "work", []error{errTokenRefused})
			},
			free:       func(k *telling) { probesEnd(k.r.state, "work", []error{nil}) },
			wantReason: "token refused",
		},
		{
			name:       "its token refused, then another token",
			read:       true,
			hold:       func(k *telling) { probesEnd(k.r.state, "work", []error{errTokenRefused}) },
			free:       func(k *telling) { k.r.state.tokenReplaced("work") },
			wantReason: "token refused",
		},
		{
			name:       "its first read refused, then a clean read",
			hold:       func(k *telling) { probesEnd(k.r.state, "work", []error{errSubscriptionEnded}) },
			free:       func(k *telling) { k.reads(0.5, 0.5) },
			wantReason: "subscription lapsed",
		},
		{
			name: "its subscription ended at its cap, then a clean read once the cap resets",
			read: true,
			hold: func(k *telling) {
				k.record(0.95, sessionEnds, 0.5)
				k.look()
				probesEnd(k.r.state, "work", []error{errSubscriptionEnded})
				k.look()
				probesEnd(k.r.state, "work", []error{nil})
			},
			free:        func(k *telling) { k.clock.now = sessionEnds },
			wantWindows: []string{"5h"},
			wantReason:  "subscription lapsed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			if tt.read {
				k.reads(0.5, 0.5)
			}
			k.look()
			tt.hold(k)
			k.look()
			if got := k.r.events(); len(got) > 0 && got[0].Kind == status.EventRoom {
				t.Fatalf("while work is held, events() = %+v, want no room again", got)
			}
			tt.free(k)
			k.look()

			got := k.r.events()
			want := status.Event{ID: got[0].ID, At: k.clock.now, Kind: status.EventRoom, Account: "work", Windows: tt.wantWindows, Reason: tt.wantReason}
			if !reflect.DeepEqual(got[0], want) {
				t.Errorf("events() = %+v, want the newest %+v", got, want)
			}
		})
	}
}

func TestAHoldIsNeverLiftedByAReadThatFailsOtherwise(t *testing.T) {
	for _, read := range []error{errTimedOut, errUnreadable, errProxyRefused} {
		t.Run(read.Error(), func(t *testing.T) {
			k := newTelling(t)
			k.reads(0.5, 0.5)
			probesEnd(k.r.state, "work", []error{errSubscriptionEnded})
			k.look()
			probesEnd(k.r.state, "work", []error{read})
			k.look()

			k.expect()
		})
	}
}

// A hold is told of as it starts, as a limit is, whatever went out about its
// account just before, under the setting a limit's notification posts under,
// saying why and what to renew.
func TestAHoldIsToldOfAsALimitIs(t *testing.T) {
	const moved = "session one moved from 2 · two to 1 · one (moved by pin)"
	tests := []struct {
		name   string
		limits bool
		held   ReadHeld
		want   []string
	}{
		{
			name: "its subscription ended", limits: true, held: ReadHeld{Account: "2", Refused: quota.SubscriptionEnded, Status: http.StatusForbidden},
			want: []string{moved, "2 · two held: subscription lapsed — its subscription needs renewing"},
		},
		{
			name: "its token refused", limits: true, held: ReadHeld{Account: "2", Refused: quota.TokenRefused, Status: http.StatusUnauthorized},
			want: []string{moved, "2 · two held: token refused — its token needs renewing"},
		},
		{
			name: "with limits off", held: ReadHeld{Account: "2", Refused: quota.SubscriptionEnded, Status: http.StatusForbidden},
			want: []string{moved},
		},
		{
			name: "told late, as a router from before kept it", limits: true, held: ReadHeld{Account: "2", Refused: quota.SubscriptionEnded, Late: true, Since: start.Add(-10 * day)},
			want: []string{moved},
		},
		{
			name: "told late, when it began unknown", limits: true, held: ReadHeld{Account: "2", Refused: quota.SubscriptionEnded, Late: true},
			want: []string{moved},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Limits: tt.limits, Room: true, Warning: 0.9, Moves: true})
				h.start()
				h.hear(Moved{Session: "one", Model: opus, From: "2", To: "1", Reason: "moved by pin"}, tt.held)

				h.expect(tt.want...)
			})
		})
	}
}

// A hold's lift posts as room again does, whatever else held its account
// back: a hold found as notifications first look, as the state file keeps
// one across a restart, included.
func TestRoomAgainOnceAHoldLifts(t *testing.T) {
	withRoom := func(h *notifying) { h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day)) }
	held := func(h *notifying) { probesEnd(h.state, "2", []error{errSubscriptionEnded}) }
	tests := []struct {
		name string
		// before sets 2 up before notifications start, and bar holds it
		// back after.
		before, bar func(h *notifying)
	}{
		{name: "held as it runs", before: withRoom, bar: held},
		{name: "held as it starts", before: func(h *notifying) { withRoom(h); held(h) }, bar: func(*notifying) {}},
		{name: "held, its first read refused", before: held, bar: func(*notifying) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Room: true})
				tt.before(h)
				h.start()
				tt.bar(h)
				h.after(lookEvery)
				h.expect()

				withRoom(h)
				h.after(lookEvery)
				h.expect("2 · two has room again")
			})
		})
	}
}

// heldOff is a session's requests of model moving off work, held by its
// read refused, to the account with the id to.
func heldOff(session, model, to string) Moved {
	return Moved{Session: session, Model: model, From: "work", To: to, Reason: "moved: work had its read refused, its subscription ended", Held: HeldByRead}
}

// probeEnding has a probe of the account with the given id end as err says,
// nil one that comes back clean, reading its windows as they stand, through
// the router's probes, which tell of what it starts.
func probeEnding(r *Router, id string, err error) {
	prober := r.probes.prober.(*stubProber)
	token := r.accounts.only([]string{id})[0].token().Reveal()
	prober.mu.Lock()
	prober.readings = map[string]quota.Probe{token: probed(nil, r.state.usage[id].latest()...)}
	prober.errs = map[string]error{token: err}
	prober.mu.Unlock()
	probeNow(r, id)
}
