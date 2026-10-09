package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
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
		{label: "requests", of: func(t views.AccountTotals, _ views.PlanCost) string { return strconv.Itoa(t.Requests) }},
		{label: "sessions served", of: func(t views.AccountTotals, _ views.PlanCost) string { return strconv.Itoa(t.Sessions) }},
		{label: "sessions moved", of: func(t views.AccountTotals, _ views.PlanCost) string { return movedText(t) }},
		{label: "limits hit", of: func(t views.AccountTotals, _ views.PlanCost) string { return limitsText(t) }},
		{label: "at cap or limit", of: func(t views.AccountTotals, _ views.PlanCost) string { return minutesText(t) }},
		{label: "left at its last reset", of: func(t views.AccountTotals, _ views.PlanCost) string { return leftText(t.LeftAtReset) }},
		{label: "plan", of: func(_ views.AccountTotals, p views.PlanCost) string { return planText(p, plans, prices) }},
		{label: "worth", of: func(t views.AccountTotals, _ views.PlanCost) string { return worthText(t.Worth) }},
		{label: "against its price", of: func(_ views.AccountTotals, p views.PlanCost) string { return againstText(p.Against) }},
		{label: "busiest day", of: func(t views.AccountTotals, _ views.PlanCost) string { return dayText(t.BusiestDay) }},
		{label: "longest run", of: func(t views.AccountTotals, _ views.PlanCost) string { return runText(t.LongestRun) }},
	}
	head := []string{"Over these days"}
	for _, t := range totals.Accounts {
		head = append(head, entryName(t.Account, t.All))
	}
	rows := [][]string{head}
	for _, f := range figures {
		row := []string{"  " + f.label}
		for i, t := range totals.Accounts {
			row = append(row, f.of(t, plans[i]))
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
	return status.Clean(account)
}

// byModelRows are the rows of each model version every account used, in the
// version table's order, its share of each one's use.
func byModelRows(totals views.Totals, prices ledger.Table) [][]string {
	all := totals.Accounts[len(totals.Accounts)-1].ByModel
	if len(all) == 0 {
		return nil
	}
	// The heading's empty cells keep the versions' shares in the columns
	// above them.
	rows := [][]string{append([]string{"  by model"}, make([]string, len(totals.Accounts))...)}
	for _, m := range all {
		row := []string{"    " + status.Clean(prices.VersionName(m.Model))}
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
	return status.Percent(*shares[i].Share)
}

// movedText says how many sessions were moved onto an account and off it, as
// "2 in · 1 out", or for every account, how many moves there were, as "4
// moves".
func movedText(t views.AccountTotals) string {
	if t.All {
		return countOf(t.MovedOn, "move", "moves")
	}
	var moved []string
	if t.MovedOn > 0 {
		moved = append(moved, fmt.Sprintf("%d in", t.MovedOn))
	}
	if t.MovedOff > 0 {
		moved = append(moved, fmt.Sprintf("%d out", t.MovedOff))
	}
	if len(moved) == 0 {
		return "none"
	}
	return strings.Join(moved, " · ")
}

// countOf counts n of something, as "1 move" or "4 moves": "none" for none.
func countOf(n int, one, many string) string {
	switch n {
	case 0:
		return "none"
	case 1:
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// limitsText says the limits an account reached, by window, as "Session ×2 ·
// Week ×1", or for every account, how many, as "3".
func limitsText(t views.AccountTotals) string {
	if len(t.Limits) == 0 {
		return "none"
	}
	keys := slices.SortedFunc(maps.Keys(t.Limits), quota.CompareKeys)
	if t.All {
		n := 0
		for _, key := range keys {
			n += t.Limits[key]
		}
		return strconv.Itoa(n)
	}
	limits := make([]string, len(keys))
	for i, key := range keys {
		limits[i] = fmt.Sprintf("%s ×%d", status.Clean(claude.WindowLabel(key)), t.Limits[key])
	}
	return strings.Join(limits, " · ")
}

// minutesText says how long an account spent at its cap or at a limit, as
// "3h 10m", and that some of its days never read it, where they didn't: a
// dash where none did.
func minutesText(t views.AccountTotals) string {
	if t.MinutesAtCap == nil || t.MinutesAtLimit == nil {
		return dash
	}
	spent := "none"
	if minutes := *t.MinutesAtCap + *t.MinutesAtLimit; minutes > 0 {
		spent = status.Countdown(time.Time{}, time.Time{}.Add(time.Duration(minutes)*time.Minute))
	}
	if len(t.Partial) > 0 {
		spent += ", part not read"
	}
	return spent
}

// leftText says what each window had left at its last reset, as "Session 0%
// · Week 59%": a dash where none reset.
func leftText(left map[string]float64) string {
	if len(left) == 0 {
		return dash
	}
	keys := slices.SortedFunc(maps.Keys(left), quota.CompareKeys)
	shares := make([]string, len(keys))
	for i, key := range keys {
		shares[i] = status.Clean(claude.WindowLabel(key)) + " " + status.Percent(left[key])
	}
	return strings.Join(shares, " · ")
}

// planText says an account's plan and its price a month, as "Max 20x ·
// $200/mo", or of every account's, the plans' prices summed and how many of
// the accounts, of plans, are on one known, as "$220/mo · 2 of 3 plans": a
// dash where none is known.
func planText(p views.PlanCost, plans []views.PlanCost, prices ledger.Table) string {
	if p.Price == nil {
		return dash
	}
	monthly := p.Price.Dollars() + "/mo"
	if !p.All {
		plan, _ := prices.Plan(p.Plan)
		return status.Clean(plan.Name) + " · " + monthly
	}
	known := 0
	for _, each := range plans {
		if !each.All && each.Plan != "" {
			known++
		}
	}
	if accounts := len(plans) - 1; known < accounts {
		return fmt.Sprintf("%s · %d of %d plans", monthly, known, accounts)
	}
	return fmt.Sprintf("%s · %d plans", monthly, known)
}

// worthText says what requests were worth, to the cent, and that part of it
// is unpriced, where it is, as "$184.20, part unpriced": "unpriced" where
// none of it is priced, and a dash where there's none.
func worthText(w views.Worth) string {
	switch {
	case w.Amount == nil && len(w.Unpriced) > 0:
		return "unpriced"
	case w.Amount == nil:
		return dash
	case len(w.Unpriced) > 0:
		return w.Amount.Cents() + ", part unpriced"
	}
	return w.Amount.Cents()
}

// againstText says the worth against what the plans cost, as "4.0× its
// $46": a dash where no plan is known, or it cost nothing.
func againstText(a views.Against) string {
	if a.Ratio == nil || a.Cost == nil {
		return dash
	}
	return fmt.Sprintf("%.1f× its %s", *a.Ratio, a.Cost.Dollars())
}

// dayText names the local day with the given date, as "Tue 22 Sep": a dash
// for none.
func dayText(date string) string {
	start, _, ok := dayfile.Day(date)
	if !ok {
		return dash
	}
	return start.Format("Mon 2 Jan")
}

// runText says how many days in a row there were, as "58 days": a dash for
// none.
func runText(days int) string {
	if days == 0 {
		return dash
	}
	return countOf(days, "day", "days")
}

// writeWeeks writes each week's peaks, a column a week, by its first day, a
// row an account's week, then one of each model's own week it has, every
// account's last, this week's so far marked "…".
func writeWeeks(out io.Writer, weeks []views.Week) error {
	if len(weeks) == 0 {
		return nil
	}
	head := []string{"Weeks at their peaks"}
	for _, w := range weeks {
		head = append(head, dayFirst(w.Week))
	}
	rows := [][]string{head}
	for i, entry := range weeks[0].Accounts {
		for _, key := range peakKeys(weeks, i) {
			label := "  " + entryName(entry.Account, entry.All)
			if key != claude.WeekWindow {
				label = "    " + status.Clean(claude.WindowLabel(key))
			}
			row := []string{label}
			for j, w := range weeks {
				row = append(row, peakText(w.Accounts[i].Peaks, key, j == len(weeks)-1))
			}
			rows = append(rows, row)
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	return writeTable(out, rows)
}

// peakKeys are the keys of the windows any of weeks gives a peak of for its
// accounts' entry with the index given, in quota's order: the account's
// week's at the least.
func peakKeys(weeks []views.Week, i int) []string {
	keys := map[string]bool{claude.WeekWindow: true}
	for _, w := range weeks {
		for key := range w.Accounts[i].Peaks {
			keys[key] = true
		}
	}
	return slices.SortedFunc(maps.Keys(keys), quota.CompareKeys)
}

// peakText says a week's peak of the window with the given key among peaks,
// as "60%", or "25%…" for this week's so far: a dash where it's not known.
func peakText(peaks map[string]float64, key string, soFar bool) string {
	peak, ok := peaks[key]
	switch {
	case !ok:
		return dash
	case soFar:
		return status.Percent(peak) + "…"
	}
	return status.Percent(peak)
}

// dayFirst names the local day with the given date by its day and month, as
// "14 Sep".
func dayFirst(date string) string {
	start, _, ok := dayfile.Day(date)
	if !ok {
		return status.Clean(date)
	}
	return start.Format("2 Jan")
}

// writeCapacity writes Weeks' verdicts: the headline; how many weeks were
// short as the accounts stand, with one fewer and with one more; each
// account's verdict, every account's last; and the one to drop, where
// there's one; or that there are no whole weeks to judge yet.
func writeCapacity(out io.Writer, c views.Capacity) error {
	if c.Weeks == 0 {
		_, err := fmt.Fprintln(out, "\nWeeks: not enough weeks yet")
		return err
	}
	lines := []string{
		"", fmt.Sprintf("Weeks: %s is %s", prose.Number(len(c.Accounts)-1), c.Verdict),
		"  " + replayText(c),
	}
	for _, v := range c.Accounts {
		lines = append(lines, "  "+verdictText(v))
	}
	if c.Drop != "" {
		lines = append(lines, "  "+status.Clean(c.Drop)+" is the one to drop")
	}
	lines = append(lines, "  an estimate, from replaying these weeks with one account fewer or more")
	_, err := fmt.Fprintln(out, strings.Join(lines, "\n"))
	return err
}

// replayText says how many weeks were short as the accounts stand, with one
// fewer, where there's more than one, and with one more, as "as now: short
// on 3 of 7 weeks  ·  with one fewer: short on 6  ·  with one more: short on
// none"; or, where one can be dropped, with two fewer in one more's place.
func replayText(c views.Capacity) string {
	replays := []string{fmt.Sprintf("as now: short on %s of %d weeks", shortCount(c.Short), c.Weeks)}
	if c.WithOneFewer != nil {
		replays = append(replays, "with one fewer: short on "+shortCount(*c.WithOneFewer))
	}
	if c.WithTwoFewer != nil {
		replays = append(replays, "with two fewer: short on "+shortCount(*c.WithTwoFewer))
	} else {
		replays = append(replays, "with one more: short on "+shortCount(c.WithOneMore))
	}
	return strings.Join(replays, status.Separator)
}

// shortCount counts weeks short, as "3", or "none".
func shortCount(n int) string {
	if n == 0 {
		return "none"
	}
	return strconv.Itoa(n)
}

// verdictText says Weeks' verdict on an account, as "work: hits its limit
// most weeks  ·  limit hit 3 of 7 weeks  ·  9% left at a reset, on average
// ·  without it, the rest: short on 6", or Accounts' on every account.
func verdictText(v views.AccountVerdict) string {
	name := entryName(v.Account, v.All) + ": "
	if v.Verdict == "" || v.LeftAtReset == nil {
		return name + "no week known"
	}
	parts := []string{v.Verdict}
	if !v.All {
		parts = append(parts, fmt.Sprintf("limit hit %d of %d weeks", v.Limits, v.Weeks))
	}
	parts = append(parts, status.Percent(*v.LeftAtReset)+" left at a reset, on average")
	if v.WithoutIt != nil {
		parts = append(parts, "without it, the rest: short on "+shortCount(*v.WithoutIt))
	}
	return name + strings.Join(parts, status.Separator)
}
