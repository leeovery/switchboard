package status_test

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a view can't number an account
// for itself, setting the Place of an AccountNow, rather than read the number
// status.Numbering gives it, without failing here. A view numbering accounts
// by a loop's index, never setting a Place, is beyond what the source can
// tell from any other count, so the tests of the numbers drawn hold that.

// numberedType and numberedField are where an account's number is set: its
// type, by its name, and its field.
const (
	numberedType  = "status.AccountNow"
	numberedField = "Place"
)

// numberFix is the fix a breach gives.
const numberFix = "read the number status gives it, as Moment.Configured or Numbering.Configured has it"

func TestOnlyStatusNumbersAnAccount(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, numbersSet(t, root, sourcecheck.ModulePath(t, root), viewPackages))
}

func TestTheNumberGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, numberFixture)
	got := numbersSet(t, root, "example.com/fixture", []string{"internal/view"})
	set := "sets an account's number, " + numberedType + "." + numberedField + "; " + numberFix
	want := []string{
		"internal/view/view.go:9: " + set,
		"internal/view/view.go:13: " + set,
		"internal/view/view.go:14: " + set,
		"internal/view/view.go:15: " + set,
		"internal/view/view.go:16: " + set,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// numbersSet are where the non-test files of the packages in dirs, of the
// module at root with the given path, set an account's number: a Place keyed
// in an AccountNow's literal, or an AccountNow's Place assigned, or stepped.
func numbersSet(t *testing.T, root, module string, dirs []string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: names, Node: numberSetBy(names)}, dirs, sourcecheck.Allowance{})
}

// numberSetBy says how a node sets an account's number, by names: "" where
// it doesn't.
func numberSetBy(names sourcecheck.Names) func(ast.Node, *types.Info) string {
	says := "sets an account's number, " + numberedType + "." + numberedField + "; " + numberFix
	numbered := func(t types.Type) bool {
		named, _ := sourcecheck.StructOf(t)
		return names.Type(named) == numberedType
	}
	placeOf := func(e ast.Expr, info *types.Info) bool {
		sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != numberedField {
			return false
		}
		s, ok := info.Selections[sel]
		return ok && s.Kind() == types.FieldVal && numbered(s.Recv())
	}
	return func(n ast.Node, info *types.Info) string {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if !numbered(info.TypeOf(n)) {
				return ""
			}
			for _, elt := range n.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok && key.Name == numberedField {
						return says
					}
				}
			}
		case *ast.AssignStmt:
			if slices.ContainsFunc(n.Lhs, func(e ast.Expr) bool { return placeOf(e, info) }) {
				return says
			}
		case *ast.IncDecStmt:
			if placeOf(n.X, info) {
				return says
			}
		}
		return ""
	}
}

// numberFixture is a module whose status gives an account's number, and a
// view package that sets it each way the guard finds, beside what it lets
// be: a literal keyed without it, its number read, and a type of the view's
// own with a field of the name.
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
	s.Place = accounts[0].Place
	return accounts
}
`,
}
