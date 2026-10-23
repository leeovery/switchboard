package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts' words, Compare's and Detail's, each said once here, of a row of
// views.AccountsPage, an account's or every account's: each in the ink
// given, text.secondary, bold where it's every account's in Compare, but
// what's said beside a figure, faint, and a figure Accounts has none of, as
// noneOf has it.

// accountsApart sets the parts of an Accounts figure apart, as "2 in · 1
// out".
const accountsApart = " · "

// partForms are a figure in its forms, the fullest first, then each shorter,
// each keeping its figures whole: only words give way.
type partForms [][]part

// fitting is the first of the forms that fits the cells given, else the
// last, for fitParts to cut.
func (f partForms) fitting(cells int) []part {
	for _, form := range f {
		if partsWidth(form) <= cells {
			return form
		}
	}
	return f.shortest()
}

// shortest is the last of the forms, the fewest cells they come to.
func (f partForms) shortest() []part {
	return f[len(f)-1]
}

// noneOf is a figure Accounts has none of, as the word given says it, "—"
// or "none", in text.faint, bold where k is: words, as no figure, so it's
// never a number, its figure appearing or going brightening nothing.
func noneOf(word string, k ink) []part {
	return []part{wordsIn(word, boldAs(faintInk, k))}
}

// periodNumbered are the parts of a figure of Accounts' period, of the
// account with the id given, "" for all accounts', each figure among them
// numbered as the one the name given names, as NumberPeriod has it: a dash
// or none, words, never.
func periodNumbered(parts []part, id, name string) []part {
	numbered := slices.Clone(parts)
	for i := range numbered {
		numbered[i].number = Number{Kind: NumberPeriod, Account: id, Of: name}
	}
	return numbered
}

// shareNamed is what a version's share of use is numbered by, after its
// name, as "Opus 5.5 share".
func shareNamed(v ledger.Version) string {
	return v.Name + " share"
}

// periodParts say a figure of the page's period as said says it: but where
// the period records nothing, as on a fresh install or a week's first
// morning, a dash, as noneOf has it, whatever said says, as 0 requests, none
// hit, or $0. Left at its last reset, of the six weeks, the plan, of the
// config, and extra usage, live, are no figures of the period.
func periodParts(page views.AccountsPage, said []part, k ink) []part {
	if !page.Recorded {
		return noneOf(noFigure.text, k)
	}
	return said
}

// boldAs is the ink given, bold where k is.
func boldAs(given, k ink) ink {
	given.bold = k.bold
	return given
}

// movedParts say the sessions moved onto an account and off it over the
// period, as Compare's ACTIVITY does: "3 out", "4 in", or both, "2 in · 1
// out"; of every account's, every move, "4 moves"; a dash where none moved.
func movedParts(t views.AccountTotals, k ink) []part {
	if t.All {
		if t.MovedOn == 0 {
			return noneOf(noFigure.text, k)
		}
		return countedParts(t.MovedOn, "move", k)
	}
	var moved []part
	for _, way := range []struct {
		n    int
		says string
	}{{t.MovedOn, " in"}, {t.MovedOff, " out"}} {
		if way.n == 0 {
			continue
		}
		if moved != nil {
			moved = append(moved, wordsIn(accountsApart, k))
		}
		moved = append(moved, figureIn(prose.Grouped(way.n), k), wordsIn(way.says, k))
	}
	if moved == nil {
		return noneOf(noFigure.text, k)
	}
	return moved
}

// movedForms say the sessions moved onto an account and off it over the
// period in its forms: as movedParts says them, "2 in · 1 out", or of every
// account's "4 moves"; then how many moves, "3", the in and out summed, or
// every account's moves, in the ink given, as limitsCount says the limits.
// Where none moved, its one form.
func movedForms(t views.AccountTotals, k ink) partForms {
	full := movedParts(t, k)
	moves := t.MovedOn
	if !t.All {
		moves += t.MovedOff
	}
	if moves == 0 {
		return partForms{full}
	}
	return partForms{full, {figureIn(prose.Grouped(moves), k)}}
}

// limitsParts say the limits an account hit over the period, by window,
// each named as name names its key, as views.LimitsHit says them, in
// state.destructive, a faint " · " between: "5-hour ×2 · week ×1"; of every
// account's, how many, as limitsCount says it; "none" where it hit none.
func limitsParts(t views.AccountTotals, name func(key string) string, k ink) []part {
	if t.LimitCount() == 0 {
		return noneOf("none", k)
	}
	if t.All {
		return limitsCount(t, k)
	}
	var hit []part
	for i, said := range t.LimitsHit(name) {
		if i > 0 {
			hit = append(hit, wordsIn(accountsApart, faintInk))
		}
		hit = append(hit, figureIn(said, errorInk))
	}
	return hit
}

// limitsForms say the limits an account hit over the period in Compare's
// LIMITS, in its forms: as limitsParts says them, each window named as name
// names its key, "5-hour ×2 · week ×1"; then as turned Compare does, as
// limitsShortForms has them. Of every account's, and of an account that hit
// none, its one form.
func limitsForms(t views.AccountTotals, name func(key string) string, k ink) partForms {
	if t.All || t.LimitCount() == 0 {
		return partForms{limitsParts(t, name, k)}
	}
	return append(partForms{limitsParts(t, name, k)}, limitsShortForms(t, name, k)...)
}

// limitsShortForms say the limits an account hit over the period as turned
// Compare does, in its forms: each window named short, as shortWindows has
// it, "5h ×2 · week ×1"; then how many, "3", as limitsCount says it and all
// accounts' does, dropping the windows. Of every account's, and of an
// account that hit none, its one form.
func limitsShortForms(t views.AccountTotals, name func(key string) string, k ink) partForms {
	short := limitsParts(t, shortWindows(name), k)
	if t.All || t.LimitCount() == 0 {
		return partForms{short}
	}
	return partForms{short, limitsCount(t, k)}
}

// limitsCount says how many limits an account hit over the period, every
// window's together, "3", in the ink given, as every account's says them.
func limitsCount(t views.AccountTotals, k ink) []part {
	return []part{figureIn(prose.Grouped(t.LimitCount()), k)}
}

// shortWindows name windows as turned Compare's LIMITS does, name naming them in
// full: one named by its hours, "5-hour", cut to "5h"; any other as
// status.Short has its name, as "Fable wk".
func shortWindows(name func(key string) string) func(key string) string {
	return func(key string) string {
		full := name(key)
		if hours, ok := strings.CutSuffix(full, "-hour"); ok {
			return hours + "h"
		}
		return status.Short(full)
	}
}

// atCapParts say how long an account spent at its cap or at a limit over the
// period, as prose.Span says a span, "3h 10m"; of every account's, all of
// theirs together; a dash where it spent none, or its days never read it.
func atCapParts(t views.AccountTotals, k ink) []part {
	minutes, known := t.AtCapOrLimit()
	if !known || minutes == 0 {
		return noneOf(noFigure.text, k)
	}
	return []part{figureIn(prose.Span(time.Duration(minutes)*time.Minute), k)}
}

// leftParts say what an account's week had left as it last reset, as
// leftShort says it, and where it ran out, the day it did, as "0% (limit
// Fri)".
func leftParts(row views.AccountsRow, k ink) []part {
	left := leftShort(row, k)
	if day, ok := dayfile.Civil(row.RanOut); ok && row.WeekLeft != nil {
		left = append(left, wordsIn(" (limit "+prose.Weekday(day.Weekday())+")", k))
	}
	return left
}

// leftForms say what an account's week had left as it last reset in its
// forms: as leftParts says it, "0% (limit Fri)", then as turned Compare
// says it, as leftShort has it, "0%", dropping the day it ran out.
func leftForms(row views.AccountsRow, k ink) partForms {
	return partForms{leftParts(row, k), leftShort(row, k)}
}

// leftShort says what an account's week had left as it last reset, "15%", as
// turned Compare's LIMITS has it: a dash where none of its weeks ended in the six
// weeks, and of every account's.
func leftShort(row views.AccountsRow, k ink) []part {
	if row.WeekLeft == nil {
		return noneOf(noFigure.text, k)
	}
	return []part{figureIn(prose.Percent(*row.WeekLeft), k)}
}

// planParts say an account's plan as Compare's VALUE does, by its name, then
// its price a month, in text.subtle: "Max 20x · $200/mo"; of every
// account's, the known plans' prices summed, then how many plans that is,
// as views.PlansKnown words it, "$600/mo · 3 plans", or of how many,
// "$400/mo · 2 of 3 plans"; a dash where none is known.
func planParts(row views.AccountsRow, k ink) []part {
	said := planNamed(row, k)
	switch {
	case row.Plan.Price == nil:
		return said
	case !row.Plan.All:
		return append(said, wordsIn(accountsApart, dimInk), figureIn(row.Plan.Monthly(), dimInk))
	}
	return append(append(said, wordsIn(accountsApart, dimInk)), saidParts(row.Plan.Plans.Words(), dimInk)...)
}

// planForms say an account's plan in Compare's VALUE, in its forms: as
// planParts says it, "Max 20x · $200/mo"; of every account's where some
// plans aren't known, then its plans known compact, "$400/mo · 2/3"; then as
// turned Compare says it, as planPriced has it, "$200/mo", dropping the
// plan's name and the plans' count.
func planForms(row views.AccountsRow, k ink) partForms {
	forms := partForms{planParts(row, k)}
	if compact := row.Plan.Plans.SaidCompact(); row.Plan.All && row.Plan.Price != nil && compact != "" {
		forms = append(forms, append(planPriced(row, k), wordsIn(accountsApart, dimInk), figureIn(compact, dimInk)))
	}
	return append(forms, planPriced(row, k))
}

// planNamed says an account's plan as Detail's THIS WEEK does, by its name
// alone, "Max 20x"; of every account's, as planPriced says it.
func planNamed(row views.AccountsRow, k ink) []part {
	if row.Plan.All || row.Plan.Price == nil {
		return planPriced(row, k)
	}
	return []part{wordsIn(prose.Clean(row.PlanName), k)}
}

// planPriced says an account's plan as turned Compare's VALUE does, by its price a
// month alone, "$200/mo"; of every account's, the known plans' prices
// summed, "$600/mo"; a dash where none is known.
func planPriced(row views.AccountsRow, k ink) []part {
	monthly := row.Plan.Monthly()
	if monthly == "" {
		return noneOf(noFigure.text, k)
	}
	return []part{figureIn(monthly, k)}
}

// extraForms say an account's extra usage in Compare's VALUE, in its forms,
// as ExtraSaid has it: "off", in text.muted; "on · billed past its limit",
// in accent.attention, then "on", dropping what it means; a dash before it's
// read. Each is bold where k is.
func extraForms(a status.AccountNow, k ink) partForms {
	all := status.ExtraOfAll([]status.AccountNow{a})
	return partForms{extraSaid(all, " · billed past its limit", "off", k), extraShort(all, k)}
}

// extraOfAllForms say every account's extra usage, taken together, in
// Compare's VALUE, in its forms: "off on all", then "off"; "on on all" where
// each is on, then "on", dropping on all; else how many are, "on for 1 of
// 3", its one form; a dash where none is read.
func extraOfAllForms(all status.AllExtra, k ink) partForms {
	return partForms{extraSaid(all, " on all", "off on all", k), extraShort(all, k)}
}

// extraShort says extra usage short, as Detail's THIS WEEK, stacked Detail
// and Compare's shortest forms do, in k's place: of the accounts given, an
// account alone or every account, taken together, "off"; "on" where each is
// on, else how many are, "on for 1 of 3"; a dash where none is read.
func extraShort(all status.AllExtra, k ink) []part {
	return extraSaid(all, "", "off", k)
}

// extraSaid says extra usage as all has it: off as the words given say it,
// in text.muted; on where each account is, the words given after it, else
// how many are, in accent.attention; a dash where none is read; each bold
// where k is.
func extraSaid(all status.AllExtra, every, off string, k ink) []part {
	switch all.State {
	case status.ExtraNotRead:
		return noneOf(noFigure.text, k)
	case status.ExtraOff:
		return []part{wordsIn(off, boldAs(mutedInk, k))}
	}
	on := boldAs(warningInk, k)
	if all.On == all.Of {
		return []part{wordsIn("on"+every, on)}
	}
	return []part{wordsIn("on for ", on), figureIn(prose.Grouped(all.On), on), wordsIn(" of ", on), figureIn(prose.Grouped(all.Of), on)}
}

// worthParts say what an account's requests over the period were worth, as
// worthSaid says it: "$184", "$184+" or "unpriced"; a dash for none.
func worthParts(row views.AccountsRow, k ink) []part {
	return []part{worthPart(row.Totals.Worth, k)}
}

// againstForms say an account's worth against its plan's price in Compare's
// VALUE, in its forms: as againstPriced says it, "4.0× its $46 week"; then
// as turned Compare says it, as againstCompact has it, "4.0×", dropping what
// the plans cost over the period. Of every account's where some plans
// aren't known, the ratio is of those alone, so every form says how many,
// the priced forms as knownForms words it, "4.4× its $92 week · 2 of 3
// plans known", then shorter, and the last compact, "4.4× · 2/3".
func againstForms(page views.AccountsPage, row views.AccountsRow) partForms {
	a := row.Plan.Against
	priced := againstPriced(a, page.Period.Unit())
	known := knownForms(a.Plans)
	if !pricedAgainst(a) || known == nil {
		return partForms{priced, againstCompact(row)}
	}
	var forms partForms
	for _, said := range known {
		forms = append(forms, append(slices.Clone(priced), wordsIn(said, dimInk)))
	}
	return append(forms, againstCompact(row))
}

// againstShort says an account's worth against its plan's price by the
// ratio alone, as againstSaid says it: "4.0×".
func againstShort(row views.AccountsRow) []part {
	return []part{figurePart(againstSaid(row.Plan.Ratio), Number{})}
}

// againstCompact says an account's worth against its plan's price as
// turned Compare's against it and stacked Detail's against do, as againstShort says it, then, of
// every account's where some plans aren't known, over those known, compact,
// as views.PlansKnown says it, in text.subtle: "4.4× · 2/3".
func againstCompact(row views.AccountsRow) []part {
	said := againstShort(row)
	if compact := row.Plan.Plans.SaidCompact(); row.Plan.Ratio != nil && compact != "" {
		said = append(said, wordsIn(accountsApart+compact, dimInk))
	}
	return said
}

// verdictParts say Weeks' verdict on a row of Detail's, as WEEKS' verdict
// line does: the verdict, bold in its colour, as verdictInk has it, then
// three cells on, in text.muted, its record, as verdictRecord says it; "not
// enough weeks yet", in text.subtle, where no week can be judged.
func verdictParts(page views.AccountsPage, row views.AccountsRow) []part {
	v := row.Verdict
	if v.Verdict == "" {
		return []part{wordsIn(views.NotEnoughWeeks, dimInk)}
	}
	return append([]part{wordsIn(v.Verdict, verdictInk(v.Verdict)), wordsIn("   ", mutedInk)}, verdictRecord(page, row)...)
}

// verdictRecord is the record of a row of Detail's over the last weeks, in
// text.muted: of an account, the weeks it hit its limit in and what they
// left at a reset, on average, "limit hit 3 of the last 7 weeks · 9% left at
// a reset, on average"; of every account's, what their weeks left at a reset,
// on average, then which account ends them with the most left and which the
// least, "30% left at a reset, on average · side ends weeks with 59% left,
// work with 9%", or with one account known, "work ends weeks with 9% left".
func verdictRecord(page views.AccountsPage, row views.AccountsRow) []part {
	v := row.Verdict
	var record []part
	if !v.All {
		record = saidParts(views.LimitHitOfTheLast(v.Limits, v.Weeks), mutedInk)
	}
	if v.LeftAtReset != nil {
		record = joinedParts(span{accountsApart, mutedInk}, record, saidParts(views.LeftOnAverage(*v.LeftAtReset), mutedInk))
	}
	if v.All {
		record = joinedParts(span{accountsApart, mutedInk}, record, endsParts(page))
	}
	return record
}

// endsParts say which account ends its weeks with the most left at a reset,
// on average, and which the least, as Ends names them, each by its label, as
// labelled has it, in text.muted: "side ends weeks with 59% left, work with
// 9%"; the one alone where they're one; none where no account's weeks are
// known.
func endsParts(page views.AccountsPage) []part {
	most, least, ok := page.Ends()
	if !ok {
		return nil
	}
	left := func(row views.AccountsRow) part { return figureIn(prose.Percent(*row.Verdict.LeftAtReset), mutedInk) }
	name := func(row views.AccountsRow) string { return labelled(row.Label, row.Totals.Account) }
	ends := []part{wordsIn(name(most)+" ends weeks with ", mutedInk), left(most), wordsIn(" left", mutedInk)}
	if least.Totals.Account == most.Totals.Account {
		return ends
	}
	return append(ends, wordsIn(", "+name(least)+" with ", mutedInk), left(least))
}

// verdictShort are verdictParts' two rows, as stacked Detail's WEEKS has them: the
// verdict alone, then its record without ", on average": of an account,
// "limit hit 3 of 7 weeks · 9% left at a reset", and of every account's,
// "30% left at a reset"; or "not enough weeks yet" alone.
func verdictShort(row views.AccountsRow) [][]part {
	v := row.Verdict
	if v.Verdict == "" {
		return [][]part{{wordsIn(views.NotEnoughWeeks, dimInk)}}
	}
	var record []part
	if !v.All {
		record = saidParts(views.LimitHit(v.Limits, v.Weeks), mutedInk)
	}
	if v.LeftAtReset != nil {
		record = joinedParts(span{accountsApart, mutedInk}, record, saidParts(views.LeftAtAReset(*v.LeftAtReset), mutedInk))
	}
	return [][]part{{wordsIn(v.Verdict, verdictInk(v.Verdict))}, record}
}

// shareParts say a version's share of a row's use over the period, as
// views.ShareSaid says it; a dash where it has none.
func shareParts(share *float64, k ink) []part {
	if said, ok := views.ShareSaid(share); ok {
		return []part{figureIn(said, k)}
	}
	return noneOf(noFigure.text, k)
}
