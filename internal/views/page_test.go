package views_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions of the page's tests: paged, whose page is built, and other,
// another session on its accounts.
const (
	paged = "0a1b2c3d-6666-4000-8000-000000000006"
	other = "0a1b2c3d-7777-4000-8000-000000000007"
)

// fable is the model of the tests' requests that read Fable's own week.
const fable = "claude-fable-5-1"

// The prompts of the turns' tests.
const (
	p1 = "prompt-1"
	p2 = "prompt-2"
)

// took returns l, its request taking the given time from its arrival to its
// end.
func took(d time.Duration, l ledger.Line) ledger.Line {
	l.TotalMS = d.Milliseconds()
	return l
}

// serving returns l, a request of the class given serving the prompt given,
// as its client's headers say.
func serving(prompt, class string, l ledger.Line) ledger.Line {
	l.Prompt, l.Class = prompt, class
	return l
}

// stopped returns l, its answer stopping as stop says, after calling the
// tools given.
func stopped(stop string, l ledger.Line, tools ...string) ledger.Line {
	l.Answer.Stop, l.Answer.Tools = stop, tools
	return l
}

// mainOn is a request of paged's own conversation on side, of Claude Opus 5.5,
// serving prompt, arriving at at and taking a minute, its answer stopping as
// stop says.
func mainOn(prompt string, at time.Time, stop string) ledger.Line {
	return stopped(stop, took(time.Minute, serving(prompt, ledger.ClassMain, line(paged, opus, "side", "sticky", at, hourUsage))))
}

// onAccount returns l, its request answered on the account given.
func onAccount(account string, l ledger.Line) ledger.Line {
	l.Account = account
	return l
}

// subagent is a request of paged's subagent with the given id on work, of
// Claude Haiku 4.5, serving prompt, as its client names it, arriving at at
// and taking a minute; started by the subagent parent, where it's given.
func subagent(agent, parent, prompt string, at time.Time) ledger.Line {
	l := took(time.Minute, serving(prompt, "subagent", line(paged, haiku, "work", "sticky", at, smallUsage)))
	l.AgentID, l.ParentAgentID = agent, parent
	return l
}

// local is the local time on 7 October at hour:minute:second.
func local(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 7, hour, minute, second, 0, time.Local)
}

// turnSaid is what a turn's tests hold of it: its number, when it started and
// ended, its requests and the accounts they went to.
type turnSaid struct {
	turn           int
	started, ended time.Time
	requests       int
	accounts       []string
}

// saidOf returns what the turns' tests hold of turns.
func saidOf(turns []views.Turn) []turnSaid {
	said := make([]turnSaid, len(turns))
	for i, t := range turns {
		said[i] = turnSaid{turn: t.Turn, started: t.Started, ended: t.Ended, requests: t.Requests, accounts: t.Accounts}
	}
	return said
}

// pageOf builds paged's page from lines, oldest first, the router listing
// routed, at now, the ledger keeping a day's lines 90 days.
func pageOf(t *testing.T, lines []ledger.Line, routed *status.Session) views.SessionPage {
	t.Helper()
	page, ok := views.SessionPageOf(paged, views.PageSources{Routed: routed, Ledger: fakeLedger{now: now, lines: lines}, Prices: ledger.Pricing,
		Keep: 90 * 24 * time.Hour, Now: now})
	if !ok {
		t.Fatalf("SessionPageOf(%s) found no session in %+v", paged, lines)
	}
	return page
}

// runningOn is paged as the router lists it, its Claude Opus 5.5 on the
// account given.
func runningOn(account string) *status.Session {
	return &status.Session{ID: paged, Assignments: []status.Assignment{{Model: opus, Account: account, Reason: "sticky", LastSeen: now}}}
}

func TestATurnIsAPromptAndEveryRequestThatServesIt(t *testing.T) {
	sideOnly := []string{"side"}
	tests := []struct {
		name    string
		lines   []ledger.Line
		running bool
		want    []turnSaid
	}{
		{
			name: "a message left to queue joins the prompt running",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), mainOn(p1, local(10, 2, 0), "tool_use"), mainOn(p1, local(10, 4, 0), "end_turn"),
				mainOn(p2, local(10, 10, 0), "end_turn")},
			want: []turnSaid{{2, local(10, 10, 0), local(10, 11, 0), 1, sideOnly}, {1, local(10, 0, 0), local(10, 5, 0), 3, sideOnly}},
		},
		{
			name:  "a message pushed in starts a prompt of its own, the one before ending with its last answer",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), mainOn(p2, local(10, 1, 0), "tool_use"), mainOn(p2, local(10, 3, 0), "end_turn")},
			want:  []turnSaid{{2, local(10, 1, 0), local(10, 4, 0), 2, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name: "a subagent counts in the turn of the prompt that started it, whatever prompts come meanwhile, never keeping it going, nor moving it, on another account",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), subagent("a1", "", p1, local(10, 1, 0)), mainOn(p2, local(10, 5, 0), "end_turn"),
				subagent("a1", "", p2, local(10, 6, 0)), subagent("a1", "", p2, local(10, 20, 0))},
			want: []turnSaid{{2, local(10, 5, 0), local(10, 6, 0), 1, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 4, sideOnly}},
		},
		{
			name: "a nested subagent counts in the turn of the prompt its first request names",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), subagent("a1", "", p1, local(10, 1, 0)), subagent("a2", "a1", p1, local(10, 2, 0)),
				mainOn(p2, local(10, 5, 0), "end_turn"), subagent("a2", "a1", p2, local(10, 6, 0))},
			want: []turnSaid{{2, local(10, 5, 0), local(10, 6, 0), 1, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 4, sideOnly}},
		},
		{
			name: "a compaction, and a side request, count in the turn of the prompt they name",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "end_turn"), took(time.Minute, serving(p1, "compaction", line(paged, opus, "side", "sticky", local(10, 1, 0), hourUsage))),
				mainOn(p2, local(10, 5, 0), "end_turn"), took(time.Minute, serving(p2, "auxiliary", line(paged, haiku, "work", "sticky", local(10, 5, 30), smallUsage)))},
			want: []turnSaid{{2, local(10, 5, 0), local(10, 6, 0), 2, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 2, sideOnly}},
		},
		{
			name:  "a request that names no prompt is in no turn",
			lines: []ledger.Line{line(paged, opus, "side", "new", local(9, 0, 0), hourUsage), mainOn(p1, local(10, 0, 0), "end_turn")},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:  "none, of lines from before prompts were told",
			lines: []ledger.Line{line(paged, opus, "side", "new", local(9, 0, 0), hourUsage), line(paged, opus, "side", "sticky", local(9, 5, 0), hourUsage)},
			want:  []turnSaid{},
		},
		{
			name:    "the newest of a running session still going, as its last answer called a tool, the one before ended",
			lines:   []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), mainOn(p2, local(10, 5, 0), "tool_use")},
			running: true,
			want:    []turnSaid{{2, local(10, 5, 0), time.Time{}, 1, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:    "the newest of a running session still going, as its last answer paused",
			lines:   []ledger.Line{mainOn(p1, local(10, 0, 0), "pause_turn")},
			running: true,
			want:    []turnSaid{{1, local(10, 0, 0), time.Time{}, 1, sideOnly}},
		},
		{
			name:    "the newest of a running session ended with its last answer",
			lines:   []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), mainOn(p1, local(10, 2, 0), "end_turn")},
			running: true,
			want:    []turnSaid{{1, local(10, 0, 0), local(10, 3, 0), 2, sideOnly}},
		},
		{
			name:    "the newest of a running session ended with its last answer, its subagent's still calling a tool",
			lines:   []ledger.Line{mainOn(p1, local(10, 0, 0), "end_turn"), stopped("tool_use", subagent("a1", "", p1, local(10, 1, 0)))},
			running: true,
			want:    []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 2, sideOnly}},
		},
		{
			name:  "the newest of a session ended, whatever its last answer",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use")},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name: "its own conversation on each account it went to, in the order it did, where it moved",
			lines: []ledger.Line{mainOn(p1, local(10, 0, 0), "tool_use"), subagent("a1", "", p1, local(10, 1, 0)),
				movedFrom("side", onAccount("personal", mainOn(p1, local(10, 2, 0), "end_turn")))},
			want: []turnSaid{{1, local(10, 0, 0), local(10, 3, 0), 3, []string{"side", "personal"}}},
		},
		{
			name:  "one with no request of its own conversation ending with its last request, on the accounts its requests went to",
			lines: []ledger.Line{subagent("a1", "", p1, local(10, 0, 0)), subagent("a1", "", p1, local(10, 3, 0))},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 4, 0), 2, []string{"work"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var routed *status.Session
			if tt.running {
				routed = runningOn("side")
			}
			for i := range tt.want {
				tt.want[i].started, tt.want[i].ended = utc(tt.want[i].started), zeroOrUTC(tt.want[i].ended)
			}
			if got := saidOf(pageOf(t, tt.lines, routed).Turns); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the turns are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// zeroOrUTC is t in UTC, or zero where it's zero.
func zeroOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}

func TestATurnCountsItsToolsTokensWorthAndAccounts(t *testing.T) {
	unknown := mainOn(p1, local(10, 2, 0), "end_turn")
	unknown.Model, unknown.Usage = "claude-opus-9", json.RawMessage(smallUsage)
	lines := []ledger.Line{
		stopped("tool_use", mainOn(p1, local(10, 0, 0), ""), "Bash", "Read", "Bash"),
		stopped("tool_use", subagent("a1", "", p1, local(10, 1, 0)), "Read", "Grep"),
		unknown,
	}
	want := []views.Turn{{
		Turn: 1, Started: utc(local(10, 0, 0)), Ended: utc(local(10, 3, 0)), Requests: 3,
		Tools: []views.ToolCalls{{Tool: "Bash", Times: 2}, {Tool: "Read", Times: 2}, {Tool: "Grep", Times: 1}},
		Read:  100000, Written: 1000, Out: 500 + 10 + 10, Worth: micro(hourWorth + smallWorth),
		Unpriced: []string{"input_tokens", "output_tokens"}, Accounts: []string{"side"},
	}}
	if got := pageOf(t, lines, nil).Turns; !reflect.DeepEqual(got, want) {
		t.Errorf("the turns are\n%+v\nwant\n%+v", got, want)
	}
}

func TestTheAccountsItRanOnFollowItsRequestsWhicheverModelEachWas(t *testing.T) {
	unsent := line(paged, opus, "", "no account has room", local(11, 30, 0), "")
	unsent.Status, unsent.Attempts = 429, 0
	// Sent again on side after work's limit, it's one request, side's.
	resent := movedFrom("work", line(paged, opus, "side", "moved: work hit its limit", local(10, 30, 0), hourUsage))
	resent.Tried, resent.Attempts = []ledger.Tried{{Account: "work", Why: "hit its limit"}}, 2
	lines := []ledger.Line{
		line(paged, opus, "work", "new", local(10, 0, 0), hourUsage),
		line(paged, haiku, "side", "new", local(10, 1, 0), smallUsage),
		resent,
		line(paged, opus, "side", "sticky", local(11, 0, 0), hourUsage),
		unsent,
	}
	page := pageOf(t, lines, nil)
	want := []views.SessionAccount{
		{Account: "work", From: utc(local(10, 0, 0)), To: utc(local(10, 0, 0)), Requests: 1, Worth: micro(hourWorth)},
		{Account: "side", From: utc(local(10, 1, 0)), To: utc(local(11, 0, 0)), Requests: 3, Worth: micro(2*hourWorth + smallWorth)},
	}
	if !reflect.DeepEqual(page.Accounts, want) {
		t.Errorf("the accounts are\n%+v\nwant\n%+v", page.Accounts, want)
	}
	totals := views.SessionTotals{Requests: 5, FromCache: 300000.0 / (3*101010 + 100), Worth: micro(3*hourWorth + smallWorth)}
	if !reflect.DeepEqual(page.Totals, totals) {
		t.Errorf("the totals are %+v, want %+v, the request no account answered among them", page.Totals, totals)
	}
}

// limited returns l, its answer's usage headers reading each window with the
// given key at the use given, resetting at the time given: as key, use,
// reset, and so on.
func limited(l ledger.Line, windows ...any) ledger.Line {
	l.Limits = make(map[string]string)
	for i := 0; i < len(windows); i += 3 {
		key := windows[i].(string)
		l.Limits[key+"-utilization"] = windows[i+1].(string)
		l.Limits[key+"-reset"] = strconv.FormatInt(windows[i+2].(time.Time).Unix(), 10)
	}
	return l
}

// tokens is a usage of n input tokens.
func tokens(n int) string {
	return `{"input_tokens":` + strconv.Itoa(n) + `}`
}

func TestThePointsOfAWindowAreItsRisesSharedOutByTokens(t *testing.T) {
	fiveHour, week, fableWeek := local(15, 0, 0), october(12, 10, 0), october(12, 10, 0)
	// ours is a request of paged, and theirs of other, of the model given on
	// work, arriving at at, taking a second, its answer's usage as given.
	ours := func(model string, at time.Time, usage string) ledger.Line {
		return took(time.Second, line(paged, model, "work", "sticky", at, usage))
	}
	theirs := func(model string, at time.Time, usage string) ledger.Line {
		return took(time.Second, line(other, model, "work", "sticky", at, usage))
	}
	tests := []struct {
		name  string
		lines []ledger.Line
		want  map[string]int
	}{
		{
			name: "two sessions' requests sharing each rise, by when they ended, every kind of token alike",
			lines: []ledger.Line{
				// other's request arrives before paged's first, but ends after
				// it, in the rise to paged's second.
				took(2*time.Minute, theirs(opus, local(9, 59, 0), tokens(100))),
				limited(ours(opus, local(9, 59, 59), tokens(100)), "5h", "0.20", fiveHour, "7d", "0.40", week),
				limited(ours(opus, local(10, 2, 0), `{"cache_read_input_tokens":200,"output_tokens":100}`), "5h", "0.28", fiveHour, "7d", "0.44", week),
				limited(theirs(opus, local(10, 3, 0), tokens(100)), "5h", "0.30", fiveHour, "7d", "0.44", week),
				limited(ours(opus, local(10, 4, 0), tokens(100)), "5h", "0.31", fiveHour, "7d", "0.44", week),
			},
			// 5h: three quarters of 0.08, none of 0.02 and all of 0.01; 7d:
			// three quarters of 0.04.
			want: map[string]int{"5h": 7, "7d": 3},
		},
		{
			name: "a later reset starting the window afresh, from nothing",
			lines: []ledger.Line{
				limited(ours(opus, local(10, 0, 0), tokens(100)), "5h", "0.90", local(10, 30, 0)),
				limited(ours(opus, local(11, 0, 0), tokens(100)), "5h", "0.05", local(15, 30, 0)),
			},
			want: map[string]int{"5h": 5},
		},
		{
			name:  "none of a window never read",
			lines: []ledger.Line{ours(opus, local(10, 0, 0), tokens(100)), ours(opus, local(10, 1, 0), tokens(100))},
		},
		{
			name: "a window every model shares counting every request, of a model whose answers never read it among them",
			lines: []ledger.Line{
				limited(ours(opus, local(10, 0, 0), tokens(100)), "5h", "0.10", fiveHour),
				theirs("claude-opus-9", local(10, 1, 0), tokens(100)),
				limited(ours(opus, local(10, 2, 0), tokens(100)), "5h", "0.20", fiveHour),
			},
			want: map[string]int{"5h": 5},
		},
		{
			name: "a model's own window counting the requests of the models that read it alone",
			lines: []ledger.Line{
				limited(ours(fable, local(10, 0, 0), tokens(100)), "5h", "0.10", fiveHour, "7d_oi", "0.10", fableWeek),
				limited(ours(haiku, local(10, 1, 0), tokens(100)), "5h", "0.12", fiveHour),
				limited(theirs(fable, local(10, 2, 0), tokens(100)), "5h", "0.15", fiveHour, "7d_oi", "0.14", fableWeek),
			},
			want: map[string]int{"5h": 2, "7d_oi": 0},
		},
		{
			name: "none of a model's own window its session never used",
			lines: []ledger.Line{
				limited(theirs(fable, local(10, 0, 0), tokens(100)), "5h", "0.10", fiveHour, "7d_oi", "0.10", fableWeek),
				limited(ours(haiku, local(10, 1, 0), tokens(100)), "5h", "0.12", fiveHour),
				limited(theirs(fable, local(10, 2, 0), tokens(100)), "5h", "0.15", fiveHour, "7d_oi", "0.14", fableWeek),
			},
			want: map[string]int{"5h": 2},
		},
		{
			name: "across midnight, its days one after another",
			lines: []ledger.Line{
				limited(ours(opus, october(6, 23, 59), tokens(100)), "7d", "0.40", week),
				limited(ours(opus, october(7, 0, 1), tokens(100)), "7d", "0.42", week),
			},
			want: map[string]int{"7d": 2},
		},
		{
			name: "none of a rise across a day it made no request on, that day's requests unread",
			lines: []ledger.Line{
				limited(ours(opus, october(5, 22, 0), tokens(100)), "7d", "0.40", week),
				limited(theirs(opus, october(6, 12, 0), tokens(100)), "7d", "0.50", week),
				limited(ours(opus, october(7, 9, 0), tokens(100)), "7d", "0.52", week),
			},
			want: map[string]int{"7d": 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := pageOf(t, tt.lines, nil)
			if len(page.Accounts) != 1 || !reflect.DeepEqual(page.Accounts[0].Points, tt.want) {
				t.Errorf("the accounts are %+v, want work alone, its points %v", page.Accounts, tt.want)
			}
		})
	}
}

func TestASessionResumesAfterAnHourOrMoreWithoutARequest(t *testing.T) {
	tests := []struct {
		name     string
		requests []time.Time
		want     time.Time
	}{
		{name: "never, its requests closer together", requests: []time.Time{local(9, 0, 0), local(9, 59, 59), local(10, 30, 0)}},
		{name: "after an hour", requests: []time.Time{local(9, 0, 0), local(10, 0, 0), local(10, 5, 0)}, want: local(10, 0, 0)},
		{name: "the latest time, of several", requests: []time.Time{october(6, 16, 40), october(6, 18, 5), local(9, 12, 0), local(11, 0, 0)}, want: local(11, 0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []ledger.Line
			for _, at := range tt.requests {
				lines = append(lines, line(paged, opus, "work", "sticky", at, hourUsage))
			}
			if got := pageOf(t, lines, nil).Resumed; !got.Equal(tt.want) {
				t.Errorf("it resumed at %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOneEndedIsKeptUntilTheLastDayItsFirstDaysLinesAre(t *testing.T) {
	lines := []ledger.Line{line(paged, opus, "work", "new", october(6, 22, 0), hourUsage), line(paged, opus, "work", "sticky", local(9, 0, 0), hourUsage)}
	tests := []struct {
		name    string
		keep    time.Duration
		running bool
		want    string
	}{
		{name: "ended, kept 90 days", keep: 90 * 24 * time.Hour, want: "2027-01-04"},
		{name: "ended, kept for good", keep: dayfile.Forever},
		{name: "running", keep: 90 * 24 * time.Hour, running: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var routed *status.Session
			if tt.running {
				routed = runningOn("work")
			}
			page, _ := views.SessionPageOf(paged, views.PageSources{Routed: routed, Ledger: fakeLedger{now: now, lines: lines}, Prices: ledger.Pricing,
				Keep: tt.keep, Now: now})
			if page.KeptUntil != tt.want {
				t.Errorf("it's kept until %q, want %q", page.KeptUntil, tt.want)
			}
		})
	}
}

func TestEachMoveSaysWhatItWroteAndWhatThatCostWhereTheCacheWouldntHaveRunOut(t *testing.T) {
	lines := []ledger.Line{
		line(paged, opus, "work", "new", local(9, 0, 0), hourUsage),
		movedFrom("work", line(paged, opus, "side", "moved: work hit its limit", local(9, 30, 0), hourUsage)),
		movedFrom("side", line(paged, opus, "work", "rescored after 2h 30m idle", local(12, 0, 0), hourUsage)),
	}
	want := []views.SessionMove{
		{At: utc(local(9, 30, 0)), From: "work", To: "side", Reason: "moved: work hit its limit", Written: 1000, Cost: cost(hourWrite)},
		{At: utc(local(12, 0, 0)), From: "side", To: "work", Reason: "rescored after 2h 30m idle", Written: 1000},
	}
	if got := pageOf(t, lines, nil).Moves; !reflect.DeepEqual(got, want) {
		t.Errorf("the moves are\n%+v\nwant\n%+v", got, want)
	}
}

func TestTheRoutersSayJoinsWhatItsLinesTell(t *testing.T) {
	lines := []ledger.Line{
		in("~/Code/api", serving(p1, ledger.ClassMain, line(paged, opus, "work", "new", local(13, 0, 0), hourUsage))),
		in("~/Code/api", movedFrom("work", serving(p1, ledger.ClassMain, line(paged, opus, "side", "pinned", local(13, 5, 0), hourUsage)))),
	}
	routed := &status.Session{ID: paged, Pin: "side", Assignments: []status.Assignment{
		{Model: opus, Account: "side", Reason: "pinned", InFlight: status.Asking, LastSeen: local(13, 11, 0)},
	}}
	got := pageOf(t, lines, routed)
	want := views.SessionPage{
		Session: paged, Dir: "~/Code/api", Running: true, LastSeen: utc(local(13, 11, 0)),
		Models: []views.SessionModel{{Model: opus, Account: "side", Reason: "pinned"}}, State: status.Asking, MoveCost: cost(hourRewrite),
		Pin: "side", Started: utc(local(13, 0, 0)),
		Accounts: []views.SessionAccount{
			{Account: "work", From: utc(local(13, 0, 0)), To: utc(local(13, 0, 0)), Requests: 1, Worth: micro(hourWorth)},
			{Account: "side", From: utc(local(13, 5, 0)), To: utc(local(13, 5, 0)), Requests: 1, Worth: micro(hourWorth)},
		},
		Moves: []views.SessionMove{{At: utc(local(13, 5, 0)), From: "work", To: "side", Reason: "pinned", Written: 1000, Cost: cost(hourWrite)}},
		Turns: []views.Turn{{Turn: 1, Started: utc(local(13, 0, 0)), Ended: utc(local(13, 5, 0)), Requests: 2, Tools: []views.ToolCalls{},
			Read: 200000, Written: 2000, Out: 1000, Worth: micro(2 * hourWorth), Accounts: []string{"work", "side"}}},
		Totals: views.SessionTotals{Turns: 1, Requests: 2, FromCache: 200000.0 / (2 * 101010), Worth: micro(2 * hourWorth)},
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("the page is\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

func TestASessionTheRouterListsIsOneWithoutLines(t *testing.T) {
	got, ok := views.SessionPageOf(paged, views.PageSources{Routed: runningOn("side"), Ledger: fakeLedger{now: now}, Prices: ledger.Pricing, Now: now})
	want := views.SessionPage{Session: paged, Running: true, LastSeen: utc(now), Models: []views.SessionModel{{Model: opus, Account: "side", Reason: "sticky"}},
		Accounts: []views.SessionAccount{}, Moves: []views.SessionMove{}, Turns: []views.Turn{}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("SessionPageOf() = %+v, %v, want %+v, true", got, ok, want)
	}
}

func TestASessionNeitherTheLedgerNorTheRouterKnowsHasNoPage(t *testing.T) {
	check := line(paged, haiku, "work", "new", local(9, 0, 0), smallUsage)
	check.Kind = ledger.KindCheck
	l := fakeLedger{now: now, lines: []ledger.Line{check, line(other, opus, "work", "new", local(9, 1, 0), hourUsage)}}
	if _, ok := views.SessionPageOf(paged, views.PageSources{Ledger: l, Prices: ledger.Pricing, Now: now}); ok {
		t.Error("SessionPageOf() found a session the ledger holds no request of, and the router doesn't list")
	}
}
