package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

func TestARequestToldOfIsInFlightTillItEndsOrIsLost(t *testing.T) {
	tests := []struct {
		name string
		told views.Told
		want bool
	}{
		{name: "sent", told: views.Told{Sent: now}, want: true},
		{name: "refused, before it goes out again", told: views.Told{Sent: now, Verdict: views.VerdictLimited, Status: 429}, want: true},
		{name: "ended", told: views.Told{Sent: now, Ended: now.Add(time.Second)}},
		{name: "lost as the stream was forgotten", told: views.Told{Sent: now, Lost: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.told.InFlight(); got != tt.want {
				t.Errorf("InFlight = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestWhatOfARequestIsInFlightIsUnknownWhileTheStreamIsntLive holds each
// view that reads the stream to the one rule of what of it they may trust,
// the Log's at each of its hard moments: the List's naming and doing, and a
// session's turns.
func TestWhatOfARequestIsInFlightIsUnknownWhileTheStreamIsntLive(t *testing.T) {
	// mover's last line, on work, served the first prompt; the stream told of
	// its next, the second's, gone out on side and answering as it closed.
	last := withID("r1", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 13, 0), hourUsage)))
	last.Prompt, last.TotalMS = "p1", 5000
	sent := october(7, 13, 10)
	flying := views.Told{
		Request: "r2", Session: mover, Model: opus, Account: "side", Prompt: "p2", Class: ledger.ClassMain,
		Sent: sent, First: sent.Add(time.Second), Chars: 800, Heard: sent.Add(5 * time.Second),
	}
	tests := []struct {
		name    string
		live    bool
		account string
		doing   views.Doing
		turns   int
	}{
		{name: "the stream live: in flight as it told", live: true, account: "side", doing: views.Answering, turns: 2},
		{name: "the stream closed: nothing in flight, as the lines and the router say", account: "work", doing: views.Idle, turns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := views.Stream{Live: tt.live, Requests: map[string]views.Told{flying.Request: flying}}
			held := fakeLedger{lines: []ledger.Line{last}, now: now}
			list := views.ListSessions(views.SessionSources{Unlisted: true, Stream: stream, Ledger: held, Prices: ledger.Pricing, Now: now})
			if len(list.Sessions) != 1 {
				t.Fatalf("the List holds %d sessions, want mover's alone", len(list.Sessions))
			}
			if s := list.Sessions[0]; s.Account != tt.account || s.Activity.Doing != tt.doing {
				t.Errorf("the List has mover on %s, doing %v, want on %s, doing %v", s.Account, s.Activity.Doing, tt.account, tt.doing)
			}
			page, ok := views.SessionPageOf(mover, views.PageSources{
				Unlisted: true, Read: views.ReadSessions(held, nil, now, mover), Stream: stream, Prices: ledger.Pricing, Keep: dayfile.Forever, Now: now,
			})
			if !ok || len(page.Turns) != tt.turns {
				t.Errorf("mover's page has %d turns, want %d", len(page.Turns), tt.turns)
			}
		})
	}
}

func TestARequestToldOfSpendsQuotaButAQuotaCheckOrACountOfTokens(t *testing.T) {
	tests := []struct {
		name string
		told views.Told
		want bool
	}{
		{name: "a message", told: views.Told{}, want: true},
		{name: "Claude Code's quota check", told: views.Told{Check: true}},
		{name: "a count of tokens", told: views.Told{Count: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.told.Spends(); got != tt.want {
				t.Errorf("Spends = %t, want %t", got, tt.want)
			}
		})
	}
}
