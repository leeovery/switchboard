package launch_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/launch"
)

// A worktree's git directory, as git links a worktree to its main
// repository's: the worktree's .git file names it, and its commondir leads
// back to the main repository's .git, two directories up.
const (
	worktreeGitDir = "api/.git/worktrees/fix-login"
	commonDir      = "../..\n"
)

func TestFindProjectWalksUpToTheRepositoryItsDirectoryIsIn(t *testing.T) {
	tests := []struct {
		name string
		// dirs and files are the fixture's directories and files, as lay
		// lays them out in a root of no link; links its links, each to the
		// path it names; and pipes its named pipes.
		dirs  []string
		files map[string]string
		links map[string]string
		pipes []string
		// start is where the session starts, and want the project it runs
		// in, by their paths from the root.
		start, want string
	}{
		{name: "a repository", dirs: []string{"api/.git"}, start: "api", want: "api"},
		{name: "a subdirectory of one", dirs: []string{"api/.git", "api/cmd/server"}, start: "api/cmd/server", want: "api"},
		{
			name: "a repository within another, the nearest", dirs: []string{"api/.git", "api/vendor/lib/.git"},
			start: "api/vendor/lib", want: "api/vendor/lib",
		},
		{
			name: "a linked worktree, its paths absolute", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "<root>/api/.git\n"},
			start: "fix-login", want: "api",
		},
		{
			name: "a linked worktree, its paths relative", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: ../" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": commonDir},
			start: "fix-login", want: "api",
		},
		{
			name: "a linked worktree, its lines ending in a carriage return", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\r\n", worktreeGitDir + "/commondir": "../..\r\n"},
			start: "fix-login", want: "api",
		},
		{
			name: "a subdirectory of a linked worktree", dirs: []string{worktreeGitDir, "fix-login/web"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": commonDir},
			start: "fix-login/web", want: "api",
		},
		{
			name: "a worktree Claude Code made, within its repository", dirs: []string{worktreeGitDir, "api/.claude/worktrees/fix-login"},
			files: map[string]string{
				"api/.claude/worktrees/fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": commonDir,
			},
			start: "api/.claude/worktrees/fix-login", want: "api",
		},
		{
			name: "a git directory without a commondir, as a submodule's, the worktree's own", dirs: []string{"app/.git/modules/lib", "app/lib"},
			files: map[string]string{"app/lib/.git": "gitdir: ../.git/modules/lib\n"},
			start: "app/lib", want: "app/lib",
		},
		{name: "in no repository, the directory itself", dirs: []string{"notes/2026"}, start: "notes/2026", want: "notes/2026"},
		{name: "within a bare repository, the directory itself", dirs: []string{"api.git/refs/heads"}, start: "api.git/refs/heads", want: "api.git/refs/heads"},
		{
			name: "a worktree of a bare repository, the bare repository", dirs: []string{"api.git/worktrees/fix-login", "fix-login/web"},
			files: map[string]string{
				"fix-login/.git": "gitdir: <root>/api.git/worktrees/fix-login\n", "api.git/worktrees/fix-login/commondir": commonDir, "api.git/config": bareConfig,
			},
			start: "fix-login/web", want: "api.git",
		},
		{
			name: "a worktree beside a bare repository in .bare, the folder holding it", dirs: []string{"api/.bare/worktrees/main", "api/main/web"},
			files: map[string]string{
				"api/main/.git": "gitdir: <root>/api/.bare/worktrees/main\n", "api/.bare/worktrees/main/commondir": commonDir, "api/.bare/config": bareConfig,
			},
			start: "api/main/web", want: "api",
		},
		{
			name: "a worktree outside the folder holding a bare repository in .bare, the bare repository", dirs: []string{"api/.bare/worktrees/main", "wt/main"},
			files: map[string]string{
				"wt/main/.git": "gitdir: <root>/api/.bare/worktrees/main\n", "api/.bare/worktrees/main/commondir": commonDir, "api/.bare/config": bareConfig,
			},
			start: "wt/main", want: "api/.bare",
		},
		{
			name: "a worktree of a bare repository in a folder named with a dot, not .bare, the bare repository, as a home's dotfiles",
			dirs: []string{"home/.dotfiles/worktrees/main", "wt/main"},
			files: map[string]string{
				"wt/main/.git": "gitdir: <root>/home/.dotfiles/worktrees/main\n", "home/.dotfiles/worktrees/main/commondir": commonDir, "home/.dotfiles/config": bareConfig,
			},
			start: "wt/main", want: "home/.dotfiles",
		},
		{
			name: "a worktree beside a bare repository in a folder named with a dot, not .bare, the bare repository, never the folder holding both",
			dirs: []string{"repos/.api/worktrees/main", "repos/api-main"},
			files: map[string]string{
				"repos/api-main/.git": "gitdir: <root>/repos/.api/worktrees/main\n", "repos/.api/worktrees/main/commondir": commonDir, "repos/.api/config": bareConfig,
			},
			start: "repos/api-main", want: "repos/.api",
		},
		{
			name:  "a worktree whose common directory, not .git, no config says is bare, the worktree's own",
			dirs:  []string{"api.git/worktrees/fix-login", "fix-login/web"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/api.git/worktrees/fix-login\n", "api.git/worktrees/fix-login/commondir": commonDir},
			start: "fix-login/web", want: "fix-login",
		},
		{
			name:  "a worktree beside a common directory in .bare that no config says is bare, the worktree's own",
			dirs:  []string{"api/.bare/worktrees/main", "api/main/web"},
			files: map[string]string{"api/main/.git": "gitdir: <root>/api/.bare/worktrees/main\n", "api/.bare/worktrees/main/commondir": commonDir},
			start: "api/main/web", want: "api/main",
		},
		{
			name: "a repository whose git directory is kept elsewhere", dirs: []string{"store/api.git", "api/cmd"},
			files: map[string]string{"api/.git": "gitdir: <root>/store/api.git\n"},
			start: "api/cmd", want: "api",
		},
		{
			name: "a worktree of a repository whose git directory is kept elsewhere, the worktree's own",
			dirs: []string{"store/api.git/worktrees/fix-login", "fix-login"},
			files: map[string]string{
				"fix-login/.git": "gitdir: <root>/store/api.git/worktrees/fix-login\n", "store/api.git/worktrees/fix-login/commondir": commonDir,
			},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "a worktree of a submodule, the worktree's own", dirs: []string{"app/.git/modules/lib/worktrees/fix", "fix/web"},
			files: map[string]string{"fix/.git": "gitdir: <root>/app/.git/modules/lib/worktrees/fix\n", "app/.git/modules/lib/worktrees/fix/commondir": commonDir},
			start: "fix/web", want: "fix",
		},
		{
			name: "a .git file naming no git directory, the directory itself", dirs: []string{"api/.git", "api/web"},
			files: map[string]string{"api/web/.git": "not a link to a git directory\n"},
			start: "api/web", want: "api/web",
		},
		{
			name: "a .git file naming an empty git directory, the directory itself", dirs: []string{"api/.git", "api/web"},
			files: map[string]string{"api/web/.git": "gitdir: \n"},
			start: "api/web", want: "api/web",
		},
		{
			name: "a .git file naming a git directory that isn't there, the directory itself", dirs: []string{"fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/gone\n"},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "a .git file longer than git writes, the directory itself", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{
				"fix-login/.git":              "gitdir: <root>/" + worktreeGitDir + "\n" + strings.Repeat("\n", 4096),
				worktreeGitDir + "/commondir": commonDir,
			},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "a commondir naming a directory that isn't there, the directory itself", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "../../../../gone/.git\n"},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "an empty commondir, the directory itself", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "\n"},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "a .git that's a pipe, never read, the directory itself", dirs: []string{"api/.git", "api/web"},
			pipes: []string{"api/web/.git"},
			start: "api/web", want: "api/web",
		},
		{
			name: "a commondir that's a pipe, never read, the directory itself", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n"},
			pipes: []string{worktreeGitDir + "/commondir"},
			start: "fix-login", want: "fix-login",
		},
		{
			name: "started through a link into a repository's subdirectory, walked with its links resolved first: the repository, by the path given",
			dirs: []string{"api/.git", "api/cmd"}, links: map[string]string{"cmd": "api/cmd"},
			start: "cmd", want: "api",
		},
		{
			name: "started through a link within a repository leading outside every one, walked as given where resolved finds none: the repository, as its pool is",
			dirs: []string{"api/.git", "other/lib"}, links: map[string]string{"api/lib": "other/lib"},
			start: "api/lib", want: "api",
		},
		{
			name: "started through a link within a worktree leading outside every repository, walked as given where resolved finds none: the worktree's repository",
			dirs: []string{"code/acme/.git/worktrees/x", "wt/x", "dropbox/docs"}, links: map[string]string{"wt/x/docs": "dropbox/docs"},
			files: map[string]string{"wt/x/.git": "gitdir: <root>/code/acme/.git/worktrees/x\n", "code/acme/.git/worktrees/x/commondir": commonDir},
			start: "wt/x/docs", want: "code/acme",
		},
		{
			name: "started through a link into a worktree's subdirectory, walked with its links resolved first: the worktree's repository, as its pool is",
			dirs: []string{"code/acme/.git/worktrees/x", "wt/x/web"}, links: map[string]string{"acme-web": "wt/x/web"},
			files: map[string]string{"wt/x/.git": "gitdir: <root>/code/acme/.git/worktrees/x\n", "code/acme/.git/worktrees/x/commondir": commonDir},
			start: "acme-web", want: "code/acme",
		},
		{
			name: "started through a link, a worktree whose repository is outside it, the repository named by the shared folder above the link",
			dirs: []string{"elsewhere/api/.git/worktrees/fix-login", "real/fix-login"}, links: map[string]string{"code": "real"},
			files: map[string]string{
				"real/fix-login/.git": "gitdir: <root>/elsewhere/api/.git/worktrees/fix-login\n", "elsewhere/api/.git/worktrees/fix-login/commondir": commonDir,
			},
			start: "code/fix-login", want: "elsewhere/api",
		},
		{
			name: "started through a link to a repository, the repository by the link's path",
			dirs: []string{"api/.git"}, links: map[string]string{"code": "api"},
			start: "code", want: "code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := realDir(t)
			lay(t, root, tt.dirs, tt.files)
			for path, target := range tt.links {
				if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, path)); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range tt.pipes {
				if err := syscall.Mkfifo(filepath.Join(root, path), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			start, want := filepath.Join(root, tt.start), filepath.Join(root, tt.want)
			if got := launch.FindProject(start); got != want {
				t.Errorf("FindProject(%q) = %q, want %q", start, got, want)
			}
		})
	}
}

func TestFindProjectGivesAMainRepositoryByThePathTheWalkTook(t *testing.T) {
	tests := []struct {
		name string
		// dirs and files are the fixture's directories and files, as lay
		// lays them out, git's paths written as git writes them, every link
		// resolved.
		dirs  []string
		files map[string]string
		// start is where the session starts, by its path from a link to the
		// fixture's root, as a session walks it; and want the project it runs
		// in, "<link>" in it standing for the link.
		start, want string
	}{
		{
			name: "a worktree within its repository, the repository through the link", dirs: []string{worktreeGitDir, "api/.claude/worktrees/fix-login/web"},
			files: map[string]string{
				"api/.claude/worktrees/fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "<root>/api/.git\n",
			},
			start: "api/.claude/worktrees/fix-login/web", want: "<link>/api",
		},
		{
			name: "a worktree within its repository, its paths relative, the repository through the link",
			dirs: []string{worktreeGitDir, "api/.claude/worktrees/fix-login"},
			files: map[string]string{
				"api/.claude/worktrees/fix-login/.git": "gitdir: ../../../.git/worktrees/fix-login\n", worktreeGitDir + "/commondir": commonDir,
			},
			start: "api/.claude/worktrees/fix-login", want: "<link>/api",
		},
		{name: "a repository, through the link", dirs: []string{"api/.git", "api/cmd"}, start: "api/cmd", want: "<link>/api"},
		{
			name: "a worktree beside its repository, the repository under the link the walk went through", dirs: []string{worktreeGitDir, "fix-login"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "<root>/api/.git\n"},
			start: "fix-login", want: "<link>/api",
		},
		{
			name: "a subdirectory of a worktree beside its repository, the repository under the link", dirs: []string{worktreeGitDir, "fix-login/cmd/server"},
			files: map[string]string{"fix-login/.git": "gitdir: <root>/" + worktreeGitDir + "\n", worktreeGitDir + "/commondir": "<root>/api/.git\n"},
			start: "fix-login/cmd/server", want: "<link>/api",
		},
		{
			name: "a worktree beside its bare repository, the bare repository under the link", dirs: []string{"api.git/worktrees/fix-login", "fix-login"},
			files: map[string]string{
				"fix-login/.git": "gitdir: <root>/api.git/worktrees/fix-login\n", "api.git/worktrees/fix-login/commondir": "<root>/api.git\n", "api.git/config": bareConfig,
			},
			start: "fix-login", want: "<link>/api.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := realDir(t)
			lay(t, root, tt.dirs, tt.files)
			link := filepath.Join(t.TempDir(), "code")
			if err := os.Symlink(root, link); err != nil {
				t.Fatal(err)
			}

			start, want := filepath.Join(link, tt.start), strings.ReplaceAll(tt.want, "<link>", link)
			if got := launch.FindProject(start); got != want {
				t.Errorf("FindProject(%q) = %q, want %q", start, got, want)
			}
		})
	}
}

func TestFindProjectTakesAnUnreadableFileForNoRepository(t *testing.T) {
	tests := []struct {
		name string
		// unreadable is the file that can't be read, by its path from the
		// fixture's root.
		unreadable string
	}{
		{name: "a .git file", unreadable: "fix-login/.git"},
		{name: "a commondir", unreadable: worktreeGitDir + "/commondir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mkdir(t, filepath.Join(root, worktreeGitDir))
			mkdir(t, filepath.Join(root, "fix-login"))
			write(t, filepath.Join(root, "fix-login/.git"), "gitdir: "+filepath.Join(root, worktreeGitDir)+"\n")
			write(t, filepath.Join(root, worktreeGitDir, "commondir"), commonDir)
			if err := os.Chmod(filepath.Join(root, tt.unreadable), 0); err != nil {
				t.Fatal(err)
			}

			start := filepath.Join(root, "fix-login")
			if got := launch.FindProject(start); got != start {
				t.Errorf("FindProject(%q) = %q, want the directory itself, %s unreadable", start, got, tt.name)
			}
		})
	}
}

func TestFindProjectOfNoDirectoryIsNone(t *testing.T) {
	if got := launch.FindProject(""); got != "" {
		t.Errorf(`FindProject("") = %q, want none`, got)
	}
}

// realDir returns a directory of the test's own, by its real path, every
// link resolved, as git writes one.
func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// lay lays out a fixture at root: its directories, dirs, and its files,
// files, each by its path from root, each file holding its text, "<root>" in
// it standing for root.
func lay(t *testing.T, root string, dirs []string, files map[string]string) {
	t.Helper()
	for _, dir := range dirs {
		mkdir(t, filepath.Join(root, dir))
	}
	for path, text := range files {
		write(t, filepath.Join(root, path), strings.ReplaceAll(text, "<root>", root))
	}
}

// mkdir makes the directory at path, and those it's in.
func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// write writes text into the file at path.
func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The folder a worktree since removed was kept in gives the project
// FindProject found of the worktree while it was there, in every layout it
// reads: Claude Code's worktrees, and a subfolder, of a repository; a bare
// repository with a worktree in a subfolder of it, and one named as a
// folder of it begins; a bare repository in .bare, with a .git naming it in
// the folder holding both and without; and a bare repository in a folder
// named with a dot, as a home's dotfiles.
func TestARemovedWorktreeIsTheProjectFindProjectFoundOfItLive(t *testing.T) {
	tests := []struct {
		name  string
		dirs  []string
		files map[string]string
		// worktree is where it's kept, and left the folder its removal
		// leaves the deepest a scratchpad's slug names, by their paths
		// from the root.
		worktree, left string
	}{
		{
			name: "Claude Code's worktree of a repository", dirs: []string{"api/.git/worktrees/fix", "api/.claude/worktrees/fix"},
			files: map[string]string{
				"api/.claude/worktrees/fix/.git": "gitdir: <root>/api/.git/worktrees/fix\n", "api/.git/worktrees/fix/commondir": commonDir,
			},
			worktree: "api/.claude/worktrees/fix", left: "api/.claude/worktrees",
		},
		{name: "a subfolder of a repository", dirs: []string{"api/.git", "api/web/src"}, worktree: "api/web/src", left: "api/web"},
		{
			name: "a worktree in a subfolder of a bare repository", dirs: []string{"api.git/hooks", "api.git/worktrees/feature", "api.git/wt/feature"},
			files: map[string]string{
				"api.git/config": bareConfig, "api.git/wt/feature/.git": "gitdir: <root>/api.git/worktrees/feature\n",
				"api.git/worktrees/feature/commondir": commonDir,
			},
			worktree: "api.git/wt/feature", left: "api.git/wt",
		},
		{
			name: "a worktree of a bare repository named as a folder of it begins", dirs: []string{"api.git/hooks", "api.git/worktrees/hooks-fix", "api.git/hooks-fix"},
			files: map[string]string{
				"api.git/config": bareConfig, "api.git/hooks-fix/.git": "gitdir: <root>/api.git/worktrees/hooks-fix\n",
				"api.git/worktrees/hooks-fix/commondir": commonDir,
			},
			worktree: "api.git/hooks-fix", left: "api.git/hooks",
		},
		{
			name: "a worktree beside a bare repository in .bare", dirs: []string{"api/.bare/worktrees/main", "api/main"},
			files: map[string]string{
				"api/.bare/config": bareConfig, "api/main/.git": "gitdir: <root>/api/.bare/worktrees/main\n", "api/.bare/worktrees/main/commondir": commonDir,
			},
			worktree: "api/main", left: "api",
		},
		{
			name: "a worktree beside a bare repository in .bare, a .git naming it beside them", dirs: []string{"api/.bare/worktrees/main", "api/main"},
			files: map[string]string{
				"api/.git": "gitdir: ./.bare\n", "api/.bare/config": bareConfig, "api/main/.git": "gitdir: <root>/api/.bare/worktrees/main\n",
				"api/.bare/worktrees/main/commondir": commonDir,
			},
			worktree: "api/main", left: "api",
		},
		{
			name: "a worktree in a bare repository in a folder named with a dot", dirs: []string{"home/.dotfiles/worktrees/main", "home/.dotfiles/wt/main"},
			files: map[string]string{
				"home/.dotfiles/config": bareConfig, "home/.dotfiles/wt/main/.git": "gitdir: <root>/home/.dotfiles/worktrees/main\n",
				"home/.dotfiles/worktrees/main/commondir": commonDir,
			},
			worktree: "home/.dotfiles/wt/main", left: "home/.dotfiles/wt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			lay(t, root, tt.dirs, tt.files)
			live := launch.FindProject(filepath.Join(root, tt.worktree))
			if err := os.RemoveAll(filepath.Join(root, tt.worktree)); err != nil {
				t.Fatal(err)
			}
			if got, ok := launch.RepositoryOf(filepath.Join(root, tt.left)); got != live || !ok {
				t.Errorf("removed, RepositoryOf(%s) = %q, %v, want %q, as FindProject found it live", tt.left, got, ok, live)
			}
		})
	}
}

// A folder of no repository, and one beside a .bare no config says is
// bare, is in none.
func TestAFolderOfNoRepositoryHasNone(t *testing.T) {
	root := t.TempDir()
	lay(t, root, []string{"code/old", "api/.bare/refs"}, map[string]string{"api/.bare/config": notBareConfig})
	for _, left := range []string{"code/old", "api"} {
		if got, ok := launch.RepositoryOf(filepath.Join(root, left)); ok {
			t.Errorf("RepositoryOf(%s) = %q, true, want none", left, got)
		}
	}
	if _, ok := launch.RepositoryOf(""); ok {
		t.Error(`RepositoryOf("") is one, want none`)
	}
}
