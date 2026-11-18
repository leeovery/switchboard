package dashboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// The session of the names' tests, and its id as status.ShortID cuts it.
const (
	namedID      = "0b5c6f2e-0000-4000-8000-000000000001"
	namedShortID = "0b5c6f2e"
)

// periodSession is a session of the period, in dir, its project named as
// given, started in the terminal session given.
func periodSession(dir, project, terminal string) views.PeriodSession {
	return views.PeriodSession{Session: namedID, Dir: dir, Name: terminal, ProjectName: project}
}

func TestASessionIsNamedAfterItsProjectOnTheListAndAloneUnderItsProject(t *testing.T) {
	inWorktree := "~/Code/switchboard/.claude/worktrees/fix-login"
	tests := []struct {
		name string
		s    views.PeriodSession
		// list is whether it's the List's, which names the project; and
		// nerdFont whether the font draws Nerd Font's glyphs.
		list, nerdFont bool
		cells          int
		want           line
	}{
		{
			name: "the List: its project, then its terminal session's name", s: periodSession("~/Code/switchboard", "switchboard", "router-fix"), list: true, cells: 29,
			want: line{{"switchboard", mutedInk}, {" router-fix", secondaryInk}},
		},
		{
			name: "the List: a worktree's name, plain without a Nerd Font", s: periodSession(inWorktree, "switchboard", ""), list: true, cells: 29,
			want: line{{"switchboard", mutedInk}, {" fix-login", secondaryInk}},
		},
		{
			name: "the List: a worktree's name, marked by the tree with one", s: periodSession(inWorktree, "switchboard", ""), list: true, nerdFont: true, cells: 29,
			want: line{{"switchboard", mutedInk}, {"  fix-login", secondaryInk}},
		},
		{
			name: "the List: a terminal session's name in a worktree, unmarked", s: periodSession(inWorktree, "switchboard", "router-fix"), list: true, nerdFont: true, cells: 29,
			want: line{{"switchboard", mutedInk}, {" router-fix", secondaryInk}},
		},
		{
			name: "the List: its id, cut short, where nothing else names it", s: periodSession("~/Code/switchboard", "switchboard", ""), list: true, cells: 29,
			want: line{{"switchboard", mutedInk}, {" " + namedShortID, secondaryInk}},
		},
		{
			name: "the List: its name alone, of no project", s: periodSession("", "", "router-fix"), list: true, cells: 29,
			want: line{{"router-fix", secondaryInk}},
		},
		{
			name: "under its project: its name alone", s: periodSession("~/Code/switchboard", "", "router-fix"), cells: 29,
			want: line{{"router-fix", secondaryInk}},
		},
		{
			name: "under its project: a worktree's, marked", s: periodSession(inWorktree, "", ""), nerdFont: true, cells: 29,
			want: line{{" fix-login", secondaryInk}},
		},
		{
			name: "the List, narrow: the project cut to its least, the name to the rest", s: periodSession("~/Code/switchboard", "switchboard", "router-fix"), list: true, cells: 11,
			want: line{{"switc…", mutedInk}, {" rou…", secondaryInk}},
		},
		{
			name: "the List, a long name: the project cut to make room, the name to the rest", s: periodSession("~/Code/switchboard", "switchboard", strings.Repeat("n", 30)),
			list: true, cells: 29,
			want: line{{"switc…", mutedInk}, {" " + strings.Repeat("n", 21) + "…", secondaryInk}},
		},
		{
			name: "the List, a short project, whole beside a long name", s: periodSession("~/Code/api", "api", strings.Repeat("n", 30)), list: true, cells: 29,
			want: line{{"api", mutedInk}, {" " + strings.Repeat("n", 24) + "…", secondaryInk}},
		},
		{
			name: "under its project, narrow: the name cut", s: periodSession("~/Code/switchboard", "", "router-fix"), cells: 8,
			want: line{{"router-…", secondaryInk}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sessionWords{project: tt.list, glyphs: nerdFont(tt.nerdFont)}
			got := w.named(tt.s, tt.cells)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("named() = %q, want %q", got.plain(), tt.want.plain())
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
	if got, want := nerdFont(true).worktree(), " "; got != want {
		t.Errorf("with a Nerd Font, a worktree is marked %q, want %q, the tree and a space", got, want)
	}
}
