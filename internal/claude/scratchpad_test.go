package claude_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/leeovery/switchboard/internal/claude"
)

// Sessions' ids, as a scratchpad's folder is named by one.
const (
	scratchSession = "0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c"
	otherSession   = "7e1d2c3b-4a5f-4e6d-8c7b-9a0f1e2d3c4b"
	thirdSession   = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
)

func TestASlugIsTheDirectoryEachCharacterButALetterOrDigitADash(t *testing.T) {
	tests := []struct {
		name, dir, want string
	}{
		{name: "a repository", dir: "/Users/you/Code/api", want: "-Users-you-Code-api"},
		{name: "a worktree Claude Code made", dir: "/Users/you/Code/agentic-workflows/.claude/worktrees/leeway-build",
			want: "-Users-you-Code-agentic-workflows--claude-worktrees-leeway-build"},
		{name: "a dotfolder", dir: "/Users/you/.dotfiles", want: "-Users-you--dotfiles"},
		{name: "dashes, dots, underscores and spaces in names", dir: "/Users/you/Code/my-app_v2.1/web site", want: "-Users-you-Code-my-app-v2-1-web-site"},
		{name: "the home folder", dir: "/Users/you", want: "-Users-you"},
		{name: "a letter beyond ASCII, one UTF-16 unit", dir: "/Users/you/café", want: "-Users-you-caf-"},
		{name: "a character beyond the Basic Multilingual Plane, two UTF-16 units", dir: "/Users/you/🚀", want: "-Users-you---"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.Slug(tt.dir); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestAScratchpadIsKnownByItsShapeAndGivesItsSessionsSlug(t *testing.T) {
	const slug = "-Users-you-Code-api"
	tests := []struct {
		name, path, want string
	}{
		{name: "the scratchpad", path: "/private/tmp/claude-501/" + slug + "/" + scratchSession + "/scratchpad", want: slug},
		{name: "a folder in it", path: "/private/tmp/claude-501/" + slug + "/" + scratchSession + "/scratchpad/probe/run", want: slug},
		{name: "under another root", path: "/Volumes/Scratch/tmp/claude-1000/" + slug + "/" + scratchSession + "/scratchpad", want: slug},
		{name: "under the home, given as ~", path: "~/tmp/claude-501/" + slug + "/" + scratchSession + "/scratchpad", want: slug},
		{name: "a session's id in capitals", path: "/tmp/claude-501/" + slug + "/0B5C6F2E-6A1D-4C3E-9F7A-2B8D4E6F1A3C/scratchpad", want: slug},
		{name: "cut short before its root", path: "…tmp/claude-501/" + slug + "/" + scratchSession + "/scratchpad/x", want: slug},
		{name: "cut within its root's name", path: "…laude-501/" + slug + "/" + scratchSession + "/scratchpad/x", want: slug},
		{name: "cut within its root's id", path: "…01/" + slug + "/" + scratchSession + "/scratchpad/x", want: slug},
		{name: "cut at its root's end", path: "…/" + slug + "/" + scratchSession + "/scratchpad/x", want: slug},
		{name: "a session started in another's scratchpad, the other's",
			path: "/tmp/claude-501/-private-tmp-claude-501-" + slug + "-" + otherSession + "-scratchpad/" + scratchSession + "/scratchpad", want: slug},
		{name: "a session started in a folder of another's scratchpad, the other's",
			path: "/tmp/claude-501/-private-tmp-claude-501-" + slug + "-" + otherSession + "-scratchpad-probe/" + scratchSession + "/scratchpad", want: slug},
		{name: "a session started in the scratchpad of one started in another's, the first's",
			path: "/tmp/claude-501/-tmp-claude-501--tmp-claude-501-" + slug + "-" + otherSession + "-scratchpad-" + thirdSession + "-scratchpad/" +
				scratchSession + "/scratchpad", want: slug},
		{name: "the folder of its session, not its scratchpad", path: "/tmp/claude-501/" + slug + "/" + scratchSession},
		{name: "a folder beside its scratchpad", path: "/tmp/claude-501/" + slug + "/" + scratchSession + "/tasks"},
		{name: "no session's id", path: "/tmp/claude-501/" + slug + "/build/scratchpad"},
		{name: "a root with no user's id", path: "/tmp/claude-/" + slug + "/" + scratchSession + "/scratchpad"},
		{name: "a root not Claude Code's", path: "/tmp/claude-cache/" + slug + "/" + scratchSession + "/scratchpad"},
		{name: "a root of no path", path: "claude-501/" + slug + "/" + scratchSession + "/scratchpad"},
		{name: "cut to what no root ends in", path: "…cache501/" + slug + "/" + scratchSession + "/scratchpad"},
		{name: "cut within its slug", path: "…ode-api/" + scratchSession + "/scratchpad"},
		{name: "a repository", path: "/Users/you/Code/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := claude.ScratchpadSlug(tt.path)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("ScratchpadSlug(%q) = %q, %v, want %q", tt.path, got, ok, tt.want)
			}
		})
	}
}

// home is a filesystem of a home's folders: two repositories, one with a
// worktree Claude Code made and a folder whose name a dash ends the other's
// in, a dotfolder, and a link.
func home() fstest.MapFS {
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	return fstest.MapFS{
		"Users/you/Code/agentic-workflows/.git":                           dir,
		"Users/you/Code/agentic-workflows/.claude/worktrees/leeway-build": dir,
		"Users/you/Code/agentic":                                          dir,
		"Users/you/Code/api/.git":                                         dir,
		"Users/you/.dotfiles":                                             dir,
		"Users/you/a-b":                                                   dir,
		"Users/you/a/b":                                                   dir,
		"Users/you/linked":                                                &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("Code/api")},
	}
}

func TestASlugNamesTheDeepestDirectoryThereWhoseSlugBeginsIt(t *testing.T) {
	tests := []struct {
		name, slug, want string
	}{
		{name: "a worktree Claude Code made, there", slug: "-Users-you-Code-agentic-workflows--claude-worktrees-leeway-build",
			want: "/Users/you/Code/agentic-workflows/.claude/worktrees/leeway-build"},
		{name: "a worktree Claude Code made, removed, the folder that held it",
			slug: "-Users-you-Code-agentic-workflows--claude-worktrees-gone", want: "/Users/you/Code/agentic-workflows/.claude/worktrees"},
		{name: "a repository, removed, the folder that held it", slug: "-Users-you-Code-old-site", want: "/Users/you/Code"},
		{name: "a folder in a repository, removed, the repository", slug: "-Users-you-Code-api-web-src", want: "/Users/you/Code/api"},
		{name: "a name a dash ends another's in, the one it is", slug: "-Users-you-Code-agentic-workflows", want: "/Users/you/Code/agentic-workflows"},
		{name: "the shorter of names a dash parts, where it's the one there", slug: "-Users-you-Code-agentic-tools", want: "/Users/you/Code/agentic"},
		{name: "a name another's begins with no dash between, not the other's", slug: "-Users-you-Code-apiary", want: "/Users/you/Code"},
		{name: "a dotfolder", slug: "-Users-you--dotfiles", want: "/Users/you/.dotfiles"},
		{name: "the home folder", slug: "-Users-you", want: "/Users/you"},
		{name: "two as deep, the one with more parts", slug: "-Users-you-a-b", want: "/Users/you/a/b"},
		{name: "a link, not followed", slug: "-Users-you-linked", want: "/Users/you"},
		{name: "none there but the root", slug: "-srv-api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := claude.SlugDir(home(), tt.slug)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("SlugDir(%q) = %q, %v, want %q", tt.slug, got, ok, tt.want)
			}
		})
	}
}

func TestTheScratchRootIsNamedByTheUsersID(t *testing.T) {
	if got := claude.ScratchRoot(501); got != "claude-501" {
		t.Errorf("ScratchRoot(501) = %q, want claude-501", got)
	}
}
