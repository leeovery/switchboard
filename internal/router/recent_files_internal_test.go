package router

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// startedAt is when the router that told the events in these tests started,
// in a zone of its own, as a router's clock reads.
var startedAt = start.Add(-time.Hour).In(time.FixedZone("BST", 3600))

func TestEachEventIsFiledAsTheDocumentGivesIt(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	files := fileEvents(t, r)
	until := start.Add(time.Hour)
	for _, e := range []Event{
		SessionStarted{Session: "0b5c6f2e", Model: opus, Account: "work", Reason: "new"},
		LimitReached{Account: "work", Windows: []string{"5h"}, Until: until, Limit: 1},
		forced("0b5c6f2e", "work", "side"),
		Refused{Account: "side", Status: http.StatusForbidden, Family: "opus", Until: until, Request: someRequest},
		HealthChanged{Reason: "5 of the 5 requests in the last 5 minutes failed"},
		Primed{Account: "side", Window: "5h", ResetsAt: until},
		RestartDue{Reason: "upgraded"},
	} {
		r.hear(e)
	}
	// Work's session is used fast enough to run out before it resets, then
	// side's limit lifts.
	opened := start.Add(-time.Hour)
	r.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: opened.Add(5 * time.Hour), Status: quota.StatusAllowed}, week}, r.state.mark())
	r.state.record("side", []quota.Window{session, week}, r.state.mark())
	r.state.limit("side", nil, start.Add(time.Minute), r.state.mark())
	r.look()
	r.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.95, ResetsAt: opened.Add(5 * time.Hour), Status: quota.StatusAllowed}, week}, r.state.mark())
	clock.now = start.Add(time.Minute)
	r.look()

	got := files.read()
	if want := asFiled(r.events()); !reflect.DeepEqual(got, want) {
		t.Errorf("the files hold\n%+v\nwant\n%+v: each event as the document gives it, its run when the router started, in UTC", got, want)
	}
	var kinds []string
	for _, line := range got {
		kinds = append(kinds, line.Kind)
	}
	for _, kind := range []string{status.EventStarted, status.EventLimit, status.EventMoved, status.EventRefused, status.EventHealth, status.EventPrimed, status.EventRestart, status.EventPressure, status.EventRoom} {
		if !slices.Contains(kinds, kind) {
			t.Errorf("the files hold the kinds %q, want one of %s", kinds, kind)
		}
	}
}

func TestAnEventIsFiledAgainAsItChangesAndReadBackAsItLastStands(t *testing.T) {
	until := start.Add(time.Hour)
	limit := LimitReached{Account: "work", Windows: []string{"5h"}, Until: until, Limit: 1}
	refused := func(account, request string) Refused {
		return Refused{Account: account, Status: http.StatusForbidden, Family: "opus", Until: until, Request: request}
	}
	tests := []struct {
		name  string
		heard []Event
		// versions are how many versions of each event are filed, by id.
		versions map[int]int
	}{
		{
			name:     "a limit reached again in another window, as it's joined",
			heard:    []Event{limit, again(limit, []string{"7d", "5h"}, until)},
			versions: map[int]int{1: 2},
		},
		{
			name:     "a limit reached again till later, as it's joined",
			heard:    []Event{limit, again(limit, []string{"5h"}, until.Add(time.Hour))},
			versions: map[int]int{1: 2},
		},
		{
			name:     "a limit reached again, changing nothing",
			heard:    []Event{limit, again(limit, []string{"5h"}, until)},
			versions: map[int]int{1: 1},
		},
		{
			name:     "a limit counting a move after it",
			heard:    []Event{limit, forced("one", "work", "side")},
			versions: map[int]int{1: 2, 2: 1},
		},
		{
			name:     "a limit counting a move to another account, so to several",
			heard:    []Event{limit, forced("one", "work", "side"), forced("two", "work", "personal")},
			versions: map[int]int{1: 3, 2: 1, 3: 1},
		},
		{
			name:     "a limit counting a session it moved already, changing nothing",
			heard:    []Event{limit, forced("one", "work", "side"), forcedModel("one", haiku, "work", "side")},
			versions: map[int]int{1: 2, 2: 1, 3: 1},
		},
		{
			name:     "a move counted once its limit is heard of",
			heard:    []Event{forced("one", "work", "side"), limit},
			versions: map[int]int{1: 2, 2: 1},
		},
		{
			name:     "a refusal lifting early",
			heard:    []Event{refused("work", someRequest), RefusalLifted{Account: "work", Family: "opus", Request: someRequest}},
			versions: map[int]int{1: 2},
		},
		{
			name: "several refusals lifting early at once",
			heard: []Event{
				Refused{Account: "work", Status: http.StatusUnauthorized, Until: until},
				Refused{Account: "work", Status: http.StatusUnauthorized, Until: until},
				RefusalLifted{Account: "work"},
			},
			versions: map[int]int{1: 2, 2: 2},
		},
		{
			name:     "another request's refusal lifting, changing nothing",
			heard:    []Event{refused("work", someRequest), RefusalLifted{Account: "work", Family: "opus", Request: "e5f6a7b8"}},
			versions: map[int]int{1: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newTestRecent(clock)
			files := fileEvents(t, r)
			for _, e := range tt.heard {
				r.hear(e)
				clock.now = clock.now.Add(time.Second)
			}

			got := files.read()
			if want := asFiled(r.events()); !reflect.DeepEqual(got, want) {
				t.Errorf("the files read back\n%+v\nwant\n%+v: each event once, as it last stands", got, want)
			}
			if got := files.versions(); !reflect.DeepEqual(got, tt.versions) {
				t.Errorf("the files hold %v versions of each event, by id, want %v", got, tt.versions)
			}
		})
	}
}

func TestEventsKeptBeforeTheFilesAreGivenAreFiledAsTheyStand(t *testing.T) {
	r := newTestRecent(&testClock{now: start})
	limit := LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 1}
	r.hear(forced("one", "work", "side"))
	r.hear(limit)
	r.hear(again(limit, []string{"5h", "7d"}, start.Add(2*time.Hour)))

	files := fileEvents(t, r)
	r.hear(RestartDue{Reason: "upgraded"})
	got := files.read()
	if want := asFiled(r.events()); !reflect.DeepEqual(got, want) {
		t.Errorf("the files read back\n%+v\nwant\n%+v", got, want)
	}
	if got, want := files.versions(), map[int]int{1: 1, 2: 1, 3: 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("the files hold %v versions of each event, by id, want %v: those kept before, once each, as they then stood", got, want)
	}
}

// again is limit reached again while it holds, in windows, till until.
func again(limit LimitReached, windows []string, until time.Time) LimitReached {
	limit.Windows, limit.Until, limit.Again = windows, until, true
	return limit
}

// asFiled is the events the document gives, newest first, as the files read
// them back: oldest first, told by the router that started at startedAt.
func asFiled(given []status.Event) []events.Line {
	var lines []events.Line
	for _, e := range slices.Backward(given) {
		lines = append(lines, events.Line{Event: e, Run: startedAt.UTC()})
	}
	return lines
}

// eventFiles are the files a test's events are filed in, in a state
// directory of its own.
type eventFiles struct {
	t        *testing.T
	stateDir string
	stop     func()
}

// fileEvents has r file its events in a state directory of the test's, as
// told by the router that started at startedAt, writing on start's day.
func fileEvents(t *testing.T, r *recent) *eventFiles {
	t.Helper()
	stateDir := t.TempDir()
	w := events.Open(ledger.Dir(stateDir), config.DefaultLedgerKeep, at(start), logger)
	r.fileTo(w, startedAt)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return &eventFiles{t: t, stateDir: stateDir, stop: stop}
}

// read stops the filing, once it has written what's noted, and returns the
// events read back, each as it last stands.
func (f *eventFiles) read() []events.Line {
	f.stop()
	reader := events.NewReader(f.stateDir, at(start.Add(day)), logger)
	return slices.Collect(reader.Between(start.Add(-day), start.Add(day)))
}

// versions returns how many lines the files hold of each event, by its id,
// once the filing has stopped.
func (f *eventFiles) versions() map[int]int {
	f.t.Helper()
	f.stop()
	paths, err := filepath.Glob(filepath.Join(ledger.Dir(f.stateDir), "events-*.jsonl"))
	if err != nil {
		f.t.Fatal(err)
	}
	versions := make(map[int]int)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			f.t.Fatal(err)
		}
		lines := bufio.NewScanner(file)
		for lines.Scan() {
			line, ok := events.In(lines.Bytes())
			if !ok {
				f.t.Errorf("%s holds the line %s, want an event", path, lines.Bytes())
			}
			versions[line.ID]++
		}
		_ = file.Close()
	}
	return versions
}
