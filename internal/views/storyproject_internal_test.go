package views

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// A session's story holds the directory its last request to name one named,
// and the project its last request to name either named, so its project is
// always that of the directory named with it: a line from before run told a
// project names none, and the session's project is then its directory's, as
// status.ProjectOf has it.
func TestAStoryHoldsTheProjectItsLastRequestToNameAPlaceNamed(t *testing.T) {
	day := LedgerDay{Start: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Date: "2026-10-07"}
	steps := []struct {
		name         string
		dir, project string
		// want are the story's directory and project once it has taken in
		// the request, and wantProject the session's project, as
		// status.ProjectOf finds it.
		wantDir, wantProject, wantOf string
	}{
		{
			name: "naming both", dir: "~/Code/api/cmd", project: "~/Code/api",
			wantDir: "~/Code/api/cmd", wantProject: "~/Code/api", wantOf: "~/Code/api",
		},
		{name: "naming neither", wantDir: "~/Code/api/cmd", wantProject: "~/Code/api", wantOf: "~/Code/api"},
		{
			name: "a line from before projects, a worktree Claude Code made", dir: "~/Code/web/.claude/worktrees/fix-login",
			wantDir: "~/Code/web/.claude/worktrees/fix-login", wantOf: "~/Code/web",
		},
		{
			name: "a project alone, the directory kept", project: "~/Code/api",
			wantDir: "~/Code/web/.claude/worktrees/fix-login", wantProject: "~/Code/api", wantOf: "~/Code/api",
		},
	}
	var lines []ledger.Line
	for i, step := range steps {
		lines = append(lines, ledger.Line{
			At: day.Start.Add(time.Duration(i+1) * time.Minute), Request: fmt.Sprintf("r%d", i+1), Kind: ledger.KindMessage, Session: "s",
			Dir: step.dir, Project: step.project, Model: foldOpus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 3000,
		})
		s := storyOf(heldRequestsOf(lines), ledger.Pricing, day)
		if s.dir != step.wantDir || s.project != step.wantProject {
			t.Errorf("%s: the story holds the directory %q and project %q, want %q and %q", step.name, s.dir, s.project, step.wantDir, step.wantProject)
		}
		if got := status.ProjectOf(s.project, s.dir); got != step.wantOf {
			t.Errorf("%s: the session's project is %q, want %q", step.name, got, step.wantOf)
		}
	}
}

// A session's state a digest holds stores its project beside its directory,
// and one stored before projects, holding none, restores with none: the
// session's project is then its directory's, as status.ProjectOf has it, as
// its lines, which named none, would give it read whole.
func TestASessionsStateStoresItsProjectAndOneFromBeforeRestoresWithItsDirectorysProject(t *testing.T) {
	day := LedgerDay{Start: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Date: "2026-10-07"}
	next := LedgerDay{Start: day.Start.AddDate(0, 0, 1), Date: "2026-10-08"}
	worktree := "~/Code/api/.claude/worktrees/fix-login"
	tests := []struct {
		name, project string
		// wantKey is whether the stored state holds the project's key, and
		// want the session's project once restored.
		wantKey bool
		want    string
	}{
		{name: "a project told", project: "~/Code/api", wantKey: true, want: "~/Code/api"},
		{name: "none told, as before projects", want: "~/Code/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []ledger.Line{{
				At: day.Start.Add(time.Hour), Request: "r1", Kind: ledger.KindMessage, Session: "s", Dir: worktree, Project: tt.project,
				Model: foldOpus, Account: "work", Reason: "new", Status: 200, Attempts: 1, TotalMS: 3000,
			}}
			held := readOf(lines, nil, ledger.Pricing, day, endOfDay(day), false)
			st, err := held.stored()
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(st)
			if err != nil {
				t.Fatal(err)
			}
			if holds := strings.Contains(string(data), `"pj":`); holds != tt.wantKey {
				t.Errorf("stored as %s, holding a project: %v, want %v", data, holds, tt.wantKey)
			}

			var back storedSession
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatal(err)
			}
			restored, err := back.restored(ledger.Pricing, next)
			if err != nil {
				t.Fatal(err)
			}
			if s := restored.story; s.dir != worktree || s.project != tt.project {
				t.Errorf("restored in %q, of the project %q, want %q, of %q", s.dir, s.project, worktree, tt.project)
			}
			if got := status.ProjectOf(restored.story.project, restored.story.dir); got != tt.want {
				t.Errorf("restored, the session's project is %q, want %q", got, tt.want)
			}
		})
	}
}
