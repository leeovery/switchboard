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

// These tests read the module's source, so no package but status can place
// an account in the accounts' order, setting the Place of an AccountNow,
// rather than read the place the moment gives it, without failing here. A
// view ordering accounts by a loop's index, never setting a Place, is beyond
// what the source can tell from any other count, so the tests of the order
// drawn hold that. An unkeyed AccountNow literal, which would set a Place by
// its position, is left to go vet's composites check, which the gates run.

// placedType and placedField are where an account's place is set: its type,
// by its name, and its field.
const (
	placedType  = "status.AccountNow"
	placedField = "Place"
)

// placeOwner is the one package, by directory from the module's root, that
// sets an account's place. Every other is held to it, statustest among
// them, as a fixture's places come from a moment as a view's do.
const placeOwner = "internal/status"

// placeFix is the fix a breach gives.
const placeFix = "read the place the moment gives it, as Moment.Account or Moment.Configured has it"

func TestOnlyStatusPlacesAnAccount(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	others := slices.DeleteFunc(sourcecheck.Packages(t, root), func(dir string) bool { return dir == placeOwner })
	sourcecheck.ReportEach(t, placesSet(t, root, sourcecheck.ModulePath(t, root), others))
}

func TestThePlaceGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, placeFixture)
	got := placesSet(t, root, "example.com/fixture", []string{"internal/view"})
	set := "sets an account's place, " + placedType + "." + placedField + "; " + placeFix
	var want []string
	for _, line := range []string{"14", "18", "19", "20", "21", "25", "26", "27", "29", "31"} {
		want = append(want, "internal/view/view.go:"+line+": "+set)
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// placesSet are where the non-test files of the packages in dirs, of the
// module at root with the given path, set an account's place: a Place keyed
// in an AccountNow's literal, an AccountNow's Place assigned, ranged into or
// stepped, or its address taken, a write by pointer; each through any struct
// that embeds an AccountNow too.
func placesSet(t *testing.T, root, module string, dirs []string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: names, Node: placeSetBy(names)}, dirs, sourcecheck.Allowance{})
}

// placeSetBy says how a node sets an account's place, by names: "" where
// it doesn't.
func placeSetBy(names sourcecheck.Names) func(ast.Node, *types.Info) string {
	says := "sets an account's place, " + placedType + "." + placedField + "; " + placeFix
	return func(n ast.Node, info *types.Info) string {
		var set bool
		switch n := n.(type) {
		case *ast.KeyValueExpr:
			key, ok := n.Key.(*ast.Ident)
			set = ok && isPlace(info.Uses[key], names)
		case *ast.AssignStmt:
			set = slices.ContainsFunc(n.Lhs, func(e ast.Expr) bool { return placeAt(e, info, names) })
		case *ast.RangeStmt:
			set = n.Tok == token.ASSIGN && (placeAt(n.Key, info, names) || placeAt(n.Value, info, names))
		case *ast.IncDecStmt:
			set = placeAt(n.X, info, names)
		case *ast.UnaryExpr:
			set = n.Op == token.AND && placeAt(n.X, info, names)
		}
		if set {
			return says
		}
		return ""
	}
}

// placeAt reports whether e is an account's place, an AccountNow's Place,
// as a selector resolves it, promoted through an embedding struct or not.
func placeAt(e ast.Expr, info *types.Info, names sourcecheck.Names) bool {
	if e == nil {
		return false
	}
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	return ok && isPlace(info.Uses[sel.Sel], names)
}

// isPlace reports whether obj is the field an account's place is: the Place
// field of the struct status.AccountNow is.
func isPlace(obj types.Object, names sourcecheck.Names) bool {
	field, ok := obj.(*types.Var)
	if !ok || !field.IsField() || field.Name() != placedField || field.Pkg() == nil {
		return false
	}
	owner, ok := field.Pkg().Scope().Lookup(strings.TrimPrefix(placedType, "status.")).(*types.TypeName)
	if !ok || names.Declared(owner) != placedType {
		return false
	}
	s, ok := owner.Type().Underlying().(*types.Struct)
	return ok && slices.Contains(slices.Collect(s.Fields()), field)
}

// placeFixture is a module whose status gives an account's place, and a
// view package that sets it each way the guard finds, through the type and
// through a struct embedding it, beside what it lets be: a literal keyed
// without it, its place read, and a type of the view's own with a field of
// the name, set and its address taken.
var placeFixture = map[string]string{
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
	for _, a.Place = range []int{8} {
	}
	for a.Place = range 9 {
	}
	for s.Place = range 3 {
	}
	return accounts
}
`,
}
