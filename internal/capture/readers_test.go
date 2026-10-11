package capture

import (
	"encoding/json"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// readersNow is when the readers' tests read: the frames' moment, in the
// local time zone, as the ledger files its days.
var readersNow = time.Date(2026, time.October, 1, 14, 42, 7, 0, time.Local)

// The readers' tests' sessions: today's, one of yesterday's and today's,
// one of two days ago asked for by its id, and one of then that isn't.
const (
	sessionToday   = "aaaa0000-0000-4000-8000-000000000001"
	sessionBoth    = "bbbb0000-0000-4000-8000-000000000002"
	sessionAsked   = "cccc0000-0000-4000-8000-000000000003"
	sessionUnasked = "dddd0000-0000-4000-8000-000000000004"
	sessionChecked = "eeee0000-0000-4000-8000-000000000005"
)

// heldLine is a request of the session given, on work, arriving at at, of the
// kind given, held as the ledger writes it.
func heldLine(t *testing.T, request, session, kind string, at time.Time) ledger.Held {
	t.Helper()
	l := ledger.Line{
		At: at.UTC(), Request: request, Kind: kind, Session: session, Model: "claude-opus-5-5", Account: "work", Reason: status.ReasonSticky,
		Status: 200, Attempts: 1, TotalMS: 1000,
		Usage: json.RawMessage(`{"input_tokens":3,"output_tokens":900,"cache_creation_input_tokens":3000,` +
			`"cache_creation":{"ephemeral_1h_input_tokens":3000},"cache_read_input_tokens":40000}`),
	}
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	return ledger.Held{Line: l, JSON: data}
}

// readersAt is readersNow moved by the days given to the clock given.
func readersAt(days, hour, minute, second int) time.Time {
	y, m, d := readersNow.Date()
	return time.Date(y, m, d+days, hour, minute, second, 0, time.Local)
}

// heldRecords are the readers' tests' records: lines of two days ago,
// yesterday and today, one arriving after now, one of today's arriving
// before another written ahead of it, and a quota check of two days ago.
func heldRecords(t *testing.T) *records {
	t.Helper()
	return &records{
		lines: []ledger.Held{
			heldLine(t, "r1", sessionAsked, ledger.KindMessage, readersAt(-2, 9, 0, 0)),
			heldLine(t, "r2", sessionUnasked, ledger.KindMessage, readersAt(-2, 10, 0, 0)),
			heldLine(t, "r3", sessionChecked, ledger.KindCheck, readersAt(-2, 11, 0, 0)),
			heldLine(t, "r4", sessionBoth, ledger.KindMessage, readersAt(-1, 22, 0, 0)),
			heldLine(t, "r5", sessionToday, ledger.KindMessage, readersAt(0, 9, 0, 5)),
			heldLine(t, "r6", sessionToday, ledger.KindMessage, readersAt(0, 9, 0, 0)),
			heldLine(t, "r7", sessionBoth, ledger.KindCount, readersAt(0, 10, 0, 0)),
			heldLine(t, "r8", sessionBoth, ledger.KindMessage, readersAt(0, 11, 0, 0)),
			heldLine(t, "r9", sessionToday, ledger.KindMessage, readersAt(0, 15, 0, 0)),
		},
		readings: []readings.Reading{
			{At: readersAt(0, 8, 0, 0), Account: "work", Key: "5h", Utilization: 0.2, ResetsAt: readersAt(0, 12, 0, 0), Source: readings.FromAnswer},
			{At: readersAt(0, 12, 0, 0), Account: "work", Key: "5h", Utilization: 0.4, ResetsAt: readersAt(0, 17, 0, 0), Source: readings.FromAnswer},
			{At: readersAt(0, 15, 0, 0), Account: "work", Key: "5h", Utilization: 0.5, ResetsAt: readersAt(0, 17, 0, 0), Source: readings.FromAnswer},
		},
		events: []events.Line{
			{ID: 1, At: readersAt(-1, 20, 0, 0), Kind: status.EventStarted, Account: "work", Session: sessionBoth, Run: readersAt(-1, 8, 0, 0)},
			{ID: 2, At: readersAt(0, 9, 0, 0), Kind: status.EventLimit, Account: "work", Until: readersAt(0, 12, 0, 0), Run: readersAt(-1, 8, 0, 0)},
			{ID: 1, At: readersAt(0, 9, 0, 0), Kind: status.EventStarted, Account: "work", Session: sessionToday, Run: readersAt(0, 8, 30, 0)},
			{ID: 2, At: readersAt(0, 9, 0, 0), Kind: status.EventLimit, Account: "work", Until: readersAt(0, 13, 0, 0), Run: readersAt(-1, 8, 0, 0)},
			{ID: 2, At: readersAt(0, 15, 0, 0), Kind: status.EventPrimed, Account: "work", Run: readersAt(0, 8, 30, 0)},
		},
	}
}

// requestsOf are the ids of the requests held.
func requestsOf(held []ledger.Held) []string {
	var ids []string
	for _, h := range held {
		ids = append(ids, h.Request)
	}
	return ids
}

// fileLedger writes the records' lines that arrived by now into a state
// directory of the test's own, as the ledger files them, and returns a
// Follower of it, by now's clock: the real reader the records' ledger reads
// as.
func fileLedger(t *testing.T, r *records, now func() time.Time) *ledger.Follower {
	t.Helper()
	state := t.TempDir()
	dir := ledger.Dir(state)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := make(dayfile.Lines)
	for _, h := range r.lines {
		if !h.At.After(now()) {
			files.Add(h.At, h.JSON)
		}
	}
	for date, data := range files {
		if err := os.WriteFile(filepath.Join(dir, "requests-"+date+".jsonl"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return ledger.NewFollower(state, now, r.caps, slog.New(slog.DiscardHandler))
}

func TestTheRecordsLedgerGivesTheDaysBeforeTodaysAsHeldAndTodaysSummarised(t *testing.T) {
	r := heldRecords(t)
	r.days = []ledger.Summary{{Version: 3, Day: dayfile.DateOf(readersAt(-2, 12, 0, 0))}, {Version: 3, Day: dayfile.DateOf(readersAt(-1, 12, 0, 0))}}
	l, read, _ := r.readers(func() time.Time { return readersNow })

	days := l.Days(readersAt(-1, 0, 0, 0))
	if len(days) != 2 || !reflect.DeepEqual(days[0], r.days[1]) {
		t.Fatalf("Days(yesterday) = %+v, want yesterday's as held, then today's", days)
	}
	today := dayfile.DateOf(readersNow)
	lines := func(yield func(ledger.Line) bool) {
		for _, h := range r.lines[4:8] {
			if !yield(h.Line) {
				return
			}
		}
	}
	want, err := ledger.Summarise(today, lines, read.Between, nil, r.caps, readersNow)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(days[1], want) {
		t.Errorf("today's summary = %+v, want %+v, today's lines to now summarised with the readings", days[1], want)
	}
	if got := l.Days(readersNow); len(got) != 1 || got[0].Day != today {
		t.Errorf("Days(today) = %+v, want today's alone", got)
	}
}

func TestTheRecordsLedgerSummarisesTodayAsAFollowerOfItsFilesDoes(t *testing.T) {
	r := heldRecords(t)
	now := func() time.Time { return readersNow }
	l, _, _ := r.readers(now)
	// The Follower summarises with no readings history, its files having none.
	r.readings = nil
	want := fileLedger(t, r, now).Days(readersNow)
	want[0].Bytes = ledger.Bytes{}
	if got := l.Days(readersNow); !reflect.DeepEqual(got, want) {
		t.Errorf("Days(today) = %+v, want %+v, as a Follower of the records' files gives it", got, want)
	}
}

func TestTheRecordsLedgerGivesTodaysLinesOldestFirstAndAfreshAsAFollowerDoes(t *testing.T) {
	r := heldRecords(t)
	now := func() time.Time { return readersNow }
	l, _, _ := r.readers(now)
	follower := fileLedger(t, r, now)

	got, mark, afresh := l.Today(ledger.Mark{})
	want, _, wantAfresh := follower.Today(ledger.Mark{})
	if !slices.Equal(requestsOf(got), []string{"r6", "r5", "r7", "r8"}) || !reflect.DeepEqual(requestsOf(got), requestsOf(want)) {
		t.Errorf("Today() gave %v, want %v, today's lines to now, oldest first, as a Follower gives them", requestsOf(got), requestsOf(want))
	}
	if !afresh || !wantAfresh {
		t.Errorf("Today() afresh = %t, want every one of today's lines given afresh from the zero Mark", afresh)
	}
	for i := range got {
		if string(got[i].JSON) != string(want[i].JSON) {
			t.Errorf("Today()'s line %s holds %s, want its JSON as written, %s", got[i].Request, got[i].JSON, want[i].JSON)
		}
	}
	again, _, afresh := l.Today(mark)
	if !afresh || !slices.Equal(requestsOf(again), requestsOf(got)) {
		t.Errorf("Today(its mark) = %v (afresh %t), want every one of today's lines again, afresh, as a ledger.Mark is the ledger's to make",
			requestsOf(again), afresh)
	}
}

func TestTheRecordsLedgerGivesASessionsLinesNewestFirstAsAFollowerDoes(t *testing.T) {
	r := heldRecords(t)
	now := func() time.Time { return readersNow }
	l, _, _ := r.readers(now)
	follower := fileLedger(t, r, now)
	for _, id := range []string{sessionToday, sessionBoth, sessionAsked} {
		got, want := slices.Collect(l.Session(id)), slices.Collect(follower.Session(id))
		if !reflect.DeepEqual(requestsOf(got), requestsOf(want)) {
			t.Errorf("Session(%s) gave %v, want %v, as a Follower gives them", id, requestsOf(got), requestsOf(want))
		}
	}
	if got := requestsOf(slices.Collect(l.Session(sessionToday))); !slices.Equal(got, []string{"r5", "r6"}) {
		t.Errorf("Session(today's) gave %v, want its lines to now, newest first", got)
	}
	next, stop := iter.Pull(l.Session(sessionBoth))
	first, _ := next()
	stop()
	if first.Request != "r8" {
		t.Errorf("Session()'s first line = %s, want its newest, r8, the caller stopping there", first.Request)
	}
}

func TestTheRecordsLedgerGivesTheSessionsAFollowerReadsAndHandsOnTheirDaysLines(t *testing.T) {
	r := heldRecords(t)
	now := func() time.Time { return readersNow }
	l, _, _ := r.readers(now)
	follower := fileLedger(t, r, now)

	var took, wantTook []string
	got := l.Sessions([]string{sessionAsked}, func(_ string, line ledger.Line) { took = append(took, line.Request) })
	want := follower.Sessions([]string{sessionAsked}, func(_ string, line ledger.Line) { wantTook = append(wantTook, line.Request) })
	ids := func(m map[string][]ledger.Line) map[string][]string {
		out := make(map[string][]string)
		for id, lines := range m {
			for _, line := range lines {
				out[id] = append(out[id], line.Request)
			}
		}
		return out
	}
	wantIDs := map[string][]string{sessionToday: {"r6", "r5"}, sessionBoth: {"r4", "r7", "r8"}, sessionAsked: {"r1"}}
	if !reflect.DeepEqual(ids(got), wantIDs) || !reflect.DeepEqual(ids(got), ids(want)) {
		t.Errorf("Sessions(asked) = %v, want %v: today's sessions and the one asked for, oldest first, as a Follower gives them", ids(got), ids(want))
	}
	slices.Sort(took)
	slices.Sort(wantTook)
	if !slices.Equal(took, []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8"}) || !slices.Equal(took, wantTook) {
		t.Errorf("Sessions() handed take %v, want %v: every line of the days it read", took, wantTook)
	}

	took = nil
	if got := l.Sessions(nil, func(_ string, line ledger.Line) { took = append(took, line.Request) }); len(got) != 2 {
		t.Errorf("Sessions(none) = %v, want today's two alone", ids(got))
	}
	slices.Sort(took)
	if !slices.Equal(took, []string{"r4", "r5", "r6", "r7", "r8"}) {
		t.Errorf("Sessions(none) handed take %v, want the lines of yesterday's and today's alone, as no earlier day names a session it reads", took)
	}
}

func TestTheRecordsReadingsAreThoseReadInTheSpanByNowInTheOrderRead(t *testing.T) {
	r := heldRecords(t)
	_, read, _ := r.readers(func() time.Time { return readersAt(0, 14, 0, 0) })
	var got []float64
	for reading := range read.Between(readersAt(0, 8, 0, 0), readersAt(0, 15, 30, 0)) {
		got = append(got, reading.Utilization)
	}
	if !slices.Equal(got, []float64{0.2, 0.4}) {
		t.Errorf("Between() gave the readings of %v, want those of 8:00 and 12:00: none read after now", got)
	}
	got = nil
	for reading := range read.Between(readersAt(0, 8, 0, 1), readersAt(0, 12, 0, 0)) {
		got = append(got, reading.Utilization)
	}
	if len(got) != 0 {
		t.Errorf("Between(after 8:00, to 12:00) gave the readings of %v, want none: from is in its span and to isn't", got)
	}
}

func TestTheRecordsEventsAreTheirLastVersionsOldestFirstAsAReaderGivesThem(t *testing.T) {
	r := heldRecords(t)
	_, _, happened := r.readers(func() time.Time { return readersNow })
	type said struct {
		run   time.Time
		id    int
		until time.Time
	}
	var got []said
	for e := range happened.Between(readersAt(-1, 0, 0, 0), readersAt(1, 0, 0, 0)) {
		got = append(got, said{run: e.Run, id: e.ID, until: e.Until})
	}
	want := []said{
		{run: readersAt(-1, 8, 0, 0), id: 1},
		{run: readersAt(-1, 8, 0, 0), id: 2, until: readersAt(0, 13, 0, 0)},
		{run: readersAt(0, 8, 30, 0), id: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Between() = %+v, want %+v: by when each happened, then its run and id, each as its last version stands, none after now", got, want)
	}
}

func TestAFixtureOverAWorldHandsTheWatchItsWorldsReaders(t *testing.T) {
	now := func() time.Time { return readersNow }
	tests := []struct {
		name                       string
		records                    *records
		lines, readings, happening int
	}{
		{name: "over a world", records: heldRecords(t), lines: 4, readings: 2, happening: 3},
		{name: "over none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Fixture{records: tt.records}.config(wide(37), now)
			lines, _, _ := cfg.Ledger.Today(ledger.Mark{})
			from, to := readersAt(-1, 0, 0, 0), readersAt(1, 0, 0, 0)
			read, happened := slices.Collect(cfg.Readings.Between(from, to)), slices.Collect(cfg.Events.Between(from, to))
			if len(lines) != tt.lines || len(read) != tt.readings || len(happened) != tt.happening {
				t.Errorf("the watch reads %d of today's lines, %d readings and %d events, want %d, %d and %d",
					len(lines), len(read), len(happened), tt.lines, tt.readings, tt.happening)
			}
		})
	}
}
