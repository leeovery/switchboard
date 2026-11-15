package status_test

import (
	"maps"
	"math"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

func TestAProjectIsTheOneRunToldElseItsDirectorysAsALineFromBeforeHasIt(t *testing.T) {
	tests := []struct {
		name, project, dir, want string
	}{
		{name: "the project run told", project: "~/Code/api", dir: "~/Code/api/cmd/server", want: "~/Code/api"},
		{name: "the project run told, a worktree's", project: "~/Code/api", dir: "~/Code/api/.claude/worktrees/fix-login", want: "~/Code/api"},
		{name: "the project run told, of no directory", project: "~/Code/api", want: "~/Code/api"},
		{name: "none told, a worktree Claude Code made, its repository", dir: "~/Code/api/.claude/worktrees/fix-login", want: "~/Code/api"},
		{name: "none told, a worktree Claude Code made outside home", dir: "/srv/api/.claude/worktrees/fix-login", want: "/srv/api"},
		{name: "none told, a worktree Claude Code made in home itself", dir: "~/.claude/worktrees/fix-login", want: "~"},
		{name: "none told, within a worktree Claude Code made, as itself", dir: "~/Code/api/.claude/worktrees/fix-login/web", want: "~/Code/api/.claude/worktrees/fix-login/web"},
		{name: "none told, Claude Code's worktrees themselves, as itself", dir: "~/Code/api/.claude/worktrees", want: "~/Code/api/.claude/worktrees"},
		{name: "none told, a worktrees directory not Claude Code's, as itself", dir: "~/Code/api/worktrees/fix-login", want: "~/Code/api/worktrees/fix-login"},
		{name: "none told, a worktrees directory of no repository, as itself", dir: "/.claude/worktrees/fix-login", want: "/.claude/worktrees/fix-login"},
		{name: "none told, a subdirectory, as itself", dir: "~/Code/api/cmd/server", want: "~/Code/api/cmd/server"},
		{name: "none told, a repository, as itself", dir: "~/Code/api", want: "~/Code/api"},
		{name: "the project run told, cleaned", project: "~/Code/api/", want: "~/Code/api"},
		{name: "none told, a worktree Claude Code made, cleaned, its repository", dir: "~/Code/api/.claude/worktrees/fix-login/", want: "~/Code/api"},
		{name: "none told, a directory, cleaned", dir: "~/Code/api/./cmd/", want: "~/Code/api/cmd"},
		{name: "neither, none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.ProjectOf(tt.project, tt.dir); got != tt.want {
				t.Errorf("ProjectOf(%q, %q) = %q, want %q", tt.project, tt.dir, got, tt.want)
			}
		})
	}
}

func TestProjectsAreNamedByTheirFoldersTheirParentsTooWhereTwoShareOne(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		cells    int
		want     map[string]string
	}{
		{
			name:     "each by its folder",
			projects: []string{"~/Code/switchboard", "~/Code/api", "/srv/site"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/Code/switchboard": "switchboard", "~/Code/api": "api", "/srv/site": "site"},
		},
		{
			name:     "two sharing a folder's name, each by its parent too",
			projects: []string{"~/Code/client/api", "~/Code/own/api", "~/Code/switchboard"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/Code/client/api": "client/api", "~/Code/own/api": "own/api", "~/Code/switchboard": "switchboard"},
		},
		{
			name:     "three sharing one",
			projects: []string{"~/client/api", "~/own/api", "/srv/api"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/client/api": "client/api", "~/own/api": "own/api", "/srv/api": "srv/api"},
		},
		{
			name:     "two sharing their parents' names too, by as many parents as tell them apart",
			projects: []string{"~/a/x/api", "~/b/x/api", "~/Code/switchboard"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/a/x/api": "a/x/api", "~/b/x/api": "b/x/api", "~/Code/switchboard": "switchboard"},
		},
		{
			name:     "three sharing a name, each by as many parents as tell it from the others",
			projects: []string{"~/a/x/api", "~/b/x/api", "~/c/y/api"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/a/x/api": "a/x/api", "~/b/x/api": "b/x/api", "~/c/y/api": "y/api"},
		},
		{
			name:     "one given twice, by its folder alone",
			projects: []string{"~/Code/api", "~/Code/api"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/Code/api": "api"},
		},
		{
			name:     "one written two ways, one project, by its path cleaned",
			projects: []string{"~/a/api", "~/a/api/", "~/a/./api"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/a/api": "api"},
		},
		{
			name:     "the root, and one at it",
			projects: []string{"/", "/api"},
			cells:    math.MaxInt,
			want:     map[string]string{"/": "/", "/api": "api"},
		},
		{
			name:     "parents added, then cut, as DirName cuts",
			projects: []string{"~/client-one/x/api", "~/client-two/x/api"},
			cells:    12,
			want:     map[string]string{"~/client-one/x/api": "client-one/…", "~/client-two/x/api": "client-two/…"},
		},
		{
			name:     "the home, and one at the root beside one in it",
			projects: []string{"~", "/api", "~/api"},
			cells:    math.MaxInt,
			want:     map[string]string{"~": "~", "/api": "/api", "~/api": "~/api"},
		},
		{
			name:     "cut to the cells given, keeping their start, as DirName cuts",
			projects: []string{"~/Code/switchboard-dashboard", "~/Code/client/api", "~/own/api"},
			cells:    8,
			want:     map[string]string{"~/Code/switchboard-dashboard": "switchb…", "~/Code/client/api": "client/…", "~/own/api": "own/api"},
		},
		{
			name:     "cleaned, as what comes from elsewhere is",
			projects: []string{"~/Code/sw\x1b[2Jitch"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/Code/sw\x1b[2Jitch": "sw [2Jitch"},
		},
		{
			name:     "two showing one name only once cleaned, each by its parent too",
			projects: []string{"~/a/api x", "~/b/api  x"},
			cells:    math.MaxInt,
			want:     map[string]string{"~/a/api x": "a/api x", "~/b/api  x": "b/api x"},
		},
		{name: "none named none", projects: []string{""}, cells: math.MaxInt, want: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.ProjectNames(tt.projects, tt.cells); !maps.Equal(got, tt.want) {
				t.Errorf("ProjectNames(%q, %d) = %q, want %q", tt.projects, tt.cells, got, tt.want)
			}
		})
	}
}
