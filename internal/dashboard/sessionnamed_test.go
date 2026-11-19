package dashboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The session of the names' tests.
const namedID = "0b5c6f2e-0000-4000-8000-000000000001"

// periodSession is a session of the period, in dir, its project named as
// given, started in the terminal session given.
func periodSession(dir, project, terminal string) views.PeriodSession {
	return views.PeriodSession{Session: namedID, Dir: dir, Name: terminal, ProjectName: project}
}

// twin is s, shown alike with another row of its list.
func twin(s views.PeriodSession) views.PeriodSession {
	s.Twinned = true
	return s
}

// A session is named in its cells: on the List, its project in a column of
// its own, padded so the names line up, in text.muted, then its name in
// text.secondary, a worktree's marked only under a Nerd Font, a twin's id
// after it in text.subtle; under its project, its name alone. Each is cut
// with an ellipsis to its cells, a twin's id kept whole while its name
// keeps a cell.
func TestASessionIsNamedAfterItsProjectOnTheListAndAloneUnderItsProject(t *testing.T) {
	inWorktree := "~/Code/switchboard/.claude/worktrees/fix-login"
	id := status.ShownID(namedID)
	tests := []struct {
		name string
		s    views.PeriodSession
		// projectCells are the cells the List gives projects, none under a
		// project; and nerdFont whether the font draws Nerd Font's glyphs.
		projectCells int
		nerdFont     bool
		cells        int
		want         line
	}{
		{
			name: "the List: its project, then its terminal session's name", s: periodSession("~/Code/switchboard", "switchboard", "router-fix"),
			projectCells: 11, cells: 29,
			want: line{{"switchboard ", mutedInk}, {"router-fix", secondaryInk}},
		},
		{
			name: "the List: a shorter project padded, so the names line up", s: periodSession("~/Code/api", "api", "api-auth"),
			projectCells: 11, cells: 29,
			want: line{{"api         ", mutedInk}, {"api-auth", secondaryInk}},
		},
		{
			name: "the List: a name that starts with its project's, whole", s: periodSession("~/Code/switchboard", "switchboard", "switchboard-Q7rWx2"),
			projectCells: 10, cells: 29,
			want: line{{"switchboa… ", mutedInk}, {"switchboard-Q7rWx2", secondaryInk}},
		},
		{
			name: "the List: a worktree's name, plain without a Nerd Font", s: periodSession(inWorktree, "switchboard", ""),
			projectCells: 11, cells: 29,
			want: line{{"switchboard ", mutedInk}, {"fix-login", secondaryInk}},
		},
		{
			name: "the List: a worktree's name, marked by the tree with one", s: periodSession(inWorktree, "switchboard", ""),
			projectCells: 11, nerdFont: true, cells: 29,
			want: line{{"switchboard ", mutedInk}, {worktreeGlyph + " fix-login", secondaryInk}},
		},
		{
			name: "the List: a terminal session's name in a worktree, unmarked", s: periodSession(inWorktree, "switchboard", "router-fix"),
			projectCells: 11, nerdFont: true, cells: 29,
			want: line{{"switchboard ", mutedInk}, {"router-fix", secondaryInk}},
		},
		{
			name: "the List: its id, where nothing else names it", s: periodSession("~/Code/switchboard", "switchboard", ""),
			projectCells: 11, cells: 29,
			want: line{{"switchboard ", mutedInk}, {id, secondaryInk}},
		},
		{
			name: "the List: of no project, its project's cells blank", s: periodSession("", "", "router-fix"),
			projectCells: 6, cells: 29,
			want: line{{"       ", mutedInk}, {"router-fix", secondaryInk}},
		},
		{
			name: "the List: a twin, its id after its name", s: twin(periodSession("~/Code/api", "api", "billing")),
			projectCells: 3, cells: 29,
			want: line{{"api ", mutedInk}, {"billing", secondaryInk}, {" " + id, dimInk}},
		},
		{
			name: "the List, narrow: a twin's name cut, its id whole", s: twin(periodSession("~/Code/api", "api", "billing")),
			projectCells: 3, cells: 11,
			want: line{{"api ", mutedInk}, {"b…", secondaryInk}, {" " + id, dimInk}},
		},
		{
			name: "the List, narrow: the project cut to its cells, the name to the rest", s: periodSession("~/Code/switchboard", "switchboard", "router-fix"),
			projectCells: 6, cells: 11,
			want: line{{"switc… ", mutedInk}, {"rou…", secondaryInk}},
		},
		{
			name: "under its project: its name alone", s: periodSession("~/Code/switchboard", "", "router-fix"), cells: 29,
			want: line{{"router-fix", secondaryInk}},
		},
		{
			name: "under its project: a worktree's, marked", s: periodSession(inWorktree, "", ""), nerdFont: true, cells: 29,
			want: line{{worktreeGlyph + " fix-login", secondaryInk}},
		},
		{
			name: "under its project: a twin, its id after its name", s: twin(periodSession("~/Code/api", "", "billing")), cells: 29,
			want: line{{"billing", secondaryInk}, {" " + id, dimInk}},
		},
		{
			name: "under its project, narrow: the name cut", s: periodSession("~/Code/switchboard", "", "router-fix"), cells: 8,
			want: line{{"router-…", secondaryInk}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sessionWords{projectCells: tt.projectCells, glyphs: nerdFont(tt.nerdFont)}
			got := w.named(tt.s, tt.cells)
			if !reflect.DeepEqual(got.inked(), tt.want.inked()) {
				t.Errorf("named() = %q in %+v, want %q in %+v", got.plain(), got.inked(), tt.want.plain(), tt.want.inked())
			}
			if got.width() > tt.cells {
				t.Errorf("named() takes %d cells, want %d at the most", got.width(), tt.cells)
			}
		})
	}
}

// The List gives its projects the widest project's cells, cut to leave the
// widest name its cells, 14 at the most, and 6 at the least; none where no
// session has a project.
func TestTheListGivesItsProjectsTheirWidestLeavingTheNamesTheirs(t *testing.T) {
	tests := []struct {
		name                   string
		cells, project, widest int
		want                   int
	}{
		{name: "both whole", cells: 29, project: 11, widest: 10, want: 11},
		{name: "the project cut to leave the name whole", cells: 29, project: 23, widest: 10, want: 18},
		{name: "a long name leaving the project 14", cells: 29, project: 23, widest: 26, want: 14},
		{name: "the project at its least in a narrow column", cells: 11, project: 23, widest: 4, want: listProjectLeast},
		{name: "a short project beside a long name", cells: 29, project: 3, widest: 30, want: 3},
		{name: "no project", cells: 29, widest: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectCellsOf(tt.cells, listNaturals{project: tt.project, name: tt.widest}); got != tt.want {
				t.Errorf("projectCellsOf(%d) = %d, want %d", tt.cells, got, tt.want)
			}
		})
	}
}

// The session page's heading names the session after its id as one phrase:
// its project, a dot and its name, the project cut first; the one alone
// where the other's none; and its directory's name where it names neither,
// as while it loads.
func TestTheSessionPagesHeadingNamesItsProjectThenItsName(t *testing.T) {
	tests := []struct {
		name         string
		project, dir string
		shown        views.ShownName
		nerdFont     bool
		cells        int
		want         line
	}{
		{
			name: "its project, then its name", project: "switchboard", shown: views.ShownName{Name: "switchboard-Q7rWx2"}, cells: 60,
			want: line{{"switchboard", titleInk}, {" · ", mutedInk}, {"switchboard-Q7rWx2", titleInk}},
		},
		{
			name: "a worktree's, marked under a Nerd Font", project: "switchboard", shown: views.ShownName{Name: "fix-login", Worktree: true}, nerdFont: true, cells: 60,
			want: line{{"switchboard", titleInk}, {" · ", mutedInk}, {worktreeGlyph + " fix-login", titleInk}},
		},
		{
			name: "narrow: the project cut, the dot and the name whole", project: "switchboard", shown: views.ShownName{Name: "router-fix"}, cells: 18,
			want: line{{"swit…", titleInk}, {" · ", mutedInk}, {"router-fix", titleInk}},
		},
		{
			name: "its project alone, named by its id", project: "switchboard", dir: "~/Code/switchboard", cells: 60,
			want: line{{"switchboard", titleInk}},
		},
		{
			name: "its name alone, of no project", shown: views.ShownName{Name: "router-fix"}, cells: 60,
			want: line{{"router-fix", titleInk}},
		},
		{
			name: "its directory's name, of neither", dir: "~/Code/switchboard", cells: 60,
			want: line{{"switchboard", titleInk}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := sessionDrawn{NerdFont: tt.nerdFont, page: views.SessionPage{Session: namedID, Dir: tt.dir, ProjectName: tt.project, Shown: tt.shown}}
			got := d.named(tt.cells)
			if !reflect.DeepEqual(got.inked(), tt.want.inked()) {
				t.Errorf("named() = %q in %+v, want %q in %+v", got.plain(), got.inked(), tt.want.plain(), tt.want.inked())
			}
			if got.width() > tt.cells {
				t.Errorf("named() takes %d cells, want %d at the most", got.width(), tt.cells)
			}
		})
	}
}

func TestOnlyANerdFontDrawsTheTree(t *testing.T) {
	if got := nerdFont(false).worktree(); got != "" {
		t.Errorf("without a Nerd Font, a worktree is marked %q, want nothing, as such a font draws the tree as a box", got)
	}
	if got, want := nerdFont(true).worktree(), worktreeGlyph+" "; got != want {
		t.Errorf("with a Nerd Font, a worktree is marked %q, want %q, the tree and a space", got, want)
	}
	if got := nerdFont(true).marked(views.ShownName{Name: "router-fix"}); got != "router-fix" {
		t.Errorf("with a Nerd Font, a terminal session's name is %q, want it unmarked", got)
	}
	if strings.Contains(nerdFont(false).marked(views.ShownName{Name: "fix-login", Worktree: true}), worktreeGlyph) {
		t.Error("without a Nerd Font, a worktree's name is marked, want it plain")
	}
}
