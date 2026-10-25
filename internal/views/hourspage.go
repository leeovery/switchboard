package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
)

// HoursPage is WHEN YOU WORK as the Graphs tab draws it: when the requests
// come over the four whole weeks before today, the 28 days ending yesterday,
// a row a weekday and a tile an hour of the day, each shaded by its requests
// a day, as History's five shades shade a day.
//
// A day counts where the ledger knows its hours, as dayHours says; one it
// doesn't is left out of its weekday's divisor, never counted as none. Where
// no day of the four weeks is known, the section has no grid: Days, Cuts and
// KeyCuts are nil, Weeks is 0 and Description is "".
type HoursPage struct {
	// Days are the grid's rows, seven, from the first day of the week, as
	// week_starts sets it.
	Days []HoursDay
	// Cuts are the cuts between the levels, as quarterCuts gives them, of
	// the active hours' requests a day, so a weekday known on fewer days is
	// shaded by its own average, as one known on four; KeyCuts are them as
	// the key words them, as hoursKeyCuts rounds them. Where days are known
	// but no hour is active, both are empty: no cuts, and no key to draw.
	Cuts    []float64
	KeyCuts []string
	// Weeks are the whole weeks the days counted span, from the first to
	// the last, a part week counted whole: 4 where the ledger knows the four
	// weeks, fewer where it began within them; an unknown day between two
	// counted narrows a weekday's divisor, never the weeks.
	Weeks int
	// Description is the weeks said, to the last day counted, as the
	// section's heading says them: "the 4 weeks to Wed 30 Sep".
	Description string
}

// HoursDay is a weekday's row of the grid: its weekday and its name, as
// "Mon"; how many of its days are known; and its 24 hours, from midnight's.
// A weekday none of whose days is known has every hour at none.
type HoursDay struct {
	Weekday time.Weekday
	Name    string
	Known   int
	Hours   [hoursADay]HoursTile
}

// HoursTile is an hour of a weekday: its requests a day, those of its days
// known summed and divided by them, and its level, 0 to 4.
type HoursTile struct {
	Requests float64
	Level    int
}

// The grid's span and shape.
const (
	// hoursWeeks are the whole weeks before today the grid counts.
	hoursWeeks = 4
	// hoursADay are the hours of a day, as a summary counts them.
	hoursADay = 24
	// weekdays are the days of a week.
	weekdays = 7
)

// Hours is WHEN YOU WORK's page: the four whole weeks before today, of the
// summaries the Past holds.
func (p Past) Hours() HoursPage {
	today := dayfile.CivilOf(p.in.Now)
	first, last := today.AddDate(0, 0, -hoursWeeks*weekdays), today.AddDate(0, 0, -1)
	var tally hoursTally
	for _, d := range between(p.days, first.Format(time.DateOnly), last.Format(time.DateOnly)) {
		tally.add(d)
	}
	return tally.page(p.in.WeekStarts)
}

// hoursTally sums the hours of the days known, by weekday, and how many of
// each weekday are known; and the first and last of them.
type hoursTally struct {
	sums        [weekdays][hoursADay]int
	known       [weekdays]int
	first, last time.Time
}

// add counts the day's hours under its weekday, where they're known.
func (t *hoursTally) add(d Day) {
	hours, ok := dayHours(d)
	day, dated := dayfile.Civil(d.Day)
	if !ok || !dated {
		return
	}
	weekday := day.Weekday()
	t.known[weekday]++
	for hour, n := range hours {
		t.sums[weekday][hour] += n
	}
	if t.first.IsZero() {
		t.first = day
	}
	t.last = day
}

// page is the grid the tally makes, its rows from the weekday given.
func (t *hoursTally) page(weekStarts time.Weekday) HoursPage {
	if t.last.IsZero() {
		return HoursPage{}
	}
	page := HoursPage{Days: make([]HoursDay, weekdays), Weeks: t.weeks()}
	var active []float64
	for i := range page.Days {
		row := t.row((weekStarts + time.Weekday(i)) % weekdays)
		for _, tile := range row.Hours {
			if tile.Requests > 0 {
				active = append(active, tile.Requests)
			}
		}
		page.Days[i] = row
	}
	page.Cuts = quarterCuts(active)
	for i := range page.Days {
		for h := range page.Days[i].Hours {
			tile := &page.Days[i].Hours[h]
			tile.Level = shade(tile.Requests, page.Cuts)
		}
	}
	page.KeyCuts, page.Description = hoursKeyCuts(page.Cuts), prose.WeeksTo(page.Weeks, t.last)
	return page
}

// row is the weekday's row, its hours' requests a day, unshaded.
func (t *hoursTally) row(weekday time.Weekday) HoursDay {
	row := HoursDay{Weekday: weekday, Name: prose.Weekday(weekday), Known: t.known[weekday]}
	if row.Known == 0 {
		return row
	}
	for hour, n := range t.sums[weekday] {
		row.Hours[hour].Requests = float64(n) / float64(row.Known)
	}
	return row
}

// weeks are the whole weeks from the first day counted to the last, a part
// week counted whole.
func (t *hoursTally) weeks() int {
	days := int(t.last.Sub(t.first).Hours()/24) + 1
	return (days + weekdays - 1) / weekdays
}

// dayHours are the day's requests upstream by the local hour, every account's
// and model's, reporting whether they're known: each model's, as modelHours
// says.
func dayHours(d Day) ([hoursADay]int, bool) {
	var hours [hoursADay]int
	for _, a := range d.Accounts {
		for _, m := range a.Models {
			counted, ok := modelHours(m.ModelDay)
			if !ok {
				return [hoursADay]int{}, false
			}
			for hour, n := range counted {
				hours[hour] += n
			}
		}
	}
	return hours, true
}

// modelHours are a model's requests upstream by the local hour, a copy of
// its summary's, as they may be the summary held's own, reporting whether
// they're known: they're not where the summary never counted them, unless
// none went upstream, nor where they're not a count, or don't sum to
// Upstream, as a summary edited by hand reads.
func modelHours(m ledger.ModelDay) ([hoursADay]int, bool) {
	if m.Hours == nil {
		return [hoursADay]int{}, m.Upstream == 0
	}
	hours, sum := *m.Hours, 0
	for _, n := range hours {
		if n < 0 {
			return [hoursADay]int{}, false
		}
		sum += n
	}
	return hours, sum == m.Upstream
}

// hoursKeySteps are what the key may round a cut to, coarsest first, in
// hundredths of a request, so a half rounds up as it's written rather than
// as a float's tenth falls; and the places each is written to.
var hoursKeySteps = []struct {
	hundredths float64
	places     int
}{{1000, 0}, {100, 0}, {10, 1}, {1, 2}}

// hoursKeyCuts are the cuts, in order, as the key words them, in requests an
// hour: each rounded on its own, as the Year's key rounds its cuts, to the
// coarsest step it isn't under, 10, then 1, a tenth and a hundredth; and
// where two that differ would then read the same, or out of order, each of
// them a step finer, till none do. An hour's average is over four days at
// most, so two that differ do by a twelfth at least, which a hundredth
// always keeps apart.
func hoursKeyCuts(cuts []float64) []string {
	scaled, steps := make([]float64, len(cuts)), make([]int, len(cuts))
	for i, cut := range cuts {
		scaled[i] = cut * 100
		steps[i] = coarsestStep(scaled[i])
	}
	for refined(scaled, steps) {
	}
	words := make([]string, len(cuts))
	for i, cut := range roundedAt(scaled, steps) {
		words[i] = prose.GroupedTo(cut/100, hoursKeySteps[steps[i]].places)
	}
	return words
}

// coarsestStep is the coarsest of hoursKeySteps the cut, in hundredths, isn't
// under: the finest where it's under every one.
func coarsestStep(cut float64) int {
	for i, s := range hoursKeySteps {
		if cut >= s.hundredths {
			return i
		}
	}
	return len(hoursKeySteps) - 1
}

// roundedAt are the cuts, each rounded to its step.
func roundedAt(cuts []float64, steps []int) []float64 {
	rounded := make([]float64, len(cuts))
	for i, cut := range cuts {
		rounded[i] = roundedTo(cut, hoursKeySteps[steps[i]].hundredths)
	}
	return rounded
}

// refined moves a step finer each cut that, rounded, reads the same as one
// that differs from it, or out of order with it, the cuts being in order;
// it reports whether it moved any.
func refined(cuts []float64, steps []int) bool {
	rounded := roundedAt(cuts, steps)
	clashing := make([]bool, len(cuts))
	for i := range cuts {
		for j := i + 1; j < len(cuts); j++ {
			if cuts[i] != cuts[j] && rounded[i] >= rounded[j] {
				clashing[i], clashing[j] = true, true
			}
		}
	}
	moved := false
	for i, clash := range clashing {
		if clash && steps[i] < len(hoursKeySteps)-1 {
			steps[i]++
			moved = true
		}
	}
	return moved
}
