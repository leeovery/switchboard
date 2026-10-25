package ledger_test

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// filedUnder are lines, each filed under the date of the day its file is of.
type filedUnder map[string][]ledger.Line

// onArrival files each of lines under the local day its request arrived on,
// as the ledger files them.
func onArrival(lines ...ledger.Line) filedUnder {
	filed := make(filedUnder)
	for _, l := range lines {
		date := dayfile.DateOf(l.At)
		filed[date] = append(filed[date], l)
	}
	return filed
}

// took is asked's request, arriving at at, ending as long after as given.
func took(request string, at time.Time, d time.Duration) ledger.Line {
	line := asked(request, at)
	line.TotalMS = d.Milliseconds()
	return line
}

// A day's lines are those that arrived on it, by now, oldest first by their
// arrival, whichever of its files or its neighbours' holds them: a request
// that ended after midnight its arrival day's, one a change of time zone
// filed beside its own day its arrival day's, a day the clocks went back on
// as long as it is, and a day with no files none.
func TestADaysLinesAreThoseThatArrivedOnIt(t *testing.T) {
	acrossMidnight := took("1", on(-1, 23, 59).Add(50*time.Second), 22*time.Second)
	// London's clocks go back an hour at 02:00 on 25 October 2026, so its
	// 01:30 comes twice and its day lasts 25 hours. The ledger's days are
	// time.Local's, so in another zone the day is what that zone makes of the
	// same instants.
	firstHalfPast := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).Local()
	fallingBack := onArrival(
		asked("1", firstHalfPast),
		asked("2", firstHalfPast.Add(time.Hour)),
		asked("3", dayfile.DayStart(firstHalfPast, 1).Add(-30*time.Minute)),
		asked("4", dayfile.DayStart(firstHalfPast, 1).Add(30*time.Minute)),
	)
	tests := []struct {
		name  string
		filed filedUnder
		now   time.Time
		// want are the lines wanted of each day, by its date.
		want filedUnder
	}{
		{
			name:  "a request arriving at 23:59:50 and ending at 00:00:12, its arrival day's",
			filed: onArrival(acrossMidnight, asked("2", on(0, 0, 1))),
			now:   on(0, 12, 0),
			want:  filedUnder{"2026-10-04": {acrossMidnight}, date: {asked("2", on(0, 0, 1))}},
		},
		{
			name: "lines a change of time zone filed under the dates beside their own",
			filed: filedUnder{
				"2026-10-04": {asked("1", on(-1, 22, 0)), asked("2", on(0, 0, 30))},
				date:         {asked("3", on(0, 9, 0)), asked("5", on(1, 0, 10))},
				"2026-10-06": {asked("4", on(0, 23, 30)), asked("6", on(1, 9, 0))},
			},
			now: on(1, 12, 0),
			want: filedUnder{
				"2026-10-04": {asked("1", on(-1, 22, 0))},
				date:         {asked("2", on(0, 0, 30)), asked("3", on(0, 9, 0)), asked("4", on(0, 23, 30))},
				"2026-10-06": {asked("5", on(1, 0, 10)), asked("6", on(1, 9, 0))},
			},
		},
		{
			name:  "lines arriving at midnight exactly, each the day it starts",
			filed: onArrival(asked("1", on(0, 0, 0)), asked("2", on(1, 0, 0))),
			now:   on(1, 12, 0),
			want:  filedUnder{date: {asked("1", on(0, 0, 0))}, "2026-10-06": {asked("2", on(1, 0, 0))}},
		},
		{
			name:  "the day the clocks go back, as long as it is",
			filed: fallingBack,
			now:   dayfile.DayStart(firstHalfPast, 2),
			want:  fallingBack,
		},
		{
			name:  "a day with no files, and a line arriving after now, as after a clock set back",
			filed: filedUnder{"2026-10-03": {asked("1", on(-2, 9, 0))}, date: {asked("2", on(0, 9, 0)), asked("3", on(0, 13, 0))}},
			now:   on(0, 12, 0),
			want:  filedUnder{"2026-10-04": nil, date: {asked("2", on(0, 9, 0))}, "not a date": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			for date, lines := range tt.filed {
				holdLines(t, dir, date, lines...)
			}
			follower := followerOf(state, &testClock{at: tt.now}, caps)

			for _, date := range slices.Sorted(maps.Keys(tt.want)) {
				var want []string
				for _, l := range tt.want[date] {
					want = append(want, jsonOf(t, l))
				}
				if got := readAs(t, follower.Day(date)); !slices.Equal(got, want) {
					t.Errorf("Day(%q) =\n%s\nwant\n%s", date, strings.Join(got, "\n"), strings.Join(want, "\n"))
				}
			}
		})
	}
}

// FirstLinesDay tells the start of the first day the ledger holds lines of,
// a summary of an earlier day, its lines pruned, aside, failing with
// fs.ErrNotExist while it holds none.
func TestTheFollowerTellsTheFirstDayTheLedgerHoldsLinesOf(t *testing.T) {
	state, dir, _ := stateDirs(t)
	at := on(0, 12, 0)
	follower := followerOf(state, &testClock{at: at}, caps)
	writeFile(t, dir, "day-2026-10-02.json", []byte(summaryOf("2026-10-02", 1, windowsRead{}, on(-3, 9, 0))+"\n"))
	if _, err := follower.FirstLinesDay(at); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FirstLinesDay() of a ledger holding a summary alone fails with %v, want fs.ErrNotExist", err)
	}

	holdLines(t, dir, "2026-10-04", asked("2", on(-1, 9, 0)))
	holdLines(t, dir, "2026-10-03", asked("1", on(-2, 9, 0)))
	if got, err := follower.FirstLinesDay(at); err != nil || !got.Equal(on(-2, 0, 0)) {
		t.Errorf("FirstLinesDay() = %v, %v; want the start of 3 October", got, err)
	}
	if got, err := follower.FirstDay(at); err != nil || !got.Equal(on(-3, 0, 0)) {
		t.Errorf("FirstDay() = %v, %v; want the start of 2 October, its summary's", got, err)
	}
}
