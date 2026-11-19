package views_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The session of the names' tests, and its id as the dashboard shows it.
const (
	namedSession = "9a7c41e2-0000-4000-8000-000000000001"
	namedShownID = "9a7c"
)

func TestASessionIsShownByItsTerminalNameElseItsWorktreesElseItsID(t *testing.T) {
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
		{name: "its id, as the dashboard shows one, in its repository", dir: "~/Code/api", want: views.ShownName{Name: namedShownID}},
		{name: "its id, in a subfolder of its repository", dir: "~/Code/api/web", want: views.ShownName{Name: namedShownID}},
		{name: "its id, in no directory", want: views.ShownName{Name: namedShownID}},
		{name: "its id, in a folder named as the worktrees' folder is", dir: "/.claude/worktrees/x", want: views.ShownName{Name: namedShownID}},
		{name: "a name cleaned, as what comes from elsewhere is", told: "fix\x1b[2J\tlogin", want: views.ShownName{Name: "fix [2J login"}},
		{name: "a name of spaces alone, none", dir: "~/Code/api", told: " \t ", want: views.ShownName{Name: namedShownID}},
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
		ownSession:      {project: "own/api", ShownName: views.ShownName{Name: status.ShownID(ownSession)}},
		clientSession:   {project: "client/api", ShownName: views.ShownName{Name: status.ShownID(clientSession)}},
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

// Rows of one list that show the same project and name each show their id
// too, as much of it as tells them apart, here whole, as the two differ only
// at their ends, on the List and on a project's SESSIONS alike; a row whose
// name no other row shows, one of another project, and one shown by its id,
// don't.
func TestRowsThatShowAlikeShowTheirIDs(t *testing.T) {
	days := heldDays(now, slices.Concat(
		namedAsks("deploy", projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "side", october(6, 9, 0))),
		namedAsks("deploy", projectAsks(worktreeSession, "/code/switchboard/.claude/worktrees/fix", "/code/switchboard", "work", october(6, 10, 0))),
		namedAsks("deploy", projectAsks(ownSession, "/code/own/api", "/code/own/api", "work", october(6, 11, 0))),
		projectAsks(clientSession, "/code/own/api", "/code/own/api", "work", october(6, 12, 0)),
	)...)
	in := projectsIn(views.PeriodThisWeek, now)
	listed := map[string]views.ShownName{
		repoSession:     {Name: "deploy", ID: repoSession},
		worktreeSession: {Name: "deploy", ID: worktreeSession},
		ownSession:      {Name: "deploy"},
		clientSession:   {Name: status.ShownID(clientSession)},
	}
	if got := rowsShown(days.SessionLog(in).Sessions); !maps.Equal(got, listed) {
		t.Errorf("the List shows %+v, want %+v", got, listed)
	}
	page, ok := days.Projects(in).Project("/code/switchboard")
	if !ok {
		t.Fatal("Project(/code/switchboard) found none")
	}
	want := map[string]views.ShownName{repoSession: listed[repoSession], worktreeSession: listed[worktreeSession]}
	if got := rowsShown(page.Sessions); !maps.Equal(got, want) {
		t.Errorf("switchboard's SESSIONS shows %+v, want %+v", got, want)
	}
}

// A session's page names its project as Projects names one alone, and the
// session as the List does, but where that's by its id, which its heading
// shows already.
func TestASessionsPageNamesItsProjectAndItsName(t *testing.T) {
	tests := []struct {
		name, dir, told string
		wantProject     string
		want            views.ShownName
	}{
		{name: "its terminal session's", dir: "~/Code/api", told: "deploy", wantProject: "api", want: views.ShownName{Name: "deploy"}},
		{name: "its worktree's", dir: "~/Code/api/.claude/worktrees/fix-login", wantProject: "api", want: views.ShownName{Name: "fix-login", Worktree: true}},
		{name: "none, named by its id", dir: "~/Code/api", wantProject: "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := pageOf(t, []ledger.Line{startedIn(tt.told, in(tt.dir, line(paged, opus, "work", "new", october(7, 9, 0), hourUsage)))}, nil)
			if page.ProjectName != tt.wantProject || page.Shown != tt.want {
				t.Errorf("the page names its project %q and it %+v, want %q and %+v", page.ProjectName, page.Shown, tt.wantProject, tt.want)
			}
		})
	}
}

// Rows of one project named by their ids, whose first 4 characters are
// alike, each show as much of its id as tells it from the others, and are
// never marked as twins; one of another project, or alone, shows its 4.
func TestRowsNamedByIDsAlikeShowAsMuchOfThemAsTellsThemApart(t *testing.T) {
	const (
		first  = "c1a0aa00-0000-4000-8000-000000000001"
		second = "c1a0ab00-0000-4000-8000-000000000002"
		third  = "c1a0b000-0000-4000-8000-000000000003"
		other  = "c1a0aa00-0000-4000-8000-000000000004"
	)
	days := heldDays(now, slices.Concat(
		projectAsks(first, "/code/api", "/code/api", "work", october(6, 9, 0)),
		projectAsks(second, "/code/api", "/code/api", "work", october(6, 10, 0)),
		projectAsks(third, "/code/api", "/code/api", "work", october(6, 11, 0)),
		projectAsks(other, "/code/web", "/code/web", "work", october(6, 12, 0)),
	)...)
	want := map[string]views.ShownName{first: {Name: "c1a0aa"}, second: {Name: "c1a0ab"}, third: {Name: "c1a0b"}, other: {Name: "c1a0"}}
	if got := rowsShown(days.SessionLog(projectsIn(views.PeriodThisWeek, now)).Sessions); !maps.Equal(got, want) {
		t.Errorf("the List shows %+v, want %+v", got, want)
	}
}

// A page asked to open, its lines still read, names its project and the
// session as the router lists them, as the page will once they're read, so
// its heading holds its shape; one the router doesn't list, nothing.
func TestAPageAskedToOpenIsNamedAsTheRouterListsItsSession(t *testing.T) {
	worktree := "~/Code/api/.claude/worktrees/fix-login"
	tests := []struct {
		name        string
		running     []status.Session
		wantProject string
		want        views.ShownName
	}{
		{
			name: "its terminal session's", wantProject: "api", want: views.ShownName{Name: "deploy"},
			running: []status.Session{{ID: paged, Assignments: []status.Assignment{{Model: opus, Account: "work", Dir: worktree, Project: "~/Code/api", SessionName: "deploy", LastSeen: now}}}},
		},
		{
			name: "its worktree's", wantProject: "api", want: views.ShownName{Name: "fix-login", Worktree: true},
			running: []status.Session{{ID: paged, Assignments: []status.Assignment{{Model: opus, Account: "work", Dir: worktree, Project: "~/Code/api", LastSeen: now}}}},
		},
		{name: "not listed, nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := views.AskedPage(paged, tt.running)
			if page.ProjectName != tt.wantProject || page.Shown != tt.want {
				t.Errorf("the page names its project %q and it %+v, want %q and %+v", page.ProjectName, page.Shown, tt.wantProject, tt.want)
			}
		})
	}
}

// rowsShown is how each of rows is shown, by its session's id.
func rowsShown(rows []views.PeriodSession) map[string]views.ShownName {
	shown := make(map[string]views.ShownName)
	for _, r := range rows {
		shown[r.Session] = r.ShownName()
	}
	return shown
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
