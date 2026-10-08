package router_test

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestRunFilesItsEventsBesideTheLedger(t *testing.T) {
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	stop := runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))
	health, err := client.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	doc := waitForStatus(t, router.SocketPath(cfg.StateDir), func(doc status.Document) bool { return len(doc.Events) == 1 })
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	file := filepath.Join(cfg.StateDir, "ledger", "events-"+now.Local().Format(time.DateOnly)+".jsonl")
	if info, err := os.Stat(file); err != nil || info.Mode() != 0o600 {
		t.Errorf("the events' file: %v, %v, want it in the ledger's directory, mode %v", info, err, fs.FileMode(0o600))
	}
	want := []events.Line{{Event: doc.Events[0], Run: health.StartedAt}}
	if got := readEvents(cfg.StateDir); !reflect.DeepEqual(got, want) {
		t.Errorf("the events filed read back as\n%+v\nwant\n%+v: the session started, its run when the router started, as its health gives it", got, want)
	}
}

func TestTwoRunsEventsAreReadBackApart(t *testing.T) {
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	var runs []time.Time
	// A session of its own starts in each run, as the next run remembers the
	// first's.
	for i, session := range []string{sessionID, "7e1d2c3b-4a59-4f68-8d7c-6b5a49382716"} {
		started := now.Add(time.Duration(i) * time.Minute)
		cfg.Now = func() time.Time { return started }
		cfg.Listen = freeAddress(t)
		stop := runRouter(t, cfg)
		readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", with(claudeCode(workToken), "X-Claude-Code-Session-Id", session), strings.NewReader(messages)))
		waitForStatus(t, router.SocketPath(cfg.StateDir), func(doc status.Document) bool { return len(doc.Events) == 1 })
		if err := stop(); err != nil {
			t.Fatalf("Run() = %v", err)
		}
		runs = append(runs, started)
	}

	got := readEvents(cfg.StateDir)
	if len(got) != 2 {
		t.Fatalf("the events filed read back as %+v, want each run's", got)
	}
	for i, line := range got {
		if line.ID != 1 || !line.Run.Equal(runs[i]) || line.Kind != status.EventStarted {
			t.Errorf("the events filed read back as %+v, want each run's session started, its id 1, apart by their runs, %v", got, runs)
		}
	}
}

func TestARouterRestartingInPlaceFilesItsEventsFirst(t *testing.T) {
	s := newSelfWatching(t, true)
	s.cfg.Upstream = newUpstream(t, answerOK).URL
	execs := replacing(&s.cfg)
	exec := s.cfg.Exec
	var filed []events.Line
	s.cfg.Exec = func(path string, argv, env []string) error {
		filed = readEvents(s.cfg.StateDir)
		return exec(path, argv, env)
	}
	r := startRouter(t, s.cfg)

	s.upgrade(t)
	r.waitForExit(t)
	execs.only(t)
	kinds := make([]string, len(filed))
	for i, line := range filed {
		kinds[i] = line.Kind
	}
	if !slices.Contains(kinds, status.EventRestart) {
		t.Errorf("as the router replaced itself, the events filed were %+v, want the restart it told of among them", filed)
	}
}

// readEvents reads back the events filed in the state directory stateDir,
// each as it last stands, of the day either side of now.
func readEvents(stateDir string) []events.Line {
	reader := events.NewReader(stateDir, func() time.Time { return now.Add(24 * time.Hour) }, slog.New(slog.DiscardHandler))
	return slices.Collect(reader.Between(now.Add(-24*time.Hour), now.Add(24*time.Hour)))
}
