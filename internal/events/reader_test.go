package events_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// filed are the lines a day's file holds, by the day of September it's of,
// and whether it's compressed, a gzip member of them.
type filed struct {
	day        int
	compressed bool
	lines      []string
}

// fileDays writes each day's file in the ledger's directory of state.
func fileDays(t *testing.T, state string, days ...filed) {
	t.Helper()
	for _, d := range days {
		name := "events-" + dateOf(september(d.day, 12, 0)) + ".jsonl"
		data := []byte(linesOf(d.lines...))
		if d.compressed {
			name, data = name+".gz", gzipped(t, string(data))
		}
		writeFile(t, ledger.Dir(state), name, data)
	}
}

// between returns the events a reader of state, by now's clock, reads from
// from up to to, as they're filed.
func between(t *testing.T, state string, from, to time.Time) []string {
	t.Helper()
	var got []events.Line
	for line := range events.NewReader(state, func() time.Time { return now }, logs.For("cli")).Between(from, to) {
		got = append(got, line)
	}
	return jsonsOf(t, got)
}

func TestEachEventIsReadAsItLastStandsOldestFirst(t *testing.T) {
	a := func(count int) events.Line { return limit(firstRun, 1, september(10, 9, 0), count) }
	b := func(count int) events.Line { return limit(firstRun, 2, september(10, 12, 0), count) }
	late := func(count int) events.Line { return limit(firstRun, 3, september(12, 23, 30), count) }
	again := limit(secondRun, 1, september(11, 8, 0), 1)
	tests := []struct {
		name string
		days []filed
		want []events.Line
	}{
		{
			name: "a version filed days after its event wins, under the day of its event",
			days: []filed{
				{day: 10, lines: []string{jsonOf(t, a(1)), jsonOf(t, b(1))}},
				{day: 11, lines: []string{jsonOf(t, again)}},
				{day: 14, lines: []string{jsonOf(t, a(2)), jsonOf(t, a(3))}},
			},
			want: []events.Line{a(3), b(1), again},
		},
		{
			name: "versions in compressed files as in plain ones, a day's compressed file's before its plain file's",
			days: []filed{
				{day: 10, compressed: true, lines: []string{jsonOf(t, a(1))}},
				{day: 14, compressed: true, lines: []string{jsonOf(t, a(2))}},
				{day: 14, lines: []string{jsonOf(t, a(4))}},
			},
			want: []events.Line{a(4)},
		},
		{
			name: "a version 8 days after its event's day still read",
			days: []filed{
				{day: 12, lines: []string{jsonOf(t, late(1))}},
				{day: 20, compressed: true, lines: []string{jsonOf(t, late(5))}},
			},
			want: []events.Line{late(5)},
		},
		{
			// 8 days after it, by the clock of a router a time zone east.
			name: "a version 8 days on filed under the day after, as a change of time zone files one",
			days: []filed{
				{day: 12, lines: []string{jsonOf(t, late(1))}},
				{day: 21, lines: []string{jsonOf(t, late(6))}},
			},
			want: []events.Line{late(6)},
		},
		{
			name: "two runs' events of the same id kept apart",
			days: []filed{
				{day: 10, lines: []string{jsonOf(t, a(1))}},
				{day: 11, lines: []string{jsonOf(t, again), jsonOf(t, a(2))}},
			},
			want: []events.Line{a(2), again},
		},
		{
			name: "oldest first by when each happened, whatever order their last versions were filed in",
			days: []filed{
				{day: 10, lines: []string{jsonOf(t, a(1)), jsonOf(t, b(1))}},
				{day: 11, lines: []string{jsonOf(t, b(2)), jsonOf(t, again)}},
				{day: 13, lines: []string{jsonOf(t, a(2))}},
			},
			want: []events.Line{a(2), b(2), again},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			fileDays(t, state, tt.days...)

			if got, want := between(t, state, september(10, 0, 0), september(13, 0, 0)), jsonsOf(t, tt.want); !slices.Equal(got, want) {
				t.Errorf("read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestAVersionFiledLaterThanAnEventChangesIsPassedOver(t *testing.T) {
	a := func(count int) events.Line { return limit(firstRun, 1, september(10, 9, 0), count) }
	state := t.TempDir()
	fileDays(t, state,
		filed{day: 10, lines: []string{jsonOf(t, a(1))}},
		filed{day: 25, lines: []string{jsonOf(t, a(9))}},
	)

	if got, want := between(t, state, september(10, 0, 0), september(30, 0, 0)), jsonsOf(t, []events.Line{a(1)}); !slices.Equal(got, want) {
		t.Errorf("read\n%s\nwant\n%s: the event once, as it stood once it settled", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTheEventsReadAreThoseOfTheSpan(t *testing.T) {
	from, to := september(10, 0, 0), september(13, 0, 0)
	before := limit(firstRun, 1, from.Add(-time.Second), 1)
	first := limit(firstRun, 2, from, 1)
	last := limit(firstRun, 3, to.Add(-time.Second), 1)
	after := limit(firstRun, 4, to, 1)
	state := t.TempDir()
	fileDays(t, state,
		filed{day: 9, lines: []string{jsonOf(t, before)}},
		filed{day: 10, lines: []string{jsonOf(t, first)}},
		filed{day: 12, lines: []string{jsonOf(t, last)}},
		filed{day: 13, lines: []string{jsonOf(t, after)}},
	)

	if got, want := between(t, state, from, to), jsonsOf(t, []events.Line{first, last}); !slices.Equal(got, want) {
		t.Errorf("read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLinesThatDontReadAreCountedAndPassedOver(t *testing.T) {
	log := logstest.Capture(t)
	event := limit(firstRun, 1, september(10, 9, 0), 1)
	state := t.TempDir()
	fileDays(t, state, filed{day: 10, lines: []string{
		"a line that isn't JSON",
		`{"id":2,"at":"2026-09-10T10:00:00Z","kind":"limit"}`,
		`{"a_later_field":true,` + strings.TrimPrefix(jsonOf(t, event), "{"),
	}})

	if got, want := between(t, state, september(10, 0, 0), september(11, 0, 0)), jsonsOf(t, []events.Line{event}); !slices.Equal(got, want) {
		t.Errorf("read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if want := []string{"level=WARN", `msg="lines of the router's events unread"`, "lines=2"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the lines that don't read counted, a line with %q", log, want)
	}
}

func TestAnEventIsReadAsItsLastVersionWasFiled(t *testing.T) {
	first, last := limit(firstRun, 1, september(10, 9, 0), 1), limit(firstRun, 1, september(10, 9, 0), 2)
	lastFiled := `{"a_later_field":true,` + strings.TrimPrefix(jsonOf(t, last), "{")
	state := t.TempDir()
	fileDays(t, state, filed{day: 10, lines: []string{jsonOf(t, first)}}, filed{day: 11, lines: []string{lastFiled}})

	var got []string
	for held := range events.NewReader(state, func() time.Time { return now }, logs.For("cli")).HeldBetween(september(10, 0, 0), september(11, 0, 0)) {
		data, err := held.Filed()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(data))
	}
	if want := []string{lastFiled}; !slices.Equal(got, want) {
		t.Errorf("read\n%s\nwant the last version as it was filed, its later field included\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestADamagedCompressedFileIsReadUpToTheDamage(t *testing.T) {
	log := logstest.Capture(t)
	a, b := limit(firstRun, 1, september(10, 9, 0), 1), limit(firstRun, 2, september(11, 9, 0), 1)
	state := t.TempDir()
	name := "events-" + dateOf(september(10, 12, 0)) + ".jsonl.gz"
	writeFile(t, ledger.Dir(state), name, append(gzipped(t, linesOf(jsonOf(t, a))), gzipped(t, linesOf(jsonOf(t, a)))[:5]...))
	fileDays(t, state, filed{day: 11, lines: []string{jsonOf(t, b)}})

	if got, want := between(t, state, september(10, 0, 0), september(12, 0, 0)), jsonsOf(t, []events.Line{a, b}); !slices.Equal(got, want) {
		t.Errorf("read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if want := []string{"level=WARN", `msg="router's events read short"`, "file=" + name}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the damage warned of, a line with %q", log, want)
	}
}

func TestAnEventIsHandedOnOnceNoFileLeftCanHoldAVersionOfIt(t *testing.T) {
	log := logstest.Capture(t)
	event := limit(firstRun, 1, september(10, 9, 0), 1)
	state := t.TempDir()
	fileDays(t, state,
		filed{day: 10, lines: []string{jsonOf(t, event)}},
		filed{day: 18, lines: []string{jsonOf(t, limit(firstRun, 2, september(18, 9, 0), 1))}},
	)
	// The day after the last that can hold a version of the 10th's event,
	// with a day for a change of time zone, which a read that has had enough
	// by then has no need to open.
	if err := os.WriteFile(filepath.Join(ledger.Dir(state), "events-"+dateOf(september(20, 12, 0))+".jsonl"), nil, 0o000); err != nil {
		t.Fatal(err)
	}

	var got []events.Line
	for line := range events.NewReader(state, func() time.Time { return now }, logs.For("cli")).Between(september(10, 0, 0), september(30, 0, 0)) {
		got = append(got, line)
		break
	}
	if want := jsonsOf(t, []events.Line{event}); !slices.Equal(jsonsOf(t, got), want) || log.Has(`msg="can't read the router's events"`) {
		t.Errorf("read %q, logging\n%s\nwant %q, handed on once the 19th was read, the 20th never opened", jsonsOf(t, got), log, want)
	}
}

func TestNoEventsReadNothing(t *testing.T) {
	if got := slices.Collect(events.Empty{}.Between(september(1, 0, 0), now)); len(got) > 0 {
		t.Errorf("Between() = %+v, want none", got)
	}
}
