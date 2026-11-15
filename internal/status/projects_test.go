package status_test

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/leeovery/switchboard/internal/status"
)

// The sessions whose scratchpads the tests' paths are in.
const (
	scratchOne = "0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c"
	scratchTwo = "7e1d2c3b-4a5f-4e6d-8c7b-9a0f1e2d3c4b"
)

// bareConfig is a bare repository's config, as git writes one.
const bareConfig = "[core]\n\trepositoryformatversion = 0\n\tbare = true\n"

// machine is a filesystem of a home with three repositories, one with a
// worktree Claude Code made; a bare repository with its worktrees kept in
// its folder, and one in .bare with its worktrees beside it; two folders
// whose names differ only in case, as a volume that keeps case can hold;
// its scratch root kept beside another temporary directory than the
// reader's; and a scratch root of the same user kept in the home, and one
// kept above it, as a CLAUDE_CODE_TMPDIR of either would keep it.
func machine() fstest.MapFS {
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	return fstest.MapFS{
		"Users/you/claude-501":                  dir,
		"Users/claude-501":                      dir,
		"Users/shared":                          dir,
		"Users/you/Code/agentic-workflows/.git": dir,
		"Users/you/Code/agentic-workflows/.claude/worktrees/leeway-build": dir,
		"Users/you/Code/api/.git":          dir,
		"Users/you/Code/api/web":           dir,
		"Users/you/Code/web.git/config":    &fstest.MapFile{Data: []byte(bareConfig)},
		"Users/you/Code/web.git/hooks":     dir,
		"Users/you/Code/web.git/wt":        dir,
		"Users/you/Code/site/.bare/config": &fstest.MapFile{Data: []byte(bareConfig)},
		"Users/you/.dotfiles/.git":         dir,
		"Users/you/Notes":                  dir,
		"Users/you/notes":                  dir,
		"scratch/tmp/claude-501":           dir,
		"private/tmp/scratch":              dir,
	}
}

// gitWalk is the project of a directory of fsys as a walk up from it to the
// first directory holding a .git finds it, as launch.FindProject walks.
func gitWalk(fsys fs.FS) func(string) (string, bool) {
	return func(dir string) (string, bool) {
		for at := dir; at != "/"; at = path.Dir(at) {
			if _, err := fs.Stat(fsys, at[1:]+"/.git"); err == nil {
				return at, true
			}
		}
		return dir, false
	}
}

// repositoryOf is the project of the repository a directory of fsys is in,
// and whether it's in one, as launch.RepositoryOf has them: of the nearest
// directory up from it that's a bare repository's directory, that; that
// holds one named .bare, that; or that holds a .git, the repository's.
func repositoryOf(fsys fs.FS) func(string) (string, bool) {
	bare := func(dir string) bool {
		config, err := fs.ReadFile(fsys, dir+"/config")
		return err == nil && string(config) == bareConfig
	}
	return func(dir string) (string, bool) {
		for at := dir; at != "/"; at = path.Dir(at) {
			if bare(at[1:]) || bare(at[1:]+"/.bare") {
				return at, true
			}
			if _, err := fs.Stat(fsys, at[1:]+"/.git"); err == nil {
				return at, true
			}
		}
		return "", false
	}
}

// machineProjectsIn reads projects against the machine, its user 501, the
// reader's temporary directory the one given.
func machineProjectsIn(fsys fs.FS, tempDir string) status.Projects {
	return status.NewProjects(status.ProjectsConfig{
		Root: fsys, Home: "/Users/you", TempDir: tempDir, UID: 501, Find: gitWalk(machine()), Repository: repositoryOf(machine()),
	})
}

// machineProjects reads projects against the machine, its user 501, the
// reader's temporary directory /var/run/tmp.
func machineProjects(fsys fs.FS) status.Projects {
	return machineProjectsIn(fsys, "/var/run/tmp/")
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
		{name: "a claude a session started in a scratchpad, recorded with the session's project, that project, not the slug's",
			project: "~/Code/api", dir: scratchpad("/private/tmp", "-Users-you-Code-agentic-workflows", scratchOne, "/probe"),
			want: "~/Code/api"},
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
		{name: "a repository removed since, held by no repository, temporary folders, not the folder that held it",
			dir: scratchpad("/private/tmp", "-Users-you-Code-old-site", scratchOne, ""), want: status.TemporaryFolders},
		{name: "a volume not mounted, temporary folders, not the folder of volumes",
			dir: scratchpad("/private/tmp", "-Volumes-Projects-api", scratchOne, ""), want: status.TemporaryFolders},
		{name: "a folder no repository holds, named whole, itself", dir: scratchpad("/private/tmp", "-Users-you-Code", scratchOne, ""), want: "~/Code"},
		{name: "a slug made of a folder named in another case, the folder in its own",
			dir: scratchpad("/private/tmp", "-Users-you-code-api", scratchOne, ""), want: "~/Code/api"},
		{name: "a worktree removed since from a bare repository's folder, the bare repository",
			dir: scratchpad("/private/tmp", "-Users-you-Code-web-git-feature", scratchOne, ""), want: "~/Code/web.git"},
		{name: "a worktree removed since from a subfolder of a bare repository, the bare repository, not the subfolder",
			dir: scratchpad("/private/tmp", "-Users-you-Code-web-git-wt-feature", scratchOne, ""), want: "~/Code/web.git"},
		{name: "a worktree removed since, named as a bare repository's folder begins, the bare repository, not the folder",
			dir: scratchpad("/private/tmp", "-Users-you-Code-web-git-hooks-fix", scratchOne, ""), want: "~/Code/web.git"},
		{name: "a worktree removed since from beside a bare repository in .bare, the folder holding both",
			dir: scratchpad("/private/tmp", "-Users-you-Code-site-feature", scratchOne, ""), want: "~/Code/site"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := machineProjects(machine()).Of(tt.project, tt.dir); got != tt.want {
				t.Errorf("Of(%q, %q) = %q, want %q", tt.project, tt.dir, got, tt.want)
			}
		})
	}
}

// A path is temporary in a temporary root, the reader's temporary directory
// among them where it isn't the home or above it; at or under a folder
// named for Claude Code's scratch root, whatever holds it; or at or under
// the folder holding the scratch root, where that holder isn't the home or
// above it. Each is compared with its case folded. Nothing else is.
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
		{name: "under /private/tmp, in another case", dir: "/Private/tmp/build-1", want: status.TemporaryFolders},
		{name: "under the reader's temporary directory, in another case", dir: "/VAR/run/tmp/call-1", want: status.TemporaryFolders},
		{name: "in the scratch root, not a scratchpad", dir: "/scratch/tmp/claude-501/tasks", want: status.TemporaryFolders},
		{name: "in a scratch root kept in the home", dir: "~/claude-501/tasks", want: status.TemporaryFolders},
		{name: "in a scratch root kept in the home, in another case", dir: "~/Claude-501/tasks", want: status.TemporaryFolders},
		{name: "a folder whose name begins as the scratch root's, its own", dir: "~/claude-501-tools", want: "~/claude-501-tools"},
		{name: "the folder holding the scratch root", dir: "/scratch/tmp", want: status.TemporaryFolders},
		{name: "beside the scratch root", dir: "/scratch/tmp/call-1", want: status.TemporaryFolders},
		{name: "two folders below the one holding the scratch root", dir: "/scratch/tmp/call-1/web", want: status.TemporaryFolders},
		{name: "beside the scratch root, in another case", dir: "/Scratch/TMP/call-1", want: status.TemporaryFolders},
		{name: "the home, holding a scratch root, its own", dir: "~", want: "~"},
		{name: "beside a scratch root kept in the home, its own", dir: "~/notes", want: "~/notes"},
		{name: "beside a scratch root kept in the home, the home in another case, its own", dir: "/users/YOU/notes", want: "~/notes"},
		{name: "beside a scratch root kept above the home, its own", dir: "/Users/shared", want: "/Users/shared"},
		{name: "beside a scratch root kept above the home, in another case, its own", dir: "/users/shared", want: "/Users/shared"},
		{name: "a folder named as a temporary one, elsewhere, its own", dir: "/srv/tmp/call-1", want: "/srv/tmp/call-1"},
		{name: "a folder whose name begins as /tmp's, its own", dir: "/tmpfiles/call-1", want: "/tmpfiles/call-1"},
		{name: "a repository in the home, its own", project: "~/Code/api", dir: "~/Code/api/web", want: "~/Code/api"},
		{name: "a folder named in another case, in its own", dir: "~/code", want: "~/Code"},
		{name: "a repository named in another case, in its own", project: "~/code/API", dir: "~/code/API", want: "~/Code/api"},
		{name: "a folder not there, in its own case as far as there are folders", dir: "~/code/gone/Web", want: "~/Code/gone/Web"},
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

// A reader's temporary directory that's the home, or above it, makes none
// of the home temporary.
func TestATemporaryDirectoryOfTheHomeOrAboveIsNoTemporaryRoot(t *testing.T) {
	for _, tempDir := range []string{"/Users/you", "/users/you/", "/Users", "/"} {
		p := machineProjectsIn(machine(), tempDir)
		for dir, want := range map[string]string{"~/Code/api": "~/Code/api", "~": "~", "/Users/shared": "/Users/shared", "/tmp/build-1": status.TemporaryFolders} {
			if got := p.Of("", dir); got != want {
				t.Errorf("of the temporary directory %s, Of(%q) = %q, want %q", tempDir, dir, got, want)
			}
		}
	}
}

// A path is given in the case its folders have: a folder named as one of two
// a case apart is that one, and one named as neither is the first of them.
func TestAPathIsGivenAsItsFoldersAreCasedTheExactFirst(t *testing.T) {
	p := machineProjects(machine())
	for dir, want := range map[string]string{"~/notes": "~/notes", "~/Notes": "~/Notes", "~/NOTES": "~/Notes", "/users/you/nOtes": "~/Notes"} {
		if got := p.Of("", dir); got != want {
			t.Errorf("Of(%q) = %q, want %q", dir, got, want)
		}
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

// openings is a filesystem counting what it opens, by its name.
type openings struct {
	fs.FS
	mu     *sync.Mutex
	opened map[string]int
}

// countingOpens is fsys, counting what it opens.
func countingOpens(fsys fs.FS) openings {
	return openings{FS: fsys, mu: new(sync.Mutex), opened: make(map[string]int)}
}

func (o openings) Open(name string) (fs.File, error) {
	o.mu.Lock()
	o.opened[name]++
	o.mu.Unlock()
	return o.FS.Open(name)
}

// total is how many opens there have been.
func (o openings) total() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, each := range o.opened {
		n += each
	}
	return n
}

func TestEachPathAndEachSlugIsReadAgainstTheFilesystemOnce(t *testing.T) {
	o := countingOpens(machine())
	p := machineProjects(o)
	dir := scratchpad("/private/tmp", "-Users-you-Code-api", scratchOne, "")
	p.Of("", dir)
	once := o.total()
	if once == 0 {
		t.Fatal("reading a scratchpad opened nothing")
	}
	for range 100 {
		p.Of("", dir)
		p.Of(dir, dir)
	}
	if got := o.total(); got != once {
		t.Errorf("reading the scratchpad 200 times more opened %d, want none more than the first's %d", got, once)
	}
	if got := p.Of("", scratchpad("/private/tmp", "-Users-you-Code-api", scratchTwo, "/probe")); got != "~/Code/api" {
		t.Errorf("another session's scratchpad of the same directory is %q's, want ~/Code/api's", got)
	}
	if got := o.total(); got != once {
		t.Errorf("reading another scratchpad of the same slug opened %d more, want none", got-once)
	}
}

// Each folder is listed once for its case, and looked in for the scratch
// root once, however many of the projects read are in it. A slug's walk
// keeps no listing: it's decoded against the folders as they are then.
func TestEachFolderIsListedAndLookedInOnce(t *testing.T) {
	m := machine()
	for i := range 50 {
		m[fmt.Sprintf("Users/you/Code/project-%d/.git", i)] = &fstest.MapFile{Mode: fs.ModeDir}
	}
	o := countingOpens(m)
	p := machineProjects(o)
	for i := range 50 {
		want := fmt.Sprintf("~/Code/project-%d", i)
		if got := p.Of("", fmt.Sprintf("~/code/project-%d", i)); got != want {
			t.Errorf("Of(~/code/project-%d) = %q, want %q", i, got, want)
		}
	}
	for _, name := range []string{".", "Users", "Users/you", "Users/you/Code", "Users/you/Code/claude-501"} {
		if got := o.opened[name]; got != 1 {
			t.Errorf("%s was opened %d times, want once", name, got)
		}
	}
}

// A slug is decoded against the folders there as it's first read, however
// long the Projects reading it has run: a folder made since its parent was
// first read is found, and a worktree removed since is gone, its folder
// left giving its repository's project.
func TestASlugIsDecodedAgainstTheFoldersThereAsItsRead(t *testing.T) {
	m := machine()
	m["Users/you/Code/web.git/wt/feature"] = &fstest.MapFile{Mode: fs.ModeDir}
	p := status.NewProjects(status.ProjectsConfig{Root: m, Home: "/Users/you", UID: 501, Find: gitWalk(m), Repository: repositoryOf(m)})
	for dir, want := range map[string]string{"~/Code/api": "~/Code/api", "~/Code/web.git/wt/other": "~/Code/web.git/wt/other"} {
		if got := p.Of("", dir); got != want {
			t.Fatalf("Of(%q) = %q, want %q", dir, got, want)
		}
	}
	m["Users/you/Code/newproj/.git"] = &fstest.MapFile{Mode: fs.ModeDir}
	delete(m, "Users/you/Code/web.git/wt/feature")
	tests := []struct {
		name, slug, want string
	}{
		{name: "a repository made since its parent was read", slug: "-Users-you-Code-newproj", want: "~/Code/newproj"},
		{name: "a worktree removed since its parent was read, its repository", slug: "-Users-you-Code-web-git-wt-feature", want: "~/Code/web.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Of("", scratchpad("/private/tmp", tt.slug, scratchOne, "")); got != tt.want {
				t.Errorf("a scratchpad of %s is %q's, want %q's", tt.slug, got, tt.want)
			}
		})
	}
}

// Projects is read from several goroutines at once, as the watch's reads
// and its event loop read it, each getting the one project.
func TestProjectsIsReadFromSeveralGoroutinesAtOnce(t *testing.T) {
	p := machineProjects(machine())
	dirs := []string{
		scratchpad("/private/tmp", "-Users-you-Code-api", scratchOne, ""), scratchpad("/private/tmp", "-Users-you-Code-api", scratchTwo, "/probe"),
		"/scratch/tmp/call-1", "~/code/api", "~/Code/api",
	}
	want := []string{"~/Code/api", "~/Code/api", status.TemporaryFolders, "~/Code/api", "~/Code/api"}
	var wg sync.WaitGroup
	got := make([][]string, 8)
	for g := range got {
		wg.Go(func() {
			for _, dir := range dirs {
				got[g] = append(got[g], p.Of("", dir))
			}
		})
	}
	wg.Wait()
	for g, read := range got {
		if !slices.Equal(read, want) {
			t.Errorf("goroutine %d read %v, want %v", g, read, want)
		}
	}
}
