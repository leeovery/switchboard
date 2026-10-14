package cli

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// writeBlocks writes, under history's days, the totals over them, each
// account's plan and its worth against it, as Accounts' Compare sets them
// side by side; each week's peaks; and Weeks' verdicts.
func writeBlocks(out io.Writer, history views.History, prices ledger.Table) error {
	if err := writeTotals(out, history.Totals, history.Plans, prices); err != nil {
		return err
	}
	if err := writeWeeks(out, history.Weeks); err != nil {
		return err
	}
	return writeCapacity(out, history.Capacity)
}

// figure is a row of the totals: its label, and what it says of an account,
// or of every account, its totals and its plan given.
type figure struct {
	label string
	of    func(t views.AccountTotals, p views.PlanCost) string
}

// writeTotals writes the totals and the plans, each account's a column, in
// the config's order, every account's last, a row a figure, then a row a
// model version, its share of each one's use.
func writeTotals(out io.Writer, totals views.Totals, plans []views.PlanCost, prices ledger.Table) error {
	figures := []figure{
		{label: "requests", of: func(t views.AccountTotals, _ views.PlanCost) string { return prose.Grouped(t.Requests) }},
		{label: "sessions served", of: func(t views.AccountTotals, _ views.PlanCost) string { return prose.Grouped(t.Sessions) }},
		{label: "sessions moved", of: func(t views.AccountTotals, _ views.PlanCost) string { return movedText(t) }},
		{label: "limits hit", of: func(t views.AccountTotals, _ views.PlanCost) string { return limitsText(t) }},
		{label: "at cap or limit", of: func(t views.AccountTotals, _ views.PlanCost) string { return minutesText(t) }},
		{label: "left at its last reset", of: func(t views.AccountTotals, _ views.PlanCost) string { return leftText(t.LeftAtReset) }},
		{label: "plan", of: func(_ views.AccountTotals, p views.PlanCost) string { return planText(p, plans, prices) }},
		{label: "worth", of: func(t views.AccountTotals, _ views.PlanCost) string { return worthText(t.Worth) }},
		{label: "against its price", of: func(_ views.AccountTotals, p views.PlanCost) string { return againstText(p.Against) }},
		{label: "busiest day", of: func(t views.AccountTotals, _ views.PlanCost) string { return orNone(views.DateNamed(t.BusiestDay)) }},
		{label: "longest run", of: func(t views.AccountTotals, _ views.PlanCost) string { return orNone(views.DaysInARow(t.LongestRun)) }},
	}
	if len(totals.Accounts) == 0 {
		return nil
	}
	head := []string{"Over these days"}
	for _, t := range totals.Accounts {
		head = append(head, entryName(t.Account, t.All))
	}
	rows := [][]string{head}
	for _, f := range figures {
		row := []string{"  " + f.label}
		for _, t := range totals.Accounts {
			row = append(row, f.of(t, planOf(plans, t.Account, t.All)))
		}
		rows = append(rows, row)
	}
	rows = append(rows, byModelRows(totals, prices)...)
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	return writeTable(out, rows)
}

// entryName names an account's column or row, cleaned, or every account's.
func entryName(account string, all bool) string {
	if all {
		return "all accounts"
	}
	return prose.Clean(account)
}

// planOf is the plan among plans of the account with the given id, or of
// every account's: none where plans give none.
func planOf(plans []views.PlanCost, account string, all bool) views.PlanCost {
	i := slices.IndexFunc(plans, func(p views.PlanCost) bool { return p.All == all && p.Account == account })
	if i < 0 {
		return views.PlanCost{}
	}
	return plans[i]
}

// byModelRows are the rows of each model version every account used, in the
// version table's order, its share of each one's use: none where the totals
// give no every account's.
func byModelRows(totals views.Totals, prices ledger.Table) [][]string {
	i := slices.IndexFunc(totals.Accounts, func(t views.AccountTotals) bool { return t.All })
	if i < 0 || len(totals.Accounts[i].ByModel) == 0 {
		return nil
	}
	all := totals.Accounts[i].ByModel
	// The heading's empty cells keep the versions' shares in the columns
	// above them.
	rows := [][]string{append([]string{"  by model"}, make([]string, len(totals.Accounts))...)}
	for _, m := range all {
		row := []string{"    " + prose.Clean(prices.VersionName(m.Model))}
		for _, t := range totals.Accounts {
			row = append(row, shareText(t.ByModel, m.Model))
		}
		rows = append(rows, row)
	}
	return rows
}

// shareText says the share of use of the model version with the given id
// among shares, as "55%", "<1%" where it rounds to none, or a dash where it
// has none, or none priced.
func shareText(shares []views.ModelShare, model string) string {
	i := slices.IndexFunc(shares, func(s views.ModelShare) bool { return s.Model == model })
	switch {
	case i < 0 || shares[i].Share == nil:
		return dash
	case *shares[i].Share > 0 && *shares[i].Share < 0.005:
		return "<1%"
	}
	return prose.Percent(*shares[i].Share)
}

// The totals' words: none, of a figure that counts nothing; and what sets the
// parts of a figure apart.
const (
	none  = "none"
	apart = " · "
)

// movedText says how many sessions were moved onto an account and off it, as
// "2 in · 1 out", or for every account, how many moves there were, as "4
// moves".
func movedText(t views.AccountTotals) string {
	if t.All {
		return prose.CountedOr(t.MovedOn, "move", none)
	}
	var moved []string
	if t.MovedOn > 0 {
		moved = append(moved, prose.Grouped(t.MovedOn)+" in")
	}
	if t.MovedOff > 0 {
		moved = append(moved, prose.Grouped(t.MovedOff)+" out")
	}
	return cmp.Or(strings.Join(moved, apart), none)
}

// limitsText says the limits an account reached, as views.LimitsHit says
// them, joined, as "5-hour ×2 · week ×1", or for every account, how many, as
// "3".
func limitsText(t views.AccountTotals) string {
	switch n := t.LimitCount(); {
	case n == 0:
		return none
	case t.All:
		return prose.Grouped(n)
	}
	return strings.Join(t.LimitsHit(status.NameOfKey), apart)
}

// minutesText says how long an account spent at its cap or at a limit, as
// "3h 10m", and that some of its days never read it, where they didn't: a
// dash where none did.
func minutesText(t views.AccountTotals) string {
	minutes, known := t.AtCapOrLimit()
	if !known {
		return dash
	}
	spent := none
	if minutes > 0 {
		spent = prose.Duration(time.Duration(minutes) * time.Minute)
	}
	if len(t.Partial) > 0 {
		spent += ", part not read"
	}
	return spent
}

// leftText says what each window had left at its last reset, each named as
// windowInProse names it, as "5-hour 0% · week 59%": a dash where none reset.
func leftText(left map[string]float64) string {
	if len(left) == 0 {
		return dash
	}
	keys := slices.SortedFunc(maps.Keys(left), quota.CompareKeys)
	shares := make([]string, len(keys))
	for i, key := range keys {
		shares[i] = windowInProse(key) + " " + prose.Percent(left[key])
	}
	return strings.Join(shares, apart)
}

// planText says an account's plan and its price a month, as "Max 20x ·
// $200/mo", or of every account's, the plans' prices summed and how many of
// the accounts, of plans, are on one known, as "$220/mo · 2 of 3 plans": a
// dash where none is known.
func planText(p views.PlanCost, plans []views.PlanCost, prices ledger.Table) string {
	monthly := p.Monthly()
	switch {
	case monthly == "":
		return dash
	case !p.All:
		plan, _ := prices.Plan(p.Plan)
		return prose.Clean(plan.Name) + apart + monthly
	}
	known, accounts := 0, 0
	for _, each := range plans {
		if !each.All {
			accounts++
		}
		if !each.All && each.Plan != "" {
			known++
		}
	}
	if known < accounts {
		return monthly + apart + prose.Grouped(known) + " of " + prose.Counted(accounts, "plan")
	}
	return monthly + apart + prose.Counted(known, "plan")
}

// worthText says what requests were worth, as views.Worth says it to the
// cent, as "$184.20" or "$184.20+": a dash where there's none.
func worthText(w views.Worth) string {
	if w.Amount == nil && len(w.Unpriced) == 0 {
		return dash
	}
	return w.Cents()
}

// againstText says the worth against what the plans cost, as "4.0× its
// $46": a dash where no plan is known, or it cost nothing.
func againstText(a views.Against) string {
	if a.Ratio == nil || a.Cost == nil {
		return dash
	}
	return prose.Ratio(*a.Ratio) + " its " + a.Cost.GroupedDollars()
}

// writeWeeks writes each week's peaks, a column a week, by its first day, a
// row an account's week, then one of each model's own week it has, every
// account's last, the peak so far of a week still running marked "…".
func writeWeeks(out io.Writer, weeks []views.Week) error {
	if len(weeks) == 0 {
		return nil
	}
	head := []string{"Weeks at their peaks"}
	for _, w := range weeks {
		head = append(head, dayFirst(w.Week))
	}
	rows := [][]string{head}
	for _, entry := range weeks[0].Accounts {
		for _, key := range peakKeys(weeks, entry) {
			label := "  " + entryName(entry.Account, entry.All)
			if key != claude.WeekWindow {
				label = "    " + status.NameOfKey(key)
			}
			row := []string{label}
			for _, w := range weeks {
				row = append(row, peakText(entryOf(w, entry), key))
			}
			rows = append(rows, row)
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	return writeTable(out, rows)
}

// entryOf is the account's entry in the week that's of the same account as
// the one given, or of every account: none where the week gives none.
func entryOf(week views.Week, entry views.AccountWeek) views.AccountWeek {
	i := slices.IndexFunc(week.Accounts, func(a views.AccountWeek) bool { return a.All == entry.All && a.Account == entry.Account })
	if i < 0 {
		return views.AccountWeek{}
	}
	return week.Accounts[i]
}

// peakKeys are the keys of the windows any of weeks gives a peak of for the
// account entry is of, in quota's order: the account's week's at the least.
func peakKeys(weeks []views.Week, entry views.AccountWeek) []string {
	keys := map[string]bool{claude.WeekWindow: true}
	for _, w := range weeks {
		for key := range entryOf(w, entry).Peaks {
			keys[key] = true
		}
	}
	return slices.SortedFunc(maps.Keys(keys), quota.CompareKeys)
}

// peakText says an account's week's peak of the window with the given key,
// as views.Peak says it, as "60%", or "25%…" so far, where its week is still
// running: a dash where it's not known.
func peakText(entry views.AccountWeek, key string) string {
	peak, ok := entry.Peaks[key]
	if !ok {
		return dash
	}
	return views.Peak(peak, slices.Contains(entry.Open, key))
}

// dayFirst names the day with the given date by its day and month, as "14
// Sep".
func dayFirst(date string) string {
	day, ok := dayfile.Civil(date)
	if !ok {
		return prose.Clean(date)
	}
	return prose.DayMonth(day)
}

// writeCapacity writes Weeks' verdicts: the headline, as views.Headline says
// it; how many weeks were short as the accounts stand, with one fewer and
// with one more; each account's verdict, every account's last; and the one
// to drop, where there's one; or that there are no whole weeks to judge yet.
func writeCapacity(out io.Writer, c views.Capacity) error {
	if c.Weeks == 0 {
		_, err := fmt.Fprintln(out, "\nWeeks: "+views.NotEnoughWeeks)
		return err
	}
	accounts := 0
	for _, v := range c.Accounts {
		if !v.All {
			accounts++
		}
	}
	lines := []string{"", "Weeks: " + views.Headline(accounts, c.Verdict).String(), "  " + replayText(c)}
	for _, v := range c.Accounts {
		lines = append(lines, "  "+verdictText(v))
	}
	if c.Drop != "" {
		lines = append(lines, "  "+views.ToDrop(prose.Clean(c.Drop)))
	}
	lines = append(lines, "  "+views.EstimateOfOneFewerOrMore)
	_, err := fmt.Fprintln(out, strings.Join(lines, "\n"))
	return err
}

// replayText says how many weeks were short as the accounts stand, as
// views.ShortOfWeeks says it, with one fewer, where there's more than one,
// and with one more, as views.ShortOn says it, as "as now: short on 3 of 7
// weeks  ·  with one fewer: short on 6  ·  with one more: short on none"; or,
// where one can be dropped, with two fewer in one more's place.
func replayText(c views.Capacity) string {
	replays := []string{views.ReplayAsNow + views.ShortOfWeeks(c.Short, c.Weeks).String()}
	if c.WithOneFewer != nil {
		replays = append(replays, views.ReplayOneFewer+views.ShortOn(*c.WithOneFewer).String())
	}
	if c.WithTwoFewer != nil {
		replays = append(replays, views.ReplayTwoFewer+views.ShortOn(*c.WithTwoFewer).String())
	} else {
		replays = append(replays, views.ReplayOneMore+views.ShortOn(c.WithOneMore).String())
	}
	return strings.Join(replays, prose.Separator)
}

// verdictText says Weeks' verdict on an account, as "work: hits its limit
// most weeks  ·  limit hit 3 of 7 weeks  ·  9% left at a reset, on average
// ·  without it, the rest: short on 6", each phrase as views says it, or
// Accounts' on every account.
func verdictText(v views.AccountVerdict) string {
	name := entryName(v.Account, v.All) + ": "
	if v.Verdict == "" || v.LeftAtReset == nil {
		return name + "no week known"
	}
	parts := []string{v.Verdict}
	if !v.All {
		parts = append(parts, views.LimitHit(v.Limits, v.Weeks).String())
	}
	parts = append(parts, views.LeftOnAverage(*v.LeftAtReset).String())
	if v.WithoutIt != nil {
		parts = append(parts, views.WithoutTheRest+views.ShortOn(*v.WithoutIt).String())
	}
	return name + strings.Join(parts, prose.Separator)
}
