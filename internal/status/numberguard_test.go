package status_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so no package but status can number
// an account, setting the Place of an AccountNow, rather than read the number
// status.Numbering gives it, without failing here. A view numbering accounts
// by a loop's index, never setting a Place, is beyond what the source can
// tell from any other count, so the tests of the numbers drawn hold that.

// numberedType and numberedField are where an account's number is set: its
// type, by its name, and its field.
const (
	numberedType  = "status.AccountNow"
	numberedField = "Place"
)

// numberOwner is the one package, by directory from the module's root, that
// sets an account's number. Every other is held to it, statustest among
// them, as a fixture's numbers come from a moment as a view's do.
const numberOwner = "internal/status"

// numberFix is the fix a breach gives.
const numberFix = "read the number status gives it, as Moment.Configured or Numbering.Configured has it"

func TestOnlyStatusNumbersAnAccount(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	others := slices.DeleteFunc(sourcecheck.Packages(t, root), func(dir string) bool { return dir == numberOwner })
	sourcecheck.ReportEach(t, numbersSet(t, root, sourcecheck.ModulePath(t, root), others))
}

func TestTheNumberGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, numberFixture)
	got := numbersSet(t, root, "example.com/fixture", []string{"internal/view"})
	set := "sets an account's number, " + numberedType + "." + numberedField + "; " + numberFix
	var want []string
	for _, line := range []string{"14", "18", "19", "20", "21", "25", "26", "27"} {
		want = append(want, "internal/view/view.go:"+line+": "+set)
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// numbersSet are where the non-test files of the packages in dirs, of the
// module at root with the given path, set an account's number: a Place keyed
// in an AccountNow's literal, an AccountNow's Place assigned or stepped, or
// its address taken, a write by pointer; each through any struct that embeds
// an AccountNow too.
func numbersSet(t *testing.T, root, module string, dirs []string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: names, Node: numberSetBy(names)}, dirs, sourcecheck.Allowance{})
}

// numberSetBy says how a node sets an account's number, by names: "" where
// it doesn't.
func numberSetBy(names sourcecheck.Names) func(ast.Node, *types.Info) string {
	says := "sets an account's number, " + numberedType + "." + numberedField + "; " + numberFix
	return func(n ast.Node, info *types.Info) string {
		var set bool
		switch n := n.(type) {
		case *ast.KeyValueExpr:
			key, ok := n.Key.(*ast.Ident)
			set = ok && isNumber(info.Uses[key], names)
		case *ast.AssignStmt:
			set = slices.ContainsFunc(n.Lhs, func(e ast.Expr) bool { return numberAt(e, info, names) })
		case *ast.IncDecStmt:
			set = numberAt(n.X, info, names)
		case *ast.UnaryExpr:
			set = n.Op == token.AND && numberAt(n.X, info, names)
		}
		if set {
			return says
		}
		return ""
	}
}

// numberAt reports whether e is an account's number, an AccountNow's Place,
// as a selector resolves it, promoted through an embedding struct or not.
func numberAt(e ast.Expr, info *types.Info, names sourcecheck.Names) bool {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	return ok && isNumber(info.Uses[sel.Sel], names)
}

// isNumber reports whether obj is the field an account's number is: the Place
// field of the struct status.AccountNow is.
func isNumber(obj types.Object, names sourcecheck.Names) bool {
	field, ok := obj.(*types.Var)
	if !ok || !field.IsField() || field.Name() != numberedField || field.Pkg() == nil {
		return false
	}
	owner, ok := field.Pkg().Scope().Lookup(strings.TrimPrefix(numberedType, "status.")).(*types.TypeName)
	if !ok || names.Declared(owner) != numberedType {
		return false
	}
	s, ok := owner.Type().Underlying().(*types.Struct)
	return ok && slices.Contains(slices.Collect(s.Fields()), field)
}

// numberFixture is a module whose status gives an account's number, and a
// view package that sets it each way the guard finds, through the type and
// through a struct embedding it, beside what it lets be: a literal keyed
// without it, its number read, and a type of the view's own with a field of
// the name, set and its address taken.
var numberFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/status/status.go": `package status

type AccountNow struct {
	ID    string
	Place int
}
`,
	"internal/view/view.go": `package view

import "example.com/fixture/internal/status"

type seat struct{ Place int }

type held struct {
	status.AccountNow
	Seen bool
}

func set(i int) []status.AccountNow {
	accounts := []status.AccountNow{
		{ID: "work", Place: i + 1},
		{ID: "side"},
	}
	a := &accounts[1]
	a.Place = 2
	accounts[0].Place, a.ID = 3, "side"
	a.Place++
	_ = &status.AccountNow{Place: 4}
	s := seat{Place: 5}
	s.Place, _ = accounts[0].Place, &s.Place
	h := held{AccountNow: accounts[0]}
	h.Place = 6
	p := &h.Place
	*(&a.Place) = 7
	*p = h.Place
	return accounts
}
`,
}
