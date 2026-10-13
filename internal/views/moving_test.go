package views_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func TestAPinThatMovesMovesTheSessionsOnTheAccountsItDoesntName(t *testing.T) {
	const (
		onWork  = "0a1b2c3d-6666-4000-8000-000000000006"
		onSide  = "0a1b2c3d-7777-4000-8000-000000000007"
		ownPin  = "0a1b2c3d-8888-4000-8000-000000000008"
		split   = "0a1b2c3d-9999-4000-8000-000000000009"
		idle    = "0a1b2c3d-aaaa-4000-8000-00000000000a"
		onWorkC = 100
		onSideC = 200
		splitC  = 300
		ownPinC = 400
	)
	on := func(id, pin string, accounts ...string) status.Session {
		s := status.Session{ID: id, Pin: pin}
		for i, account := range accounts {
			model := opus
			if i > 0 {
				model = haiku
			}
			s.Assignments = append(s.Assignments, status.Assignment{Model: model, Account: account})
		}
		return s
	}
	// split's conversation, of Claude Opus 5.5, runs on side; its titles,
	// of Claude Haiku 4.5, go to work.
	running := []status.Session{on(onWork, "", "work"), on(onSide, "", "side"), on(ownPin, "work", "work"), on(split, "", "side", "work")}
	listed := []views.ListedSession{
		{Session: onWork, Account: "work", MoveCost: cost(onWorkC)},
		{Session: onSide, Account: "side", MoveCost: cost(onSideC)},
		{Session: ownPin, Account: "work", MoveCost: cost(ownPinC)},
		{Session: split, Account: "side", MoveCost: cost(splitC)},
	}
	tests := []struct {
		name     string
		accounts []string
		running  []status.Session
		listed   []views.ListedSession
		want     views.Moving
	}{
		{
			name:    "auto, which moves nothing",
			running: running,
			listed:  listed,
			want:    views.Moving{},
		},
		{
			name:     "to side: work's, and split, whose conversation stays",
			accounts: []string{"side"},
			running:  running,
			listed:   listed,
			want:     views.Moving{Sessions: 2, Cost: cost(onWorkC)},
		},
		{
			name:     "to work: side's, and split, whose conversation moves",
			accounts: []string{"work"},
			running:  running,
			listed:   listed,
			want:     views.Moving{Sessions: 2, Cost: cost(onSideC + splitC)},
		},
		{
			name:     "to personal: every one but the one pinned on its own",
			accounts: []string{"personal"},
			running:  running,
			listed:   listed,
			want:     views.Moving{Sessions: 3, Cost: cost(onWorkC + onSideC + splitC)},
		},
		{
			name:     "to work and side, where they all are",
			accounts: []string{"work", "side"},
			running:  running,
			listed:   listed,
			want:     views.Moving{},
		},
		{
			name:     "their costs not listed yet",
			accounts: []string{"personal"},
			running:  running,
			want:     views.Moving{Sessions: 3},
		},
		{
			name:     "one idle past its cache, which costs nothing to move",
			accounts: []string{"side"},
			running:  []status.Session{on(onWork, "", "work"), on(idle, "", "work")},
			listed:   []views.ListedSession{{Session: onWork, Account: "work", MoveCost: cost(onWorkC)}, {Session: idle, Account: "work"}},
			want:     views.Moving{Sessions: 2, Cost: cost(onWorkC)},
		},
		{
			name:     "every one idle past its cache",
			accounts: []string{"side"},
			running:  []status.Session{on(idle, "", "work")},
			listed:   []views.ListedSession{{Session: idle, Account: "work"}},
			want:     views.Moving{Sessions: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.MovingTo(tt.accounts, tt.running, tt.listed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MovingTo(%q) = %d sessions at %v, want %d at %v", tt.accounts, got.Sessions, amount(got.Cost), tt.want.Sessions, amount(tt.want.Cost))
			}
		})
	}
}

func TestAMovesCostIsWhatTheListPricesIt(t *testing.T) {
	seen := now.Add(-10 * time.Minute)
	l := fakeLedger{now: now, lines: []ledger.Line{line(mover, opus, "work", "new", seen, hourUsage)}}
	running := []status.Session{{ID: mover, Assignments: []status.Assignment{{Model: opus, Account: "work", Reason: "new", LastSeen: seen}}}}
	list := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})

	got := views.MovingTo([]string{"side"}, running, list.Sessions)
	if want := (views.Moving{Sessions: 1, Cost: cost(hourRewrite)}); !reflect.DeepEqual(got, want) {
		t.Errorf("MovingTo(side) = %d sessions at %v, want %d at %v: its conversation's prompt written again for an hour",
			got.Sessions, amount(got.Cost), want.Sessions, amount(want.Cost))
	}
}

func TestTwoListingsPlaceTheSessionsAlikeWhereEachModelGoesWhereItWent(t *testing.T) {
	listed := func(pin string, seats ...status.Assignment) status.Session {
		return status.Session{ID: asking, Pin: pin, Assignments: seats}
	}
	opusOn := func(account string, seen time.Duration, inFlight string) status.Assignment {
		return seatOn("claude-opus-5-5", "opus", account, status.ReasonNew, time.Hour, seen, inFlight)
	}
	haikuOn := func(account string) status.Assignment {
		return seatOn("claude-haiku-4-5", "haiku", account, status.ReasonNew, time.Hour, time.Minute, "")
	}
	was := []status.Session{listed("", opusOn("work", time.Minute, ""), haikuOn("side")), {ID: idler, Assignments: []status.Assignment{opusOn("side", 0, "")}}}
	tests := []struct {
		name string
		is   []status.Session
		want bool
	}{
		{name: "as they were", is: was, want: true},
		{name: "in another order", is: []status.Session{was[1], was[0]}, want: true},
		{name: "busy, and seen again since", is: []status.Session{listed("", opusOn("work", 0, status.Answering), haikuOn("side")), was[1]}, want: true},
		{name: "its models listed the other way round", is: []status.Session{listed("", haikuOn("side"), opusOn("work", time.Minute, "")), was[1]}, want: true},
		{name: "a model gone elsewhere", is: []status.Session{listed("", opusOn("side", time.Minute, ""), haikuOn("side")), was[1]}},
		{name: "a model fewer", is: []status.Session{listed("", opusOn("work", time.Minute, "")), was[1]}},
		{name: "pinned since", is: []status.Session{listed("side", opusOn("work", time.Minute, ""), haikuOn("side")), was[1]}},
		{name: "a session gone", is: was[:1]},
		{name: "another in its place", is: []status.Session{was[0], {ID: gone, Assignments: []status.Assignment{opusOn("side", 0, "")}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.SamePlaces(tt.is, was); got != tt.want {
				t.Errorf("SamePlaces = %t, want %t", got, tt.want)
			}
		})
	}
}
