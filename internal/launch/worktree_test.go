package launch_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
)

// gitFile is a worktree's .git file as git writes it by default, naming its
// git directory, gitDir, by its absolute path, gitDir being its path from
// the fixture's root. A submodule's, and a worktree's made with relative
// paths, name it relative to the .git file instead, written out as they are.
func gitFile(gitDir string) string {
	return "gitdir: <root>/" + gitDir + "\n"
}

// toMain is a worktree's commondir as git writes it, leading back from its
// git directory to the repository's common directory, two directories up.
const toMain = "../..\n"

// A common directory's config as git writes it: of a bare repository; of
// one that isn't, as a separate git directory's; and of a submodule's, which
// names its checkout.
const (
	bareConfig      = "[core]\n\trepositoryformatversion = 0\n\tbare = true\n"
	notBareConfig   = "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"
	submoduleConfig = "[core]\n\trepositoryformatversion = 0\n\tbare = false\n\tworktree = ../../../lib\n"
)

// perWorktree is a common directory's config turning on
// extensions.worktreeConfig, as git's sparse-checkout does, moving a bare
// repository's core.bare to its config.worktree; more is written in its
// [core] section.
func perWorktree(more string) string {
	return "[core]\n\trepositoryformatversion = 1\n" + more + "[remote \"origin\"]\n\turl = x\n[extensions]\n\tworktreeConfig = true\n"
}

// movedBare is a bare repository's config.worktree once core.bare has been
// moved there.
const movedBare = "[core]\n\tbare = true\n"

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

func TestFindWorktreeFindsEveryLinkedWorktreeAroundADirectory(t *testing.T) {
	// fix is a worktree beside api, its git directory fixGit.
	const fixGit = "api/.git/worktrees/fix"
	fix := map[string]string{"fix/.git": gitFile(fixGit), fixGit + "/commondir": toMain}
	// withFix is fix's files and more, more's in place of fix's.
	withFix := func(more map[string]string) map[string]string {
		all := maps.Clone(fix)
		maps.Copy(all, more)
		return all
	}
	tests := []struct {
		name  string
		dirs  []string
		files map[string]string
		links map[string]string
		pipes []string
		// start is where the session starts; wantAround the linked
		// worktrees around it, nearest first; and wantLost the root of one
		// that can't be placed, wantNoCheckout saying its repository has no
		// checkout: each path by its path from the root.
		start          string
		wantAround     []launch.Linked
		wantLost       string
		wantNoCheckout bool
	}{
		{name: "a main checkout: none", dirs: []string{"api/.git", "api/cmd"}, start: "api/cmd"},
		{name: "in no repository: none", dirs: []string{"notes"}, start: "notes"},
		{
			name: "a linked worktree beside its repository, at its root", dirs: []string{fixGit, "fix"}, files: fix,
			start: "fix", wantAround: []launch.Linked{{Root: "fix", Place: "api"}},
		},
		{
			name: "a subdirectory of one, the same below the main checkout", dirs: []string{fixGit, "fix/cmd/server"}, files: fix,
			start: "fix/cmd/server", wantAround: []launch.Linked{{Root: "fix", Place: "api/cmd/server"}},
		},
		{
			name: "a worktree Claude Code made within its repository", dirs: []string{"api/.git/worktrees/agent-1", "api/.claude/worktrees/agent-1/web"},
			files: map[string]string{
				"api/.claude/worktrees/agent-1/.git": gitFile("api/.git/worktrees/agent-1"), "api/.git/worktrees/agent-1/commondir": toMain,
			},
			start: "api/.claude/worktrees/agent-1/web", wantAround: []launch.Linked{{Root: "api/.claude/worktrees/agent-1", Place: "api/web"}},
		},
		{
			name: "a worktree made with relative paths", dirs: []string{fixGit, "fix"},
			files: withFix(map[string]string{"fix/.git": "gitdir: ../api/.git/worktrees/fix\n"}),
			start: "fix", wantAround: []launch.Linked{{Root: "fix", Place: "api"}},
		},
		{
			name: "the same, reached through a link, its path read from where the link leads", dirs: []string{fixGit, "fix", "links"},
			files: withFix(map[string]string{"fix/.git": "gitdir: ../api/.git/worktrees/fix\n"}), links: map[string]string{"links/fix": "../fix"},
			start: "links/fix", wantAround: []launch.Linked{{Root: "links/fix", Place: "api"}},
		},
		{
			name: "a submodule of a main checkout: none", dirs: []string{"app/.git/modules/lib", "app/lib/src"},
			files: map[string]string{"app/lib/.git": "gitdir: ../.git/modules/lib\n"},
			start: "app/lib/src",
		},
		{
			name: "the same, reached through a link: none", dirs: []string{"app/.git/modules/lib", "app/lib/src", "links"},
			files: map[string]string{"app/lib/.git": "gitdir: ../.git/modules/lib\n"}, links: map[string]string{"links/lib": "../app/lib"},
			start: "links/lib/src",
		},
		{
			name: "a submodule within a linked worktree: the worktree", dirs: []string{fixGit + "/modules/lib", "fix/lib/src"},
			files: withFix(map[string]string{"fix/lib/.git": "gitdir: ../../api/.git/worktrees/fix/modules/lib\n"}),
			start: "fix/lib/src", wantAround: []launch.Linked{{Root: "fix", Place: "api/lib/src"}},
		},
		{
			name: "a clone nested within a linked worktree: the worktree", dirs: []string{fixGit, "fix/vendor/dep/.git"}, files: fix,
			start: "fix/vendor/dep", wantAround: []launch.Linked{{Root: "fix", Place: "api/vendor/dep"}},
		},
		{
			name: "a linked worktree nested within another: both, the nearest first", dirs: []string{fixGit, "web/.git/worktrees/w", "fix/web-wt/src"},
			files:      withFix(map[string]string{"fix/web-wt/.git": gitFile("web/.git/worktrees/w"), "web/.git/worktrees/w/commondir": toMain}),
			start:      "fix/web-wt/src",
			wantAround: []launch.Linked{{Root: "fix/web-wt", Place: "web/src"}, {Root: "fix", Place: "api/web-wt/src"}},
		},
		{
			name: "a bare repository's worktree: the bare repository, whatever the folder below", dirs: []string{"api.git/worktrees/fix", "fix/web"},
			files: map[string]string{"fix/.git": gitFile("api.git/worktrees/fix"), "api.git/worktrees/fix/commondir": toMain, "api.git/config": bareConfig},
			start: "fix/web", wantAround: []launch.Linked{{Root: "fix", Place: "api.git", Bare: true}},
		},
		{
			name: "a worktree beside a bare repository in .bare", dirs: []string{"api/.bare/worktrees/main", "api/main/web"},
			files: map[string]string{"api/main/.git": gitFile("api/.bare/worktrees/main"), "api/.bare/worktrees/main/commondir": toMain, "api/.bare/config": bareConfig},
			start: "api/main/web", wantAround: []launch.Linked{{Root: "api/main", Place: "api/.bare", Bare: true}},
		},
		{
			name: "a worktree of a separate git directory: no checkout", dirs: []string{"store/api.git/worktrees/fix", "fix"},
			files: map[string]string{"fix/.git": gitFile("store/api.git/worktrees/fix"), "store/api.git/worktrees/fix/commondir": toMain, "store/api.git/config": notBareConfig},
			start: "fix", wantLost: "fix", wantNoCheckout: true,
		},
		{
			name: "a worktree of a submodule: no checkout", dirs: []string{"app/.git/modules/lib/worktrees/fix", "fix"},
			files: map[string]string{
				"fix/.git": gitFile("app/.git/modules/lib/worktrees/fix"), "app/.git/modules/lib/worktrees/fix/commondir": toMain,
				"app/.git/modules/lib/config": submoduleConfig,
			},
			start: "fix", wantLost: "fix", wantNoCheckout: true,
		},
		{
			name: "a separate git directory's checkout: none", dirs: []string{"store/api.git", "api/cmd"},
			files: map[string]string{"api/.git": gitFile("store/api.git"), "store/api.git/config": notBareConfig},
			start: "api/cmd",
		},
		{
			name: "a .git file naming a git directory that isn't there, at its root: lost", dirs: []string{"fix"},
			files: map[string]string{"fix/.git": gitFile("gone/.git/worktrees/fix")},
			start: "fix", wantLost: "fix",
		},
		{
			name: "below its root: lost, by its root", dirs: []string{"fix/web"},
			files: map[string]string{"fix/.git": gitFile("gone/.git/worktrees/fix")},
			start: "fix/web", wantLost: "fix",
		},
		{
			name: "a linked worktree within a lost one: lost", dirs: []string{fixGit, "lost/fix"},
			files: map[string]string{"lost/.git": gitFile("gone/.git/worktrees/lost"), "lost/fix/.git": gitFile(fixGit), fixGit + "/commondir": toMain},
			start: "lost/fix", wantLost: "lost",
		},
		{
			name: "a .git file naming no git directory: lost", dirs: []string{"fix"},
			files: map[string]string{"fix/.git": "not a link to a git directory\n"},
			start: "fix", wantLost: "fix",
		},
		{
			name: "a .git file longer than git writes: lost", dirs: []string{fixGit, "fix"},
			files: withFix(map[string]string{"fix/.git": gitFile(fixGit) + strings.Repeat("\n", 4096)}),
			start: "fix", wantLost: "fix",
		},
		{name: "a .git that's a pipe, never read: lost", dirs: []string{"fix"}, pipes: []string{"fix/.git"}, start: "fix", wantLost: "fix"},
		{
			name: "an empty commondir: lost", dirs: []string{fixGit, "fix"},
			files: withFix(map[string]string{fixGit + "/commondir": "\n"}),
			start: "fix", wantLost: "fix",
		},
		{
			name: "a commondir naming a directory that isn't there: lost", dirs: []string{fixGit, "fix"},
			files: withFix(map[string]string{fixGit + "/commondir": "../../../../gone/.git\n"}),
			start: "fix", wantLost: "fix",
		},
		{
			name: "a commondir that's a pipe, never read: lost", dirs: []string{fixGit, "fix"},
			files: map[string]string{"fix/.git": gitFile(fixGit)}, pipes: []string{fixGit + "/commondir"},
			start: "fix", wantLost: "fix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := resolvedTempDir(t)
			planted(t, root, tt.dirs, tt.files, tt.links)
			for _, path := range tt.pipes {
				if err := syscall.Mkfifo(filepath.Join(root, path), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			want := launch.Worktree{NoCheckout: tt.wantNoCheckout}
			for _, w := range tt.wantAround {
				want.Around = append(want.Around, launch.Linked{Root: filepath.Join(root, w.Root), Place: filepath.Join(root, w.Place), Bare: w.Bare})
			}
			if tt.wantLost != "" {
				want.Lost = filepath.Join(root, tt.wantLost)
			}
			if got := launch.FindWorktree(filepath.Join(root, tt.start)); !sameWorktree(got, want) {
				t.Errorf("FindWorktree(%q) = %+v, want %+v", tt.start, got, want)
			}
		})
	}
}

// sameWorktree reports whether a and b found the same.
func sameWorktree(a, b launch.Worktree) bool {
	return slices.Equal(a.Around, b.Around) && a.Lost == b.Lost && a.NoCheckout == b.NoCheckout
}

func TestFindWorktreeReadsWhetherARepositoryIsBare(t *testing.T) {
	tests := []struct {
		name   string
		config string
		// worktree is the common directory's config.worktree, none where "".
		worktree string
		bare     bool
	}{
		{name: "bare = true", config: bareConfig, bare: true},
		{name: "bare = false", config: notBareConfig},
		{name: "no bare", config: "[core]\n\trepositoryformatversion = 0\n"},
		{name: "no config"},
		{name: "bare alone", config: "[core]\n\tbare\n", bare: true},
		{name: "spelled otherwise, with a comment", config: "[CORE]\n\tBare = Yes ; set by hand\n", bare: true},
		{name: "1", config: "[core]\n\tbare = 1\n", bare: true},
		{name: "commented out", config: "[core]\n\t# bare = true\n"},
		{name: "in another section", config: "[core]\n\tfilemode = true\n[remote \"origin\"]\n\tbare = true\n"},
		{name: "set twice, the last counting", config: "[core]\n\tbare = true\n\tbare = false\n"},
		{name: "moved to config.worktree, as sparse-checkout moves it", config: perWorktree(""), worktree: movedBare, bare: true},
		{name: "config.worktree's winning", config: perWorktree("\tbare = true\n"), worktree: "[core]\n\tbare = false\n"},
		{name: "config's standing, config.worktree not setting it", config: perWorktree("\tbare = true\n"), worktree: "[core]\n\tsparseCheckout = true\n", bare: true},
		{name: "config.worktree's last counting", config: perWorktree(""), worktree: "[core]\n\tbare = false\n\tbare = true\n", bare: true},
		{name: "config.worktree unread, the extension off", config: "[core]\n\tbare = false\n", worktree: movedBare},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := resolvedTempDir(t)
			files := map[string]string{"fix/.git": gitFile("api.git/worktrees/fix"), "api.git/worktrees/fix/commondir": toMain}
			if tt.config != "" {
				files["api.git/config"] = tt.config
			}
			if tt.worktree != "" {
				files["api.git/config.worktree"] = tt.worktree
			}
			planted(t, root, []string{"api.git/worktrees/fix", "fix"}, files, nil)

			want := launch.Worktree{Lost: filepath.Join(root, "fix"), NoCheckout: true}
			if tt.bare {
				want = launch.Worktree{Around: []launch.Linked{{Root: filepath.Join(root, "fix"), Place: filepath.Join(root, "api.git"), Bare: true}}}
			}
			if got := launch.FindWorktree(filepath.Join(root, "fix")); !sameWorktree(got, want) {
				t.Errorf("FindWorktree() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFindWorktreeTakesAnUnreadableFileForLost(t *testing.T) {
	const fixGit = "api/.git/worktrees/fix"
	for _, unreadable := range []string{"fix/.git", fixGit + "/commondir"} {
		t.Run(unreadable, func(t *testing.T) {
			root := resolvedTempDir(t)
			planted(t, root, []string{fixGit, "fix"}, map[string]string{"fix/.git": gitFile(fixGit), fixGit + "/commondir": toMain}, nil)
			if err := os.Chmod(filepath.Join(root, unreadable), 0); err != nil {
				t.Fatal(err)
			}

			if got, want := launch.FindWorktree(filepath.Join(root, "fix")), (launch.Worktree{Lost: filepath.Join(root, "fix")}); !sameWorktree(got, want) {
				t.Errorf("FindWorktree() = %+v, want it lost, %s unreadable", got, unreadable)
			}
		})
	}
}

func TestFindWorktreeOfNoDirectoryIsNone(t *testing.T) {
	if got := launch.FindWorktree(""); !sameWorktree(got, launch.Worktree{}) {
		t.Errorf(`FindWorktree("") = %+v, want none`, got)
	}
}

// worktreeTree lays out, under a directory of the test's own, which it
// returns, links resolved, the repositories the worktree tests start in, as
// git writes them, each in Code but where it says:
//   - acme, in Acme's fence, with a submodule, lib, a link, clientward, into
//     Client's fence, and a worktree of lib, lib-wt; its worktrees
//     acme-feature, beside it, with lib checked out, a clone nested at
//     vendor/dep, a worktree of web nested at web-wt, holding a worktree of
//     mono, mono-in, a worktree of repos/tools.git nested at tools, a plain
//     folder clientward, and links shared, out to elsewhere-data, and
//     gone-link, into gone-wt; one Claude Code made within it;
//     client/acme-wt, in Client's fence; via-wt, whose .git names acme
//     through links/acme; viagit-wt, whose .git names its git directory
//     through a link to it, links/viagit; and rel-wt, made with relative
//     paths, holding a link, webby, to web's worktree web-wt2, and reached
//     through shortcuts/rel-wt too;
//   - web, in no fence, with a submodule, lib, and worktrees inside Acme's
//     fence, acme/web-wt, and beside it, web-wt2;
//   - mono, whose api subfolder is Client's, with a worktree beside it,
//     mono-wt, holding a link, to-acme, to acme-feature/api;
//   - other/app, in no fence, with a submodule, lib;
//   - bare repositories: acme/api.git, with worktrees beside acme, bare-wt,
//     and in Client's fence, client/api-wt; client/svc/.bare, with worktrees
//     client/svc/main and svc-hot; client/sparse/.bare, its core.bare moved
//     to its config.worktree, with a worktree client/sparse/main;
//     tools/.bare, in no fence, with a worktree tools/main; and, beside Code,
//     in no fence, repos/app.git, with a worktree in Client's fence,
//     client/app, and repos/tools.git;
//   - gitdirs/proj.git, beside Code, the separate git directory of
//     client/proj, with a worktree beside it, proj-wt;
//   - gone-wt, a worktree whose git directory isn't there, holding a link,
//     into-feature, to acme-feature/api;
//   - and the links link-wt, to acme-feature, link-api, to mono-wt's api,
//     to-gone, to gone-wt/sub, and, beside Code, alib and wlib, to acme's and
//     web's lib.
func worktreeTree(t *testing.T) string {
	t.Helper()
	root := resolvedTempDir(t)
	planted(t, root,
		[]string{
			"Code/acme/api", "Code/acme/.git/modules/lib/worktrees/lib-wt", "Code/acme/lib/src", "Code/acme/.claude/worktrees/agent-1",
			"Code/acme/.git/worktrees/feature/modules/lib", "Code/acme-feature/api", "Code/acme-feature/lib/src",
			"Code/acme-feature/vendor/dep/.git", "Code/acme-feature/clientward", "Code/elsewhere-data", "Code/lib-wt",
			"Code/client/acme-wt", "Code/client/site", "Code/acme/web-wt", "Code/web/.git/modules/lib", "Code/web/lib/src",
			"Code/web-wt2", "Code/mono/api", "Code/mono-wt/api", "Code/via-wt", "Code/rel-wt", "shortcuts",
			"Code/other/app/.git/modules/lib", "Code/other/app/lib", "Code/bare-wt", "Code/client/svc/main", "Code/svc-hot",
			"Code/tools/main", "Code/client/app", "Code/client/proj", "Code/proj-wt", "Code/gone-wt/sub", "links",
			"Code/lib-wt/src", "Code/acme-feature/web-wt/mono-in", "Code/acme-feature/tools", "Code/viagit-wt",
			"Code/client/api-wt", "Code/client/sparse/main",
		},
		map[string]string{
			"Code/acme/lib/.git": "gitdir: ../.git/modules/lib\n",
			"Code/lib-wt/.git":   gitFile("Code/acme/.git/modules/lib/worktrees/lib-wt"),
			"Code/acme/.git/modules/lib/worktrees/lib-wt/commondir": toMain,
			"Code/acme/.git/modules/lib/config":                     submoduleConfig,
			"Code/acme-feature/.git":                                gitFile("Code/acme/.git/worktrees/feature"),
			"Code/acme/.git/worktrees/feature/commondir":            toMain,
			"Code/acme-feature/lib/.git":                            "gitdir: ../../acme/.git/worktrees/feature/modules/lib\n",
			"Code/acme-feature/web-wt/.git":                         gitFile("Code/web/.git/worktrees/in-feature"),
			"Code/web/.git/worktrees/in-feature/commondir":          toMain,
			"Code/acme/.claude/worktrees/agent-1/.git":              gitFile("Code/acme/.git/worktrees/agent-1"),
			"Code/acme/.git/worktrees/agent-1/commondir":            toMain,
			"Code/client/acme-wt/.git":                              gitFile("Code/acme/.git/worktrees/client-wt"),
			"Code/acme/.git/worktrees/client-wt/commondir":          toMain,
			"Code/via-wt/.git":                                      gitFile("links/acme/.git/worktrees/via"),
			"Code/acme/.git/worktrees/via/commondir":                toMain,
			"Code/rel-wt/.git":                                      "gitdir: ../acme/.git/worktrees/rel\n",
			"Code/acme/.git/worktrees/rel/commondir":                toMain,
			"Code/web/lib/.git":                                     "gitdir: ../.git/modules/lib\n",
			"Code/acme/web-wt/.git":                                 gitFile("Code/web/.git/worktrees/in-acme"),
			"Code/web/.git/worktrees/in-acme/commondir":             toMain,
			"Code/web-wt2/.git":                                     gitFile("Code/web/.git/worktrees/wt2"),
			"Code/web/.git/worktrees/wt2/commondir":                 toMain,
			"Code/mono-wt/.git":                                     gitFile("Code/mono/.git/worktrees/mono-wt"),
			"Code/mono/.git/worktrees/mono-wt/commondir":            toMain,
			"Code/other/app/lib/.git":                               "gitdir: ../.git/modules/lib\n",
			"Code/bare-wt/.git":                                     gitFile("Code/acme/api.git/worktrees/bare-wt"),
			"Code/acme/api.git/worktrees/bare-wt/commondir":         toMain,
			"Code/acme/api.git/config":                              bareConfig,
			"Code/client/svc/main/.git":                             gitFile("Code/client/svc/.bare/worktrees/main"),
			"Code/client/svc/.bare/worktrees/main/commondir":        toMain,
			"Code/svc-hot/.git":                                     gitFile("Code/client/svc/.bare/worktrees/svc-hot"),
			"Code/client/svc/.bare/worktrees/svc-hot/commondir":     toMain,
			"Code/client/svc/.bare/config":                          bareConfig,
			"Code/tools/main/.git":                                  gitFile("Code/tools/.bare/worktrees/main"),
			"Code/tools/.bare/worktrees/main/commondir":             toMain,
			"Code/tools/.bare/config":                               bareConfig,
			"Code/client/app/.git":                                  gitFile("repos/app.git/worktrees/app"),
			"repos/app.git/worktrees/app/commondir":                 toMain,
			"repos/app.git/config":                                  bareConfig,
			"Code/client/proj/.git":                                 gitFile("gitdirs/proj.git"),
			"gitdirs/proj.git/config":                               notBareConfig,
			"Code/proj-wt/.git":                                     gitFile("gitdirs/proj.git/worktrees/proj-wt"),
			"gitdirs/proj.git/worktrees/proj-wt/commondir":          toMain,
			"Code/gone-wt/.git":                                     gitFile("Code/gone/.git/worktrees/gone-wt"),
			"Code/acme-feature/web-wt/mono-in/.git":                 gitFile("Code/mono/.git/worktrees/mono-in"),
			"Code/mono/.git/worktrees/mono-in/commondir":            toMain,
			"Code/acme-feature/tools/.git":                          gitFile("repos/tools.git/worktrees/tools"),
			"repos/tools.git/worktrees/tools/commondir":             toMain,
			"repos/tools.git/config":                                bareConfig,
			"Code/viagit-wt/.git":                                   gitFile("links/viagit"),
			"Code/acme/.git/worktrees/viagit/commondir":             toMain,
			"Code/client/api-wt/.git":                               gitFile("Code/acme/api.git/worktrees/api-wt"),
			"Code/acme/api.git/worktrees/api-wt/commondir":          toMain,
			"Code/client/sparse/.git":                               "gitdir: ./.bare\n",
			"Code/client/sparse/main/.git":                          gitFile("Code/client/sparse/.bare/worktrees/main"),
			"Code/client/sparse/.bare/worktrees/main/commondir":     toMain,
			"Code/client/sparse/.bare/config":                       perWorktree(""),
			"Code/client/sparse/.bare/config.worktree":              movedBare,
		},
		map[string]string{
			"links/acme": "../Code/acme", "shortcuts/rel-wt": "../Code/rel-wt", "Code/rel-wt/webby": "../web-wt2",
			"Code/acme/clientward": "../client/site", "Code/acme-feature/shared": "../elsewhere-data",
			"Code/acme-feature/gone-link": "../gone-wt/sub", "Code/gone-wt/into-feature": "../acme-feature/api",
			"Code/mono-wt/to-acme": "../acme-feature/api", "Code/link-wt": "acme-feature", "Code/link-api": "mono-wt/api",
			"Code/to-gone": "gone-wt/sub", "alib": "Code/acme/lib", "wlib": "Code/web/lib",
			"links/viagit": "../Code/acme/.git/worktrees/viagit",
		},
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

// goneWhy is why claude doesn't start in dir, inside gone-wt, whose
// repository can't be found, by their paths from home.
func goneWhy(dir string) string {
	return "can't tell which pool ~/" + dir + " is in: its .git file at ~/Code/gone-wt leads to no repository that can be found"
}

// noCheckoutWhy is why claude doesn't start in dir, in a submodule's or a
// separate git directory's worktree, rooted at root, by their paths from
// home.
func noCheckoutWhy(dir, root string) string {
	return "can't tell which pool ~/" + dir + " is in: its worktree at ~/" + root + " is a submodule's or a separate git directory's, which can't be placed"
}

func TestRunPlacesAWorktreeInItsRepositorysPool(t *testing.T) {
	root := worktreeTree(t)
	tests := []struct {
		name string
		dir  string
		// folded has dir spelled in another case from the fixture's.
		folded bool
		// only are the ids of the pools configured, every one of
		// worktreeConfig's where nil.
		only []string
		// wantPool is the pool the session starts in, "" where it's refused,
		// stderr hearing wantWhy.
		wantPool string
		wantWhy  string
	}{
		{name: "a main checkout inside a fence", dir: "Code/acme/api", wantPool: "q7mt"},
		{name: "a submodule of one", dir: "Code/acme/lib/src", wantPool: "q7mt"},
		{name: "a submodule of a fenced main checkout, reached through a link", dir: "alib", wantPool: "q7mt"},
		{name: "a submodule of a main checkout in no fence", dir: "Code/web/lib/src", wantPool: "b2n8"},
		{name: "a submodule of a main checkout in no fence, reached through a link", dir: "wlib", wantPool: "b2n8"},
		{name: "a folder of it, reached through a link", dir: "wlib/src", wantPool: "b2n8"},
		{name: "a submodule of another main checkout in no fence", dir: "Code/other/app/lib", wantPool: "b2n8"},
		{name: "a worktree Claude Code made within a fenced repository", dir: "Code/acme/.claude/worktrees/agent-1", wantPool: "q7mt"},
		{name: "a worktree outside every fence, of a fenced repository: the repository's", dir: "Code/acme-feature", wantPool: "q7mt"},
		{name: "a subfolder of one", dir: "Code/acme-feature/api", wantPool: "q7mt"},
		{name: "a submodule checked out in one", dir: "Code/acme-feature/lib/src", wantPool: "q7mt"},
		{name: "a clone nested in one", dir: "Code/acme-feature/vendor/dep", wantPool: "q7mt"},
		{name: "a link in one leading out of it: the worktree's, as given", dir: "Code/acme-feature/shared", wantPool: "q7mt"},
		{name: "one whose .git names its repository through a link", dir: "Code/via-wt", wantPool: "q7mt"},
		{name: "one reached through a link", dir: "Code/link-wt", wantPool: "q7mt"},
		{name: "one made with relative paths", dir: "Code/rel-wt", wantPool: "q7mt"},
		{name: "the same, reached through a link, its path read from where the link leads", dir: "shortcuts/rel-wt", wantPool: "q7mt"},
		{name: "a link in an unfenced repository's worktree into a fenced one's: where it leads decides", dir: "Code/mono-wt/to-acme", wantPool: "q7mt"},
		{name: "a fenced subfolder of a repository, in a worktree", dir: "Code/mono-wt/api", wantPool: "h4w2"},
		{name: "a fenced subfolder in a worktree, reached through a link", dir: "Code/link-api", wantPool: "h4w2"},
		{name: "the same, spelled in another case", dir: "code/MONO-WT/API", folded: true, wantPool: "h4w2"},
		{name: "a worktree of a repository whose fence is a subfolder, outside it", dir: "Code/mono-wt", wantPool: "b2n8"},
		{name: "a bare repository's worktree, the repository within a fence: the repository's", dir: "Code/bare-wt", wantPool: "q7mt"},
		{name: "a bare clone's worktree inside its fence", dir: "Code/client/svc/main", wantPool: "h4w2"},
		{name: "the same's worktree outside it", dir: "Code/svc-hot", wantPool: "h4w2"},
		{name: "a bare clone's worktree in no fence", dir: "Code/tools/main", wantPool: "b2n8"},
		{name: "a bare clone's worktree inside its fence, core.bare moved to config.worktree", dir: "Code/client/sparse/main", wantPool: "h4w2"},
		{name: "one whose .git names its git directory through a link, commondir read from where it leads", dir: "Code/viagit-wt", wantPool: "q7mt"},
		{
			name: "a worktree in a fence, of a bare repository outside every fence: refused, saying how to start there", dir: "Code/client/app",
			wantWhy: "~/Code/client/app is in pool Client, but in its repository it's at ~/repos/app.git, in pool Default: add that bare repository to pool Client's directories",
		},
		{name: "a separate git directory's checkout: where it lies", dir: "Code/client/proj", wantPool: "h4w2"},
		{
			name: "a worktree in a fence, of a bare repository in another: refused, naming no remedy", dir: "Code/client/api-wt",
			wantWhy: "~/Code/client/api-wt is in pool Client, but in its repository it's at ~/Code/acme/api.git, in pool Acme",
		},
		{
			name: "a bare repository's worktree outside every fence, lying in one through an outer worktree: refused, naming no remedy", dir: "Code/acme-feature/tools",
			wantWhy: "~/Code/acme-feature/tools lies in pool Acme, through the worktree ~/Code/acme-feature, but in its repository it's at ~/repos/tools.git, in pool Default",
		},
		{name: "a separate git directory's worktree", dir: "Code/proj-wt", wantWhy: noCheckoutWhy("Code/proj-wt", "Code/proj-wt")},
		{name: "a submodule's worktree", dir: "Code/lib-wt", wantWhy: noCheckoutWhy("Code/lib-wt", "Code/lib-wt")},
		{name: "below a submodule's worktree's root, by its root", dir: "Code/lib-wt/src", wantWhy: noCheckoutWhy("Code/lib-wt/src", "Code/lib-wt")},
		{
			name: "a worktree in one fence, of a repository in another", dir: "Code/client/acme-wt",
			wantWhy: "~/Code/client/acme-wt is in pool Client, but in its repository it's at ~/Code/acme, in pool Acme",
		},
		{
			name: "a worktree in a fence, of an unfenced repository", dir: "Code/acme/web-wt",
			wantWhy: "~/Code/acme/web-wt is in pool Acme, but in its repository it's at ~/Code/web, in pool Default",
		},
		{
			name: "the same, nested in a fenced repository's worktree outside the fence, naming that worktree", dir: "Code/acme-feature/web-wt",
			wantWhy: "~/Code/acme-feature/web-wt lies in pool Acme, through the worktree ~/Code/acme-feature, but in its repository it's at ~/Code/web, in pool Default",
		},
		{
			name: "a worktree nested in two, naming the outer one whose place is fenced", dir: "Code/acme-feature/web-wt/mono-in",
			wantWhy: "~/Code/acme-feature/web-wt/mono-in lies in pool Acme, through the worktree ~/Code/acme-feature, but in its repository it's at ~/Code/mono, in pool Default",
		},
		{
			name: "a link in a fenced repository's worktree made with relative paths, to an unfenced one's", dir: "Code/rel-wt/webby",
			wantWhy: "~/Code/rel-wt/webby lies in pool Acme, through the worktree ~/Code/rel-wt, but in its repository it's at ~/Code/web, in pool Default",
		},
		{
			name: "that link, reached through a link to the worktree", dir: "shortcuts/rel-wt/webby",
			wantWhy: "~/shortcuts/rel-wt/webby lies in pool Acme, through the worktree ~/shortcuts/rel-wt, but in its repository it's at ~/Code/web, in pool Default",
		},
		{
			name: "a worktree in a fence, of a repository in no pool, every pool fenced", dir: "Code/acme/web-wt",
			only:    []string{acmePool.ID, clientPool.ID},
			wantWhy: "~/Code/acme/web-wt is in pool Acme, but in its repository it's at ~/Code/web, in no pool",
		},
		{
			name: "a worktree whose place in its repository is in two pools, through its links", dir: "Code/acme-feature/clientward",
			wantWhy: "in its repository, ~/Code/acme-feature/clientward is at ~/Code/acme/clientward, in pools Acme and Client, as its links lead",
		},
		{name: "a worktree whose git directory is gone, a pool fenced", dir: "Code/gone-wt", wantWhy: goneWhy("Code/gone-wt")},
		{name: "below its root, by its root", dir: "Code/gone-wt/sub", wantWhy: goneWhy("Code/gone-wt/sub")},
		{name: "in none as given, lost where its link leads", dir: "Code/to-gone", wantWhy: goneWhy("Code/to-gone")},
		{name: "found as given, lost where its link leads", dir: "Code/acme-feature/gone-link", wantWhy: goneWhy("Code/acme-feature/gone-link")},
		{name: "lost as given, found where its link leads", dir: "Code/gone-wt/into-feature", wantWhy: goneWhy("Code/gone-wt/into-feature")},
		{name: "a worktree whose git directory is gone, no pool fenced: the unfenced pool", dir: "Code/gone-wt", only: []string{defaultPool.ID}, wantPool: "b2n8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := worktreeHarness(t, root, tt.dir)
			if _, err := os.Stat(h.launcher.Dir); err != nil && tt.folded {
				t.Skipf("the filesystem tells case apart: %v", err)
			}
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
	// inAcme is why claude doesn't start on its own login in dir, by its
	// path from home, at the same place in acme.
	inAcme := func(dir string) string {
		return "in its repository, ~/Code/acme-feature" + dir + " is at ~/Code/acme" + dir + ", in pool Acme, where claude never starts on its own login"
	}
	tests := []struct {
		name string
		dir  string
		// wantWhy is why stderr hears claude doesn't start, "" where it
		// starts.
		wantWhy string
	}{
		{name: "a worktree outside every fence, of a fenced repository", dir: "Code/acme-feature", wantWhy: inAcme("")},
		{name: "a subfolder of one", dir: "Code/acme-feature/api", wantWhy: inAcme("/api")},
		{name: "a submodule checked out in one", dir: "Code/acme-feature/lib/src", wantWhy: inAcme("/lib/src")},
		{name: "a clone nested in one", dir: "Code/acme-feature/vendor/dep", wantWhy: inAcme("/vendor/dep")},
		{name: "a fenced repository's submodule, reached through a link", dir: "alib", wantWhy: "~/alib is in pool Acme, where claude never starts on its own login"},
		{name: "an unfenced repository's submodule, reached through a link", dir: "wlib"},
		{
			name: "a link in an unfenced repository's worktree into a fenced one's, naming the place that decides", dir: "Code/mono-wt/to-acme",
			wantWhy: "in its repository, ~/Code/mono-wt/to-acme is at ~/Code/acme/api, in pool Acme, where claude never starts on its own login",
		},
		{
			name: "a worktree in one fence, of a repository in another", dir: "Code/client/acme-wt",
			wantWhy: "~/Code/client/acme-wt is in pool Client, but in its repository it's at ~/Code/acme, in pool Acme",
		},
		{
			name: "a link in a fenced repository's worktree, reached through a link, to an unfenced one's", dir: "shortcuts/rel-wt/webby",
			wantWhy: "~/shortcuts/rel-wt/webby lies in pool Acme, through the worktree ~/shortcuts/rel-wt, but in its repository it's at ~/Code/web, in pool Default",
		},
		{name: "a worktree whose git directory is gone", dir: "Code/gone-wt", wantWhy: goneWhy("Code/gone-wt")},
		{name: "found as given, lost where its link leads", dir: "Code/acme-feature/gone-link", wantWhy: goneWhy("Code/acme-feature/gone-link")},
		{name: "a separate git directory's worktree", dir: "Code/proj-wt", wantWhy: noCheckoutWhy("Code/proj-wt", "Code/proj-wt")},
		{name: "a submodule's worktree", dir: "Code/lib-wt", wantWhy: noCheckoutWhy("Code/lib-wt", "Code/lib-wt")},
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

func TestNoWorktreeIsLookedForWhereNoPoolIsFenced(t *testing.T) {
	root := worktreeTree(t)
	cfg := worktreeConfig(root)
	cfg.Pools = config.Pools{defaultPool}
	starts := map[string]func(h *harness) error{
		"Run":      func(h *harness) error { return h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil) },
		"--direct": func(h *harness) error { return h.launcher.Direct(nil, cfg.Pools) },
	}
	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			h := worktreeHarness(t, root, "Code/gone-wt")
			h.launcher.Worktree = func(dir string) launch.Worktree {
				t.Errorf("looked for a worktree around %s, no pool fenced", dir)
				return launch.Worktree{}
			}

			if err := start(h); err != nil || len(h.starts) != 1 {
				t.Errorf("error = %v, starting %q; want claude started", err, h.starts)
			}
		})
	}
}
