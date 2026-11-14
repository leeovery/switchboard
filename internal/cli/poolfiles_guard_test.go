package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// This test reads the cli package's source, so a verb can't read the
// recorded files in its pool but through poolfiles.go, where what it reads
// there is built from the pool's read: never the request ledger whole, nor
// every day of it, once its pool is known.

// poolFilesAllowed are the functions allowed to read the recorded files
// raw outside poolfiles.go, by path, a colon and the function's name, a
// method's its receiver's type's then its own. The list only shrinks: an
// entry must still be there, and still break the rule.
var poolFilesAllowed = []string{
	// The verbs' readers and the dashboard's, made: a verb reads its pool's
	// through poolfiles.go.
	"internal/cli/readers.go:app.stateReadersKnowing",
	// The dashboard's ledger, and its readers, handed to the watch, which
	// reads them through the pool it shows, its PoolRead.
	"internal/cli/dashboardpool.go:shownPools.ledgerOn",
	"internal/cli/readers.go:app.readers",
}

// poolFilesFix is how a raw read breaks the rule, and how to fix it.
const poolFilesFix = "read the recorded files in the pool through filedPool's methods in internal/cli/poolfiles.go, adding one there"

// poolFilesAllowedFix is how to fix an entry of poolFilesAllowed that's
// gone, or keeps the rule.
const poolFilesAllowedFix = "remove it from poolFilesAllowed in internal/cli/poolfiles_guard_test.go"

// rawReads are what reads the recorded files raw, or a pool's read itself,
// each by its name under the module's internal directory.
var rawReads = []string{
	"cli.app.followerIn",
	"cli.filedPool.read",
	"cli.stateReaders.ledger",
	"ledger.Reader.Days",
	"ledger.Reader.DaysOn",
	"ledger.Reader.FirstDay",
	"ledger.Reader.Lines",
}

func TestTheVerbsReadThePoolsRecordedFilesThroughItsReadAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, poolFilesBreaches(t, root, sourcecheck.ModulePath(t, root), poolFilesAllowed))
}

// poolFilesBreaches are where the non-test files of the cli package of the
// module at root with the given path read the recorded files raw, or a
// pool's read, but in poolfiles.go and the functions allowed; then each
// function allowed that's gone, or keeps the rule.
func poolFilesBreaches(t *testing.T, root, module string, allowed []string) []string {
	t.Helper()
	used := make(map[string]sourcecheck.Ban, len(rawReads))
	for _, name := range rawReads {
		used[name] = sourcecheck.Ban{Says: rawReadSays(name)}
	}
	rule := sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Used: used}
	return sourcecheck.Check(t, root, module, rule, []string{"internal/cli"}, sourcecheck.Allowance{
		Entries: append([]string{"internal/cli/poolfiles.go"}, allowed...), List: "poolFilesAllowed", Fix: poolFilesAllowedFix,
	})
}

// rawReadSays says how using what's named breaks the rule.
func rawReadSays(name string) string {
	return name + " reads the recorded files outside the pool's read; " + poolFilesFix
}

func TestThePoolFilesGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, poolFilesFixture)
	got := poolFilesBreaches(t, root, "example.com/fixture", []string{
		"internal/cli/dashboard.go:readers",
		"internal/cli/dashboard.go:kept",
	})
	want := []string{
		"internal/cli/verbs.go:21: " + rawReadSays("ledger.Reader.Lines"),
		"internal/cli/verbs.go:25: " + rawReadSays("cli.filedPool.read"),
		"internal/cli/verbs.go:25: " + rawReadSays("cli.stateReaders.ledger"),
		"internal/cli/verbs.go:29: " + rawReadSays("ledger.Reader.Days"),
		"internal/cli/dashboard.go:kept: poolFilesAllowed allows it, but it keeps the rule; " + poolFilesAllowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// poolFilesFixture is a module of a ledger reader, a pool's read and a cli
// package: its poolfiles.go reads every one of them, as the rule lets it;
// its verbs.go reads lines, a pool's read of a ledger, and days raw; and its
// dashboard.go hands the ledger on, allowed, and reads nothing, allowed all
// the same.
var poolFilesFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/ledger/ledger.go": `package ledger

type Reader struct{}

func (*Reader) Lines() []string { return nil }
func (*Reader) Days() []string  { return nil }
`,
	"internal/views/views.go": `package views

type PoolRead struct{}

func (PoolRead) Ledger(l *int) *int { return l }
`,
	"internal/cli/poolfiles.go": `package cli

import (
	"example.com/fixture/internal/ledger"
	"example.com/fixture/internal/views"
)

type filedPool struct{ read views.PoolRead }

type stateReaders struct{ ledger *int }

func (p filedPool) sessions(r stateReaders) *int { return p.read.Ledger(r.ledger) }

func (p filedPool) lines(r *ledger.Reader) []string { return r.Lines() }
`,
	"internal/cli/verbs.go": `package cli

import "example.com/fixture/internal/ledger"

func requests(r *ledger.Reader, p filedPool) []string {
	return p.lines(r)
}

func sessions(r stateReaders, p filedPool) *int {
	return p.sessions(r)
}

func ok(r *ledger.Reader) *ledger.Reader {
	return r
}

func raw(r *ledger.Reader, s stateReaders, p filedPool) {
	_ = ok(r)
	_ = requests(r, p)
	_ = sessions(s, p)
	_ = r.Lines()
}

func rawSessions(s stateReaders, p filedPool) *int {
	return p.read.Ledger(s.ledger)
}

func rawDays(r *ledger.Reader) []string {
	return r.Days()
}
`,
	"internal/cli/dashboard.go": `package cli

func readers(r stateReaders) *int {
	return r.ledger
}

func kept() {}
`,
}
