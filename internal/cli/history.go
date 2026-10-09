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
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

// historyDays is how many days history shows unless --since says otherwise:
// today and those before it.
const historyDays = 30

// historyDocument is what history --json prints: the date of the prices the
// days are priced at, whether the config's prices stand in place of any of
// them, and the days.
type historyDocument struct {
	PricesAsOf       string          `json:"prices_as_of"`
	PricesFromConfig bool            `json:"prices_from_config,omitempty"`
	Days             []ledger.Priced `json:"days"`
}

func newHistoryCommand(a *app) *cobra.Command {
	var (
		given  string
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show each day of the request ledger, by account and model, with what it was worth",
		Long: `Show each day of the request ledger: the last 30, unless --since says
otherwise, a row for each account and model with its requests, tokens,
sessions and worth, what they'd have cost through the API, at the prices
switchboard carries; then of each account, its sessions, those moved onto it
and off it, the limits it reached, and its windows' highest use. Today's is
summarised from its lines, as far as it has gone. It reads the ledger's files,
so it needs no router.

--since starts at a day, as 2026-10-01, a time today, as 14:00, or how long
ago, as 3h or 2d, the day it falls on.

With --json, print the summaries of the days asked for, as the ledger holds
them, from the first it holds, so the last is today's, each model's worth in
US dollars added, and what it leaves unpriced, for an agent or a script to
read.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.history(cmd.OutOrStdout(), given, asJSON)
		},
	}
	cmd.Flags().StringVar(&given, "since", "", "start on the day of `WHEN`: "+sinceForms+" (default the last 30 days)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the days as JSON")
	return cmd
}

// history prints the ledger's days from the one given on, else the last 30,
// priced at today's prices.
func (a *app) history(out io.Writer, given string, asJSON bool) error {
	reader, cfg, err := a.ledgerReader()
	if err != nil {
		return err
	}
	prices := pricing(cfg)
	now := a.Now()
	from, err := sinceOr(given, now, startOfDay(now, historyDays-1))
	if err != nil {
		return err
	}
	today := now.Local().Format(time.DateOnly)
	days := []ledger.Priced{}
	for _, summary := range reader.Days(from) {
		days = append(days, prices.Priced(summary, today))
	}
	if asJSON {
		return writeJSON(out, historyDocument{PricesAsOf: prices.AsOf, PricesFromConfig: prices.Overridden, Days: days})
	}
	return writeHistory(out, days, prices, from, now)
}

// writeHistory writes days as history's text, a table a day of those with
// requests, then which prices they're worth at; or says there were none since
// from. Given a day whose date isn't one, it fails, writing nothing.
func writeHistory(out io.Writer, days []ledger.Priced, prices ledger.Table, from, now time.Time) error {
	tables, err := historyTables(days, now)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		_, err := fmt.Fprintf(out, "no requests since %s\n", from.In(now.Location()).Format(dayLayout))
		return err
	}
	at, err := pricedAt(prices)
	if err != nil {
		return err
	}
	for i, table := range tables {
		if err := table.write(out, now, i == 0); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "\nworth is what they'd have cost through the API, %s\n", at)
	return err
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

// historyTables are the tables of those of days with requests, as
// historyTable makes each.
func historyTables(days []ledger.Priced, now time.Time) ([]dayTable, error) {
	var tables []dayTable
	for _, day := range days {
		if len(day.Accounts) == 0 {
			continue
		}
		table, err := historyTable(day, now)
		if err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

// historyTable is the day's table: a row for each account's model, the
// account named on its first, and a note of each account's. It fails for a
// day whose date isn't one, which the ledger's reader never gives.
func historyTable(day ledger.Priced, now time.Time) (dayTable, error) {
	start, _, ok := dayfile.Day(day.Day)
	if !ok {
		return dayTable{}, fmt.Errorf("a summary in the ledger is of %q, which isn't a date", redact.Text(day.Day))
	}
	table := dayTable{day: start}
	for _, a := range day.Accounts {
		for i, m := range a.Models {
			account := ""
			if i == 0 {
				account = accountName(a.Account)
			}
			tokens := m.Tokens()
			table.rows = append(table.rows, []string{account, modelName(m.ModelDay), requestCount(m.ModelDay),
				prose.Count(tokensIn(tokens)+tokens.Output) + " tokens", status.SessionCount(m.Sessions), worthOf(m)})
		}
		if note := accountNote(a, now); note != "" {
			table.notes = append(table.notes, note)
		}
	}
	return table, nil
}

// accountName names the account with the given id, cleaned: "no account" for
// the requests none answered.
func accountName(id string) string {
	if id == "" {
		return "no account"
	}
	return status.Clean(id)
}

// modelName names the model a model's day is of, and the inference geo its
// requests asked for, where they asked, as "claude-opus-5-5 (us)", cleaned: a
// dash for the requests whose bodies couldn't be read.
func modelName(m ledger.ModelDay) string {
	name := orNone(status.Clean(m.Model))
	if m.InferenceGeo != "" {
		name += " (" + status.Clean(m.InferenceGeo) + ")"
	}
	return name
}

// requestCount counts a model's day's requests, however each went, as "412
// requests".
func requestCount(m ledger.ModelDay) string {
	n := m.Upstream + m.Unsent + m.Checks + m.Counts
	if n == 1 {
		return "1 request"
	}
	return fmt.Sprintf("%d requests", n)
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
// off it, the limits it reached, and each window's highest use, times in now's
// time zone, as "work: 6 sessions  ·  2 moved on, 1 off  ·  limits: Session at
// 14:37  ·  highest: Session 100%, Week 41%": "" for the requests no account
// answered.
func accountNote(a ledger.PricedAccount, now time.Time) string {
	if a.Account == "" {
		return ""
	}
	parts := []string{status.SessionCount(a.Sessions)}
	if moved := moves(a.MovedOn, a.MovedOff); moved != "" {
		parts = append(parts, moved)
	}
	if len(a.Limits) > 0 {
		limits := make([]string, len(a.Limits))
		for i, l := range a.Limits {
			limits[i] = claude.WindowLabel(l.Window) + " at " + status.TimeOfDay(now, l.At)
		}
		parts = append(parts, "limits: "+status.Clean(prose.List(limits)))
	}
	if len(a.Highest) > 0 {
		var highest []string
		for _, key := range slices.Sorted(maps.Keys(a.Highest)) {
			highest = append(highest, claude.WindowLabel(key)+" "+status.Percent(a.Highest[key]))
		}
		parts = append(parts, "highest: "+status.Clean(strings.Join(highest, ", ")))
	}
	return status.Clean(a.Account) + ": " + strings.Join(parts, status.Separator)
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
