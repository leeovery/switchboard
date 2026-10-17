package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// DayAccounts are a day's accounts as history's day table shows them, beside
// the Log's DayTotals, which say the day's figures over every account: each
// account's own, a row each of its models with requests, and what's of note
// in its day.
type DayAccounts struct {
	// Date is the local day's date.
	Date string
	// Accounts are the day's accounts, in the order its summary has them:
	// each that served a request; and of one that served none, only where
	// its day was eventful, as eventful says, as its highest use alone says
	// little, so a day's idle accounts don't crowd its table.
	Accounts []DayAccount
}

// DayAccount is an account's day, as history's day table shows it.
type DayAccount struct {
	// Account is its id: "" for the requests no account answered.
	Account string
	// Models are its models with requests, in the order the summary has
	// them: a model's quota checks and counts of tokens alone, which aren't
	// requests, make none.
	Models []DayModel
	// Sessions counts the sessions its requests were of, and MovedOn and
	// MovedOff those its requests moved onto it, and off it.
	Sessions, MovedOn, MovedOff int
	// Limits are the limits it reached, and WeeksReset the resets of its
	// windows a week long, its own week or a model's, as quota.WeekLong
	// says, each in the order they came.
	Limits, WeeksReset []WindowAt
	// Highest is each of its windows' highest use that day, in the order of
	// care, as status.CompareWindows has it by Claude's policy.
	Highest []WindowUse
}

// DayModel is a model's day on an account, as a row of history's day table:
// the model its requests asked for, "" for those whose bodies couldn't be
// read, and the inference geo they asked for, "" where they didn't; its
// requests; every token their usage counts, in and out, those read from the
// cache and written to it among them; the sessions they were of; and what
// they were worth, at today's prices.
type DayModel struct {
	Model, Geo                 string
	Requests, Tokens, Sessions int
	Worth
}

// WindowAt is a window, by its key, and when something befell it.
type WindowAt struct {
	Window string
	At     time.Time
}

// WindowUse is a window, by its key, and its use, as a share of it.
type WindowUse struct {
	Window string
	Use    float64
}

// Noted reports whether the account's day has anything to note of it: it's
// an account's, not the requests no account answered, and it had sessions,
// sessions moved onto it or off it, limits reached, weeks reset, or a
// window's use read.
func (a DayAccount) Noted() bool {
	return a.Account != "" && (a.Sessions > 0 || a.MovedOn > 0 || a.MovedOff > 0 || len(a.Limits) > 0 || len(a.WeeksReset) > 0 || len(a.Highest) > 0)
}

// DayAccountsOf are the accounts of each of days, priced, as history's day
// table shows them, oldest first: a day with nothing to show, neither a row
// of a model nor an account's note, left out.
func DayAccountsOf(days []Day) []DayAccounts {
	var shown []DayAccounts
	for _, d := range days {
		day := DayAccounts{Date: d.Day}
		for _, a := range d.Accounts {
			if account := dayAccountOf(a); len(account.Models) > 0 || eventful(account) {
				day.Accounts = append(day.Accounts, account)
			}
		}
		if slices.ContainsFunc(day.Accounts, func(a DayAccount) bool { return len(a.Models) > 0 || a.Noted() }) {
			shown = append(shown, day)
		}
	}
	return shown
}

// dayAccountOf is the account's day a, priced, as the day table shows it.
func dayAccountOf(a ledger.PricedAccount) DayAccount {
	account := DayAccount{Account: a.Account, Sessions: a.Sessions, MovedOn: a.MovedOn, MovedOff: a.MovedOff}
	for _, m := range a.Models {
		if !used(m.ModelDay) {
			continue
		}
		tokens := m.Tokens()
		model := DayModel{Model: m.Model, Geo: m.InferenceGeo, Requests: m.Requests(), Sessions: m.Sessions,
			Tokens: tokens.Input + tokens.CacheRead + tokens.CacheWrite + tokens.Output}
		model.addModel(m)
		account.Models = append(account.Models, model)
	}
	for _, l := range a.Limits {
		account.Limits = append(account.Limits, WindowAt{Window: l.Window, At: l.At})
	}
	for _, r := range a.Resets {
		if quota.WeekLong(r.Window) {
			account.WeeksReset = append(account.WeeksReset, WindowAt{Window: r.Window, At: r.At})
		}
	}
	for _, key := range slices.SortedFunc(maps.Keys(a.Highest), status.CompareWindows(claude.Policy())) {
		account.Highest = append(account.Highest, WindowUse{Window: key, Use: a.Highest[key]})
	}
	return account
}

// eventful reports whether the day of an account is eventful, worth noting
// though it served no request: it reached a limit, or one of its weeks reset.
func eventful(a DayAccount) bool {
	return len(a.Limits) > 0 || len(a.WeeksReset) > 0
}
