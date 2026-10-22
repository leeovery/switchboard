package viewstest

import (
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// DayMoment is the request ledger at a moment: the lines its files hold,
// each filed under the local day its request arrived on, and the summaries
// the router wrote of its days, read at Now from the local day From falls
// on; and the figures wanted of each day from it to Now's, oldest first.
// Its days are time.Local's, as the ledger's are.
type DayMoment struct {
	Name      string
	Lines     []ledger.Line
	Summaries []ledger.Summary
	From, Now time.Time
	Want      []DayFigures
}

// DayFigures are a day's figures, as the Log's day rule says them: the
// local day's date; its requests, as the ledger counts them; its sessions,
// each once; the limits its accounts reached; and what its requests were
// worth at the price table's prices, none where none was priced.
type DayFigures struct {
	Date                       string
	Requests, Sessions, Limits int
	Worth                      *ledger.Picodollars
}

// Caps are the caps the moments' days are summarised with: no reserves, and
// the 5-hour window and the week every model's.
var Caps = ledger.Caps{Shared: []string{fiveHour, week}}

// The models the moments' requests ask for, each request a million tokens of
// input, worth $4 of Claude Opus 5.5, $2 of Claude Sonnet 5.5 and $1 of
// Claude Haiku 4.5 at ledger.Pricing's prices.
const (
	opus, sonnet, haiku = "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-4-5"
	aMillionIn          = `{"input_tokens":1000000}`
)

// DayMoments are the moments views have got a day's figures wrong at, or
// could, in time.Local, on and around Wednesday 7 October 2026, and as the
// clocks go back in London on Sunday 25 October.
func DayMoments() []DayMoment {
	on := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local) }
	return []DayMoment{
		summarisedDay(on),
		unsummarisedToday(on),
		linelessDay(on),
		checksAloneDay(on),
		outdatedSummary(on),
		acrossMidnightDay(on),
		fallingBackDay(),
	}
}

// summarisedDay is 6 October as the router summarised it, its lines pruned
// since, so its summary alone tells of it: three requests on work and side,
// a session on both, a quota check, which isn't a request and is worth
// nothing, whatever it spent, and one no account had room for, which the
// router answered itself; and the limits the readings history told of, two
// of them in the 5-hour window.
func summarisedDay(on func(day, hour, minute int) time.Time) DayMoment {
	pruned := []ledger.Line{
		request("r1", "s1", work, opus, on(6, 10, 0)),
		request("r2", "s2", work, sonnet, on(6, 11, 0)),
		request("r3", "s2", side, sonnet, on(6, 12, 0)),
		checked("r4", "s1", work, on(6, 12, 30)),
		unsent("r5", "s3", on(6, 13, 0)),
	}
	summary := summarised(pruned, on(7, 2, 0),
		limited(work, fiveHour, on(6, 14, 0)), limited(side, fiveHour, on(6, 15, 0)), limited(side, week, on(6, 16, 0)))
	return DayMoment{
		Name: "a summarised day, its lines pruned", Summaries: []ledger.Summary{summary},
		From: on(6, 0, 0), Now: on(7, 3, 0),
		Want: []DayFigures{
			{Date: "2026-10-06", Requests: 4, Sessions: 3, Limits: 3, Worth: worth(8)},
			{Date: "2026-10-07"},
		},
	}
}

// unsummarisedToday is today, summarised from its lines as it's read.
func unsummarisedToday(on func(day, hour, minute int) time.Time) DayMoment {
	return DayMoment{
		Name: "today, unsummarised",
		Lines: []ledger.Line{
			request("t1", "s4", work, opus, on(7, 9, 0)),
			request("t2", "s4", work, opus, on(7, 9, 30)),
			request("t3", "s5", side, haiku, on(7, 10, 0)),
		},
		From: on(7, 0, 0), Now: on(7, 15, 0),
		Want: []DayFigures{{Date: "2026-10-07", Requests: 3, Sessions: 2, Worth: worth(9)}},
	}
}

// linelessDay is 6 October, with no lines and no summary, between two days
// with lines.
func linelessDay(on func(day, hour, minute int) time.Time) DayMoment {
	return DayMoment{
		Name:  "a day with no lines",
		Lines: []ledger.Line{request("a1", "s1", work, opus, on(5, 18, 0)), request("a2", "s2", side, haiku, on(7, 8, 0))},
		From:  on(5, 0, 0), Now: on(7, 9, 0),
		Want: []DayFigures{
			{Date: "2026-10-05", Requests: 1, Sessions: 1, Worth: worth(4)},
			{Date: "2026-10-06"},
			{Date: "2026-10-07", Requests: 1, Sessions: 1, Worth: worth(1)},
		},
	}
}

// checksAloneDay is today with Claude Code's quota checks alone, as it
// resumed two sessions that went no further: no request, no session, and
// worth nothing, whatever the checks spent.
func checksAloneDay(on func(day, hour, minute int) time.Time) DayMoment {
	return DayMoment{
		Name:  "a day of quota checks alone",
		Lines: []ledger.Line{checked("k1", "s1", work, on(7, 9, 0)), checked("k2", "s2", side, on(7, 9, 5))},
		From:  on(7, 0, 0), Now: on(7, 10, 0),
		Want: []DayFigures{{Date: "2026-10-07", Worth: worth(0)}},
	}
}

// outdatedSummary is 6 October summarised from two of its lines, a limit
// among what it read, then a third filed under it, its request arriving
// before midnight and ending once the summary was written: the day is
// summarised again from its lines as it's read, knowing no less than the
// summary held.
func outdatedSummary(on func(day, hour, minute int) time.Time) DayMoment {
	lines := []ledger.Line{
		request("b1", "s1", work, opus, on(6, 10, 0)),
		request("b2", "s2", side, sonnet, on(6, 20, 0)),
	}
	late := request("b3", "s3", work, haiku, on(6, 23, 55))
	late.TotalMS = (90 * time.Minute).Milliseconds()
	return DayMoment{
		Name:      "a day whose summary is older than its last line",
		Lines:     append(slices.Clone(lines), late),
		Summaries: []ledger.Summary{summarised(lines, on(7, 1, 0), limited(work, fiveHour, on(6, 15, 0)))},
		From:      on(6, 0, 0), Now: on(7, 2, 0),
		Want: []DayFigures{
			{Date: "2026-10-06", Requests: 3, Sessions: 3, Limits: 1, Worth: worth(7)},
			{Date: "2026-10-07"},
		},
	}
}

// acrossMidnightDay is a request arriving at 23:59:50 on 6 October and
// ending at 00:00:12, counted on the day it arrived, and one the next
// morning.
func acrossMidnightDay(on func(day, hour, minute int) time.Time) DayMoment {
	late := request("c1", "s1", work, opus, on(6, 23, 59).Add(50*time.Second))
	late.TotalMS = (22 * time.Second).Milliseconds()
	return DayMoment{
		Name:  "a request across midnight",
		Lines: []ledger.Line{late, request("c2", "s2", side, sonnet, on(7, 9, 0))},
		From:  on(6, 0, 0), Now: on(7, 10, 0),
		Want: []DayFigures{
			{Date: "2026-10-06", Requests: 1, Sessions: 1, Worth: worth(4)},
			{Date: "2026-10-07", Requests: 1, Sessions: 1, Worth: worth(2)},
		},
	}
}

// fallingBackDay is two requests at 01:30 on 25 October as London's clocks
// go back an hour at 02:00, read an hour after the second: in time.Local, so
// both fall on one day in London, a day 25 hours long, and in any zone whose
// midnight doesn't come between them.
func fallingBackDay() DayMoment {
	first := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).Local()
	second := first.Add(time.Hour)
	return DayMoment{
		Name:  "two requests at 01:30 as the clocks go back",
		Lines: []ledger.Line{request("d1", "s1", work, opus, first), request("d2", "s2", side, sonnet, second)},
		From:  dayfile.DayStart(first, 0), Now: second.Add(time.Hour),
		Want: []DayFigures{{Date: dayfile.DateOf(first), Requests: 2, Sessions: 2, Worth: worth(6)}},
	}
}

// request is a request of the session given, asking the model given for a
// million tokens of input, that arrived at at and went to the account given.
func request(id, session, account, model string, at time.Time) ledger.Line {
	return ledger.Line{At: at, Request: id, Kind: ledger.KindMessage, Session: session, Model: model, Account: account,
		Reason: "new", Status: 200, Attempts: 1, TotalMS: 4000, Usage: json.RawMessage(aMillionIn)}
}

// checked is Claude Code's quota check, of the session given, on the account
// given, at at: never a request. It spends a million tokens of input, as no
// check does, so what it spent would show in a worth that priced it.
func checked(id, session, account string, at time.Time) ledger.Line {
	return ledger.Line{At: at, Request: id, Kind: ledger.KindCheck, Session: session, Model: haiku, Account: account,
		Reason: "new", Status: 200, Attempts: 1, TotalMS: 300, Usage: json.RawMessage(aMillionIn)}
}

// unsent is a request of the session given that no account had room for,
// which the router answered itself at at, never sending it on.
func unsent(id, session string, at time.Time) ledger.Line {
	return ledger.Line{At: at, Request: id, Kind: ledger.KindMessage, Session: session, Model: opus, Reason: "no account has room", Status: 429, TotalMS: 2}
}

// limit is a limit the account given reached in the window with the given
// key at at, as the readings history told a summary of it.
type limit struct {
	account string
	limit   ledger.Limit
}

// limited is the account given reaching its limit in the window with the
// given key at at.
func limited(account, key string, at time.Time) limit {
	return limit{account: account, limit: ledger.Limit{Window: key, At: at.UTC()}}
}

// summarised is the summary the router wrote at asOf of the local day the
// lines arrived on, from them, and the limits its accounts reached, as the
// readings history told it.
func summarised(lines []ledger.Line, asOf time.Time, limits ...limit) ledger.Summary {
	date := dayfile.DateOf(lines[0].At)
	summary, err := ledger.Summarise(date, slices.Values(lines), noReadings, nil, Caps, asOf)
	if err != nil {
		panic(fmt.Sprintf("viewstest: %v", err))
	}
	for _, l := range limits {
		i := slices.IndexFunc(summary.Accounts, func(a ledger.AccountDay) bool { return a.Account == l.account })
		summary.Accounts[i].Limits = append(summary.Accounts[i].Limits, l.limit)
	}
	return summary
}

// noReadings reads no readings of any time.
func noReadings(time.Time, time.Time) iter.Seq[readings.Reading] {
	return func(func(readings.Reading) bool) {}
}

// worth is n dollars, as the ledger counts money.
func worth(n int) *ledger.Picodollars {
	w := ledger.Picodollars(n) * ledger.Dollar
	return &w
}
