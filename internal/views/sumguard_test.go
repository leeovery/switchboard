package views_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a drawing package can't add up
// what views sums, rather than read the sum views gives, without failing
// here. They hold the facts summed alone, never a sum in general: a width or
// a row's place is the drawing's own arithmetic.

// summedInViews are what views sums, by what's summed, each with the sum
// that owns it: a drawing package adds none of them with + or +=.
var summedInViews = map[string]string{
	// An account's moves over a period, in and out together, or every
	// account's.
	"views.AccountTotals.MovedOn":  "views.AccountTotals.Moves",
	"views.AccountTotals.MovedOff": "views.AccountTotals.Moves",
	// The sessions on the pool's accounts, each counted as SessionsOn
	// counts it, and as ROUTING holds each account's.
	"views.SessionsOn":                  "views.PoolSessions",
	"status.AccountNow.Sessions":        "views.PoolSessions",
	"dashboard.accountRouting.sessions": "views.PoolSessions",
	// The pool's requests, its accounts' together.
	"views.Trace.Rate": "views.Traces.Pool",
}

// summedSays says how adding up what's summed breaks the rule, and its owner.
func summedSays(name, owner string) string {
	return "adds up " + name + "; read " + owner + ", the sum's one owner"
}

func TestDrawingAddsUpNothingViewsSums(t *testing.T) {
	t.Parallel()
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, sumsBreaches(t, root, sourcecheck.ModulePath(t, root), drawingPackages, summedInViews))
}

func TestTheSumsGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, sumsFixture)
	got := sumsBreaches(t, root, "example.com/fixture", []string{"internal/dashboard"}, map[string]string{
		"views.Totals.MovedOn": "views.Totals.Moves", "views.Trace.Rate": "views.PoolRate", "dashboard.routed.sessions": "views.PoolSessions",
	})
	want := []string{
		"internal/dashboard/dashboard.go:13: " + summedSays("views.Totals.MovedOn", "views.Totals.Moves"),
		"internal/dashboard/dashboard.go:17: " + summedSays("views.Totals.MovedOn", "views.Totals.Moves"),
		"internal/dashboard/dashboard.go:23: " + summedSays("views.Trace.Rate", "views.PoolRate"),
		"internal/dashboard/dashboard.go:28: " + summedSays("views.Totals.MovedOn", "views.Totals.Moves"),
		"internal/dashboard/dashboard.go:30: " + summedSays("views.Totals.MovedOn", "views.Totals.Moves"),
		"internal/dashboard/dashboard.go:32: " + summedSays("views.Trace.Rate", "views.PoolRate"),
		"internal/dashboard/dashboard.go:37: " + summedSays("dashboard.routed.sessions", "views.PoolSessions"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// sumsBreaches are where the non-test files of the packages in dirs, of the
// module at root with the given path, add up one of summed, by Names' name,
// with + or +=: an operand that reads the field named, through any struct
// embedding its own, or calls the function named. An operand that's worked
// out of one, as a call given it is, isn't adding it up.
func sumsBreaches(t *testing.T, root, module string, dirs []string, summed map[string]string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	addsUp := func(info *types.Info, operands ...ast.Expr) string {
		for _, e := range operands {
			if name := summedNamed(e, info, names); summed[name] != "" {
				return summedSays(name, summed[name])
			}
		}
		return ""
	}
	rule := sourcecheck.Rule{Names: names, Node: func(n ast.Node, info *types.Info) string {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.ADD {
				return addsUp(info, n.X, n.Y)
			}
		case *ast.AssignStmt:
			if n.Tok == token.ADD_ASSIGN {
				return addsUp(info, n.Rhs...)
			}
		}
		return ""
	}}
	return sourcecheck.Check(t, root, module, rule, dirs, sourcecheck.Allowance{})
}

// summedNamed names what the operand e is, as Names names it, through any
// conversion: the field it reads, by the type declaring it, or the function
// it calls; "" where it's neither.
func summedNamed(e ast.Expr, info *types.Info, names sourcecheck.Names) string {
	if call, ok := ast.Unparen(e).(*ast.CallExpr); ok {
		if info.Types[call.Fun].IsType() && len(call.Args) == 1 {
			return summedNamed(call.Args[0], info, names)
		}
		e = call.Fun
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.SelectorExpr:
		if s, ok := info.Selections[e]; ok && s.Kind() == types.FieldVal {
			return fieldDeclared(s, names)
		}
		if fn, ok := info.Uses[e.Sel].(*types.Func); ok {
			return names.Func(fn)
		}
	case *ast.Ident:
		if fn, ok := info.Uses[e].(*types.Func); ok {
			return names.Func(fn)
		}
	}
	return ""
}

// fieldDeclared names the field sel reads by the type declaring it, as
// "status.AccountNow.Sessions", through each struct embedded on the way: ""
// where that's no named struct of the module's internal packages.
func fieldDeclared(sel *types.Selection, names sourcecheck.Names) string {
	t, path := sel.Recv(), sel.Index()
	for _, index := range path[:len(path)-1] {
		_, s := sourcecheck.StructOf(t)
		if s == nil {
			return ""
		}
		t = s.Field(index).Type()
	}
	named, _ := sourcecheck.StructOf(t)
	if name := names.Type(named); name != "" {
		return name + "." + sel.Obj().Name()
	}
	return ""
}

// sumsFixture is a module whose views sums moves and rates, and a dashboard
// that adds them up itself each way the guard finds: a field with +, a
// field promoted through an embedding struct with +=, a method's call, one
// operand of three, a field and a call converted, and a field of the
// dashboard's own the guard names; beside what keeps the rule: the sum views
// gives, a width worked out of a field, and a field of its own of the name.
var sumsFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/views/views.go": `package views

type Totals struct{ MovedOn, MovedOff int }

func (t Totals) Moves() int { return t.MovedOn + t.MovedOff }

type Trace struct{ Sent int }

func (t Trace) Rate() int { return t.Sent }

func PoolRate(traces []Trace) int { return 0 }
`,
	"internal/dashboard/dashboard.go": `package dashboard

import (
	"strconv"

	"example.com/fixture/internal/views"
)

type row struct{ views.Totals }

type routed struct{ sessions int }

func moves(t views.Totals) int { return t.MovedOn + t.MovedOff }

func promoted(r row) int {
	n := 0
	n += r.MovedOn
	return n
}

func rate(traces []views.Trace) (n int) {
	for _, t := range traces {
		n += t.Rate()
	}
	return n
}

func three(t views.Totals) int { return 1 + 2 + (t.MovedOn) }

func converted(t views.Totals) int { return 1 + int(t.MovedOn) }

func rated(tr views.Trace) float64 { return 1 + float64(tr.Rate()) }

func sessions(routing []routed) int {
	n := 0
	for _, r := range routing {
		n += r.sessions
	}
	return n
}

func kept(t views.Totals, traces []views.Trace) int {
	type own struct{ MovedOn int }
	width := 2 + len(strconv.Itoa(t.MovedOn))
	return t.Moves() + views.PoolRate(traces) + width + own{}.MovedOn
}
`,
}
