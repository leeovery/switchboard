package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// Tokens are tokens by kind, as ccusage counts them: CacheWrite is the
// cache's writes for five minutes and for an hour together, and Total the
// four kinds.
type Tokens struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheWrite int `json:"cache_write"`
	CacheRead  int `json:"cache_read"`
	Total      int `json:"total"`
}

// count adds the tokens given.
func (t *Tokens) count(given quota.Tokens) {
	t.Input += given.Input
	t.Output += given.Output
	t.CacheWrite += given.CacheWrite
	t.CacheRead += given.CacheRead
	t.Total += given.Input + given.Output + given.CacheWrite + given.CacheRead
}

// TokenWeek is a row of Tokens, a calendar week, by its first day: its
// tokens and their worth, what the plans cost that week and the worth
// against it; then by account, in the config's order, each by model version
// too, and by model version, with the accounts it ran on.
type TokenWeek struct {
	Week string `json:"week"`
	Tokens
	Worth
	Against
	ByAccount []AccountTokens `json:"by_account,omitempty"`
	ByModel   []ModelTokens   `json:"by_model,omitempty"`
}

// NoAccount is what a row names the account of the requests none answered,
// whose account is "".
const NoAccount = "no account"

// AccountTokens are an account's tokens in a week, and their worth, then by
// model version. The account is left out of those the router answered with
// none. Label is its label, as the config gives it, none where the config
// doesn't configure it: the JSON names an account by its id alone, as
// everywhere.
type AccountTokens struct {
	Account string `json:"account,omitempty"`
	Label   string `json:"-"`
	Tokens
	Worth
	ByModel []ModelTokens `json:"by_model,omitempty"`
}

// ModelTokens are a model version's tokens in a week, by the shortest id the
// version table names it by, or its own where the table doesn't, and their
// worth, each of its ids at its own prices. On, of every account's, are the
// accounts it ran on, the one it was worth most on first.
type ModelTokens struct {
	Model string   `json:"model"`
	On    []string `json:"on,omitempty"`
	Tokens
	Worth
}

// tokensOf are Tokens' rows over days, asked for from the day with the date
// from, a calendar week each, weeks starting on the day given, as
// tokenPeriodsOf gives them.
func tokensOf(days []Day, p plans, v versions, from string, weekStarts time.Weekday) []TokenWeek {
	periods := tokenPeriodsOf(days, p, v, from, TokensByWeek.shape(weekStarts))
	weeks := make([]TokenWeek, len(periods))
	for i, t := range periods {
		weeks[i] = TokenWeek{Week: t.First, Tokens: t.Tokens, Worth: t.Worth, Against: t.Against, ByAccount: t.ByAccount, ByModel: t.ByModel}
	}
	return weeks
}

// tokenPeriodsOf are Tokens' rows over days, asked for from the day with the
// date from, a period each, as the shape given makes them, oldest first,
// each costing its plans its share of a month as of its first day, the
// period so far whole, but for one the days asked for begin part-way
// through, as cutShort says.
func tokenPeriodsOf(days []Day, p plans, v versions, from string, by periodShape) []TokenPeriod {
	periods := []TokenPeriod{}
	for _, s := range spansOf(days, by.key) {
		period := tokenPeriodOf(s, p, v)
		first, last := by.dates(s.key)
		priced, at := cutShort(first, last, from, by.share)
		period.Against = p.against(accountWorths(s.days), at, priced)
		periods = append(periods, period)
	}
	return periods
}

// tokenPeriodOf is the period the span's days make: their tokens and worth,
// by account, in the config's order, those it doesn't configure after, and
// by model version, and how many versions they were of. An account whose
// days hold no model's, as one whose windows were read but that wasn't used,
// has none.
func tokenPeriodOf(s span, p plans, v versions) TokenPeriod {
	period := TokenPeriod{First: s.key}
	models, accounts := modelTally{}, accountTally{}
	for _, d := range s.days {
		for _, a := range d.Accounts {
			if len(a.Models) == 0 {
				continue
			}
			account := accounts.of(a.Account)
			for _, m := range a.Models {
				tokens := m.Tokens()
				period.count(tokens)
				period.addModel(m)
				account.count(tokens)
				account.addModel(m)
				account.models.add(v, m, tokens, "")
				models.add(v, m, tokens, a.Account)
			}
		}
	}
	period.ByAccount, period.ByModel = accounts.list(p.accounts), models.list()
	period.Versions = len(period.ByModel)
	return period
}

// accountTally is accounts' tokens and worth, by account, and by model
// version within each.
type accountTally map[string]*talliedAccount

// talliedAccount is an account's tokens and worth, and by model version.
type talliedAccount struct {
	AccountTokens
	models modelTally
}

// of returns the account's tally, begun where there's none.
func (t accountTally) of(id string) *talliedAccount {
	account, ok := t[id]
	if !ok {
		account = &talliedAccount{Account: id, models: modelTally{}}
		t[id] = account
	}
	return account
}

// list lists the accounts in the config's order, those it doesn't configure
// after, by id, each with its label and its model versions.
func (t accountTally) list(configured config.Accounts) []AccountTokens {
	listed := make([]AccountTokens, 0, len(t))
	for _, id := range slices.SortedFunc(maps.Keys(t), byConfig(configured)) {
		account := t[id].AccountTokens
		account.Label, account.ByModel = labelOf(configured, id), t[id].models.list()
		listed = append(listed, account)
	}
	return listed
}

// labelOf is the config's label for the account with the given id: none
// where it doesn't configure it.
func labelOf(configured config.Accounts, id string) string {
	if i := slices.IndexFunc(configured, func(a config.Account) bool { return a.ID == id }); i >= 0 {
		return configured[i].Label
	}
	return ""
}

// weekKey is a key giving the first day of the week a day falls in, weeks
// starting on the day given.
func weekKey(starts time.Weekday) func(Day) (string, bool) {
	week := weekDate(starts)
	return func(d Day) (string, bool) { return week(d.Day) }
}

// weekDate gives the date of the first day of the week the day with the
// date given falls in, weeks starting on the day given, reporting false for
// a date that isn't one.
func weekDate(starts time.Weekday) func(date string) (string, bool) {
	return func(date string) (string, bool) {
		day, ok := civil(date)
		return weekOf(day, starts).Format(time.DateOnly), ok
	}
}

// modelTally is model versions' tokens and worth, by the version's id, as
// versions.of gives it, with where the version table places it.
type modelTally map[string]*rankedModel

// rankedModel is a model version's tokens and worth, where the version table
// places it, and what it was worth on each account it ran on, by the
// account's id.
type rankedModel struct {
	ModelTokens
	rank int
	on   map[string]Worth
}

// add adds a model's day, its tokens given, to its version's, and what it
// was worth on the account it ran on, where one's given, to the version's
// there: none of the requests whose bodies couldn't be read, which name no
// model, and so spent nothing.
func (t modelTally) add(v versions, m ledger.PricedModel, tokens quota.Tokens, account string) {
	if m.Model == "" {
		return
	}
	id, rank := v.of(m.Model)
	model, ok := t[id]
	if !ok {
		model = &rankedModel{Model: id, rank: rank, on: make(map[string]Worth)}
		t[id] = model
	}
	model.count(tokens)
	model.addModel(m)
	if account != "" {
		on := model.on[account]
		on.addModel(m)
		model.on[account] = on
	}
}

// list lists the versions in the version table's order, those it doesn't
// name after, by id, each with the accounts it ran on, as onByWorth orders
// them.
func (t modelTally) list() []ModelTokens {
	ranked := slices.SortedFunc(maps.Values(t), func(a, b *rankedModel) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(a.Model, b.Model))
	})
	listed := make([]ModelTokens, len(ranked))
	for i, model := range ranked {
		listed[i] = model.ModelTokens
		listed[i].On = model.onByWorth()
	}
	return listed
}

// onByWorth are the accounts the version ran on, the one it was worth most
// on first, then by id: nil where it ran on none it was told of.
func (m *rankedModel) onByWorth() []string {
	if len(m.on) == 0 {
		return nil
	}
	return slices.SortedFunc(maps.Keys(m.on), func(a, b string) int {
		return cmp.Or(cmp.Compare(m.on[b].amount(), m.on[a].amount()), cmp.Compare(a, b))
	})
}
