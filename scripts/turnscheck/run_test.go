package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

func TestTheCheckPrintsCountsAloneOfTheLedgerInTheStateDirectory(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "switchboard")
	dir := ledger.Dir(state)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var file strings.Builder
	for _, line := range []ledger.Line{
		asked("session-work", at(9, 0, 0), 1, "tool_use", opus),
		asked("session-work", at(9, 1, 0), 3, "end_turn", opus),
		asked("", at(9, 2, 0), 1, "end_turn", opus),
	} {
		line.Request, line.Dir, line.Account = "request-work", "/placeholder/project", "work"
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		file.Write(append(data, '\n'))
	}
	file.WriteString(`{"at":"2026-10-05T11:00:00Z","requ`)
	if err := os.WriteFile(filepath.Join(dir, "requests-"+day.Format(time.DateOnly)+".jsonl"), []byte(file.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(name string) string { return map[string]string{"XDG_STATE_HOME": root}[name] }
	noHome := func() (string, error) { return "", os.ErrNotExist }

	var out strings.Builder
	if err := run(&out, getenv, noHome, func() time.Time { return at(18, 0, 0) }); err != nil {
		t.Fatal(err)
	}
	want := `message lines read: 3
  without a session, left out: 1
  without a thread length: 0
the reader's warnings: 1
  of lines it couldn't read: 1

sessions: 1
turns: 1
requests a turn: least 2, median 2, most 2

turns ended by stop reason:
  end_turn: 1

turns ended by cancelling: 0
turns still going: 0

side requests' end_turns passed over, by model:
  none

turns whose first request is shorter than the main thread before them: 0
turns ended that ran over an hour: 0
`
	if out.String() != want {
		t.Errorf("run() printed\n%s\nwant\n%s", out.String(), want)
	}
	for _, of := range []string{"session-work", "request-work", "placeholder", root, "2026", "09:0", "requests-"} {
		if strings.Contains(out.String(), of) {
			t.Errorf("run() printed %q, of a line or where it's kept", of)
		}
	}
}
