package ledger_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so no sum of worth wraps round past
// what Picodollars hold: every one goes through Picodollars.Plus, which
// reports a sum too large, so it reads as unpriced.

// sumAllowed are the functions allowed to add Picodollars with +, by path
// from the module's root, a colon and the function's name, a method's its
// receiver's type's then its own. The list only shrinks: an entry must still
// be there, and still add them.
var sumAllowed = []string{
	// The one helper that sums, and checks the sum fits.
	"internal/ledger/worth.go:Picodollars.Plus",
	// A cent past a row's most, a constant some thousands of dollars.
	"internal/views/pagesaid.go:worthSaidWidest",
}

// The fix each kind of breach gives.
const (
	sumSays       = "adds Picodollars with +, which wraps round past what they hold; sum them with Picodollars.Plus, a sum too large reading as unpriced, or name the function in sumAllowed in internal/ledger/sumguard_test.go, saying why it can't pass what they hold"
	sumAllowedFix = "remove it from sumAllowed in internal/ledger/sumguard_test.go"
)

func TestWorthIsSummedThroughPlusAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, sumBreaches(t, root, sourcecheck.ModulePath(t, root), sourcecheck.Packages(t, root), sumAllowed))
}

func TestTheSumGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, sumFixture)
	got := sumBreaches(t, root, "example.com/fixture", sourcecheck.Packages(t, root), []string{
		"internal/ledger/ledger.go:Picodollars.Plus",
		"internal/ledger/ledger.go:kept",
		"internal/ledger/ledger.go:gone",
	})
	want := []string{
		"internal/views/views.go:6: " + sumSays,
		"internal/views/views.go:11: " + sumSays,
		"internal/ledger/ledger.go:kept: sumAllowed allows it, but it keeps the rule; " + sumAllowedFix,
		"internal/ledger/ledger.go:gone: sumAllowed allows it, but it's gone; " + sumAllowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// picodollars is the type a sum of worth is held in, as sourcecheck names it.
const picodollars = "ledger.Picodollars"

// sumBreaches are where the non-test files of the packages in dirs, of the
// module at root with the given path, add Picodollars with + or +=, but in
// the functions allowed. Then each function allowed that's gone, or adds
// none.
func sumBreaches(t *testing.T, root, module string, dirs, allowed []string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	isPicodollars := func(e ast.Expr, info *types.Info) bool {
		named, _ := types.Unalias(info.TypeOf(e)).(*types.Named)
		return names.Type(named) == picodollars
	}
	rule := sourcecheck.Rule{Names: names, Node: func(n ast.Node, info *types.Info) string {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.ADD && isPicodollars(n, info) {
				return sumSays
			}
		case *ast.AssignStmt:
			if n.Tok == token.ADD_ASSIGN && isPicodollars(n.Lhs[0], info) {
				return sumSays
			}
		}
		return ""
	}}
	return sourcecheck.Check(t, root, module, rule, dirs, sourcecheck.Allowance{Entries: allowed, List: "sumAllowed", Fix: sumAllowedFix})
}

// sumFixture is a module of the ledger, whose Plus adds Picodollars, as it's
// allowed to, and which adds other numbers, which keeps the rule; views,
// which adds them with + and +=; and a test file adding them.
var sumFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/ledger/ledger.go": `package ledger

type Picodollars int64

func (p Picodollars) Plus(o Picodollars) (Picodollars, bool) { return p + o, true }

func kept(a, b int64) int64 { return a + b }
`,
	"internal/views/views.go": `package views

import "example.com/fixture/internal/ledger"

func total(a, b ledger.Picodollars) ledger.Picodollars {
	return a + b
}

func sum(all []ledger.Picodollars) (s ledger.Picodollars) {
	for _, p := range all {
		s += p
	}
	return s
}

func count(all []ledger.Picodollars) (n int) {
	for range all {
		n += 1
	}
	return n
}
`,
	"internal/views/views_test.go": `package views

import "example.com/fixture/internal/ledger"

func tested(a, b ledger.Picodollars) ledger.Picodollars { return a + b }
`,
}
