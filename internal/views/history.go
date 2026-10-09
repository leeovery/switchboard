package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// History is what History's views and Accounts' build over the days asked
// for, block by block, each oldest first, as history prints them: the days,
// Year's grid, BY MONTH's rows, Weeks' weeks, Tokens' weeks, the accounts'
// totals and plans, and Weeks' verdicts.
type History struct {
	Days     []Day       `json:"days"`
	Year     Year        `json:"year"`
	Months   []Month     `json:"months"`
	Weeks    []Week      `json:"weeks"`
	Tokens   []TokenWeek `json:"tokens"`
	Totals   Totals      `json:"totals"`
	Plans    []PlanCost  `json:"plans"`
	Capacity Capacity    `json:"capacity"`
}

// HistoryInput is what a History is built from.
type HistoryInput struct {
	// Ledger gives the days' summaries.
	Ledger LedgerDays
	// From is when the days asked for start, and Now the time now: they're
	// the local days from From's to today's, those the ledger holds.
	From, Now time.Time
	// Prices price the days' requests, at today's prices, and the plans, and
	// name the models' versions and families.
	Prices ledger.Table
	// Accounts are the configured accounts, whose plans the worth is priced
	// against.
	Accounts config.Accounts
	// WeekStarts is the day the calendar's weeks start on.
	WeekStarts time.Weekday
	// WeekWindow is the key of the window a week long every model shares,
	// as internal/claude names it: the accounts' own weeks, which Weeks
	// counts, are its.
	WeekWindow string
	// Family reads a model's family from its id, as internal/claude does,
	// for a model the version table doesn't name.
	Family func(model string) string
}

// NewHistory builds the History of the days in asks for.
func NewHistory(in HistoryInput) History {
	from, to := dateOf(in.From), dateOf(in.Now)
	asked, today := from.Format(time.DateOnly), to.Format(time.DateOnly)
	v := versions{table: in.Prices, family: in.Family}
	p := plans{accounts: in.Accounts, table: in.Prices}
	days := pricedDays(in.Ledger.Days(in.From), in.Prices, today, v)
	placed := weeklyOf(in.WeekWindow, in.WeekStarts, in.Now).placedWeeks(days, in.Accounts)
	return History{
		Days:     days,
		Year:     yearOf(days),
		Months:   monthsOf(days, p, v, asked, today),
		Weeks:    weeksOf(placed, in.Accounts),
		Tokens:   tokensOf(days, p, v, asked, in.WeekStarts),
		Totals:   totalsOf(days, in.Accounts, v),
		Plans:    plansOf(days, p, asked, askedShare(from, to, in.WeekStarts)),
		Capacity: capacityOf(placed, wholeWeek(days, weekOf(to, in.WeekStarts)), in.Accounts, p, in.WeekWindow),
	}
}

// wholeWeek reports whether a calendar week, by its first day's date, is
// whole among days, which run to today, this week the one with the first day
// given: begun on their first day or after, and over, so its accounts' weeks
// placed there have each ended by its reset, a week still running being
// placed under this week.
func wholeWeek(days []Day, this time.Time) func(week string) bool {
	current := this.Format(time.DateOnly)
	first := current
	if len(days) > 0 {
		first = days[0].Day
	}
	return func(week string) bool { return week >= first && week < current }
}

// Day is a day of the ledger as Days draws it: its summary as the ledger
// holds it, priced, and its worth by family.
type Day struct {
	ledger.Priced
	ByFamily []FamilyWorth `json:"by_family,omitempty"`
}

// FamilyWorth is what a family's requests were worth, by the family's name
// as the version table gives it, as opus.
type FamilyWorth struct {
	Family string `json:"family"`
	Worth
}

// pricedDays are the summaries priced at the prices in effect on the day
// with the date today, each with its worth by family.
func pricedDays(summaries []ledger.Summary, prices ledger.Table, today string, v versions) []Day {
	days := make([]Day, len(summaries))
	for i, s := range summaries {
		priced := prices.Priced(s, today)
		sums := familySums{}
		for _, a := range priced.Accounts {
			for _, m := range a.Models {
				sums.addModel(v, m)
			}
		}
		days[i] = Day{Priced: priced, ByFamily: v.byFamily(sums)}
	}
	return days
}

// requests counts the day's requests, every account's.
func (d Day) requests() int {
	n := 0
	for _, a := range d.Accounts {
		for _, m := range a.Models {
			n += m.Requests()
		}
	}
	return n
}

// accountWorths are what the days' requests were worth, by account.
func accountWorths(days []Day) map[string]Worth {
	worths := make(map[string]Worth)
	for _, d := range days {
		for _, a := range d.Accounts {
			w := worths[a.Account]
			for _, m := range a.Models {
				w.addModel(m)
			}
			worths[a.Account] = w
		}
	}
	return worths
}

// familySums are what requests were worth, by family.
type familySums map[string]Worth

// add adds w to the family's.
func (s familySums) add(family string, w Worth) {
	sum := s[family]
	sum.add(w)
	s[family] = sum
}

// addModel adds what a model's day was worth to its family's: none of the
// requests whose bodies couldn't be read, which name no model, and so spent
// nothing.
func (s familySums) addModel(v versions, m ledger.PricedModel) {
	if m.Model == "" {
		return
	}
	var w Worth
	w.addModel(m)
	s.add(v.familyOf(m.Model), w)
}

// versions name the models' versions and families as the version table
// does, and as family reads a family from its id, for a model it doesn't
// name.
type versions struct {
	table  ledger.Table
	family func(model string) string
}

// familyOf is the family of the model with the given id.
func (v versions) familyOf(model string) string {
	if version, ok := v.table.Version(model); ok {
		return version.Family
	}
	return v.family(model)
}

// familyRank places a family as the version table orders them: Opus,
// Sonnet, Fable, Haiku, then Mythos, those it doesn't know after.
func (v versions) familyRank(family string) int {
	i := slices.IndexFunc(v.table.Versions, func(version ledger.Version) bool { return version.Family == family })
	if i < 0 {
		return len(v.table.Versions)
	}
	return i
}

// of is the version of the model with the given id, by the shortest id the
// version table names it by, its alias, as claude-haiku-4-5, so its ids
// count as one, and where the table places it; or its own id, after every
// version the table names, for one it doesn't.
func (v versions) of(model string) (id string, rank int) {
	i := slices.IndexFunc(v.table.Versions, func(version ledger.Version) bool { return slices.Contains(version.IDs, model) })
	if i < 0 {
		return model, len(v.table.Versions)
	}
	return slices.MinFunc(v.table.Versions[i].IDs, func(a, b string) int { return cmp.Compare(len(a), len(b)) }), i
}

// byFamily lists sums, in the version table's order of families, then by
// name.
func (v versions) byFamily(sums familySums) []FamilyWorth {
	names := slices.SortedFunc(maps.Keys(sums), func(a, b string) int {
		return cmp.Or(cmp.Compare(v.familyRank(a), v.familyRank(b)), cmp.Compare(a, b))
	})
	listed := make([]FamilyWorth, len(names))
	for i, name := range names {
		listed[i] = FamilyWorth{Family: name, Worth: sums[name]}
	}
	return listed
}

// span is a run of days that fall in one period, by its key.
type span struct {
	key  string
	days []Day
}

// spansOf splits days, in order, into runs of those key gives the same key,
// passing over those it reports false for.
func spansOf(days []Day, key func(Day) (string, bool)) []span {
	var spans []span
	for _, d := range days {
		k, ok := key(d)
		switch n := len(spans); {
		case !ok:
		case n > 0 && spans[n-1].key == k:
			spans[n-1].days = append(spans[n-1].days, d)
		default:
			spans = append(spans, span{key: k, days: []Day{d}})
		}
	}
	return spans
}

// civil is the local day with the given date as a date in UTC, where every
// date has its midnight and its 24 hours, for counting days. It reports
// false for a date that isn't one.
func civil(date string) (time.Time, bool) {
	day, err := time.Parse(time.DateOnly, date)
	return day, err == nil
}

// dateOf is the local day t falls on, as civil gives it.
func dateOf(t time.Time) time.Time {
	y, m, d := t.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
