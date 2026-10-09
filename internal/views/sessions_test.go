package views_test

import (
	"encoding/json"
	"iter"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// october is the local time on the given day of October 2026, at
// hour:minute: the ledger files its lines under local days.
func october(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local)
}

// now is the time by the clock in the List's tests: Wednesday 7 October,
// 13:12 local time.
var now = october(7, 13, 12)

// The sessions of the List's tests.
const (
	// older started yesterday, and runs on work.
	older = "0a1b2c3d-1111-4000-8000-000000000001"
	// mover started today on work, its Claude Opus 5.5 moved to side at noon
	// as work hit its limit, its Claude Haiku 4.5 still on work.
	mover = "0a1b2c3d-2222-4000-8000-000000000002"
	// quiet is one the router lists that sent nothing the ledger holds.
	quiet = "0a1b2c3d-3333-4000-8000-000000000003"
	// early ended at 11:00, some of its requests unpriced.
	early = "0a1b2c3d-4444-4000-8000-000000000004"
	// rescored ended at noon, moved to side three hours after its last
	// request, so its cache had run out.
	rescored = "0a1b2c3d-5555-4000-8000-000000000005"
)

const (
	opus  = "claude-opus-5-5"
	haiku = "claude-haiku-4-5"
)

// The usages of the List's tests' answers. hourUsage writes the cache for an
// hour: 10 input, 1,000 written and 100,000 read, worth $0.03804 at Claude
// Opus 5.5's prices, its prompt 101,010 tokens. fiveMinuteUsage writes as
// much for five minutes, worth $0.03504.
const (
	hourUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`
	fiveMinuteUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":0},"output_tokens":500}`
	smallUsage = `{"input_tokens":100,"output_tokens":10}`
)

// micro is an amount of millionths of a dollar, as picodollars.
func micro(n int64) ledger.Picodollars {
	return ledger.Picodollars(n * 1_000_000)
}

// cost is a pointer to an amount of millionths of a dollar.
func cost(n int64) *ledger.Picodollars {
	c := micro(n)
	return &c
}

// What the tests' requests are worth, and cost to move, in millionths of a
// dollar.
const (
	hourWorth  = 38040
	smallWorth = 150
	// hourRewrite is hourUsage's prompt written again for an hour, and
	// fiveMinuteRewrite for five minutes.
	hourRewrite       = 101010 * 8
	fiveMinuteRewrite = 101010 * 5
	// hourWrite is hourUsage's own write to the cache.
	hourWrite = 1000 * 8
)

// line is a request of the session with the given id, of model, on account,
// routed for reason, at at, its answer's usage as given: none for "".
func line(session, model, account, reason string, at time.Time, usage string) ledger.Line {
	l := ledger.Line{At: at, Kind: ledger.KindMessage, Session: session, Model: model, Account: account, Reason: reason, Status: 200, Attempts: 1}
	if usage != "" {
		l.Usage = json.RawMessage(usage)
	}
	return l
}

// in returns l in the directory given.
func in(dir string, l ledger.Line) ledger.Line {
	l.Dir = dir
	return l
}

// movedFrom returns l, a request that moved its session from the account
// given.
func movedFrom(from string, l ledger.Line) ledger.Line {
	l.From = from
	return l
}

// fakeLedger is a ledger of lines, oldest first, and of the summaries of the
// days before now's, which name each day's sessions, read at now.
type fakeLedger struct {
	lines     []ledger.Line
	summaries []ledger.Summary
	now       time.Time
}

func (f fakeLedger) Days(time.Time) []ledger.Summary {
	return f.summaries
}

func (f fakeLedger) DaysBefore(t time.Time) []ledger.Summary {
	date := t.Local().Format(time.DateOnly)
	return slices.DeleteFunc(slices.Clone(f.summaries), func(s ledger.Summary) bool { return s.Day >= date })
}

func (f fakeLedger) Today(ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	start := dayfile.DayStart(f.now, 0)
	var today []ledger.Held
	for _, l := range f.lines {
		if !l.At.Before(start) {
			today = append(today, ledger.Held{Line: l})
		}
	}
	return today, ledger.Mark{}, true
}

func (f fakeLedger) Session(id string) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		for _, l := range slices.Backward(f.lines) {
			if l.Session == id && !yield(ledger.Held{Line: l}) {
				return
			}
		}
	}
}

func (f fakeLedger) DayLines(date string) iter.Seq[ledger.Line] {
	return func(yield func(ledger.Line) bool) {
		for _, l := range f.lines {
			if l.At.Local().Format(time.DateOnly) == date && !yield(l) {
				return
			}
		}
	}
}

// naming is a summary of the day with the given date naming the sessions
// with the given ids, as work's.
func naming(date string, ids ...string) ledger.Summary {
	return ledger.Summary{Version: 2, Day: date, Accounts: []ledger.AccountDay{{Account: "work", Sessions: len(ids), SessionIDs: ids}}}
}

// world is the ledger of the List's tests.
func world() fakeLedger {
	noSession := line("", opus, "work", "new", october(7, 8, 30), hourUsage)
	check := line(early, haiku, "work", "new", october(7, 8, 0), smallUsage)
	check.Kind = ledger.KindCheck
	count := line(early, opus, "work", "sticky", october(7, 12, 5), "")
	count.Kind = ledger.KindCount
	cutOff := line(early, opus, "work", "sticky", october(7, 11, 0), "")
	cutOff.CutOff = true
	return fakeLedger{now: now, summaries: []ledger.Summary{naming("2026-10-06", older)}, lines: []ledger.Line{
		in("~/Code/api", line(older, opus, "work", "new", october(6, 22, 0), hourUsage)),
		in("~/Code/web", line(early, opus, "work", "new", october(7, 8, 0), hourUsage)),
		check,
		noSession,
		in("~/Code/api", line(older, opus, "work", "sticky", october(7, 9, 0), hourUsage)),
		line(rescored, opus, "work", "new", october(7, 9, 0), hourUsage),
		in("~/Code/cli", line(mover, opus, "work", "new", october(7, 10, 0), hourUsage)),
		line(early, "claude-opus-9", "work", "sticky", october(7, 10, 30), smallUsage),
		cutOff,
		line(mover, haiku, "work", "new", october(7, 11, 30), smallUsage),
		line(mover, opus, "work", "sticky", october(7, 11, 40), hourUsage),
		movedFrom("work", line(mover, opus, "side", "moved: work hit its limit", october(7, 12, 0), hourUsage)),
		movedFrom("work", line(rescored, opus, "side", "rescored after 3h idle", october(7, 12, 0), hourUsage)),
		count,
	}}
}

// routed is what the router lists of the sessions running in the List's
// tests.
var routed = []status.Session{
	{ID: mover, Assignments: []status.Assignment{
		{Model: opus, Account: "side", Dir: "~/Code/cli", InFlight: status.Answering, Reason: "moved: work hit its limit", LastSeen: october(7, 13, 11)},
		{Model: haiku, Account: "work", InFlight: status.Asking, Reason: "sticky", LastSeen: october(7, 13, 5)},
	}},
	{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", Reason: "sticky", LastSeen: october(7, 13, 10)}}},
	{ID: quiet, Assignments: []status.Assignment{{Model: "claude-sonnet-5-5", Account: "side", Reason: "new", LastSeen: october(7, 12, 40)}}},
}

// utc is t in UTC, as the List gives its times.
func utc(t time.Time) time.Time {
	return t.UTC()
}

func TestTheListHoldsTodaysSessionsRunningThenEnded(t *testing.T) {
	got := views.ListSessions(views.SessionSources{Running: routed, Ledger: world(), Prices: ledger.Pricing, Now: now})
	want := views.SessionList{GeneratedAt: utc(now), PricesAsOf: "2026-10-07", Sessions: []views.ListedSession{
		{
			Session: older, Dir: "~/Code/api", Account: "work", Model: opus, Running: true, LastSeen: utc(october(7, 13, 10)),
			// Its last request ended at 9:00, and its cache with it, however
			// lately the router saw it.
			Models:  []views.SessionModel{{Model: opus, Account: "work", Reason: "sticky"}},
			Started: utc(october(6, 22, 0)), Requests: 1, Worth: micro(hourWorth),
		},
		{
			Session: mover, Dir: "~/Code/cli", Account: "side", Model: opus, Running: true, LastSeen: utc(october(7, 13, 11)),
			Models: []views.SessionModel{
				{Model: opus, Account: "side", Reason: "moved: work hit its limit"},
				{Model: haiku, Account: "work", Reason: "sticky"},
			},
			State: status.Answering, MoveCost: cost(hourRewrite), Started: utc(october(7, 10, 0)), Requests: 4, Worth: micro(3*hourWorth + smallWorth),
			Moved: &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work hit its limit", Cost: cost(hourWrite)},
		},
		{
			Session: quiet, Account: "side", Model: "claude-sonnet-5-5", Running: true, LastSeen: utc(october(7, 12, 40)),
			Models: []views.SessionModel{{Model: "claude-sonnet-5-5", Account: "side", Reason: "new"}},
		},
		{
			Session: rescored, Account: "side", Model: opus, Ended: utc(october(7, 12, 0)), Started: utc(october(7, 9, 0)), Requests: 2, Worth: micro(2 * hourWorth),
			Moved: &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "rescored after 3h idle"},
		},
		{
			Session: early, Dir: "~/Code/web", Account: "work", Model: opus, Ended: utc(october(7, 11, 0)), Started: utc(october(7, 8, 0)),
			Requests: 3, Worth: micro(hourWorth), Unpriced: []string{"input_tokens", "no_usage", "output_tokens"},
		},
	}, Today: views.SessionsToday{Sessions: 5, Requests: 10, Worth: micro(5*hourWorth + smallWorth + 2*hourWorth),
		Unpriced: []string{"input_tokens", "no_usage", "output_tokens"}}}
	assertList(t, got, want)
}

func TestWithoutTheRouterTheListHoldsTodaysSessionsAsEnded(t *testing.T) {
	got := views.ListSessions(views.SessionSources{Ledger: world(), Prices: ledger.Pricing, Now: now})
	var sessions []string
	for _, s := range got.Sessions {
		if s.Running || !s.LastSeen.IsZero() || s.Models != nil || s.State != "" || s.MoveCost != nil || s.Ended.IsZero() {
			t.Errorf("without the router, session %s is %+v, want it ended, with nothing the router says", s.Session, s)
		}
		sessions = append(sessions, s.Session)
	}
	// mover and rescored ended at noon alike, and are told apart by their ids.
	if want := []string{mover, rescored, early, older}; !slices.Equal(sessions, want) {
		t.Errorf("without the router, the List holds %q, want %q, the latest ended first", sessions, want)
	}
	if mover := got.Sessions[0]; mover.Account != "side" || mover.Dir != "~/Code/cli" || mover.Moved == nil || !mover.Ended.Equal(october(7, 12, 0)) {
		t.Errorf("without the router, mover is %+v, want it on side, in ~/Code/cli, moved there, ended at noon, as its lines say", mover)
	}
	if older := got.Sessions[3]; !older.Started.Equal(october(6, 22, 0)) || older.Requests != 1 {
		t.Errorf("without the router, older is %+v, want it started yesterday, with today's one request", older)
	}
	want := views.SessionsToday{Sessions: 4, Requests: 10, Worth: micro(7*hourWorth + smallWorth), Unpriced: []string{"input_tokens", "no_usage", "output_tokens"}}
	if !reflect.DeepEqual(got.Today, want) {
		t.Errorf("without the router, today sums to %+v, want %+v", got.Today, want)
	}
}

func TestTheListIsEmptyWithoutSessions(t *testing.T) {
	got := views.ListSessions(views.SessionSources{Ledger: fakeLedger{now: now}, Prices: ledger.Pricing, Now: now})
	want := views.SessionList{GeneratedAt: utc(now), PricesAsOf: "2026-10-07", Sessions: []views.ListedSession{}}
	assertList(t, got, want)
}

func TestAMovesCostIsLeftOutWhereTheSessionsCacheWouldHaveRunOut(t *testing.T) {
	tests := []struct {
		name     string
		usage    string
		lastSeen time.Duration
		inFlight string
		want     *ledger.Picodollars
	}{
		{name: "idle within an hour, its writes an hour's", usage: hourUsage, lastSeen: 59 * time.Minute, want: cost(hourRewrite)},
		{name: "idle an hour, its writes an hour's", usage: hourUsage, lastSeen: time.Hour, want: cost(hourRewrite)},
		{name: "idle over an hour, its writes an hour's", usage: hourUsage, lastSeen: 61 * time.Minute},
		{name: "idle within five minutes, its writes five minutes'", usage: fiveMinuteUsage, lastSeen: 4 * time.Minute, want: cost(fiveMinuteRewrite)},
		{name: "idle over five minutes, its writes five minutes'", usage: fiveMinuteUsage, lastSeen: 6 * time.Minute},
		{name: "asking, however long since it was seen", usage: fiveMinuteUsage, lastSeen: 6 * time.Minute, inFlight: status.Asking, want: cost(fiveMinuteRewrite)},
		{name: "its last answer without usage, its prompt not known", lastSeen: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := now.Add(-tt.lastSeen)
			l := fakeLedger{now: now, lines: []ledger.Line{line(mover, opus, "work", "new", seen, tt.usage)}}
			running := []status.Session{{ID: mover, Assignments: []status.Assignment{
				{Model: opus, Account: "work", Reason: "new", InFlight: tt.inFlight, LastSeen: seen},
			}}}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || !reflect.DeepEqual(got.Sessions[0].MoveCost, tt.want) {
				t.Errorf("the List holds %+v, want one session, its move cost %v", got.Sessions, amount(tt.want))
			}
		})
	}
}

func TestAMovesCostIsIdleFromTheEndOfTheRequestItPrices(t *testing.T) {
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	tests := []struct {
		name string
		// took is how long the turn it prices took, arriving 70 minutes ago;
		// flying, the model a request of which is in flight, if any.
		took   time.Duration
		flying string
		want   *ledger.Picodollars
	}{
		{name: "over its cache's hour since it ended, a side request of another model since keeping nothing warm"},
		{name: "within its cache's hour since it ended, though it arrived longer ago", took: 15 * time.Minute, want: cost(hourRewrite)},
		{name: "over its cache's hour since it ended, another model's request in flight", flying: haiku},
		{name: "a request of its model in flight", flying: opus, want: cost(hourRewrite)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turn := of("main", line(mover, opus, "work", "new", ago(70*time.Minute), hourUsage))
			turn.TotalMS = tt.took.Milliseconds()
			l := fakeLedger{now: now, lines: []ledger.Line{turn, of("auxiliary", line(mover, haiku, "work", "new", ago(2*time.Minute), smallUsage))}}
			assignment := func(model string, seen time.Time) status.Assignment {
				a := status.Assignment{Model: model, Account: "work", Reason: "sticky", LastSeen: seen}
				if model == tt.flying {
					a.InFlight = status.Asking
				}
				return a
			}
			running := []status.Session{{ID: mover, Assignments: []status.Assignment{assignment(haiku, ago(2*time.Minute)), assignment(opus, ago(70*time.Minute))}}}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || !reflect.DeepEqual(got.Sessions[0].MoveCost, tt.want) {
				t.Errorf("the List holds %+v, want one session, its move cost %v", got.Sessions, amount(tt.want))
			}
		})
	}
}

// of returns l, a request of the class given, as its client's headers say.
func of(class string, l ledger.Line) ledger.Line {
	l.Class = class
	return l
}

func TestAMovesCostIsOfTheSessionsOwnConversation(t *testing.T) {
	// A title's request of Claude Haiku 4.5: its prompt of 100 tokens written
	// again for an hour.
	const titleRewrite = 100 * 2
	tests := []struct {
		name  string
		lines []ledger.Line
		want  *ledger.Picodollars
	}{
		{
			name: "its last turn's, a side request after it",
			lines: []ledger.Line{of("main", line(mover, opus, "work", "new", october(7, 13, 0), hourUsage)),
				of("auxiliary", line(mover, haiku, "work", "new", october(7, 13, 10), smallUsage))},
			want: cost(hourRewrite),
		},
		{
			name: "its last turn whose answer gave usage",
			lines: []ledger.Line{of("main", line(mover, opus, "work", "new", october(7, 12, 50), hourUsage)),
				of("auxiliary", line(mover, haiku, "work", "new", october(7, 13, 0), smallUsage)),
				of("main", line(mover, opus, "work", "sticky", october(7, 13, 5), ""))},
			want: cost(hourRewrite),
		},
		{
			name: "none, where no turn of its own gave usage",
			lines: []ledger.Line{of("subagent", line(mover, opus, "work", "new", october(7, 13, 0), hourUsage)),
				line(mover, haiku, "work", "new", october(7, 13, 10), smallUsage)},
		},
		{
			name: "its last request's, where none says its class",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 13, 0), hourUsage),
				line(mover, haiku, "work", "new", october(7, 13, 10), smallUsage)},
			want: cost(titleRewrite),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			running := []status.Session{{ID: mover, Assignments: []status.Assignment{{Model: opus, Account: "work", Reason: "sticky", LastSeen: now}}}}
			l := fakeLedger{now: now, lines: tt.lines}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || !reflect.DeepEqual(got.Sessions[0].MoveCost, tt.want) {
				t.Errorf("the List holds %+v, want one session, its move cost %v", got.Sessions, amount(tt.want))
			}
		})
	}
}

func TestAMoveIsTheOneThatBroughtTheSessionToItsAccountToday(t *testing.T) {
	moving := movedFrom("work", line(mover, opus, "side", "moved: work is at its reserve", october(7, 12, 0), hourUsage))
	tests := []struct {
		name  string
		lines []ledger.Line
		// on is the account the router has the session's last request go
		// to: none where it doesn't list it.
		on   string
		want *views.Move
	}{
		{
			name:  "within its cache's hour of its model's last request",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 11, 0), hourUsage), moving},
			want:  &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work is at its reserve", Cost: cost(hourWrite)},
		},
		{
			name:  "over its cache's hour after its model's last request",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 10, 59), hourUsage), moving},
			want:  &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work is at its reserve"},
		},
		{
			name:  "over five minutes after its model's last request, whose writes were five minutes'",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 11, 54), fiveMinuteUsage), moving},
			want:  &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work is at its reserve"},
		},
		{
			name: "however long after another model's last request",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 9, 0), hourUsage),
				line(mover, haiku, "work", "new", october(7, 11, 59), fiveMinuteUsage), moving},
			want: &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work is at its reserve"},
		},
		{
			name:  "its request without usage, its cost not known",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 11, 0), hourUsage), movedFrom("work", line(mover, opus, "side", "moved: work is at its reserve", october(7, 12, 0), ""))},
			want:  &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work is at its reserve"},
		},
		{
			name:  "none, where the router has it on another account since",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 11, 0), hourUsage), moving},
			on:    "work",
		},
		{
			name: "the later, where another moved it on since",
			lines: []ledger.Line{line(mover, opus, "work", "new", october(7, 11, 0), hourUsage), moving,
				movedFrom("side", line(mover, haiku, "personal", "moved: side has no room", october(7, 12, 30), smallUsage))},
			want: &views.Move{At: utc(october(7, 12, 30)), From: "side", Reason: "moved: side has no room", Cost: cost(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var running []status.Session
			if tt.on != "" {
				running = []status.Session{{ID: mover, Assignments: []status.Assignment{{Model: opus, Account: tt.on, Reason: "sticky", LastSeen: now}}}}
			}
			l := fakeLedger{now: now, lines: tt.lines}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || !reflect.DeepEqual(got.Sessions[0].Moved, tt.want) {
				t.Errorf("the List holds %+v, want one session, moved %+v", got.Sessions, tt.want)
			}
		})
	}
}

func TestTheListsAccountModelAndMoveAreItsConversations(t *testing.T) {
	// Its conversation, of Claude Opus 5.5, moved to side; its titles, of
	// Claude Haiku 4.5, were rescored onto personal after it.
	l := fakeLedger{now: now, lines: []ledger.Line{
		of("main", line(mover, opus, "work", "new", october(7, 11, 0), hourUsage)),
		of("auxiliary", line(mover, haiku, "work", "new", october(7, 11, 1), smallUsage)),
		of("main", movedFrom("work", line(mover, opus, "side", "moved: work hit its limit", october(7, 12, 0), hourUsage))),
		of("auxiliary", movedFrom("work", line(mover, haiku, "personal", "rescored after 1h 29m idle", october(7, 12, 30), smallUsage))),
	}}
	moved := &views.Move{At: utc(october(7, 12, 0)), From: "work", Reason: "moved: work hit its limit", Cost: cost(hourWrite)}
	tests := []struct {
		name    string
		running []status.Session
	}{
		{name: "ended, as its last request of its conversation left it"},
		{name: "running, as the router routes its conversation", running: []status.Session{{ID: mover, Assignments: []status.Assignment{
			{Model: opus, Account: "side", Reason: "moved: work hit its limit", LastSeen: october(7, 12, 0)},
			{Model: haiku, Account: "personal", Reason: "rescored after 1h 29m idle", LastSeen: october(7, 12, 30)},
		}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.ListSessions(views.SessionSources{Running: tt.running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || got.Sessions[0].Account != "side" || got.Sessions[0].Model != opus || !reflect.DeepEqual(got.Sessions[0].Moved, moved) {
				t.Errorf("the List holds %+v, want one session, on side, of Claude Opus 5.5, moved there %+v", got.Sessions, moved)
			}
		})
	}
}

func TestAMovesCostLastsAsTheSessionsLastWriteToTheCache(t *testing.T) {
	// Its last turn wrote nothing to the cache, which says nothing of how
	// long its writes last: the one before wrote for five minutes.
	const readOnly = `{"input_tokens":10,"cache_read_input_tokens":100000,"output_tokens":500}`
	tests := []struct {
		name string
		idle time.Duration
		want *ledger.Picodollars
	}{
		{name: "within its five minutes, at a five-minute write's price", idle: 4 * time.Minute, want: cost(100010 * 5)},
		{name: "past its five minutes", idle: 6 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{now: now, lines: []ledger.Line{
				of("main", line(mover, opus, "work", "new", now.Add(-tt.idle-time.Minute), fiveMinuteUsage)),
				of("main", line(mover, opus, "work", "sticky", now.Add(-tt.idle), readOnly)),
			}}
			running := []status.Session{{ID: mover, Assignments: []status.Assignment{{Model: opus, Account: "work", Reason: "sticky", LastSeen: now.Add(-tt.idle)}}}}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || !reflect.DeepEqual(got.Sessions[0].MoveCost, tt.want) {
				t.Errorf("the List holds %+v, want one session, its move cost %v", got.Sessions, amount(tt.want))
			}
		})
	}
}

func TestAMoveOnAnEarlierDayIsntToday(t *testing.T) {
	l := fakeLedger{now: now, summaries: []ledger.Summary{naming("2026-10-06", mover)}, lines: []ledger.Line{
		line(mover, opus, "work", "new", october(6, 23, 0), hourUsage),
		movedFrom("work", line(mover, opus, "side", "moved: work hit its limit", october(6, 23, 30), hourUsage)),
		line(mover, opus, "side", "sticky", october(7, 0, 10), hourUsage),
	}}
	got := views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: now})
	if len(got.Sessions) != 1 || got.Sessions[0].Moved != nil || !got.Sessions[0].Started.Equal(october(6, 23, 0)) || got.Sessions[0].Requests != 1 {
		t.Errorf("the List holds %+v, want one session, started yesterday, with today's request alone, and no move today", got.Sessions)
	}
}

// readCounts count a ledger's reads: of each day's lines, by its date, of a
// session's lines, of the days' summaries from a day to today's, and of
// those before a day.
type readCounts struct {
	dayLines                   map[string]int
	sessions, days, daysBefore int
}

// countedLedger is a fakeLedger that counts its reads.
type countedLedger struct {
	fakeLedger
	counts *readCounts
}

func (c countedLedger) DayLines(date string) iter.Seq[ledger.Line] {
	c.counts.dayLines[date]++
	return c.fakeLedger.DayLines(date)
}

func (c countedLedger) Session(id string) iter.Seq[ledger.Held] {
	c.counts.sessions++
	return c.fakeLedger.Session(id)
}

func (c countedLedger) Days(from time.Time) []ledger.Summary {
	c.counts.days++
	return c.fakeLedger.Days(from)
}

func (c countedLedger) DaysBefore(t time.Time) []ledger.Summary {
	c.counts.daysBefore++
	return c.fakeLedger.DaysBefore(t)
}

func TestTheListReadsEachEarlierDayItNeedsOnceAndNeverSummarisesToday(t *testing.T) {
	counts := &readCounts{dayLines: make(map[string]int)}
	l := countedLedger{counts: counts, now: now,
		summaries: []ledger.Summary{naming("2026-10-05", rescored), naming("2026-10-06", older, mover)},
		lines: []ledger.Line{
			line(rescored, opus, "work", "new", october(5, 9, 0), hourUsage),
			line(older, opus, "work", "new", october(6, 22, 0), hourUsage),
			line(mover, opus, "work", "new", october(6, 23, 0), hourUsage),
			line(older, opus, "work", "sticky", october(7, 9, 0), hourUsage),
			line(mover, opus, "work", "sticky", october(7, 9, 30), hourUsage),
		}}
	got := views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: now})
	started := make(map[string]time.Time)
	for _, s := range got.Sessions {
		started[s.Session] = s.Started
	}
	if want := map[string]time.Time{older: utc(october(6, 22, 0)), mover: utc(october(6, 23, 0))}; !maps.Equal(started, want) {
		t.Errorf("the List's sessions started %v, want %v", started, want)
	}
	if want := (readCounts{dayLines: map[string]int{"2026-10-06": 1}, daysBefore: 1}); !reflect.DeepEqual(*counts, want) {
		t.Errorf("the List read the ledger %+v, want the days before today's summaries once, yesterday's lines once, the one day "+
			"before today naming its sessions, and no session's or today's summary", *counts)
	}
}

func TestTheListOfNoSessionLooksBackOverNoDay(t *testing.T) {
	counts := &readCounts{dayLines: make(map[string]int)}
	l := countedLedger{counts: counts, fakeLedger: summarisedLedger([]ledger.Line{line(older, opus, "work", "new", october(6, 22, 0), hourUsage)})}
	if got := views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: now}); len(got.Sessions) != 0 {
		t.Errorf("the List holds %+v, want none", got.Sessions)
	}
	if want := (readCounts{dayLines: map[string]int{}}); !reflect.DeepEqual(*counts, want) {
		t.Errorf("the List read the ledger %+v, want nothing of the days before today, as no session of today's needs them", *counts)
	}
}

func TestASessionsDirectoryIsTheRoutersElseItsLines(t *testing.T) {
	tests := []struct {
		name        string
		assignments []status.Assignment
		want        string
	}{
		{name: "the router's, of the model used last that names one", assignments: []status.Assignment{
			{Model: opus, Account: "work"}, {Model: haiku, Account: "work", Dir: "~/Code/new"},
		}, want: "~/Code/new"},
		{name: "its lines', where the router names none", assignments: []status.Assignment{{Model: opus, Account: "work"}}, want: "~/Code/old"},
		{name: "its lines', without the router", want: "~/Code/old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{now: now, lines: []ledger.Line{
				in("~/Code/first", line(mover, opus, "work", "new", october(7, 9, 0), hourUsage)),
				in("~/Code/old", line(mover, opus, "work", "sticky", october(7, 10, 0), hourUsage)),
				line(mover, opus, "work", "sticky", october(7, 11, 0), hourUsage),
			}}
			var running []status.Session
			if tt.assignments != nil {
				running = []status.Session{{ID: mover, Assignments: tt.assignments}}
			}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || got.Sessions[0].Dir != tt.want {
				t.Errorf("the List holds %+v, want one session in %s", got.Sessions, tt.want)
			}
		})
	}
}

func TestASessionsStateIsItsBusiestModels(t *testing.T) {
	tests := []struct {
		inFlight []string
		want     string
	}{
		{inFlight: []string{"", ""}},
		{inFlight: []string{"", status.Asking}, want: status.Asking},
		{inFlight: []string{status.Asking, status.Answering}, want: status.Answering},
		{inFlight: []string{status.Answering, status.Asking}, want: status.Answering},
	}
	for _, tt := range tests {
		running := []status.Session{{ID: mover, Assignments: []status.Assignment{
			{Model: opus, Account: "work", InFlight: tt.inFlight[0], LastSeen: now},
			{Model: haiku, Account: "side", InFlight: tt.inFlight[1], LastSeen: now},
		}}}
		got := views.ListSessions(views.SessionSources{Running: running, Ledger: fakeLedger{now: now}, Prices: ledger.Pricing, Now: now})
		if len(got.Sessions) != 1 || got.Sessions[0].State != tt.want {
			t.Errorf("with its models' in flight %q, the List holds %+v, want one session, %q", tt.inFlight, got.Sessions, tt.want)
		}
	}
}

// assertList fails the test where got isn't want, showing each as JSON.
func assertList(t *testing.T, got, want views.SessionList) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("the List is\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

// amount shows a cost, or none.
func amount(c *ledger.Picodollars) string {
	if c == nil {
		return "none"
	}
	return c.Cents()
}
