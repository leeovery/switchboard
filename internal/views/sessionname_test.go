package views_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The session of the names' tests, and its id as status.ShortID cuts it.
const (
	namedSession = "9a7c41e2-0000-4000-8000-000000000001"
	namedShortID = "9a7c41e2"
)

func TestASessionIsShownByItsTerminalNameElseItsWorktreesElseItsShortID(t *testing.T) {
	tests := []struct {
		name      string
		dir, told string
		want      views.ShownName
	}{
		{name: "its terminal session's", dir: "~/Code/api", told: "deploy", want: views.ShownName{Name: "deploy"}},
		{
			name: "its terminal session's, in a worktree, unmarked", dir: "~/Code/api/.claude/worktrees/fix-login", told: "deploy",
			want: views.ShownName{Name: "deploy"},
		},
		{name: "its worktree's, marked", dir: "~/Code/api/.claude/worktrees/fix-login", want: views.ShownName{Name: "fix-login", Worktree: true}},
		{
			name: "its worktree's, started in a folder within it", dir: "~/Code/api/.claude/worktrees/fix-login/web/src",
			want: views.ShownName{Name: "fix-login", Worktree: true},
		},
		{
			name: "the innermost worktree's, of one made in another's", dir: "~/Code/api/.claude/worktrees/fix-login/.claude/worktrees/agent-1/web",
			want: views.ShownName{Name: "agent-1", Worktree: true},
		},
		{name: "its id, cut short, in its repository", dir: "~/Code/api", want: views.ShownName{Name: namedShortID}},
		{name: "its id, in a subfolder of its repository", dir: "~/Code/api/web", want: views.ShownName{Name: namedShortID}},
		{name: "its id, in no directory", want: views.ShownName{Name: namedShortID}},
		{name: "its id, in a folder named as the worktrees' folder is", dir: "/.claude/worktrees/x", want: views.ShownName{Name: namedShortID}},
		{name: "a name cleaned, as what comes from elsewhere is", told: "fix\x1b[2J\tlogin", want: views.ShownName{Name: "fix [2J login"}},
		{name: "a name of spaces alone, none", dir: "~/Code/api", told: " \t ", want: views.ShownName{Name: namedShortID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := views.ListedSession{Session: namedSession, Dir: tt.dir, Name: tt.told}
			if got := s.ShownName(); got != tt.want {
				t.Errorf("ShownName() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTheListNamesASessionAsTheRouterDoesElseAsItsLinesLastDid(t *testing.T) {
	tests := []struct {
		name        string
		assignments []status.Assignment
		want        string
	}{
		{name: "the router's, of the model seen most lately that names a directory", assignments: []status.Assignment{
			{Model: opus, Account: "work", Dir: "~/Code/api", SessionName: "first", LastSeen: october(7, 12, 0)},
			{Model: haiku, Account: "work", Dir: "~/Code/api", SessionName: "review", LastSeen: october(7, 13, 0)},
		}, want: "review"},
		{name: "none, where the router's names none beside its directory", assignments: []status.Assignment{
			{Model: opus, Account: "work", Dir: "~/Code/api", LastSeen: now},
		}},
		{name: "its lines', where the router names no directory", assignments: []status.Assignment{{Model: opus, Account: "work"}}, want: "deploy"},
		{name: "its lines', without the router", want: "deploy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{now: now, lines: []ledger.Line{
				startedIn("first", in("~/Code/api", line(mover, opus, "work", "new", october(7, 9, 0), hourUsage))),
				startedIn("deploy", in("~/Code/api", line(mover, opus, "work", "sticky", october(7, 10, 0), hourUsage))),
				line(mover, opus, "work", "sticky", october(7, 11, 0), hourUsage),
			}}
			var running []status.Session
			if tt.assignments != nil {
				running = []status.Session{{ID: mover, Assignments: tt.assignments}}
			}
			got := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
			if len(got.Sessions) != 1 || got.Sessions[0].Name != tt.want {
				t.Errorf("the List holds %+v, want one session named %q", got.Sessions, tt.want)
			}
		})
	}
}

func TestTheLogNamesEachSessionOfThePeriodAsItsLastLineToNameAPlaceNamedIt(t *testing.T) {
	inWorktree := "/code/switchboard/.claude/worktrees/fix"
	days := heldDays(now, slices.Concat(
		namedAsks("deploy", projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "side", october(5, 9, 0))),
		namedAsks("review", projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "side", october(6, 9, 0))),
		projectAsks(worktreeSession, inWorktree, "/code/switchboard", "work", october(6, 10, 0)),
		namedAsks("docs", projectAsks(ownSession, "/code/own/api", "/code/own/api", "work", october(5, 12, 0))),
		projectAsks(ownSession, "/code/own/api", "/code/own/api", "work", october(7, 12, 0)),
		projectAsks(clientSession, "/code/client/api", "/code/client/api", "work", october(7, 12, 30)),
	)...)

	log := days.SessionLog(projectsIn(views.PeriodThisWeek, now))

	type shown struct {
		project string
		views.ShownName
	}
	want := map[string]shown{
		repoSession:     {project: "switchboard", ShownName: views.ShownName{Name: "review"}},
		worktreeSession: {project: "switchboard", ShownName: views.ShownName{Name: "fix", Worktree: true}},
		ownSession:      {project: "own/api", ShownName: views.ShownName{Name: status.ShortID(ownSession)}},
		clientSession:   {project: "client/api", ShownName: views.ShownName{Name: status.ShortID(clientSession)}},
	}
	got := make(map[string]shown)
	for _, s := range log.Sessions {
		got[s.Session] = shown{project: s.ProjectName, ShownName: s.ShownName()}
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("the log shows %s as %+v, want %+v", id, got[id], w)
		}
	}
}

// startedIn is l, of a session started in the terminal session given.
func startedIn(name string, l ledger.Line) ledger.Line {
	l.Name = name
	return l
}

// namedAsks are lines, each of a session started in the terminal session
// given.
func namedAsks(name string, lines []ledger.Held) []ledger.Held {
	for i := range lines {
		lines[i].Name = name
	}
	return lines
}
