package status_test

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// marked runs words together, each account they name in brackets and each
// duration in braces, so a test sees both what they say and the kind of each
// part.
func marked(w status.Words) string {
	var b strings.Builder
	for _, p := range w {
		switch p.Kind {
		case status.PartAccount:
			b.WriteString("[" + p.Text + "]")
		case status.PartDuration:
			b.WriteString("{" + p.Text + "}")
		default:
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func TestReasonWords(t *testing.T) {
	now := time.Date(2026, 10, 9, 13, 2, 0, 0, time.FixedZone("UTC+1", 60*60))
	pressure := status.Facts{Until: time.Date(2026, 10, 9, 14, 10, 0, 0, time.UTC), Now: now}
	tests := []struct {
		name    string
		reason  string
		facts   status.Facts
		event   string
		routing string
		session string
	}{
		{
			name:    "a move off an account at its reserve",
			reason:  "moved: work is at its reserve",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "[work] reached its cap",
			routing: "from [work], at its cap",
			session: "[work] reached its cap",
		},
		{
			name:    "a move off an account at its limit",
			reason:  "moved: work hit its limit",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "[work] reached its limit",
			routing: "from [work], at its limit",
			session: "[work] reached its limit",
		},
		{
			name:    "a move off an account that refused the request",
			reason:  "moved: work was refused",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "[work] refused the request",
			routing: "from [work], refused",
			session: "[work] refused the request",
		},
		{
			name:    "a move off an account with no room",
			reason:  "moved: work has no room",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "[work] had no room",
			routing: "from [work], with no room",
			session: "[work] had no room",
		},
		{
			name:    "a move off an account for a why the dashboard has no words for, as the router gives it",
			reason:  "moved: work was throttled",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "moved: [work] was throttled",
			routing: "moved: [work] was throttled",
			session: "moved: [work] was throttled",
		},
		{
			name:    "a move by pin, to the pin's account",
			reason:  "moved by pin",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "pinned to [side]",
			routing: "from [work], pinned to [side]",
			session: "pinned to [side]",
		},
		{
			name:    "a move by pin, the account it left not given",
			reason:  "moved by pin",
			facts:   status.Facts{To: "side"},
			event:   "pinned to [side]",
			routing: "pinned to [side]",
			session: "pinned to [side]",
		},
		{
			name:    "a move by pin, the pin's account not given",
			reason:  "moved by pin",
			facts:   status.Facts{From: "work"},
			event:   "pinned",
			routing: "from [work], pinned",
			session: "pinned",
		},
		{
			name:    "a session rescored after it idled, its page saying what the choice found",
			reason:  "rescored after 15h idle",
			facts:   status.Facts{From: "side", To: "personal"},
			event:   "rescored after {15h} idle",
			routing: "from [side], rescored after {15h} idle",
			session: "rescored after {15h} idle: [personal] had the most room",
		},
		{
			name:    "a session rescored that stayed where it was, so moved from nowhere",
			reason:  "rescored after 12h 14m idle",
			facts:   status.Facts{To: "side"},
			event:   "rescored after {12h 14m} idle",
			routing: "rescored after {12h 14m} idle",
			session: "rescored after {12h 14m} idle: [side] had the most room",
		},
		{
			name:    "a session rescored, what the choice found not given",
			reason:  "rescored after 15h idle",
			facts:   status.Facts{From: "side"},
			event:   "rescored after {15h} idle",
			routing: "from [side], rescored after {15h} idle",
			session: "rescored after {15h} idle",
		},
		{
			name:    "a choice that passed an account over for pressure, when it runs out in now's time zone",
			reason:  "new, personal under pressure",
			facts:   pressure,
			event:   "[personal] came under pressure, its 5-hour to run out at 15:10",
			routing: "[personal] came under pressure, its 5-hour to run out at 15:10",
			session: "[personal] came under pressure, its 5-hour to run out at 15:10",
		},
		{
			name:    "a move that passed an account over for pressure, when it runs out unknown",
			reason:  "rescored after 12h 14m idle, personal under pressure",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "[personal] came under pressure",
			routing: "[personal] came under pressure",
			session: "[personal] came under pressure",
		},
		{
			name:    "a session's own pin yielding",
			reason:  "pin yields: side has no room",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "its pin to [side] yields: [side] has no room",
			routing: "its pin to [side] yields: [side] has no room",
			session: "its pin to [side] yields: [side] has no room",
		},
		{
			name:    "a new session",
			reason:  "new",
			facts:   status.Facts{To: "side"},
			event:   "started",
			routing: "new session",
			session: "since it started",
		},
		{
			name:    "a session back where it was before its request",
			reason:  "back where it was before its request",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "back where it was",
			routing: "back where it was",
			session: "back where it was",
		},
		{
			name:    "a session kept while its cache is warm, which moves nothing",
			reason:  "sticky",
			facts:   status.Facts{To: "work"},
			event:   "sticky",
			session: "sticky",
		},
		{
			name:    "a session kept for its thinking, which moves nothing",
			reason:  "bound",
			facts:   status.Facts{To: "work"},
			event:   "kept for its thinking",
			session: "kept for its thinking",
		},
		{
			name:    "a session's own pin, which moves nothing",
			reason:  "pinned",
			facts:   status.Facts{To: "work"},
			event:   "pinned here",
			session: "pinned here",
		},
		{
			name:    "the global pin, which moves nothing",
			reason:  "pinned (global)",
			facts:   status.Facts{To: "side"},
			event:   "pinned to [side]",
			session: "pinned to [side]",
		},
		{
			name:    "the global pin, its account not given",
			reason:  "pinned (global)",
			event:   "pinned",
			session: "pinned",
		},
		{
			name:    "any other reason, as the router gives it",
			reason:  "no account has room",
			facts:   status.Facts{To: "work"},
			event:   "no account has room",
			routing: "no account has room",
			session: "no account has room",
		},
		{
			name:    "a reason and accounts from elsewhere, cleaned",
			reason:  "moved  by\tpin\x1b",
			facts:   status.Facts{From: "work\x07", To: "\nside"},
			event:   "pinned to [side]",
			routing: "from [work], pinned to [side]",
			session: "pinned to [side]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marked(status.EventWords(tt.reason, tt.facts)); got != tt.event {
				t.Errorf("EventWords(%q) = %q, want %q", tt.reason, got, tt.event)
			}
			if got := marked(status.RoutingWords(tt.reason, tt.facts)); got != tt.routing {
				t.Errorf("RoutingWords(%q) = %q, want %q", tt.reason, got, tt.routing)
			}
			if got := marked(status.SessionWords(tt.reason, tt.facts)); got != tt.session {
				t.Errorf("SessionWords(%q) = %q, want %q", tt.reason, got, tt.session)
			}
		})
	}
}

func TestWordsString(t *testing.T) {
	w := status.RoutingWords("moved: work is at its reserve", status.Facts{From: "work", To: "side"})
	if got, want := w.String(), "from work, at its cap"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestRescored(t *testing.T) {
	if got, want := status.Rescored("15h"), "rescored after 15h idle"; got != want {
		t.Errorf("Rescored(15h) = %q, want %q", got, want)
	}
}
