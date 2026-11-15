package views_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// scratchMachine is a home's filesystem of the api repository, and its
// worktree Claude Code made, its scratch root kept in /scratch/tmp; and
// how projects are read against it, its user 501.
func scratchMachine() status.Projects {
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	machine := fstest.MapFS{
		"Users/you/Code/api/.claude/worktrees/fix": dir,
		"scratch/tmp/claude-501":                   dir,
	}
	return status.NewProjects(status.ProjectsConfig{Root: machine, Home: "/Users/you", TempDir: "/var/run/tmp", UID: 501})
}

// scratchID is the id of the session given by its number among many.
func scratchID(n int) string {
	return fmt.Sprintf("5c0a0000-0000-4000-8000-%012d", n)
}

// scratchWorld is a session in the api repository, many tools' sessions in
// its sessions' scratchpads, each a request, and a few in folders beside the
// scratch root, all on Monday: of the scratchpads' lines, those of a third
// from before projects name the scratchpad alone; another third's name it
// their project too, as v0.9.0's do; and the last third's of a worktree's
// session, cut from their front as a long directory was recorded.
func scratchWorld(scratched, beside int) []ledger.Held {
	at := october(5, 10, 0)
	lines := projectAsks(repoSession, "~/Code/api", "~/Code/api", "work", at)
	for n := range scratched {
		id := scratchID(n)
		pad := "/scratch/tmp/claude-501/-Users-you-Code-api/" + scratchID(1000+n) + "/scratchpad"
		dir, project := pad, ""
		switch n % 3 {
		case 1:
			project = pad
		case 2:
			dir = "…tmp/claude-501/-Users-you-Code-api--claude-worktrees-fix/" + scratchID(1000+n) + "/scratchpad/probe"
		}
		lines = append(lines, projectAsk(id, dir, project, "work", opus, at.Add(time.Duration(n)*time.Second)))
	}
	for n := range beside {
		lines = append(lines, projectAsk(scratchID(2000+n), fmt.Sprintf("/scratch/tmp/call-%d", n), "", "work", opus, at.Add(time.Hour)))
	}
	return lines
}

// Every session started in a scratchpad counts as the project the
// scratchpad's session ran in, whichever way its line names it; those
// started in folders beside the scratch root, as temporary folders, a row
// with no name of a path, as no directory's has none.
func TestTheSessionsOfAProjectsScratchpadsAreItsOwn(t *testing.T) {
	in := projectsIn(views.PeriodThisWeek, now)
	in.Projects = scratchMachine()
	page := heldDays(now, scratchWorld(30, 4)...).Projects(in)
	type row struct {
		path, name         string
		sessions, requests int
		temporary          bool
	}
	var got []row
	for _, r := range page.Rows {
		got = append(got, row{path: r.Path, name: r.Name, sessions: r.Sessions, requests: r.Requests, temporary: r.Temporary()})
	}
	want := []row{
		{path: "~/Code/api", name: "api", sessions: 31, requests: 31},
		{path: status.TemporaryFolders, sessions: 4, requests: 4, temporary: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rows are %+v, want %+v", got, want)
	}
	project, ok := page.Project("~/Code/api")
	if !ok {
		t.Fatal("no page of ~/Code/api")
	}
	for _, s := range project.Sessions {
		if s.Project != "~/Code/api" {
			t.Errorf("session %s of ~/Code/api lists its project as %q", s.Session, s.Project)
		}
	}
}

// The List reads a session's project as Projects does.
func TestTheListReadsAScratchpadsSessionAsItsProjects(t *testing.T) {
	pad := "/scratch/tmp/claude-501/-Users-you-Code-api/" + scratchID(1000) + "/scratchpad"
	line := ledger.Line{
		At: now.Add(-time.Minute).UTC(), Request: "r1", Session: scratchID(0), Kind: ledger.KindMessage, Dir: pad, Project: pad,
		Account: "work", Model: opus, Status: 200, Class: ledger.ClassMain,
	}
	list := views.ListSessions(views.SessionSources{Ledger: fakeLedger{lines: []ledger.Line{line}, now: now}, Projects: scratchMachine(), Now: now})
	i := slices.IndexFunc(list.Sessions, func(s views.ListedSession) bool { return s.Session == scratchID(0) })
	if i < 0 {
		t.Fatal("the List lists no session of the scratchpad")
	}
	if got := list.Sessions[i]; got.Project != "~/Code/api" || got.Dir != pad {
		t.Errorf("the List has the session in %q of %q, want in %q of ~/Code/api", got.Dir, got.Project, pad)
	}
}

// A day's head keeps each session's directory and project as its lines named
// them, so Projects read of the heads of days gone counts a scratchpad's
// sessions as it counts them read of their lines, through the rule as it
// reads now: which is why the rule needs no new version of the heads.
func TestTheHeadsOfDaysGoneCountAScratchpadsSessionsAsTheirLinesDo(t *testing.T) {
	lines := scratchWorld(9, 2)
	build := views.Digests{}.Day()
	for _, l := range lines {
		build.Take(l.Line)
	}
	built, err := build.Built()
	if err != nil {
		t.Fatal(err)
	}
	day := dayfile.DateOf(lines[0].At)
	headed := views.ReadSessionDays(sidedLedger{sides: map[string]json.RawMessage{day: built.Side}}, now.AddDate(-1, 0, 0), now, views.PoolRead{})
	in := projectsIn(views.PeriodThisWeek, now)
	in.Projects = scratchMachine()
	rowsOf := func(page views.ProjectsPage) []string {
		var rows []string
		for _, r := range page.Rows {
			rows = append(rows, fmt.Sprintf("%s %d sessions", r.Path, r.Sessions))
		}
		return rows
	}
	got, read := rowsOf(headed.Projects(in)), rowsOf(heldDays(now, lines...).Projects(in))
	if want := []string{"~/Code/api 10 sessions", status.TemporaryFolders + " 2 sessions"}; !reflect.DeepEqual(got, want) || !reflect.DeepEqual(read, want) {
		t.Errorf("of the day's head, the rows are %v, and of its lines %v, want %v", got, read, want)
	}
}

// Projects' page over a week of many scratchpads' sessions, each read after
// the first, of a Projects that has read every path already.
func BenchmarkProjectsOfManyScratchpads(b *testing.B) {
	days := heldDays(now, scratchWorld(3000, 300)...)
	in := projectsIn(views.PeriodThisWeek, now)
	in.Projects = scratchMachine()
	for b.Loop() {
		days.Projects(in)
	}
}

// Projects' page over the same week, each read of a Projects that has read
// nothing yet, as the first read of the dashboard or a command is.
func BenchmarkProjectsOfManyScratchpadsReadFirst(b *testing.B) {
	days := heldDays(now, scratchWorld(3000, 300)...)
	in := projectsIn(views.PeriodThisWeek, now)
	for b.Loop() {
		in.Projects = scratchMachine()
		days.Projects(in)
	}
}

// Projects' page over the same week, of no Projects reading anything, to
// measure the read's cost against.
func BenchmarkProjectsOfManyScratchpadsReadAsProjectOf(b *testing.B) {
	days := heldDays(now, scratchWorld(3000, 300)...)
	in := projectsIn(views.PeriodThisWeek, now)
	for b.Loop() {
		days.Projects(in)
	}
}
