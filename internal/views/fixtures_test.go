package views_test

import (
	"encoding/json"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// now is the time by the clock in the views' tests: Wednesday 7 October
// 2026, 13:00 local time.
var now = time.Date(2026, 10, 7, 13, 0, 0, 0, time.Local)

// on is the start of the local day with the given date.
func on(date string) time.Time {
	day, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		panic(err)
	}
	return day
}

// The usages the views' tests' requests give, and what each is worth.
const (
	// opusUsage is worth $7.60 of Claude Opus 5.5: $4 of input, $2 of
	// output, $1 of five-minute cache writes and $0.60 of cache reads.
	opusUsage = `{"input_tokens":1000000,"output_tokens":100000,"cache_creation_input_tokens":200000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":200000},"cache_read_input_tokens":3000000}`
	// inputUsage is worth $4 of Claude Opus 5.5, $2 of Claude Sonnet 5.5
	// and $1 of Claude Haiku 4.5.
	inputUsage = `{"input_tokens":1000000}`
	// hourUsage writes the cache for an hour: worth $8 of Claude Opus 5.5.
	hourUsage = `{"cache_creation_input_tokens":1000000,"cache_creation":{"ephemeral_1h_input_tokens":1000000}}`
)

// dollar is a dollar, as the ledger counts money.
const dollar ledger.Picodollars = 1_000_000_000_000

// ledgerOf is a ledger that holds the summaries given, the days asked for.
type ledgerOf []ledger.Summary

func (l ledgerOf) Days(time.Time) []ledger.Summary { return l }

func (ledgerOf) Today(mark ledger.Mark) ([]ledger.Held, ledger.Mark, bool) { return nil, mark, false }

func (ledgerOf) Session(string) iter.Seq[ledger.Held] { return func(func(ledger.Held) bool) {} }

// daysFrom are the summaries of each day from the one with the date first
// to today's, of no requests but those given, by their days.
func daysFrom(first string, given ...ledger.Summary) []ledger.Summary {
	return daysTo(first, now, given...)
}

// daysTo are the summaries of each day from the one with the date first to
// the one last falls on, as daysFrom gives them.
func daysTo(first string, last time.Time, given ...ledger.Summary) []ledger.Summary {
	var days []ledger.Summary
	for day := on(first); !day.After(last); day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		summary := ledger.Summary{Version: 2, Day: date}
		for _, s := range given {
			if s.Day == date {
				summary = s
			}
		}
		days = append(days, summary)
	}
	return days
}

// day is the summary of the day with the given date, of the accounts given.
func day(date string, accounts ...ledger.AccountDay) ledger.Summary {
	return ledger.Summary{Version: 2, Day: date, Accounts: accounts}
}

// onAt is the local time on the day with the given date, at the clock given,
// as 10:00.
func onAt(date, clock string) time.Time {
	t, err := time.ParseInLocation(time.DateOnly+" 15:04", date+" "+clock, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// reset is a reset of the window with the given key at the local time on the
// day with the given date, at the clock given, its use as last read before
// it the one given.
func reset(key, date, clock string, before float64) ledger.Reset {
	return ledger.Reset{Window: key, At: onAt(date, clock).UTC(), Before: before}
}

// limit is a limit of the window with the given key reached at the local
// time on the day with the given date, at the clock given.
func limit(key, date, clock string) ledger.Limit {
	return ledger.Limit{Window: key, At: onAt(date, clock).UTC()}
}

// account is an account's day, of the models' days given.
func account(id string, models ...ledger.ModelDay) ledger.AccountDay {
	return ledger.AccountDay{Account: id, Models: models}
}

// sent are n requests of the model that went upstream, their usage summed
// as given.
func sent(model string, n int, usage string) ledger.ModelDay {
	m := ledger.ModelDay{Model: model, Upstream: n}
	if usage != "" {
		m.Usage = json.RawMessage(usage)
	}
	return m
}

// family reads a model's family from its id as internal/claude does, the
// part after claude-, without knowing any.
func family(model string) string {
	return strings.Split(strings.TrimPrefix(model, "claude-"), "-")[0]
}

// accounts are the configured accounts in the views' tests: work, on Max
// 20x, side, on Pro, and spare, on a plan not known.
var accounts = config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "side", Plan: "pro"}, {ID: "spare"}}

// input is what the views' tests build a History from: the days given, asked
// for from a day before the ledger began, so its first day cuts no period
// short, by the built-in price table, with accounts configured, the weeks
// starting on Monday, and the accounts' weeks those of the window 7d.
func input(days []ledger.Summary) views.HistoryInput {
	return views.HistoryInput{
		Ledger: ledgerOf(days), From: on("2026-01-01"), Now: now, Prices: ledger.Pricing,
		Accounts: accounts, WeekStarts: time.Monday, WeekWindow: "7d", Family: family,
	}
}

// jsonOf is v as JSON.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// overridden is the built-in price table with the config's prices given in
// place of its own.
func overridden(t *testing.T, prices config.Prices) ledger.Table {
	t.Helper()
	table, passed := ledger.Pricing.With(prices)
	if len(passed) > 0 {
		t.Fatalf("the config's prices pass over %q", passed)
	}
	return table
}
