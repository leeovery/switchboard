package capture

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// sampleWeek is work's tokens this week, as Tokens draws them: 1.2M input,
// 0.8M output, 11.0M written to the cache for five minutes, and 178M read
// from it, worth $183.75 at the frames' prices. Every request of the worlds'
// is a share of it, so its worth is its tokens'.
var sampleWeek = struct {
	input, output, written, read float64
	worth                        float64
}{1.2e6, 0.8e6, 11.0e6, 178e6, 183.75}

// summaries are the ledger's days of the accounts given, from the year's
// first to yesterday, as the ledger keeps them: each summarised from the
// readings history as the ledger summarises a day, its windows' highest,
// rises, resets, limits and minutes at the cap and at a limit; and each
// account's requests by model, and by the hour, its sessions and its moves,
// as the day's lines would give them.
func (y year) summaries(h history, accounts config.Accounts) []ledger.Summary {
	caps := ledger.CapsOf(accounts, claude.SharedWindows())
	none := func(func(ledger.Line) bool) {}
	days := make([]ledger.Summary, 0, today)
	for d := range today {
		date := dateOf(d)
		s, err := ledger.Summarise(date, none, h.Between, nil, caps, dayStart(y.now, d+1))
		if err != nil {
			panic(fmt.Sprintf("summarise %s: %v", date, err))
		}
		for _, a := range accounts {
			if use := y.days[a.ID][d]; use.requests > 0 || use.movedOn > 0 {
				s.Lines += use.requests
				s.Accounts = withUse(s.Accounts, a.ID, use, d)
			}
		}
		days = append(days, s)
	}
	return days
}

// withUse is the accounts' days with the account with the given id's use of
// day d of the year in its, which is added where there's none, in the order
// of their ids.
func withUse(days []ledger.AccountDay, id string, use accountDay, d int) []ledger.AccountDay {
	i, found := slices.BinarySearchFunc(days, id, func(a ledger.AccountDay, id string) int { return cmp.Compare(a.Account, id) })
	if !found {
		days = slices.Insert(days, i, ledger.AccountDay{Account: id})
	}
	day := &days[i]
	day.Sessions, day.SessionIDs = len(use.sessions), slices.Clone(use.sessions)
	day.MovedOn, day.MovedOff = use.movedOn, use.movedOff
	day.Models = modelsOf(use, d)
	return days
}

// modelsOf are an account's day's requests, on day d of the year, by the
// model each asked for, in the order of their names: each version's share of
// the day's worth, and of its requests, sent upstream, by the hour as hoursOf
// shares them, of the day's first sessions, by their ids, one a request at
// most; the worth of a version too little used for a request the most used
// one's.
func modelsOf(use accountDay, d int) []ledger.ModelDay {
	versions := ids(use.versions)
	counts := apportion(use.requests, weightsOf(indexes(len(versions)), func(i int) float64 { return use.versions[versions[i]] }))
	most := slices.Index(counts, slices.Max(counts))
	worths := make([]float64, len(versions))
	for i, v := range versions {
		worths[i] = use.worth * use.versions[v]
	}
	for i, n := range counts {
		if n == 0 {
			worths[most] += worths[i]
		}
	}
	var models []ledger.ModelDay
	for i, v := range versions {
		if counts[i] > 0 {
			of := slices.Clone(use.sessions[:min(len(use.sessions), counts[i])])
			models = append(models, ledger.ModelDay{Model: v, Upstream: counts[i], Hours: hoursOf(counts[i], d), Sessions: len(of), SessionIDs: of,
				Usage: usageWorth(worths[i])})
		}
	}
	slices.SortFunc(models, func(a, b ledger.ModelDay) int { return cmp.Compare(a.Model, b.Model) })
	return models
}

// weekdayHours and weekendHours weigh a day's requests by the local hour they
// come in, as the design's WHEN YOU WORK was drawn with them: on a weekday,
// most from 09:00 to 18:00 but at noon, half as many at noon and from 18:00,
// fewer through the evening, a few at 08:00 and 23:00, and none overnight; at
// a weekend, some from 10:00 to 17:00, and a few to 21:00.
var (
	weekdayHours = [24]float64{8: 6, 9: 80, 10: 80, 11: 80, 12: 40, 13: 80, 14: 80, 15: 80, 16: 80, 17: 80, 18: 40,
		19: 18, 20: 18, 21: 18, 22: 18, 23: 6}
	weekendHours = [24]float64{10: 22, 11: 22, 12: 22, 13: 22, 14: 22, 15: 22, 16: 22, 17: 6, 18: 6, 19: 6, 20: 6}
)

// hoursOf shares n requests of day d of the year among its hours, as
// apportion shares them, by its weekday's weights, each varied as the
// design's were, to between 0.55 and 1.45 times itself, by a draw from the
// day's date and the hour, the same every run.
func hoursOf(n, d int) *[24]int {
	weights := weekdayHours
	if d%7 >= 5 {
		weights = weekendHours
	}
	for h := range weights {
		weights[h] *= 0.55 + 0.9*float64(drawn(fmt.Appendf(nil, "%s/%02d", dateOf(d), h)))/math.MaxUint64
	}
	var hours [24]int
	copy(hours[:], apportion(n, weights[:]))
	return &hours
}

// indexes are the indexes of a list of n.
func indexes(n int) []int {
	held := make([]int, n)
	for i := range held {
		held[i] = i
	}
	return held
}

// usageWorth is the usage of requests worth the dollars given at the frames'
// prices: a share of sampleWeek's tokens, the cache written for five minutes,
// as an answer's usage gives them.
func usageWorth(dollars float64) json.RawMessage {
	share := dollars / sampleWeek.worth
	written := int64(math.Round(sampleWeek.written * share))
	usage, _ := json.Marshal(map[string]any{
		"input_tokens":                int64(math.Round(sampleWeek.input * share)),
		"output_tokens":               int64(math.Round(sampleWeek.output * share)),
		"cache_creation_input_tokens": written,
		"cache_creation":              map[string]int64{"ephemeral_5m_input_tokens": written},
		"cache_read_input_tokens":     int64(math.Round(sampleWeek.read * share)),
	})
	return usage
}
