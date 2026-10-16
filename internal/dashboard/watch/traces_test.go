package watch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// shiftTabKey is shift+tab, as the terminal sends it.
var shiftTabKey = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

// todays is a request ledger whose today's lines are those the test has
// written by each read of them, every one, whatever the Mark, as a ledger
// whose day's file was replaced gives them; and which notes how many lines
// each read gave.
type todays struct {
	ledger.Empty
	written []ledger.Held
	given   []int
}

func (l *todays) Today(ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	l.given = append(l.given, len(l.written))
	return slices.Clone(l.written), ledger.Mark{}, true
}

// followed is a request ledger followed as it grows, as the dashboard
// follows it, which notes how many lines each read of today's gave.
type followed struct {
	views.Ledger
	given []int
}

func (l *followed) Today(mark ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	lines, next, afresh := l.Ledger.Today(mark)
	l.given = append(l.given, len(lines))
	return lines, next, afresh
}

// tracedHarness is routedHarness of three, its watch reading the ledger
// given.
func tracedHarness(t *testing.T, l views.Ledger) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.model.cfg.Ledger = l
	return h
}

// arrived is the ledger's line of a message of d28c's opus, the client
// getting its answer from the account given, having arrived d past when
// the clock starts, and gone upstream once.
func arrived(request, account string, d time.Duration) ledger.Held {
	return ledger.Held{
		At: past(d), Request: request, Kind: ledger.KindMessage, Session: idD28C, Model: opus,
		Account: account, Reason: "sticky", Status: 200, Attempts: 1,
	}
}

// took is the line, its request having taken d from when it arrived to when
// it ended.
func took(l ledger.Held, d time.Duration) ledger.Held {
	l.TotalMS = d.Milliseconds()
	return l
}

// counted are the requests the trace with the given id among traces counts,
// each as long before the clock started as it went.
func counted(traces map[string]views.Trace, id string) []time.Duration {
	ago := []time.Duration{}
	for _, at := range traces[id].Sent {
		ago = append(ago, start.Sub(at))
	}
	return ago
}

// tracedOn are the requests the frame's traces count on the account with
// the given id, as counted has them.
func tracedOn(h *harness, account string) []time.Duration {
	return counted(h.model.frame(h.clock.now).Traces.Accounts, account)
}

func TestTheTracesCountTodaysLinesOnceTheStreamOpens(t *testing.T) {
	check := arrived("0a000003", "work", -3*time.Minute)
	check.Kind = ledger.KindCheck
	l := &todays{written: []ledger.Held{
		arrived("0a000001", "work", -5*time.Minute),
		arrived("0a000002", "side", -2*time.Minute),
		check,
		arrived("0a000004", "side", -40*time.Minute),
	}}
	h := tracedHarness(t, l)
	h.start()

	if h.opened() != 1 || !slices.Equal(l.given, []int{4}) {
		t.Fatalf("the stream opened %d times, and today's lines were read %v; want it opened once, and them read once", h.opened(), l.given)
	}
	for account, want := range map[string][]time.Duration{"work": {5 * time.Minute}, "side": {2 * time.Minute}} {
		if got := tracedOn(h, account); !slices.Equal(got, want) {
			t.Errorf("the frame's traces count requests on %s sent %v ago, want %v: those of today's lines that spent quota, in the last 30 minutes", account, got, want)
		}
	}
	if got, want := counted(h.model.frame(h.clock.now).Traces.Sessions, idD28C), []time.Duration{5 * time.Minute, 2 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("the frame's traces count d28c's requests sent %v ago, want %v", got, want)
	}
}

func TestTheStreamsRequestsAreCountedOnceEachWhicheverTellsOfThemFirst(t *testing.T) {
	l := &todays{written: []ledger.Held{arrived("0a000001", "work", -5*time.Minute)}}
	h := tracedHarness(t, l)
	h.start()

	h.hear(
		alter(told(router.StreamInFlight, "0a000002", "work", 0), func(e *router.StreamEvent) { e.SentAt = past(-30 * time.Second) }),
		told(router.StreamSent, "0a000003", "side", 0),
		alter(told(router.StreamSent, "0a000004", "work", 0), func(e *router.StreamEvent) { e.Check = true }),
		alter(told(router.StreamInFlight, "0a000005", "side", 0), func(e *router.StreamEvent) { e.SentAt, e.Count = past(-20*time.Second), true }),
		alter(told(router.StreamSent, "0a000006", "side", 0), func(e *router.StreamEvent) { e.Count = true }),
		told(router.StreamFirst, "0a000003", "side", time.Second),
		alter(told(router.StreamProgress, "0a000003", "side", time.Second), func(e *router.StreamEvent) { e.Chars = 4936 }),
	)
	want := map[string][]time.Duration{"work": {5 * time.Minute, 30 * time.Second}, "side": {0}}
	for account, w := range want {
		if got := tracedOn(h, account); !slices.Equal(got, w) {
			t.Errorf("as the stream tells of them, the traces count requests on %s sent %v ago, want %v: one in flight as it opened, from when it was sent, and one going out; never the quota check, a count of tokens, or an answer's progress", account, got, w)
		}
	}

	// The stream ends, the requests end, and their lines are written; it's
	// joined again, and today's lines read afresh.
	h.hear(alter(told(router.StreamDone, "0a000003", "side", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	h.endStream()
	l.written = append(l.written, took(arrived("0a000002", "work", -31*time.Second), 31*time.Second), took(arrived("0a000003", "side", -time.Second/5), 2200*time.Millisecond))
	rejoin, ok := h.pendingRejoin()
	if !ok {
		t.Fatal("once the stream ended, no rejoin was armed")
	}
	h.fire(rejoin)
	if !slices.Equal(l.given, []int{1, 1, 3, 3}) {
		t.Fatalf("today's lines were read %v, want once as the stream opened, and once by the tail as it told of a request ended, whose line it awaits; then again as it was joined again, and by the tail, as it opened again", l.given)
	}
	for account, w := range want {
		if got := tracedOn(h, account); !slices.Equal(got, w) {
			t.Errorf("their lines read too, the traces count requests on %s sent %v ago, want each once, as the stream first told of it, %v", account, got, w)
		}
	}
}

func TestEachTimeTheStreamOpensTheTracesReadWhatTodaysLinesGained(t *testing.T) {
	state := t.TempDir()
	write := func(lines ...ledger.Held) {
		t.Helper()
		writeLines(t, state, lines...)
	}
	write(arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	l := &followed{}
	h := tracedHarness(t, l)
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.start()

	// Away from the Overview the stream closes; back, it opens again.
	write(arrived("0a000003", "work", -3*time.Minute))
	h.keys(tabKey)
	if got, want := tracedOn(h, "work"), []time.Duration{5 * time.Minute}; !slices.Equal(got, want) {
		t.Errorf("the stream closed, the traces count requests on work sent %v ago, want %v, kept", got, want)
	}
	h.keys(shiftTabKey)

	// The stream ends, as the router restarts, and is joined again.
	write(arrived("0a000004", "side", -2*time.Minute))
	h.endStream()
	rejoin, ok := h.pendingRejoin()
	if !ok {
		t.Fatal("once the stream ended, no rejoin was armed")
	}
	h.fire(rejoin)

	if h.opened() != 3 || !slices.Equal(l.given, []int{2, 1, 1}) {
		t.Errorf("the stream opened %d times, today's lines read giving %v lines; want it opened 3 times, the lines each time, each read giving what the day's file gained since the last, [2 1 1]", h.opened(), l.given)
	}
	for account, want := range map[string][]time.Duration{"work": {5 * time.Minute, 3 * time.Minute}, "side": {4 * time.Minute, 2 * time.Minute}} {
		if got := tracedOn(h, account); !slices.Equal(got, want) {
			t.Errorf("the traces count requests on %s sent %v ago, want %v", account, got, want)
		}
	}
}

func TestTodaysLinesArentReadWhileTheStreamDoesntOpen(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T, l views.Ledger) *harness
	}{
		{name: "probing, without the router", start: func(t *testing.T, l views.Ledger) *harness {
			h := newHarness(t, probedWithoutTheRouter())
			h.model.cfg.Ledger = l
			return h
		}},
		{name: "a stream that won't open", start: func(t *testing.T, l views.Ledger) *harness {
			h := tracedHarness(t, l)
			h.source.streamErr = errors.New("ask the router: connection refused")
			return h
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &todays{written: []ledger.Held{arrived("0a000001", "work", -5*time.Minute)}}
			h := tt.start(t, l)
			h.start()
			if len(l.given) != 0 {
				t.Errorf("today's lines were read %d times, want none", len(l.given))
			}
		})
	}
}

func TestAWatchWithNoLedgerCountsWhatTheStreamTells(t *testing.T) {
	h := tracedHarness(t, nil)
	h.start()
	h.hear(told(router.StreamSent, "0a000001", "work", 0))
	if got, want := tracedOn(h, "work"), []time.Duration{0}; !slices.Equal(got, want) {
		t.Errorf("with no ledger to read, the traces count requests on work sent %v ago, want %v", got, want)
	}
}

// writeLines writes the lines to the request ledger in the state directory
// given, each in the file of the day it arrived on, as the router writes them.
func writeLines(t *testing.T, state string, lines ...ledger.Held) {
	t.Helper()
	dir := ledger.Dir(state)
	for _, d := range []string{dir, readings.Dir(state)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range lines {
		data, err := json.Marshal(l.Line)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(dir, "requests-"+dayfile.DateOf(l.At)+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(append(data, '\n'))
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
