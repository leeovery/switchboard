package launch_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
)

// gitFile is a worktree's .git file as git writes it, naming its git
// directory, gitDir, by its path from the fixture's root.
func gitFile(gitDir string) string {
	return "gitdir: <root>/" + gitDir + "\n"
}

// toMain is a worktree's commondir as git writes it, leading back from its
// git directory to the main repository's .git, two directories up.
const toMain = "../..\n"

// planted lays out a fixture at root: its directories, dirs; its files,
// files, each holding its text, "<root>" in it standing for root, the
// directories they're in made too; and its links, each to the path it names,
// written as given. Each is by its path from root.
func planted(t *testing.T, root string, dirs []string, files, links map[string]string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, text := range files {
		at := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(strings.ReplaceAll(text, "<root>", root)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for path, target := range links {
		if err := os.Symlink(target, filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
}

// resolvedTempDir returns a directory of the test's own, by its real path,
// every link resolved, as git writes one.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindWorktreeMapsALinkedWorktreeOntoItsMainCheckout(t *testing.T) {
	tests := []struct {
		name  string
		dirs  []string
		files map[string]string
		pipes []string
		// start is where the session starts, and wantMapped its place in the
		// main checkout, by their paths from the root, "" for none.
		start, wantMapped string
		wantLost          bool
	}{
		{name: "a main checkout: in none", dirs: []string{"api/.git", "api/cmd"}, start: "api/cmd"},
		{name: "in no repository: in none", dirs: []string{"notes"}, start: "notes"},
		{
			name: "a linked worktree beside its repository, at its root", dirs: []string{"api/.git/worktrees/fix", "fix"},
			files: map[string]string{"fix/.git": gitFile("api/.git/worktrees/fix"), "api/.git/worktrees/fix/commondir": toMain},
			start: "fix", wantMapped: "api",
		},
		{
			name: "a subdirectory of one, the same below the main checkout", dirs: []string{"api/.git/worktrees/fix", "fix/cmd/server"},
			files: map[string]string{"fix/.git": gitFile("api/.git/worktrees/fix"), "api/.git/worktrees/fix/commondir": toMain},
			start: "fix/cmd/server", wantMapped: "api/cmd/server",
		},
		{
			name: "a worktree Claude Code made within its repository", dirs: []string{"api/.git/worktrees/agent-1", "api/.claude/worktrees/agent-1/web"},
			files: map[string]string{
				"api/.claude/worktrees/agent-1/.git": gitFile("api/.git/worktrees/agent-1"), "api/.git/worktrees/agent-1/commondir": toMain,
			},
			start: "api/.claude/worktrees/agent-1/web", wantMapped: "api/web",
		},
		{
			name: "a submodule: in none", dirs: []string{"app/.git/modules/lib", "app/lib/src"},
			files: map[string]string{"app/lib/.git": "gitdir: ../.git/modules/lib\n"},
			start: "app/lib/src",
		},
		{
			name: "a bare repository's worktree: in none", dirs: []string{"api.git/worktrees/fix", "fix"},
			files: map[string]string{"fix/.git": gitFile("api.git/worktrees/fix"), "api.git/worktrees/fix/commondir": toMain},
			start: "fix",
		},
		{
			name: "a .git file naming a git directory that isn't there, at its root: lost", dirs: []string{"fix"},
			files: map[string]string{"fix/.git": gitFile("gone/.git/worktrees/fix")},
			start: "fix", wantLost: true,
		},
		{
			name: "below its root: lost", dirs: []string{"fix/web"},
			files: map[string]string{"fix/.git": gitFile("gone/.git/worktrees/fix")},
			start: "fix/web", wantLost: true,
		},
		{
			name: "a .git file naming no git directory: lost", dirs: []string{"fix"},
			files: map[string]string{"fix/.git": "not a link to a git directory\n"},
			start: "fix", wantLost: true,
		},
		{
			name: "a .git file longer than git writes: lost", dirs: []string{"api/.git/worktrees/fix", "fix"},
			files: map[string]string{
				"fix/.git": gitFile("api/.git/worktrees/fix") + strings.Repeat("\n", 4096), "api/.git/worktrees/fix/commondir": toMain,
			},
			start: "fix", wantLost: true,
		},
		{name: "a .git that's a pipe, never read: lost", dirs: []string{"fix"}, pipes: []string{"fix/.git"}, start: "fix", wantLost: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := resolvedTempDir(t)
			planted(t, root, tt.dirs, tt.files, nil)
			for _, path := range tt.pipes {
				if err := syscall.Mkfifo(filepath.Join(root, path), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			want := launch.Worktree{Lost: tt.wantLost}
			if tt.wantMapped != "" {
				want.Mapped = filepath.Join(root, tt.wantMapped)
			}
			if got := launch.FindWorktree(filepath.Join(root, tt.start)); got != want {
				t.Errorf("FindWorktree(%q) = %+v, want %+v", tt.start, got, want)
			}
		})
	}
}

func TestFindWorktreeTakesAnUnreadableGitFileForLost(t *testing.T) {
	root := resolvedTempDir(t)
	planted(t, root, []string{"api/.git/worktrees/fix", "fix"}, map[string]string{
		"fix/.git": gitFile("api/.git/worktrees/fix"), "api/.git/worktrees/fix/commondir": toMain,
	}, nil)
	if err := os.Chmod(filepath.Join(root, "fix", ".git"), 0); err != nil {
		t.Fatal(err)
	}

	if got := launch.FindWorktree(filepath.Join(root, "fix")); got != (launch.Worktree{Lost: true}) {
		t.Errorf("FindWorktree() = %+v, want it lost, its .git file unreadable", got)
	}
}

func TestFindWorktreeOfNoDirectoryIsNone(t *testing.T) {
	if got := launch.FindWorktree(""); got != (launch.Worktree{}) {
		t.Errorf(`FindWorktree("") = %+v, want none`, got)
	}
}

// worktreeTree lays out, under a directory of the test's own, which it
// returns, links resolved, the repositories the worktree tests start in, as
// git writes them, each in Code: acme, Acme's directory, with a worktree
// beside it, acme-feature, one Claude Code made within it, one inside
// Client's directory, client/acme-wt, and one, via-wt, whose .git names acme
// through the link links/acme; web, in no fence, with a worktree inside
// Acme's, acme/web-wt; mono, whose api subfolder is Client's, with a worktree
// beside it, mono-wt; a submodule, other/app/lib, its git directory within
// acme's; a worktree, bare-wt, of a bare repository within acme; a worktree,
// gone-wt, whose git directory isn't there; and the links link-wt, to
// acme-feature, and link-api, to mono-wt's api.
func worktreeTree(t *testing.T) string {
	t.Helper()
	root := resolvedTempDir(t)
	planted(t, root,
		[]string{
			"Code/acme/api", "Code/acme/.git/modules/lib", "Code/acme-feature/api", "Code/acme/.claude/worktrees/agent-1",
			"Code/client/acme-wt", "Code/acme/web-wt", "Code/web", "Code/mono/api", "Code/mono-wt/api", "Code/via-wt",
			"Code/other/app/.git", "Code/other/app/lib", "Code/bare-wt", "Code/gone-wt/sub", "links",
		},
		map[string]string{
			"Code/acme-feature/.git":                        gitFile("Code/acme/.git/worktrees/feature"),
			"Code/acme/.git/worktrees/feature/commondir":    toMain,
			"Code/acme/.claude/worktrees/agent-1/.git":      gitFile("Code/acme/.git/worktrees/agent-1"),
			"Code/acme/.git/worktrees/agent-1/commondir":    toMain,
			"Code/client/acme-wt/.git":                      gitFile("Code/acme/.git/worktrees/client-wt"),
			"Code/acme/.git/worktrees/client-wt/commondir":  toMain,
			"Code/via-wt/.git":                              gitFile("links/acme/.git/worktrees/via"),
			"Code/acme/.git/worktrees/via/commondir":        toMain,
			"Code/acme/web-wt/.git":                         gitFile("Code/web/.git/worktrees/in-acme"),
			"Code/web/.git/worktrees/in-acme/commondir":     toMain,
			"Code/mono-wt/.git":                             gitFile("Code/mono/.git/worktrees/mono-wt"),
			"Code/mono/.git/worktrees/mono-wt/commondir":    toMain,
			"Code/other/app/lib/.git":                       gitFile("Code/acme/.git/modules/lib"),
			"Code/bare-wt/.git":                             gitFile("Code/acme/api.git/worktrees/bare-wt"),
			"Code/acme/api.git/worktrees/bare-wt/commondir": toMain,
			"Code/gone-wt/.git":                             gitFile("Code/gone/.git/worktrees/gone-wt"),
		},
		map[string]string{"links/acme": "../Code/acme", "Code/link-wt": "acme-feature", "Code/link-api": "mono-wt/api"},
	)
	return root
}

// worktreeConfig is pooledConfig with Client too, side its account, the
// pools fenced in worktreeTree, made under root: Acme to Code/acme, and
// Client to Code/client and Code/mono/api.
func worktreeConfig(root string) *config.Config {
	cfg := pooledConfig()
	client := config.Pool{ID: clientPool.ID, Label: clientPool.Label, Dirs: []string{filepath.Join(root, "Code", "client"), filepath.Join(root, "Code", "mono", "api")}}
	cfg.Pools = append(cfg.Pools, client)
	cfg.Pools[1].Dirs = []string{filepath.Join(root, "Code", "acme")}
	cfg.Accounts[2].Pool = client.ID
	return cfg
}

// worktreeHarness is a harness whose launches start in dir, by its path from
// root, home, every link resolved and every worktree found.
func worktreeHarness(t *testing.T, root, dir string) *harness {
	t.Helper()
	h := newHarness(t)
	h.launcher.Home = root
	h.launcher.EvalSymlinks = filepath.EvalSymlinks
	h.launcher.Worktree = launch.FindWorktree
	h.launcher.Dir = filepath.Join(root, dir)
	return h
}

func TestRunPlacesAWorktreeInItsRepositorysPool(t *testing.T) {
	root := worktreeTree(t)
	tests := []struct {
		name string
		dir  string
		// only are the ids of the pools configured, every one of
		// worktreeConfig's where nil.
		only []string
		// wantPool is the pool the session starts in, "" where it's refused,
		// stderr hearing wantWhy.
		wantPool string
		wantWhy  string
	}{
		{name: "a main checkout inside a fence", dir: "Code/acme/api", wantPool: "q7mt"},
		{name: "a worktree Claude Code made within a fenced repository", dir: "Code/acme/.claude/worktrees/agent-1", wantPool: "q7mt"},
		{name: "a worktree outside every fence, of a fenced repository: the repository's", dir: "Code/acme-feature", wantPool: "q7mt"},
		{name: "a subfolder of one", dir: "Code/acme-feature/api", wantPool: "q7mt"},
		{name: "one whose .git names its repository through a link", dir: "Code/via-wt", wantPool: "q7mt"},
		{name: "one reached through a link", dir: "Code/link-wt", wantPool: "q7mt"},
		{name: "a fenced subfolder of a repository, in a worktree", dir: "Code/mono-wt/api", wantPool: "h4w2"},
		{name: "the same, reached through a link", dir: "Code/link-api", wantPool: "h4w2"},
		{name: "a worktree of a repository whose fence is a subfolder, outside it", dir: "Code/mono-wt", wantPool: "b2n8"},
		{name: "a submodule, its git directory within a fence: where it lies", dir: "Code/other/app/lib", wantPool: "b2n8"},
		{name: "a bare repository's worktree, the repository within a fence: where it lies", dir: "Code/bare-wt", wantPool: "b2n8"},
		{
			name: "a worktree in one fence, of a repository in another", dir: "Code/client/acme-wt",
			wantWhy: "~/Code/client/acme-wt is in pool Client, its repository ~/Code/acme in pool Acme",
		},
		{
			name: "a worktree in a fence, of an unfenced repository", dir: "Code/acme/web-wt",
			wantWhy: "~/Code/acme/web-wt is in pool Acme, its repository ~/Code/web in pool Default",
		},
		{
			name: "a worktree in a fence, of a repository in no pool, every pool fenced", dir: "Code/acme/web-wt",
			only:    []string{acmePool.ID, clientPool.ID},
			wantWhy: "~/Code/acme/web-wt is in pool Acme, its repository ~/Code/web in no pool",
		},
		{
			name: "a worktree whose git directory is gone, a pool fenced", dir: "Code/gone-wt",
			wantWhy: "can't tell which pool ~/Code/gone-wt is in: its .git file leads to no repository that can be found",
		},
		{
			name: "below its root", dir: "Code/gone-wt/sub",
			wantWhy: "can't tell which pool ~/Code/gone-wt/sub is in: its .git file leads to no repository that can be found",
		},
		{name: "a worktree whose git directory is gone, no pool fenced: the unfenced pool", dir: "Code/gone-wt", only: []string{defaultPool.ID}, wantPool: "b2n8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := worktreeHarness(t, root, tt.dir)
			cfg := worktreeConfig(root)
			if tt.only != nil {
				cfg.Pools = slices.DeleteFunc(cfg.Pools, func(p config.Pool) bool { return !slices.Contains(tt.only, p.ID) })
			}
			rt := knowing(cfg)

			err := h.launcher.Run(t.Context(), pooledRoute(cfg, rt, ""), nil)
			if tt.wantPool == "" {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				if said, want := h.stderr.String(), "switchboard: "+tt.wantWhy+" — not starting claude\n"; said != want {
					t.Errorf("said %q on stderr, want %q", said, want)
				}
				if rt.asks > 0 {
					t.Errorf("asked the router how it is %d times, want it never asked, the pools refusing", rt.asks)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v, stderr hearing %q", err, h.stderr.String())
			}
			if got := h.environment(t)["SWITCHBOARD_POOL"]; got != tt.wantPool {
				t.Errorf("started in pool %q, want %q", got, tt.wantPool)
			}
		})
	}
}

func TestOnItsOwnLoginClaudeNeverStartsInAFencedRepositorysWorktree(t *testing.T) {
	root := worktreeTree(t)
	modes := []struct {
		name string
		// start starts claude on its own login, in the pools given; and
		// said is what stderr hears before why it refuses.
		start func(l launch.Launcher, pools config.Pools) error
		said  string
	}{
		{name: "--direct", start: func(l launch.Launcher, pools config.Pools) error { return l.Direct(nil, pools) }},
		{
			name: "unaided",
			start: func(l launch.Launcher, pools config.Pools) error {
				return l.Unaided(nil, pools, launch.CouldntReadConfig, errors.New("invalid config"))
			},
			said: "couldn't read the config (invalid config) — ",
		},
	}
	tests := []struct {
		name string
		dir  string
		// wantWhy is why stderr hears claude doesn't start, "" where it
		// starts.
		wantWhy string
	}{
		{
			name: "a worktree outside every fence, of a fenced repository", dir: "Code/acme-feature",
			wantWhy: "~/Code/acme-feature is in pool Acme, as its repository ~/Code/acme is, where claude never starts on its own login",
		},
		{
			name: "a subfolder of one", dir: "Code/acme-feature/api",
			wantWhy: "~/Code/acme-feature/api is in pool Acme, as its repository ~/Code/acme/api is, where claude never starts on its own login",
		},
		{
			name: "a worktree in one fence, of a repository in another", dir: "Code/client/acme-wt",
			wantWhy: "~/Code/client/acme-wt is in pool Client, its repository ~/Code/acme in pool Acme",
		},
		{
			name: "a worktree whose git directory is gone", dir: "Code/gone-wt",
			wantWhy: "can't tell which pool ~/Code/gone-wt is in: its .git file leads to no repository that can be found",
		},
		{name: "a worktree of an unfenced repository, outside every fence", dir: "Code/mono-wt"},
	}
	for _, mode := range modes {
		for _, tt := range tests {
			t.Run(mode.name+", "+tt.name, func(t *testing.T) {
				h := worktreeHarness(t, root, tt.dir)

				err := mode.start(h.launcher, worktreeConfig(root).Pools)
				if tt.wantWhy == "" {
					if err != nil || len(h.starts) != 1 {
						t.Errorf("error = %v, starting %q; want claude started", err, h.starts)
					}
					return
				}
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				if said, want := h.stderr.String(), "switchboard: "+mode.said+tt.wantWhy+" — not starting claude\n"; said != want {
					t.Errorf("said %q on stderr, want %q", said, want)
				}
			})
		}
	}
}
