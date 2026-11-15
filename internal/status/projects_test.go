package status_test

import (
	"io/fs"
	"path"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/leeovery/switchboard/internal/status"
)

// The sessions whose scratchpads the tests' paths are in.
const (
	scratchOne = "0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c"
	scratchTwo = "7e1d2c3b-4a5f-4e6d-8c7b-9a0f1e2d3c4b"
)

// machine is a filesystem of a home with three repositories, one with a
// worktree Claude Code made, its scratch root kept beside another
// temporary directory than the reader's.
func machine() fstest.MapFS {
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	return fstest.MapFS{
		"Users/you/Code/agentic-workflows/.git":                           dir,
		"Users/you/Code/agentic-workflows/.claude/worktrees/leeway-build": dir,
		"Users/you/Code/api/.git":                                         dir,
		"Users/you/Code/api/web":                                          dir,
		"Users/you/.dotfiles/.git":                                        dir,
		"scratch/tmp/claude-501":                                          dir,
		"private/tmp/scratch":                                             dir,
	}
}

// gitWalk is the project of a directory of fsys as a walk up from it to the
// first directory holding a .git finds it, as launch.FindProject walks.
func gitWalk(fsys fs.FS) func(string) string {
	return func(dir string) string {
		for at := dir; at != "/"; at = path.Dir(at) {
			if _, err := fs.Stat(fsys, at[1:]+"/.git"); err == nil {
				return at
			}
		}
		return dir
	}
}

// machineProjects reads projects against the machine, its user 501, the
// reader's temporary directory /var/run/tmp.
func machineProjects(fsys fs.FS) status.Projects {
	return status.NewProjects(status.ProjectsConfig{Root: fsys, Home: "/Users/you", TempDir: "/var/run/tmp/", UID: 501, Find: gitWalk(machine())})
}

// scratchpad is the scratchpad of the session given, started in the
// directory whose slug is given, under root, with the rest given below it.
func scratchpad(root, slug, session, rest string) string {
	return root + "/claude-501/" + slug + "/" + session + "/scratchpad" + rest
}

func TestAProjectInAScratchpadIsTheProjectItsSlugNames(t *testing.T) {
	tests := []struct {
		name, project, dir, want string
	}{
		{name: "a line from before projects, in a worktree's scratchpad, the worktree's repository",
			dir:  scratchpad("/private/tmp", "-Users-you-Code-agentic-workflows--claude-worktrees-leeway-build", scratchOne, ""),
			want: "~/Code/agentic-workflows"},
		{name: "a line recording the scratchpad as its project, as v0.9.0's do",
			project: scratchpad("/scratch/tmp", "-Users-you-Code-api", scratchOne, ""), dir: scratchpad("/scratch/tmp", "-Users-you-Code-api", scratchOne, ""),
			want: "~/Code/api"},
		{name: "a folder in the scratchpad", dir: scratchpad("/scratch/tmp", "-Users-you-Code-api", scratchOne, "/probe/run"), want: "~/Code/api"},
		{name: "a worktree removed since, its repository",
			dir: scratchpad("/private/tmp", "-Users-you-Code-agentic-workflows--claude-worktrees-gone", scratchOne, ""), want: "~/Code/agentic-workflows"},
		{name: "a folder of a repository, its repository", dir: scratchpad("/private/tmp", "-Users-you-Code-api-web", scratchOne, ""), want: "~/Code/api"},
		{name: "a dotfolder's", dir: scratchpad("/private/tmp", "-Users-you--dotfiles", scratchOne, ""), want: "~/.dotfiles"},
		{name: "the home folder's, the home's project", dir: scratchpad("/private/tmp", "-Users-you", scratchOne, ""), want: "~"},
		{name: "a dir cut short from its front, as a long one was recorded",
			dir: "…tmp/claude-501/-Users-you-Code-api/" + scratchOne + "/scratchpad/probe", want: "~/Code/api"},
		{name: "a dir cut within its scratch root", dir: "…01/-Users-you-Code-api/" + scratchOne + "/scratchpad/probe", want: "~/Code/api"},
		{name: "a scratchpad of a session started in another's, the first's project",
			dir:  scratchpad("/private/tmp", "-private-tmp-claude-501--Users-you-Code-api-"+scratchOne+"-scratchpad", scratchTwo, ""),
			want: "~/Code/api"},
		{name: "a slug naming none of the home's, temporary folders", dir: scratchpad("/private/tmp", "-Users-them-Code-api", scratchOne, ""),
			want: status.TemporaryFolders},
		{name: "a slug naming nothing there, temporary folders", dir: scratchpad("/scratch/tmp", "-srv-api", scratchOne, ""),
			want: status.TemporaryFolders},
		{name: "a slug naming a temporary folder, temporary folders", dir: scratchpad("/private/tmp", "-private-tmp-scratch", scratchOne, ""),
			want: status.TemporaryFolders},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := machineProjects(machine()).Of(tt.project, tt.dir); got != tt.want {
				t.Errorf("Of(%q, %q) = %q, want %q", tt.project, tt.dir, got, tt.want)
			}
		})
	}
}

func TestAProjectInATemporaryFolderIsTemporaryFolders(t *testing.T) {
	tests := []struct {
		name, project, dir, want string
	}{
		{name: "under /tmp", dir: "/tmp/build-1", want: status.TemporaryFolders},
		{name: "under /private/tmp", dir: "/private/tmp/build-1/src", want: status.TemporaryFolders},
		{name: "under /var/folders", dir: "/var/folders/xy/abc123/T/probe-1", want: status.TemporaryFolders},
		{name: "under /private/var/folders", dir: "/private/var/folders/xy/abc123/T/probe-1", want: status.TemporaryFolders},
		{name: "under the reader's temporary directory", dir: "/var/run/tmp/call-1", want: status.TemporaryFolders},
		{name: "the project recorded, under /tmp", project: "/tmp/clone-1", dir: "/tmp/clone-1/web", want: status.TemporaryFolders},
		{name: "beside Claude Code's scratch root", dir: "/scratch/tmp/call-1", want: status.TemporaryFolders},
		{name: "below a folder beside the scratch root, its own", dir: "/scratch/tmp/call-1/web", want: "/scratch/tmp/call-1/web"},
		{name: "a folder named as a temporary one, elsewhere, its own", dir: "/srv/tmp/call-1", want: "/srv/tmp/call-1"},
		{name: "a folder whose name begins as /tmp's, its own", dir: "/tmpfiles/call-1", want: "/tmpfiles/call-1"},
		{name: "a repository in the home, its own", project: "~/Code/api", dir: "~/Code/api/web", want: "~/Code/api"},
		{name: "temporary folders, read again, temporary folders", project: status.TemporaryFolders, dir: "/tmp/build-1", want: status.TemporaryFolders},
		{name: "no directory, none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := machineProjects(machine()).Of(tt.project, tt.dir); got != tt.want {
				t.Errorf("Of(%q, %q) = %q, want %q", tt.project, tt.dir, got, tt.want)
			}
		})
	}
}

func TestTheZeroProjectsReadsAsProjectOf(t *testing.T) {
	for _, dir := range []string{"/tmp/build-1", scratchpad("/private/tmp", "-Users-you-Code-api", scratchOne, ""), "~/Code/api/.claude/worktrees/fix"} {
		if got, want := (status.Projects{}).Of("", dir), status.ProjectOf("", dir); got != want {
			t.Errorf("the zero Projects reads %q as %q, want %q", dir, got, want)
		}
	}
}

func TestWithoutFindADecodedDirectoryIsReadAsProjectOfReadsIt(t *testing.T) {
	p := status.NewProjects(status.ProjectsConfig{Root: machine(), Home: "/Users/you", UID: 501})
	dir := scratchpad("/private/tmp", "-Users-you-Code-agentic-workflows--claude-worktrees-leeway-build", scratchOne, "")
	if got := p.Of("", dir); got != "~/Code/agentic-workflows" {
		t.Errorf("Of(%q) = %q, want ~/Code/agentic-workflows", dir, got)
	}
}

// openings is a filesystem counting what it opens.
type openings struct {
	fs.FS
	opened *atomic.Int64
}

func (o openings) Open(name string) (fs.File, error) {
	o.opened.Add(1)
	return o.FS.Open(name)
}

func TestEachPathAndEachSlugIsReadAgainstTheFilesystemOnce(t *testing.T) {
	o := openings{FS: machine(), opened: new(atomic.Int64)}
	p := machineProjects(o)
	dir := scratchpad("/private/tmp", "-Users-you-Code-api", scratchOne, "")
	p.Of("", dir)
	once := o.opened.Load()
	if once == 0 {
		t.Fatal("reading a scratchpad opened nothing")
	}
	for range 100 {
		p.Of("", dir)
		p.Of(dir, dir)
	}
	if got := o.opened.Load(); got != once {
		t.Errorf("reading the scratchpad 200 times more opened %d, want none more than the first's %d", got, once)
	}
	if got := p.Of("", scratchpad("/private/tmp", "-Users-you-Code-api", scratchTwo, "/probe")); got != "~/Code/api" {
		t.Errorf("another session's scratchpad of the same directory is %q's, want ~/Code/api's", got)
	}
	if got := o.opened.Load(); got != once {
		t.Errorf("reading another scratchpad of the same slug opened %d more, want none", got-once)
	}
}
