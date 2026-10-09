package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

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
// against it; then by account, each by model version too, and by model
// version, with the accounts it ran on.
type TokenWeek struct {
	Week string `json:"week"`
	Tokens
	Worth
	Against
	ByAccount []AccountTokens `json:"by_account,omitempty"`
	ByModel   []ModelTokens   `json:"by_model,omitempty"`
}

// AccountTokens are an account's tokens in a week, and their worth, then by
// model version. The account is left out of those the router answered with
// none.
type AccountTokens struct {
	Account string `json:"account,omitempty"`
	Tokens
	Worth
	ByModel []ModelTokens `json:"by_model,omitempty"`
}

// ModelTokens are a model version's tokens in a week, by the first id the
// version table names it by, or its own where the table doesn't, and their
// worth, each of its ids at its own prices. On, of every account's, are the
// accounts it ran on.
type ModelTokens struct {
	Model string   `json:"model"`
	On    []string `json:"on,omitempty"`
	Tokens
	Worth
}

// tokensOf are Tokens' rows over days, asked for from the day with the date
// from, a calendar week each, weeks starting on the day given, each costing
// its plans 12/52 of a month as of its first day, the week so far whole, but
// for one the days asked for begin part-way through, as cutShort says.
func tokensOf(days []Day, p plans, v versions, from string, weekStarts time.Weekday) []TokenWeek {
	weeks := []TokenWeek{}
	for _, s := range spansOf(days, weekKey(weekStarts)) {
		week := tokenWeekOf(s, v)
		first, _ := civil(s.key)
		priced, at := cutShort(s.key, first.AddDate(0, 0, 6).Format(time.DateOnly), from, aWeek)
		week.Against = p.against(accountWorths(s.days), at, priced)
		weeks = append(weeks, week)
	}
	return weeks
}

// tokenWeekOf is the week the span's days make: their tokens and worth, by
// account and by model version.
func tokenWeekOf(s span, v versions) TokenWeek {
	week := TokenWeek{Week: s.key}
	models, accounts := modelTally{}, accountTally{}
	for _, d := range s.days {
		for _, a := range d.Accounts {
			account := accounts.of(a.Account)
			for _, m := range a.Models {
				tokens := m.Tokens()
				week.count(tokens)
				week.addModel(m)
				account.count(tokens)
				account.addModel(m)
				account.models.add(v, m, tokens, "")
				models.add(v, m, tokens, a.Account)
			}
		}
	}
	week.ByAccount, week.ByModel = accounts.list(), models.list()
	return week
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

// list lists the accounts in the order of their ids, each with its model
// versions.
func (t accountTally) list() []AccountTokens {
	listed := make([]AccountTokens, 0, len(t))
	for _, id := range slices.Sorted(maps.Keys(t)) {
		account := t[id].AccountTokens
		account.ByModel = t[id].models.list()
		listed = append(listed, account)
	}
	return listed
}

// weekKey is a key giving the first day of the week a day falls in, weeks
// starting on the day given.
func weekKey(starts time.Weekday) func(Day) (string, bool) {
	return func(d Day) (string, bool) {
		day, ok := civil(d.Day)
		return weekOf(day, starts).Format(time.DateOnly), ok
	}
}

// modelTally is model versions' tokens and worth, by the version's id, as
// versions.of gives it, with where the version table places it.
type modelTally map[string]*rankedModel

// rankedModel is a model version's tokens and worth, and where the version
// table places it.
type rankedModel struct {
	ModelTokens
	rank int
}

// add adds a model's day, its tokens given, to its version's, and the
// account it ran on, where one's given, to those the version ran on: none
// of the requests whose bodies couldn't be read, which name no model, and so
// spent nothing.
func (t modelTally) add(v versions, m ledger.PricedModel, tokens quota.Tokens, account string) {
	if m.Model == "" {
		return
	}
	id, rank := v.of(m.Model)
	model, ok := t[id]
	if !ok {
		model = &rankedModel{Model: id, rank: rank}
		t[id] = model
	}
	model.count(tokens)
	model.addModel(m)
	if i, found := slices.BinarySearch(model.On, account); account != "" && !found {
		model.On = slices.Insert(model.On, i, account)
	}
}

// list lists the versions in the version table's order, those it doesn't
// name after, by id.
func (t modelTally) list() []ModelTokens {
	ranked := slices.SortedFunc(maps.Values(t), func(a, b *rankedModel) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(a.Model, b.Model))
	})
	listed := make([]ModelTokens, len(ranked))
	for i, model := range ranked {
		listed[i] = model.ModelTokens
	}
	return listed
}
