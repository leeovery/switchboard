package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// historyDays is how many days history shows unless --since says otherwise:
// today and those before it.
const historyDays = 30

// historyDocument is what history --json prints: the date of the prices the
// days are priced at, whether the config's prices stand in place of any of
// them, and History's blocks.
type historyDocument struct {
	PricesAsOf       string `json:"prices_as_of"`
	PricesFromConfig bool   `json:"prices_from_config,omitempty"`
	views.History
}

func newHistoryCommand(a *app) *cobra.Command {
	var (
		given   string
		windows bool
		form    formFlags
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show each day of the request ledger, by account and model, with what it was worth",
		Long: `Show each day of the request ledger: the last 30, unless --since says
otherwise, a row for each account and model with its requests, tokens,
sessions and worth, what they'd have cost through the API, at the prices
switchboard carries, or the config's [prices] where it gives them; then of
each account, its sessions, those moved onto it and off it, the limits it
reached, the times its weeks reset, and its windows' highest use: of an
account that served no request that day, only where it reached a limit or
one of its weeks reset. Requests are messages,
whether they went upstream or the router answered them itself: Claude Code's
quota checks and its counts of tokens are never among them. Today's is
summarised from its lines, as far as it has gone. Then, where any of those
days has requests, over them, each account's totals, its plan and its worth
against it, as the dashboard's Accounts sets them side by side; each week's
peaks, each account's own week, from one reset of its week to the next,
placed under the calendar week it began nearest; and the verdicts of
History's Weeks, from replaying each whole week with an account fewer or
more. It reads the ledger's files, so it needs no router.

--since starts at a day, as 2026-10-01, a time today, as 14:00, or how long
ago, as 3h or 2d, the day it falls on.

--windows prints, in place of the days, each account's windows over those
days: a line each time a window's use, its reset or its status changed, as
the readings history holds it.

On a terminal, history prints its days as text. Anywhere else, as in a pipe
or an agent's shell, it prints as JSON the summaries of the days asked for, as
the ledger holds them, from the first it holds, so the last is today's, each
model's worth in US dollars added, and what it leaves unpriced, with each
day's worth by family; then, over those days, the year's grid of requests a
day, each month's, each week's peaks and its tokens, each account's totals,
its plan and its worth against it, and the capacity Weeks gives its verdicts
from, for an agent or a script to read; or, with --windows, each account's
windows' readings. --json prints the JSON, and --pretty the text, wherever
stdout is.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			asJSON := a.printsJSON(form, out)
			if windows {
				return a.historyWindows(out, given, asJSON)
			}
			return a.history(out, given, asJSON)
		},
	}
	cmd.Flags().StringVar(&given, "since", "", "start on the day of `WHEN`: "+sinceForms+" (default the last 30 days)")
	cmd.Flags().BoolVar(&windows, "windows", false, "print each account's windows' readings over those days, in place of the days")
	form.add(cmd)
	return cmd
}

// historyAsked is what history reads: the config, the state directory, and
// the days asked for, from from's to now's.
type historyAsked struct {
	cfg       *config.Config
	stateDir  string
	from, now time.Time
}

// askHistory returns what history reads, the days asked for from the one
// given on, else the last 30.
func (a *app) askHistory(given string) (historyAsked, error) {
	cfg, dir, err := a.ledgerState()
	if err != nil {
		return historyAsked{}, err
	}
	now := a.Now()
	from, err := sinceOr(given, now, startOfDay(now, historyDays-1))
	if err != nil {
		return historyAsked{}, err
	}
	return historyAsked{cfg: cfg, stateDir: dir, from: from, now: now}, nil
}

// history prints the ledger's days from the one given on, else the last 30,
// priced at today's prices, and the blocks over them. It reads the days once,
// through a reader, which reads the readings history ahead once for each run
// of days it summarises, by the time it was asked at, so a run across
// midnight places every day against one today.
func (a *app) history(out io.Writer, given string, asJSON bool) error {
	asked, err := a.askHistory(given)
	if err != nil {
		return err
	}
	prices := pricing(asked.cfg)
	reader := ledgerReader(asked.stateDir, asked.cfg, func() time.Time { return asked.now })
	history := views.NewHistory(views.HistoryInput{
		Ledger: reader, From: asked.from, Now: asked.now, Prices: prices, Accounts: asked.cfg.Accounts,
		WeekStarts: asked.cfg.WeekStarts, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	})
	if asJSON {
		return writeJSON(out, historyDocument{PricesAsOf: prices.AsOf, PricesFromConfig: prices.Overridden, History: history})
	}
	return writeHistory(out, history, prices, asked.from, asked.now)
}

// historyWindows prints each account's windows' readings over the days from
// the one given on, else the last 30.
func (a *app) historyWindows(out io.Writer, given string, asJSON bool) error {
	asked, err := a.askHistory(given)
	if err != nil {
		return err
	}
	windows := views.NewWindows(readings.NewReader(asked.stateDir, logger), asked.cfg.Accounts, asked.from, asked.now)
	if asJSON {
		return writeJSON(out, windows)
	}
	return writeWindows(out, windows, asked.from, asked.now)
}

// writeHistory writes history as its text: a table a day of its days with
// requests or notes, then the blocks over them, then which prices they're
// worth at; but where none of its days has requests, that there were none
// since from, then the tables of those with notes alone, as the blocks and
// prices tell of nothing. Given a day whose date isn't one, it fails,
// writing nothing.
func writeHistory(out io.Writer, history views.History, prices ledger.Table, from, now time.Time) error {
	tables, err := historyTables(history.Days, now)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(tables, func(t dayTable) bool { return len(t.rows) > 0 }) {
		if _, err := fmt.Fprintf(out, "no requests since %s\n", from.In(now.Location()).Format(dayLayout)); err != nil {
			return err
		}
		return writeTables(out, tables, now, false)
	}
	at, err := pricedAt(prices)
	if err != nil {
		return err
	}
	if err := writeTables(out, tables, now, true); err != nil {
		return err
	}
	if err := writeBlocks(out, history, prices); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "\nworth is what they'd have cost through the API, %s\n", at)
	return err
}

// writeTables writes the tables, each after a blank line but the first,
// where first says it's first.
func writeTables(out io.Writer, tables []dayTable, now time.Time, first bool) error {
	for i, table := range tables {
		if err := table.write(out, now, first && i == 0); err != nil {
			return err
		}
	}
	return nil
}

// pricedAt says which prices worth is at, as "at its prices as of 7 Oct
// 2026", and the config's, where any of them stand in place of the table's.
// It fails for a table whose date isn't one.
func pricedAt(prices ledger.Table) (string, error) {
	asOf, _, ok := dayfile.Day(prices.AsOf)
	if !ok {
		return "", fmt.Errorf("the prices are as of %q, which isn't a date", prices.AsOf)
	}
	at := "at its prices as of " + asOf.Format("2 Jan 2006")
	if prices.Overridden {
		at += " and the config's"
	}
	return at, nil
}

// historyTables are the tables of those of days with requests or notes, as
// historyTable makes each: a day of no accounts is passed over, its date
// unread.
func historyTables(days []views.Day, now time.Time) ([]dayTable, error) {
	var tables []dayTable
	for _, day := range days {
		if len(day.Accounts) == 0 {
			continue
		}
		table, err := historyTable(day.Priced, now)
		if err != nil {
			return nil, err
		}
		if len(table.rows) > 0 || len(table.notes) > 0 {
			tables = append(tables, table)
		}
	}
	return tables, nil
}

// historyTable is the day's table: each account's rows, as modelRows makes
// them, and a note of each account's, as accountNote says; but of one that
// served no request, a note only where its day eventful reports, as its
// highest use alone says little, so a day's idle accounts don't crowd its
// table. It fails for a day whose date isn't one, which the ledger's reader
// never gives.
func historyTable(day ledger.Priced, now time.Time) (dayTable, error) {
	start, _, ok := dayfile.Day(day.Day)
	if !ok {
		return dayTable{}, fmt.Errorf("a summary in the ledger is of %q, which isn't a date", redact.Text(day.Day))
	}
	table := dayTable{day: start}
	for _, a := range day.Accounts {
		rows := modelRows(a)
		table.rows = append(table.rows, rows...)
		if len(rows) == 0 && !eventful(a) {
			continue
		}
		if note := accountNote(a, now); note != "" {
			table.notes = append(table.notes, note)
		}
	}
	return table, nil
}

// modelRows are the rows of an account's day: one for each of its models
// with requests, the account named on the first. A model's quota checks and
// counts of tokens alone, which aren't requests, make no row.
func modelRows(a ledger.PricedAccount) [][]string {
	var rows [][]string
	for _, m := range a.Models {
		if m.Requests() == 0 {
			continue
		}
		account := ""
		if len(rows) == 0 {
			account = accountName(a.Account)
		}
		tokens := m.Tokens()
		rows = append(rows, []string{account, modelName(m.ModelDay), requestCount(m.ModelDay),
			prose.Tokens(tokens.Input+tokens.CacheRead+tokens.CacheWrite+tokens.Output) + " tokens", prose.SessionCount(m.Sessions), worthOf(m)})
	}
	return rows
}

// eventful reports whether the day of an account that served no request is
// worth a note: it reached a limit, or one of its weeks reset, as weekResets
// says.
func eventful(a ledger.PricedAccount) bool {
	return len(a.Limits) > 0 || len(weekResets(a)) > 0
}

// weekResets are the resets of an account's weeks, its windows a week long,
// as quota.WeekLong says, its own week, or a model's, in the order they came.
func weekResets(a ledger.PricedAccount) []ledger.Reset {
	return slices.DeleteFunc(slices.Clone(a.Resets), func(r ledger.Reset) bool { return !quota.WeekLong(r.Window) })
}

// accountName names the account with the given id, cleaned: "no account" for
// the requests none answered.
func accountName(id string) string {
	if id == "" {
		return "no account"
	}
	return prose.Clean(id)
}

// modelName names the model a model's day is of, and the inference geo its
// requests asked for, where they asked, as "claude-opus-5-5 (us)", cleaned: a
// dash for the requests whose bodies couldn't be read.
func modelName(m ledger.ModelDay) string {
	name := orNone(prose.Clean(m.Model))
	if m.InferenceGeo != "" {
		name += " (" + prose.Clean(m.InferenceGeo) + ")"
	}
	return name
}

// requestCount counts a model's day's requests, its messages, as "412
// requests".
func requestCount(m ledger.ModelDay) string {
	return prose.Counted(m.Requests(), "request")
}

// worthOf says what a model's day was worth, to the cent, as "$412.53", and
// that part of it is unpriced, where it is: "unpriced" for a model the price
// table doesn't know.
func worthOf(m ledger.PricedModel) string {
	switch {
	case m.Worth == nil:
		return "unpriced"
	case len(m.Unpriced) > 0:
		return m.Worth.Cents() + ", part unpriced"
	}
	return m.Worth.Cents()
}

// accountNote says, of an account's day, its sessions, those moved onto it and
// off it, the limits it reached, the resets of its weeks, and each window's
// highest use, times in now's time zone, as "work: 6 sessions  ·  2 moved on,
// 1 off  ·  limits: Session at 14:37  ·  week reset 10:00  ·  highest:
// Session 100%, Week 41%": "" for the requests no account answered, and for
// an account's day with nothing to say, of no sessions and none of the rest.
func accountNote(a ledger.PricedAccount, now time.Time) string {
	if a.Account == "" {
		return ""
	}
	parts := []string{prose.SessionCount(a.Sessions)}
	for _, part := range []string{moves(a.MovedOn, a.MovedOff), limitsOf(a, now), weeksResetOf(a, now), highestOf(a)} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if a.Sessions == 0 && len(parts) == 1 {
		return ""
	}
	return prose.Clean(a.Account) + ": " + strings.Join(parts, prose.Separator)
}

// limitsOf says the limits an account reached, as "limits: Session at
// 14:37", times in now's time zone: "" for none.
func limitsOf(a ledger.PricedAccount, now time.Time) string {
	if len(a.Limits) == 0 {
		return ""
	}
	limits := make([]string, len(a.Limits))
	for i, l := range a.Limits {
		limits[i] = claude.WindowLabel(l.Window) + " at " + prose.TimeOfDay(now, l.At)
	}
	return "limits: " + prose.Clean(prose.List(limits))
}

// weeksResetOf says when an account's weeks reset, as weekResets gives them,
// each window named as events name it, as "week reset 10:00", times in now's
// time zone: "" for none.
func weeksResetOf(a ledger.PricedAccount, now time.Time) string {
	resets := weekResets(a)
	said := make([]string, len(resets))
	for i, r := range resets {
		said[i] = status.InProse(status.NameOfKey(r.Window)) + " reset " + prose.TimeOfDay(now, r.At)
	}
	return prose.Clean(prose.List(said))
}

// highestOf says each of an account's windows' highest use, as "highest:
// Session 100%, Week 41%": "" for none.
func highestOf(a ledger.PricedAccount) string {
	if len(a.Highest) == 0 {
		return ""
	}
	var highest []string
	for _, key := range slices.Sorted(maps.Keys(a.Highest)) {
		highest = append(highest, claude.WindowLabel(key)+" "+prose.Percent(a.Highest[key]))
	}
	return "highest: " + prose.Clean(strings.Join(highest, ", "))
}

// moves says how many sessions were moved onto an account and off it, as "2
// moved on, 1 off", "2 moved on" or "1 moved off": "" for none.
func moves(on, off int) string {
	switch {
	case on > 0 && off > 0:
		return fmt.Sprintf("%d moved on, %d off", on, off)
	case on > 0:
		return fmt.Sprintf("%d moved on", on)
	case off > 0:
		return fmt.Sprintf("%d moved off", off)
	}
	return ""
}
