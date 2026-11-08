package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
)

// report is what the tool tells of a history: the first and last hours it
// forecast at; the first it scored, two days in, and whether it scored any,
// as it didn't where that's after the last; the accounts configured; and how
// the router's forecasts scored, and the baseline's beside them.
type report struct {
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	ScoredFrom time.Time `json:"scored_from"`
	Scored     bool      `json:"scored"`
	Accounts   []string  `json:"accounts"`
	Forecast   sheet     `json:"forecast"`
	Baseline   sheet     `json:"baseline"`
}

// sheet is how a set of forecasts scored, by how much of their week was used
// as they were made, and all together.
type sheet struct {
	ByUse []row `json:"by_use"`
	All   row   `json:"all"`
}

// row is a tally, as a report gives it: the share of the week used as its
// forecasts were made, from and up to; how many there were; how far off they
// were on average, either way, in hours; how many were early, and how many
// late, by more than missBy; and how many moved from the hour before's
// forecast of their week, and how far on average, either way, in hours.
type row struct {
	UsedFrom   float64 `json:"used_from"`
	UsedTo     float64 `json:"used_to"`
	Forecasts  int     `json:"forecasts"`
	OffHours   float64 `json:"off_hours"`
	Early      int     `json:"early"`
	Late       int     `json:"late"`
	Moves      int     `json:"moves"`
	MovedHours float64 `json:"moved_hours"`
}

// reportOf is the report of the forecasts played of the accounts configured,
// scored against what became of their weeks, as o has it.
func reportOf(configured config.Accounts, pl played, o outcomes) report {
	return report{
		From: pl.from, To: pl.to, ScoredFrom: pl.scored, Scored: !pl.scored.After(pl.to), Accounts: configured.IDs(),
		Forecast: sheetOf(scoresOf(pl.forecasts, o)), Baseline: sheetOf(scoresOf(pl.baseline, o)),
	}
}

// sheetOf is s as a report gives it.
func sheetOf(s scores) sheet {
	sh := sheet{ByUse: make([]row, len(s.byUse)), All: rowOf(s.all, 0, 1)}
	for i, t := range s.byUse {
		to := 1.0
		if i+1 < len(usedFrom) {
			to = usedFrom[i+1]
		}
		sh.ByUse[i] = rowOf(t, usedFrom[i], to)
	}
	return sh
}

// rowOf is t, of forecasts made with from up to to of their week used, as a
// report gives it.
func rowOf(t tally, from, to float64) row {
	return row{
		UsedFrom: from, UsedTo: to, Forecasts: t.forecasts, OffHours: meanHours(t.off, t.forecasts), Early: t.early, Late: t.late,
		Moves: t.moves, MovedHours: meanHours(t.moved, t.moves),
	}
}

// meanHours is total over n, in hours: none of none.
func meanHours(total time.Duration, n int) float64 {
	if n == 0 {
		return 0
	}
	return total.Hours() / float64(n)
}

// writeJSON writes r to w as JSON.
func writeJSON(w io.Writer, r report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("write the scores: %w", err)
	}
	return nil
}

// hourLayout is how the table says the hours it forecast at, in UTC.
const hourLayout = "2006-01-02 15:04 MST"

// legend says what the table's columns are.
const legend = `off is how far a forecast's end was from its week's, either way, on average:
its run-out, or its reset where it was forecast short, from when the week ran
out, or its reset where it didn't; early and late, how often it was more than
12 hours before or after; moved, how far it moved from the hour before's
forecast of its week, either way, on average.
`

// writeTable writes r to w as a table: the router's forecasts' scores, then
// the baseline's, each by how much of the week was used, and all together;
// or, where it scored none, that nothing could be scored.
func writeTable(w io.Writer, r report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "forecasts of each week at each hour, %s to %s, of %s\n",
		hourOf(r.From), hourOf(r.To), strings.Join(r.Accounts, ", "))
	if r.Scored {
		fmt.Fprintf(&b, "scored from %s, two days in\n\n", hourOf(r.ScoredFrom))
		writeSheets(&b, r)
	} else {
		fmt.Fprintf(&b, "nothing could be scored: the history ends before %s, two days in, where scoring starts\n", hourOf(r.ScoredFrom))
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write the scores: %w", err)
	}
	return nil
}

// hourOf is the hour at, as the table says it.
func hourOf(at time.Time) string {
	return at.UTC().Format(hourLayout)
}

// writeSheets writes r's sheets to b as the table's rows, under its columns'
// names, with what they are below them.
func writeSheets(b *strings.Builder, r report) {
	fmt.Fprintf(b, "%-20s %9s %7s %6s %6s %7s\n", "", "forecasts", "off", "early", "late", "moved")
	for _, part := range []struct {
		name  string
		sheet sheet
	}{{"the router's forecast", r.Forecast}, {"pace since start", r.Baseline}} {
		b.WriteString(part.name + "\n")
		for _, row := range part.sheet.ByUse {
			writeRow(b, fmt.Sprintf("%d–%d%% used", prose.WholePercent(row.UsedFrom), prose.WholePercent(row.UsedTo)), row)
		}
		writeRow(b, "all", part.sheet.All)
	}
	b.WriteString("\n" + legend)
}

// writeRow writes r to b as a line of the table, named label: a dash for
// what it has none of to say.
func writeRow(b *strings.Builder, label string, r row) {
	off, early, late, moved := "–", "–", "–", "–"
	if r.Forecasts > 0 {
		off, early, late = hours(r.OffHours), share(r.Early, r.Forecasts), share(r.Late, r.Forecasts)
	}
	if r.Moves > 0 {
		moved = hours(r.MovedHours)
	}
	fmt.Fprintf(b, "  %-18s %9d %7s %6s %6s %7s\n", label, r.Forecasts, off, early, late, moved)
}

// hours is h hours, to a tenth.
func hours(h float64) string {
	return fmt.Sprintf("%.1fh", h)
}

// share is n of all as a whole percentage.
func share(n, all int) string {
	return prose.Percent(float64(n) / float64(all))
}
