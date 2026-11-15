package views_test

import (
	"encoding/json"
	"iter"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// splitDays are the lines of Monday 5 October to today, Wednesday 7
// October: work's and side's requests, of Opus and Haiku, a session moved
// from side to work each day, a quota check, and a request no account had
// room for; and work's 5-hour window read through each day.
func splitDays() ([]ledger.Line, []readings.Reading) {
	var lines []ledger.Line
	var read []readings.Reading
	for day := 5; day <= 7; day++ {
		asked := func(request, session, account, model string, hour int) ledger.Line {
			return ledger.Line{At: october(day, hour, 0), Request: request, Kind: ledger.KindMessage, Session: session, Model: model, Account: account,
				Reason: "sticky", Status: 200, Attempts: 1, Usage: json.RawMessage(inputUsage)}
		}
		moved := asked("m", "two", "work", opusVersion, 12)
		moved.From = "side"
		check := asked("c", "resuming", "work", haikuVersion, 9)
		check.Kind = ledger.KindCheck
		lines = append(lines, asked("1", "one", "work", opusVersion, 9), asked("2", "one", "work", haikuVersion, 10), asked("3", "two", "side", opusVersion, 11),
			moved, check, asked("4", "one", "work", opusVersion, 13),
			ledger.Line{At: october(day, 14, 0), Request: "5", Kind: ledger.KindMessage, Session: "three", Model: opusVersion, Reason: "no account has room", Status: 429})
		read = append(read, readings.Reading{At: october(day, 9, 0), Account: "work", Key: "5h", Utilization: 0.2 * float64(day-4),
			ResetsAt: october(day, 13, 0), Status: quota.StatusAllowed, Source: readings.FromAnswer})
	}
	return lines, read
}

// The models of splitDays' requests.
const (
	opusVersion  = "claude-opus-5-5"
	haikuVersion = "claude-haiku-4-5"
)

// summarisedDays are the summaries of the days from Monday 5 October to
// today of lines, each routed in the pool pool gives it, with the readings
// given, as the ledger summarises them: today's as far as it has gone.
func summarisedDays(t *testing.T, lines []ledger.Line, read []readings.Reading, pool func(ledger.Line) string) []ledger.Summary {
	t.Helper()
	history := func(from, to time.Time) iter.Seq[readings.Reading] {
		return func(yield func(readings.Reading) bool) {
			for _, r := range read {
				if !r.At.Before(from) && r.At.Before(to) && !yield(r) {
					return
				}
			}
		}
	}
	var days []ledger.Summary
	for _, date := range dayfile.Span(on("2026-10-05"), on("2026-10-07")) {
		var routed []ledger.Line
		for _, l := range lines {
			if dayfile.DateOf(l.At) == date {
				l.Pool = pool(l)
				routed = append(routed, l)
			}
		}
		s, err := ledger.Summarise(date, slices.Values(routed), history, nil, ledger.Caps{Shared: []string{"5h", "7d"}}, now)
		if err != nil {
			t.Fatal(err)
		}
		days = append(days, s)
	}
	return days
}

// Every view of History reads the same of days split by pool as of the same
// days before pools: each account's day holds its work in every pool
// together, which they read, whatever pools it was in.
func TestHistoryReadsTheSameWhicheverWayItsDaysSplitByPool(t *testing.T) {
	lines, read := splitDays()
	whole := summarisedDays(t, lines, read, func(ledger.Line) string { return "" })
	// Work moved from one pool to another at noon each day, side in the
	// second, and the router's own answers in it too.
	split := summarisedDays(t, lines, read, func(l ledger.Line) string {
		if l.Account == "work" && l.At.Hour() < 12 {
			return "b2n8"
		}
		return "q7mt"
	})
	if !slices.ContainsFunc(split[0].Accounts, func(a ledger.AccountDay) bool { return len(a.Pools) == 2 }) {
		t.Fatalf("the split days are %+v, want work's split between two pools", split)
	}
	pages := map[string]func(views.Past) any{
		"Days":                   func(p views.Past) any { return p.Days() },
		"Weeks":                  func(p views.Past) any { return p.Weeks() },
		"the Year":               func(p views.Past) any { return p.Year(views.YearAsk{}) },
		"the Year of an account": func(p views.Past) any { return p.Year(views.YearAsk{Account: "work"}) },
		"Tokens":                 func(p views.Past) any { return p.Tokens(views.TokensAsk{}) },
		"WHEN YOU WORK":          func(p views.Past) any { return p.Hours() },
		"Accounts, today":        func(p views.Past) any { return p.Accounts(views.AccountsToday) },
		"Accounts, this week":    func(p views.Past) any { return p.Accounts(views.AccountsThisWeek) },
		"the lanes' summaries":   func(p views.Past) any { return windowsOf(p.Summaries(on("2026-10-05"), now)) },
	}
	// A page is held to what it gives, as its JSON says: some keep the Past
	// they were built of, its days among it, for their methods to read.
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			if got, want := jsonOf(t, page(views.NewPast(input(split), split))), jsonOf(t, page(views.NewPast(input(whole), whole))); got != want {
				t.Errorf("of the days split by pool, the page is\n%s\nwant it as of the days before pools\n%s", got, want)
			}
		})
	}
	t.Run("history", func(t *testing.T) {
		got, want := views.NewHistory(input(split)), views.NewHistory(input(whole))
		names := status.NamesOf(accounts, nil)
		if g, w := views.DayAccountsOf(got.Days, names), views.DayAccountsOf(want.Days, names); !reflect.DeepEqual(g, w) {
			t.Errorf("of the days split by pool, the days' accounts are\n%+v\nwant\n%+v", g, w)
		}
		got.Days, want.Days = nil, nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("of the days split by pool, history is\n%+v\nwant\n%+v", got, want)
		}
	})
}

// windowsOf are the summaries' accounts' days, but for their work: what
// Runway's lanes read of them.
func windowsOf(days []ledger.Summary) [][]ledger.AccountDay {
	windows := make([][]ledger.AccountDay, len(days))
	for i, s := range days {
		for _, a := range s.Accounts {
			a.Work, a.Pool, a.Pools = ledger.Work{}, "", nil
			windows[i] = append(windows[i], a)
		}
	}
	return windows
}
