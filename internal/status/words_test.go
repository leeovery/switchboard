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

// Words are built of plain words, as they are, accounts named, each by its
// id, cleaned, as it's from elsewhere, figures, and a window's use.
func TestWordsAreBuiltOfPlainWordsAccountsNamedAndFigures(t *testing.T) {
	tests := []struct {
		name string
		part status.Part
		want status.Part
	}{
		{name: "plain words", part: status.Plain(" moved to "), want: status.Part{Text: " moved to "}},
		{name: "an account", part: status.Named("side"), want: status.Part{Text: "side", Kind: status.PartAccount}},
		{name: "an account named from elsewhere", part: status.Named("si\x1b[2Jde"), want: status.Part{Text: "si [2Jde", Kind: status.PartAccount}},
		{name: "a figure", part: status.Figure("7"), want: status.Part{Text: "7", Kind: status.PartFigure}},
		{name: "a window's use", part: status.Use("95%"), want: status.Part{Text: "95%", Kind: status.PartUse}},
	}
	for _, tt := range tests {
		if tt.part != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, tt.part, tt.want)
		}
	}
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
			name:    "a move off an account, routed from the account the line's from gives",
			reason:  "moved: work has no room",
			facts:   status.Facts{From: "personal", To: "side"},
			event:   "[work] had no room",
			routing: "from [personal], with no room",
			session: "[work] had no room",
		},
		{
			name:    "a move off an account that the line says didn't move its session",
			reason:  "moved: work hit its limit",
			facts:   status.Facts{To: "work"},
			event:   "[work] reached its limit",
			session: "[work] reached its limit",
		},
		{
			name:    "a move off an account for a why the dashboard has no words for, as the router gives it",
			reason:  "moved: work was throttled",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "moved: [work] was throttled",
			routing: "from [work], moved: [work] was throttled",
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
			name:    "a move by pin, the account it left not given, so no move",
			reason:  "moved by pin",
			facts:   status.Facts{To: "side"},
			event:   "pinned to [side]",
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
			name:    "a session rescored that stayed where it was, which routing leaves blank",
			reason:  "rescored after 12h 14m idle",
			facts:   status.Facts{To: "side"},
			event:   "rescored after {12h 14m} idle",
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
			name:    "a new session that passed an account over for pressure, when it runs out in now's time zone",
			reason:  "new, personal under pressure",
			facts:   pressure,
			event:   "[personal] came under pressure, its 5-hour to run out at 15:10",
			routing: "new session",
			session: "[personal] came under pressure, its 5-hour to run out at 15:10",
		},
		{
			name:    "a new session that passed an account over for pressure, which runs out on another day, as Dated says it",
			reason:  "new, personal under pressure",
			facts:   status.Facts{Until: time.Date(2026, 10, 9, 23, 30, 0, 0, time.UTC), Now: now},
			event:   "[personal] came under pressure, its 5-hour to run out at Sat 00:30",
			routing: "new session",
			session: "[personal] came under pressure, its 5-hour to run out at Sat 00:30",
		},
		{
			name:    "a move that passed an account over for pressure, when it runs out unknown",
			reason:  "rescored after 12h 14m idle, personal under pressure",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "[personal] came under pressure",
			routing: "from [side], [personal] came under pressure",
			session: "[personal] came under pressure",
		},
		{
			name:    "a choice that passed an account over for pressure but didn't move its session",
			reason:  "rescored after 12h 14m idle, personal under pressure",
			facts:   status.Facts{To: "side"},
			event:   "[personal] came under pressure",
			session: "[personal] came under pressure",
		},
		{
			name:    "a session's own pin yielding",
			reason:  "pin yields: side has no room",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "its pin to [side] yields: [side] has no room",
			routing: "from [side], its pin to [side] yields: [side] has no room",
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
			routing: "from [side], back where it was",
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
			name:    "a session's own pin, where it stayed",
			reason:  "pinned",
			facts:   status.Facts{To: "work"},
			event:   "pinned here",
			session: "pinned here",
		},
		{
			name:    "a session's own pin, which moved it, routed as the router gives it",
			reason:  "pinned",
			facts:   status.Facts{From: "side", To: "work"},
			event:   "pinned here",
			routing: "from [side], pinned here",
			session: "pinned here",
		},
		{
			name:    "the global pin, where the session stayed",
			reason:  "pinned (global)",
			facts:   status.Facts{To: "side"},
			event:   "pinned to [side]",
			session: "pinned to [side]",
		},
		{
			name:    "the global pin, a forced move to one of its accounts",
			reason:  "pinned (global)",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "pinned to [side]",
			routing: "from [work], pinned to [side]",
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
			facts:   status.Facts{From: "work", To: "side"},
			event:   "no account has room",
			routing: "from [work], no account has room",
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
		{
			name:    "a new session, an account it left given all the same: none it left",
			reason:  "new",
			facts:   status.Facts{From: "work", To: "side"},
			event:   "started",
			routing: "new session",
			session: "since it started",
		},
		{
			name:    "a new session's reason from elsewhere, cleaned",
			reason:  "new\n",
			event:   "started",
			routing: "new session",
			session: "since it started",
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
			left, moved := strings.CutPrefix(tt.routing, "from ")
			if got := marked(status.LeftWords(tt.reason, tt.facts)); !moved && got != "" || moved && got != left {
				t.Errorf("LeftWords(%q) = %q, want %q's after its \"from \", none where it starts otherwise", tt.reason, got, tt.routing)
			}
			if got := marked(status.SessionWords(tt.reason, tt.facts)); got != tt.session {
				t.Errorf("SessionWords(%q) = %q, want %q", tt.reason, got, tt.session)
			}
		})
	}
}

func TestTriedWordsSayWhereARequestWentOutBeforeItWentAgainElsewhere(t *testing.T) {
	tests := []struct {
		name  string
		tried []status.Tried
		want  string
	}{
		{name: "none tried"},
		{name: "after a limit", tried: []status.Tried{{Account: "work", Why: status.WhyLimit}}, want: "after [work]'s limit"},
		{name: "after a refusal", tried: []status.Tried{{Account: "work", Why: status.WhyRefused}}, want: "after [work] refused it"},
		{
			name:  "after two, in the order tried",
			tried: []status.Tried{{Account: "work", Why: status.WhyLimit}, {Account: "personal", Why: status.WhyRefused}},
			want:  "after [work]'s limit, [personal] refused it",
		},
		{name: "a why the dashboard has no words for, as the router gives it", tried: []status.Tried{{Account: "work", Why: "has a new token"}},
			want: "after [work] has a new token"},
		{name: "from elsewhere, cleaned", tried: []status.Tried{{Account: "work\x07", Why: "has a\tnew token"}}, want: "after [work] has a new token"},
	}
	for _, tt := range tests {
		if got := marked(status.TriedWords(tt.tried...)); got != tt.want {
			t.Errorf("%s: TriedWords(%v) = %q, want %q", tt.name, tt.tried, got, tt.want)
		}
	}
}

func TestFromWordsAreHowRoutingWordsStart(t *testing.T) {
	from := marked(status.FromWords("work\x07"))
	if routing := marked(status.RoutingWords("moved: work hit its limit", status.Facts{From: "work"})); from != "from [work]" || !strings.HasPrefix(routing, from+", ") {
		t.Errorf("FromWords(work) = %q, and RoutingWords %q, want \"from [work]\", as the routing words start", from, routing)
	}
}

func TestWordsString(t *testing.T) {
	w := status.RoutingWords("moved: work is at its reserve", status.Facts{From: "work", To: "side"})
	if got, want := w.String(), "from work, at its cap"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// rescoredFiled are reasons for sessions rescored after they idled, as the
// build before Rescored took a duration filed them, which ledgers and events
// files keep: how long each idled, the reason filed, and the span its words
// read in it.
var rescoredFiled = []struct {
	name        string
	idle        time.Duration
	filed, span string
}{
	{name: "less than none", idle: -time.Minute, filed: "rescored after now idle", span: "now"},
	{name: "none", idle: 0, filed: "rescored after now idle", span: "now"},
	{name: "under a minute", idle: 59 * time.Second, filed: "rescored after 0m idle", span: "0m"},
	{name: "a minute", idle: time.Minute, filed: "rescored after 1m idle", span: "1m"},
	{name: "just under an hour, cut to the minute", idle: time.Hour - time.Second, filed: "rescored after 59m idle", span: "59m"},
	{name: "an hour", idle: time.Hour, filed: "rescored after 1h idle", span: "1h"},
	{name: "an hour, and seconds too few for a minute", idle: time.Hour + 59*time.Second, filed: "rescored after 1h idle", span: "1h"},
	{name: "just past an hour, its minutes unpadded", idle: time.Hour + time.Minute, filed: "rescored after 1h 1m idle", span: "1h 1m"},
	{name: "hours and minutes", idle: 12*time.Hour + 14*time.Minute, filed: "rescored after 12h 14m idle", span: "12h 14m"},
	{name: "whole hours", idle: 15 * time.Hour, filed: "rescored after 15h idle", span: "15h"},
	{name: "just under a day", idle: 24*time.Hour - time.Second, filed: "rescored after 23h 59m idle", span: "23h 59m"},
	{name: "a day", idle: 24 * time.Hour, filed: "rescored after 1d idle", span: "1d"},
	{name: "a day, and minutes too few for an hour", idle: 24*time.Hour + 59*time.Minute, filed: "rescored after 1d idle", span: "1d"},
	{name: "just past a day", idle: 25 * time.Hour, filed: "rescored after 1d 1h idle", span: "1d 1h"},
	{name: "days and hours, cut to the hour", idle: 5*24*time.Hour + 12*time.Hour + 59*time.Minute, filed: "rescored after 5d 12h idle", span: "5d 12h"},
	{name: "hundreds of days", idle: 400 * 24 * time.Hour, filed: "rescored after 400d idle", span: "400d"},
}

func TestARescoredReasonFilesHowLongItsSessionIdledAsReasonsAlwaysHave(t *testing.T) {
	for _, tt := range rescoredFiled {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Rescored(tt.idle); got != tt.filed {
				t.Errorf("Rescored(%v) = %q, want %q", tt.idle, got, tt.filed)
			}
		})
	}
}

func TestARescoredReasonReadsBackItsIdleSpanFiledBeforeOrSince(t *testing.T) {
	for _, tt := range rescoredFiled {
		t.Run(tt.name, func(t *testing.T) {
			want := "rescored after {" + tt.span + "} idle"
			for _, reason := range []string{tt.filed, status.Rescored(tt.idle)} {
				if got := marked(status.EventWords(reason, status.Facts{})); got != want {
					t.Errorf("EventWords(%q) = %q, want %q", reason, got, want)
				}
			}
		})
	}
}

func TestPassingOverEndsAReasonAsPassedOverReadsIt(t *testing.T) {
	tests := []struct{ reason, account, want string }{
		{reason: status.ReasonNew, account: "personal", want: "new, personal under pressure"},
		{reason: status.Rescored(15 * time.Hour), account: "work", want: "rescored after 15h idle, work under pressure"},
	}
	for _, tt := range tests {
		got := status.PassingOver(tt.reason, tt.account)
		if got != tt.want {
			t.Errorf("PassingOver(%q, %q) = %q, want %q", tt.reason, tt.account, got, tt.want)
		}
		if passed := status.PassedOver(got); passed != tt.account {
			t.Errorf("PassedOver(%q) = %q, want %q", got, passed, tt.account)
		}
	}
}

func TestPassedOverIsTheAccountUnderPressureAChoicePassedOver(t *testing.T) {
	tests := []struct{ reason, want string }{
		{reason: "new, personal under pressure", want: "personal"},
		{reason: "moved: work hit its limit, personal under pressure", want: "personal"},
		{reason: "new,  personal\x1b under pressure", want: "personal"},
		{reason: "new", want: ""},
		{reason: "new, two words under pressure", want: ""},
		{reason: "personal under pressure", want: ""},
	}
	for _, tt := range tests {
		if got := status.PassedOver(tt.reason); got != tt.want {
			t.Errorf("PassedOver(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestRunningWordsSayWhyASessionIsOnItsAccountAndSinceWhen(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("UTC+1", 60*60))
	at := time.Date(2026, 10, 1, 13, 31, 13, 0, time.UTC)
	tests := []struct {
		name   string
		reason string
		facts  status.Facts
		want   string
	}{
		{name: "a new session", reason: status.ReasonNew, facts: status.Facts{At: at, Now: now}, want: "started here 14:31"},
		{name: "a new session on the account new sessions still go to", reason: status.ReasonNew, facts: status.Facts{At: at, Best: true, Now: now}, want: "started here 14:31, the best"},
		{name: "a new session that passed an account over: started, all the same", reason: "new, personal under pressure", facts: status.Facts{At: at, Now: now}, want: "started here 14:31"},
		{name: "its own pin, never the best", reason: status.ReasonPinned, facts: status.Facts{At: at, Best: true, Now: now}, want: "pinned here 14:31"},
		{name: "a move off an account at its reserve, the account it left the reason's", reason: "moved: work is at its reserve", facts: status.Facts{To: "side", At: at, Now: now}, want: "moved from [work] 14:31: [work] reached its cap"},
		{name: "a move by the global pin, the account it left the event's", reason: status.ReasonMovedByPin, facts: status.Facts{From: "work", To: "side", At: at, Now: now}, want: "moved from [work] 14:31: pinned to [side]"},
		{name: "its pin yielding: no when", reason: "pin yields: side has no room", facts: status.Facts{At: at, Now: now}, want: "its pin to [side] yields: [side] has no room"},
		{name: "chosen afresh under the global pin, from nowhere told", reason: status.ReasonGlobalPin, facts: status.Facts{To: "side", At: at, Now: now}, want: "here since 14:31: pinned to [side]"},
		{name: "a reason from a later router, cleaned", reason: "balanced\x1b", facts: status.Facts{At: at, Now: now}, want: "here since 14:31: balanced"},
		{name: "no reason", facts: status.Facts{At: at, Now: now}, want: "here since 14:31"},
		{name: "no reason, from an account told", facts: status.Facts{From: "work", At: at, Now: now}, want: "moved from [work] 14:31"},
		{name: "when unknown: unsaid", reason: status.ReasonGlobalPin, facts: status.Facts{To: "side", Now: now}, want: "pinned to [side]"},
		{name: "on another day: its weekday too", reason: status.ReasonNew, facts: status.Facts{At: at.Add(-48 * time.Hour), Now: now}, want: "started here Tue 14:31"},
		{name: "yesterday, within a day: its weekday too, as Dated says it", reason: status.ReasonNew, facts: status.Facts{At: at.Add(-16 * time.Hour), Now: now}, want: "started here Wed 22:31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marked(status.RunningWords(tt.reason, tt.facts)); got != tt.want {
				t.Errorf("RunningWords(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}
