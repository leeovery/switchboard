package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// tracedAt is when the traces' tests draw: the frames' moment, 14:42:07,
// 7 seconds into a 30-second slice of a trace 60 cells wide, as at 160
// columns.
var tracedAt = time.Date(2026, 10, 1, 14, 42, 7, 0, time.UTC)

// The sample sessions the traces' tests count, each a Claude Code session's.
const (
	tracedC61B = "c61b2f0a-1d4e-4c8b-9a37-5e0f6b8d2c41"
	tracedD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
)

// sentAgo is a request of the session given going out on the account
// given, ago before tracedAt.
func sentAgo(request, session, account string, ago time.Duration) views.Told {
	return views.Told{Request: request, Session: session, Account: account, Sent: tracedAt.Add(-ago)}
}

// requestLine is the ledger's line of a message of the session given, which
// went upstream attempts times, the client getting its answer from the
// account given, having arrived ago before tracedAt.
func requestLine(request, session, account string, attempts int, ago time.Duration) ledger.Held {
	return ledger.Held{
		At: tracedAt.Add(-ago), Request: request, Kind: ledger.KindMessage, Session: session,
		Model: "claude-opus-5-5", Account: account, Status: 200, Attempts: attempts,
	}
}

// kindOf is l, of the kind given.
func kindOf(l ledger.Held, kind string) ledger.Held {
	l.Kind = kind
	return l
}

// sentTimes are the trace's requests, each as long before tracedAt as it
// went.
func sentTimes(t views.Trace) []time.Duration {
	ago := make([]time.Duration, len(t.Sent))
	for i, at := range t.Sent {
		ago[i] = tracedAt.Sub(at)
	}
	return ago
}

func TestEachCellIsTheMeanOfFourSamplesOfRequestsAMinuteAcrossItsSlice(t *testing.T) {
	// 60 cells over 30 minutes are 30 seconds each, the clock's: the newest
	// from 14:42:00, the one before from 14:41:30. Each sample counts the
	// requests of the minute to it, itself included, at the end of each
	// quarter of its cell's slice.
	trace := views.Trace{Sent: []time.Time{
		time.Date(2026, 10, 1, 14, 41, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 14, 41, 40, 0, time.UTC),
	}}
	cells := trace.Cells(60, tracedAt)

	if len(cells) != 60 {
		t.Fatalf("the trace 60 cells wide has %d, want 60", len(cells))
	}
	// 14:40:30 to 14:41:00 samples 14:41:00 alone after the first request;
	// 14:41:00 to 14:41:30 has it at each sample; 14:41:30 to 14:42:00 has
	// it at 37.5s, both at 45s and 52.5s, and the second alone at 14:42:00,
	// the first a minute old; the newest, at 14:42:07, has the second.
	want := []float64{0, 0.25, 1, 1.5, 1}
	if got := cells[55:]; !slices.Equal(got, want) {
		t.Errorf("the trace's newest cells are %v, want %v", got, want)
	}
	if busy := slices.IndexFunc(cells, func(v float64) bool { return v != 0 }); busy != 56 {
		t.Errorf("the first cell with requests is %d, want 56: those before them are empty", busy)
	}
}

func TestTheNewestCellsSamplesYetToComeAreTakenAtNow(t *testing.T) {
	// Three requests in the minute to 14:42:07: the newest cell, of 14:42:00
	// to 14:42:30, has its four samples at now, so it shows the rate.
	trace := views.Trace{Sent: []time.Time{
		tracedAt.Add(-50 * time.Second), tracedAt.Add(-5 * time.Second), tracedAt,
	}}
	cells := trace.Cells(60, tracedAt)
	if newest := cells[59]; newest != 3 {
		t.Errorf("the newest cell is %v, want 3: the requests of the minute to now", newest)
	}
}

func TestTheRateIsTheRequestsOfTheLastMinute(t *testing.T) {
	trace := views.Trace{Sent: []time.Time{
		tracedAt.Add(-2 * time.Minute),
		tracedAt.Add(-time.Minute),
		tracedAt.Add(-59 * time.Second),
		tracedAt.Add(-time.Second),
		tracedAt,
	}}
	if got := trace.Rate(tracedAt); got != 3 {
		t.Errorf("the rate is %d, want 3: those sent in the minute to now, now's own included, never one a minute old", got)
	}
	if got := (views.Trace{}).Rate(tracedAt); got != 0 {
		t.Errorf("a trace with no requests has a rate of %d, want 0", got)
	}
}

func TestATraceStepsLeftACellAsItsSlicePasses(t *testing.T) {
	// A request every 7 seconds, and a burst, over the last 40 minutes.
	var trace views.Trace
	for at := tracedAt.Add(-40 * time.Minute); !at.After(tracedAt); at = at.Add(7 * time.Second) {
		trace.Sent = append(trace.Sent, at)
	}
	burst := tracedAt.Add(-10 * time.Minute)
	trace.Sent = append(trace.Sent, burst, burst.Add(time.Second), burst.Add(2*time.Second))
	slices.SortFunc(trace.Sent, time.Time.Compare)

	cells := trace.Cells(60, tracedAt)
	// Within the slice, the cells stand still but the newest.
	later := trace.Cells(60, tracedAt.Add(20*time.Second))
	if !slices.Equal(later[:59], cells[:59]) {
		t.Errorf("20 seconds on, within the newest's slice, the cells but the newest are\n%v\nwant them as they were\n%v", later[:59], cells[:59])
	}
	// A slice on, each cell has stepped left one, but the newest, whose
	// slice has ended, filled in.
	next := trace.Cells(60, tracedAt.Add(30*time.Second))
	if !slices.Equal(next[:58], cells[1:59]) {
		t.Errorf("a slice on, the cells are\n%v\nwant those before stepped left a cell\n%v", next[:58], cells[1:59])
	}
}

func TestTheOldestCellCountsTheMinuteBeforeIt(t *testing.T) {
	// The oldest cell's slice starts at 14:12:30, 30 minutes before the
	// newest's ends: its samples at 14:12:37.5, 45 and 52.5 count a request
	// at 14:12:00, 30 minutes and 7 seconds before now; its last, at
	// 14:13:00, a minute after it, doesn't.
	traces := views.Traces{}.Told([]views.Told{sentAgo("0a000001", tracedC61B, "side", 30*time.Minute+7*time.Second)}, tracedAt)
	if got := traces.Accounts["side"].Cells(60, tracedAt)[0]; got != 0.75 {
		t.Errorf("the oldest cell is %v, want 0.75", got)
	}
}

func TestOnlyRequestsThatSpendQuotaAndWentUpstreamAreReadFromTheLines(t *testing.T) {
	lines := []ledger.Held{
		requestLine("0a000001", tracedC61B, "side", 1, 5*time.Minute),
		kindOf(requestLine("0a000002", tracedC61B, "side", 1, 4*time.Minute), ledger.KindCheck),
		kindOf(requestLine("0a000003", tracedC61B, "side", 1, 3*time.Minute), ledger.KindCount),
		// The router answered it itself, as no account had room.
		requestLine("0a000004", tracedC61B, "", 0, 2*time.Minute),
		// Sent again on personal once work refused it.
		requestLine("0a000005", tracedD28C, "personal", 2, time.Minute),
		requestLine("0a000006", "", "side", 1, 30*time.Second),
	}
	traces := views.Traces{}.Read(lines, tracedAt)

	wantAccounts := map[string][]time.Duration{"side": {5 * time.Minute, 30 * time.Second}, "personal": {time.Minute}}
	if len(traces.Accounts) != len(wantAccounts) {
		t.Errorf("the traces are of the accounts %v, want %v", traces.Accounts, wantAccounts)
	}
	for id, want := range wantAccounts {
		if got := sentTimes(traces.Accounts[id]); !slices.Equal(got, want) {
			t.Errorf("%s's trace counts requests sent %v ago, want %v: messages that went upstream, on the account that answered", id, got, want)
		}
	}
	wantSessions := map[string][]time.Duration{tracedC61B: {5 * time.Minute}, tracedD28C: {time.Minute}}
	if len(traces.Sessions) != len(wantSessions) {
		t.Errorf("the traces are of the sessions %v, want %v: a request of none is its account's alone", traces.Sessions, wantSessions)
	}
	for id, want := range wantSessions {
		if got := sentTimes(traces.Sessions[id]); !slices.Equal(got, want) {
			t.Errorf("session %s's trace counts requests sent %v ago, want %v", id[:4], got, want)
		}
	}

	uncounted := []ledger.Held{lines[1], lines[2], lines[3]}
	if allocs := testing.AllocsPerRun(10, func() { traces.Read(uncounted, tracedAt) }); allocs != 0 {
		t.Errorf("reading lines the traces don't count allocates %.0f times, want none: only a line counted is kept", allocs)
	}
}

func TestOnlyRequestsThatSpendQuotaAndWentUpstreamAreCountedFromTheStream(t *testing.T) {
	check := sentAgo("0a000002", tracedC61B, "side", 4*time.Minute)
	check.Check = true
	count := sentAgo("0a000003", tracedC61B, "side", 3*time.Minute)
	count.Count = true
	told := []views.Told{
		sentAgo("0a000001", tracedC61B, "side", 5*time.Minute),
		check,
		count,
		// Told of by an event that doesn't say it went upstream.
		{Request: "0a000004", Session: tracedC61B, Account: "side", Heard: tracedAt},
	}
	traces := views.Traces{}.Told(told, tracedAt)
	if got, want := sentTimes(traces.Accounts["side"]), []time.Duration{5 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("side's trace counts requests sent %v ago, want %v: never a quota check, a count of tokens, or one not sent", got, want)
	}
}

func TestARequestIsCountedOnceWhicheverTellsOfItFirst(t *testing.T) {
	sent := sentAgo("0a000001", tracedC61B, "side", 2*time.Minute)
	// Its line says when it arrived, a moment before it went, and that it
	// took 30 seconds.
	written := requestLine("0a000001", tracedC61B, "side", 1, 2*time.Minute+200*time.Millisecond)
	written.TotalMS = (30 * time.Second).Milliseconds()

	tests := []struct {
		name  string
		count func() views.Traces
	}{
		{name: "the stream, then its line", count: func() views.Traces {
			return views.Traces{}.Told([]views.Told{sent}, tracedAt).Read([]ledger.Held{written}, tracedAt)
		}},
		{name: "its line, then the stream", count: func() views.Traces {
			return views.Traces{}.Read([]ledger.Held{written}, tracedAt).Told([]views.Told{sent}, tracedAt)
		}},
		{name: "the stream, twice", count: func() views.Traces {
			return views.Traces{}.Told([]views.Told{sent}, tracedAt).Told([]views.Told{sent}, tracedAt)
		}},
		{name: "its line, read twice", count: func() views.Traces {
			return views.Traces{}.Read([]ledger.Held{written, written}, tracedAt).Read([]ledger.Held{written}, tracedAt)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traces := tt.count()
			if got := len(traces.Accounts["side"].Sent); got != 1 {
				t.Errorf("side's trace counts the request %d times, want once", got)
			}
			if got := len(traces.Sessions[tracedC61B].Sent); got != 1 {
				t.Errorf("c61b's trace counts the request %d times, want once", got)
			}
		})
	}
}

// A router's ids are unique only while it runs: a line and a request the
// stream told of are one request only where the stream's went out while the
// line's was in flight, and two lines only where they're one line; another
// run's request under the same id counts too.
func TestARequestUnderTheIdOfAnotherRunsCountsToo(t *testing.T) {
	earlier := requestLine("0a000001", tracedC61B, "side", 1, 20*time.Minute)
	earlier.TotalMS = (30 * time.Second).Milliseconds()
	later := requestLine("0a000001", tracedC61B, "side", 1, 5*time.Minute)
	later.TotalMS = (30 * time.Second).Milliseconds()
	sentLater := sentAgo("0a000001", tracedC61B, "side", 5*time.Minute)

	tests := []struct {
		name  string
		count func() views.Traces
		want  []time.Duration
	}{
		{name: "an earlier run's line, then the stream's request", want: []time.Duration{20 * time.Minute, 5 * time.Minute}, count: func() views.Traces {
			return views.Traces{}.Read([]ledger.Held{earlier}, tracedAt).Told([]views.Told{sentLater}, tracedAt)
		}},
		{name: "the stream's request, then an earlier run's line", want: []time.Duration{20 * time.Minute, 5 * time.Minute}, count: func() views.Traces {
			return views.Traces{}.Told([]views.Told{sentLater}, tracedAt).Read([]ledger.Held{earlier}, tracedAt)
		}},
		{name: "two runs' lines", want: []time.Duration{20 * time.Minute, 5 * time.Minute}, count: func() views.Traces {
			return views.Traces{}.Read([]ledger.Held{earlier, later}, tracedAt)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sentTimes(tt.count().Accounts["side"]); !slices.Equal(got, tt.want) {
				t.Errorf("side's trace counts requests sent %v ago, want %v, one a run", got, tt.want)
			}
		})
	}
}

func TestARequestSentAgainOnAnotherAccountCountsThereAsItFirstWent(t *testing.T) {
	traces := views.Traces{}.Told([]views.Told{sentAgo("0a000001", tracedD28C, "work", 3*time.Minute)}, tracedAt)
	traces = traces.Told([]views.Told{sentAgo("0a000001", tracedD28C, "side", 3*time.Minute-2*time.Second)}, tracedAt)

	if got := traces.Accounts["work"].Sent; len(got) != 0 {
		t.Errorf("work, which refused it, counts the request at %v, want none", got)
	}
	if got := sentTimes(traces.Accounts["side"]); !slices.Equal(got, []time.Duration{3 * time.Minute}) {
		t.Errorf("side, which it went out on last, counts it %v ago, want once, 3m ago, as it was first sent", got)
	}
}

func TestASessionMovedCarriesItsWholeTraceAcrossItsAccounts(t *testing.T) {
	traces := views.Traces{}.Read([]ledger.Held{requestLine("0a000001", tracedD28C, "work", 1, 20*time.Minute)}, tracedAt)
	traces = traces.Told([]views.Told{
		sentAgo("0a000002", tracedD28C, "work", 12*time.Minute),
		sentAgo("0a000003", tracedD28C, "side", 10*time.Minute),
	}, tracedAt)

	if got, want := sentTimes(traces.Sessions[tracedD28C]), []time.Duration{20 * time.Minute, 12 * time.Minute, 10 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("d28c's trace counts requests sent %v ago, want %v: every one of its, on either account, the oldest first", got, want)
	}
	if got, want := sentTimes(traces.Accounts["work"]), []time.Duration{20 * time.Minute, 12 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("work's trace counts requests sent %v ago, want %v: those that went out on it", got, want)
	}
	if got, want := sentTimes(traces.Accounts["side"]), []time.Duration{10 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("side's trace counts requests sent %v ago, want %v", got, want)
	}
}

func TestRequestsBeyondWhatTheTracesReachAreLetGo(t *testing.T) {
	traces := views.Traces{}.Read([]ledger.Held{requestLine("0a000001", tracedC61B, "side", 1, 40*time.Minute)}, tracedAt)
	traces = traces.Told([]views.Told{sentAgo("0a000002", tracedD28C, "work", 45*time.Minute)}, tracedAt)
	if len(traces.Accounts) != 0 || len(traces.Sessions) != 0 {
		t.Errorf("of requests 40 and 45 minutes old, the traces are %v and %v, want none", traces.Accounts, traces.Sessions)
	}

	traces = views.Traces{}.Told([]views.Told{sentAgo("0a000003", tracedC61B, "side", 5*time.Minute)}, tracedAt)
	traces = traces.Told([]views.Told{sentAgo("0a000004", tracedC61B, "side", -35*time.Minute)}, tracedAt.Add(35*time.Minute))
	if got := len(traces.Accounts["side"].Sent); got != 1 {
		t.Errorf("40 minutes after a request, as another is sent, side's trace counts %d, want the new one alone", got)
	}
}

func TestATraceOfNoCellsHasNone(t *testing.T) {
	trace := views.Trace{Sent: []time.Time{tracedAt}}
	for _, n := range []int{0, -1} {
		if got := trace.Cells(n, tracedAt); len(got) != 0 {
			t.Errorf("a trace %d cells wide has %d, want none", n, len(got))
		}
	}
}
