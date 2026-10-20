package dashboard_test

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a view can't choose how it's laid
// out by the frame's width against the phone's, or cut a name to a constant
// width, rather than lay its parts out by flex, without failing here.

// drawingPackages are the packages that draw the dashboard, by directory from
// the module's root: each cuts a name to the width flex lays its column out
// at. Every package keeps to the chrome's breakpoint.
var drawingPackages = []string{"internal/dashboard", "internal/dashboard/watch"}

// chrome are the files of the page's chrome, by path from the module's root:
// the frame, which holds the one breakpoint of its own, and the title row,
// the status line, the view's heading row and the key line, which change at
// it. Each is a file of the chrome that calls Phone, so the list only shrinks
// as the breakpoint does: a file it names must still be there, and still
// call it.
var chrome = []string{
	"internal/dashboard/frame.go",
	"internal/dashboard/headingrow.go",
	"internal/dashboard/keyline.go",
	"internal/dashboard/statusline.go",
	"internal/dashboard/title.go",
}

// phoneAllowed are the functions outside the chrome allowed to choose by
// Phone all the same, by path, a colon and the function's name, a method's
// its receiver's type's then its own, each under the slice of the fluid pass
// that moves it to its parts. The list only shrinks: an entry must still be
// there, and still call it.
var phoneAllowed = []string{
	// S5: the Switchboard's lanes.
	"internal/dashboard/sessionslanes.go:Frame.lanesOf",
	// S6: the Log's columns, Requests' and Events'.
	"internal/dashboard/logcolumns.go:logColumns",
	"internal/dashboard/logevents.go:Frame.eventColumns",
	// S7: the List's columns.
	"internal/dashboard/sessionslist.go:Frame.listColumns",
	// S8: Runway's label column, and the form it's drawn in.
	"internal/dashboard/runway.go:RunwayShown.lanesFrom",
	"internal/dashboard/runway.go:Frame.runwayForm",
	// S9: Runway's legend and COMING UP.
	"internal/dashboard/runwaycoming.go:Frame.runwayComing",
	"internal/dashboard/runwaylegend.go:Frame.runwayLegendLines",
	// S10: the Overview's sections, side by side or not.
	"internal/dashboard/overview.go:Frame.joins",
	// S12: USAGE's key, and the room it says is left.
	"internal/dashboard/overview_usage.go:Frame.overviewBlock",
	"internal/dashboard/overview_usage.go:Frame.roomLeft",
	// S13: GRAPHS.
	"internal/dashboard/graphs.go:Frame.graphsOf",
	// S14: USAGE's key, spaced apart.
	"internal/dashboard/usage.go:keyApart",
	// S15: Compare's form, its focus, the help that tells of it, and the
	// watch that keeps it.
	"internal/dashboard/accountscompare.go:Frame.compareState",
	"internal/dashboard/accounts_help.go:Frame.compareHelp",
	"internal/dashboard/watch/accounts.go:Model.comparing",
	// S15, S16 and S17: the form Accounts draws its views in.
	"internal/dashboard/accounts.go:Frame.buildAccountsForm",
	// S18, S19 and S20: the form History draws its views in.
	"internal/dashboard/historypage.go:Frame.phoneForm",
	// S20: whether Weeks' cycles show.
	"internal/dashboard/historyasks.go:Frame.weeksCycles",
	// S25: what's left, LATELY's sentences, ROUTING's mode, and the cards.
	"internal/dashboard/lately.go:Frame.latelySaid",
	"internal/dashboard/routing.go:Frame.routingSection",
	"internal/dashboard/card.go:Frame.cardWidth",
	"internal/dashboard/routingcard.go:RoutingCard.card",
}

// cutAllowed are the functions of drawingPackages allowed to cut a name to a
// constant width all the same, named as phoneAllowed names them, each under
// the slice of the fluid pass that gives it its width from flex. The list
// only shrinks: an entry must still be there, and still give one.
var cutAllowed = []string{
	// S6: the Log's sessions' directories and accounts.
	"internal/dashboard/logpage.go:sessionNamed",
	"internal/dashboard/logpanel.go:LogPanel.sessionWords",
	"internal/dashboard/logtable.go:sessionCell",
	// S11: SESSIONS' model families.
	"internal/dashboard/overview_sessions.go:Frame.wideRunning",
	// S15: Compare's BY MODEL names.
	"internal/dashboard/accountscompare.go:compared.byModel",
	// S16: the stacked Detail's window names.
	"internal/dashboard/accountsphone.go:stacked.usage",
	// S17: Detail's window names and its sessions' model families.
	"internal/dashboard/accountsdetail.go:detailDrawn.usage",
	"internal/dashboard/accountsdetail.go:detailDrawn.session",
	// S22: a session's page, its accounts' stays and the account it's on.
	"internal/dashboard/sessionpage.go:sessionDrawn.account",
	"internal/dashboard/sessionpage.go:sessionDrawn.phoneOn",
	// S25: the routing card's choices.
	"internal/dashboard/routingcard.go:RoutingCard.choice",
}

// What breaks the rules: the phone's breakpoint, each by its package's name
// under the module's internal directory, then its type's, where it's a
// method, then its own; and the cutters of names, each by its path under the
// module's, as a cut's width is the argument at index widthArg.
var (
	breakpoints = []string{"dashboard.Phone", "dashboard.Frame.phone"}
	cutters     = []string{"/internal/status.DirName", "/internal/prose.Cut"}
)

// widthArg is the index of a cutter's width among its arguments.
const widthArg = 1

// whole is the one constant width a name may be given: the most there is, so
// it's cut to none, as a name is measured whole.
var whole = constant.MakeInt64(math.MaxInt)

// The fix each kind of breach gives.
const (
	phoneFix        = "declare the view's parts and let flex.Lay say where its arrangement shifts; Phone is the chrome's breakpoint alone"
	cutFix          = "give the name the width flex lays its column out at"
	phoneAllowedFix = "remove it from phoneAllowed, or the file from chrome, in internal/dashboard/guard_test.go"
	cutAllowedFix   = "remove it from cutAllowed in internal/dashboard/guard_test.go"
)

// phoneSays and cutSays say how a use of what's named breaks the rule.
func phoneSays(name string) string {
	return name + " chooses by the phone's width outside the chrome; " + phoneFix
}

func cutSays(name string) string {
	return name + " is given a constant width; " + cutFix
}

func TestViewsLayTheirPartsOutByFlex(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	module := sourcecheck.ModulePath(t, root)
	sourcecheck.ReportEach(t, slices.Concat(
		phoneBreaches(t, root, module, sourcecheck.Packages(t, root), slices.Concat(chrome, phoneAllowed)),
		cutBreaches(t, root, module, drawingPackages, cutAllowed),
	))
}

// phoneBreaches are where the non-test files of the packages in dirs, of the
// module at root with the given path, use the phone's breakpoint, called or
// not, but in the files and functions allowed. Then each allowed that's gone,
// or uses none.
func phoneBreaches(t *testing.T, root, module string, dirs, allowed []string) []string {
	t.Helper()
	used := make(map[string]sourcecheck.Ban)
	for _, name := range breakpoints {
		used[name] = sourcecheck.Ban{Says: phoneSays(name)}
	}
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Used: used}, dirs, sourcecheck.Allowance{
		Entries: allowed, List: "phoneAllowed", Fix: phoneAllowedFix,
	})
}

// cutBreaches are where the non-test files of the packages in dirs, of the
// module at root with the given path, give a cutter a constant width other
// than whole, or use one as a value, but in the functions allowed. Then each
// allowed that's gone, or gives none.
func cutBreaches(t *testing.T, root, module string, dirs, allowed []string) []string {
	t.Helper()
	calls := make(map[string]sourcecheck.CallBan)
	for _, cutter := range cutters {
		name := module + cutter
		calls[name] = sourcecheck.CallBan{Says: cutSays(name), Breaks: constantWidth}
	}
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Calls: calls}, dirs, sourcecheck.Allowance{
		Entries: allowed, List: "cutAllowed", Fix: cutAllowedFix,
	})
}

// constantWidth breaks a cutter's call whose width is a constant other than
// whole, as sourcecheck's Breaks judges a call: one known only at run time
// isn't judged; nor is one whose type the loader lost, where it names a
// variable, so it's no constant; any other whose type is lost can't be.
// sourcecheck.ConstantOtherThan judges strings alone.
func constantWidth(call *ast.CallExpr, info *types.Info) (bool, ast.Expr) {
	if widthArg >= len(call.Args) {
		return false, nil
	}
	width := call.Args[widthArg]
	tv, seen := info.Types[width]
	switch {
	case seen && tv.Type.Underlying() != types.Typ[types.Invalid]:
		return tv.Value != nil && !constant.Compare(tv.Value, token.EQL, whole), nil
	case namesAVariable(width, info):
		return false, nil
	}
	return false, width
}

// namesAVariable reports whether e names a variable, so it's no constant, as
// the Go spec has a constant expression: but within len, cap or unsafe's,
// which may make a constant of an array.
func namesAVariable(e ast.Expr, info *types.Info) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && measures(call, info) {
			return false
		}
		if id, ok := n.(*ast.Ident); ok {
			_, isVar := info.Uses[id].(*types.Var)
			found = found || isVar
		}
		return !found
	})
	return found
}

// measures reports whether the call is of len, cap or unsafe's, which may
// make a constant of what it's given.
func measures(call *ast.CallExpr, info *types.Info) bool {
	switch fn := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		b, ok := info.Uses[fn].(*types.Builtin)
		return ok && (b.Name() == "len" || b.Name() == "cap")
	case *ast.SelectorExpr:
		_, ok := info.Uses[fn.Sel].(*types.Builtin)
		return ok
	}
	return false
}

func TestTheFlexGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, flexFixture)
	module := "example.com/fixture"
	dirs := []string{"internal/dashboard", "internal/dashboard/watch"}
	got := slices.Concat(
		phoneBreaches(t, root, module, dirs, []string{
			"internal/dashboard/chrome.go",
			"internal/dashboard/gone.go",
			"internal/dashboard/views.go:Frame.allowed",
			"internal/dashboard/views.go:Frame.kept",
		}),
		cutBreaches(t, root, module, dirs, []string{
			"internal/dashboard/names.go:allowedCut",
			"internal/dashboard/names.go:keptCut",
			"internal/dashboard/names.go:goneCut",
		}),
	)
	unjudged := func(width string) string {
		return module + "/internal/prose.Cut can't be judged, as the type of its argument " + width +
			" is lost to a package the loader stubs; give the guard a type it can see, as a variable declared with one"
	}
	want := []string{
		"internal/dashboard/views.go:8: " + phoneSays("dashboard.Frame.phone"),
		"internal/dashboard/views.go:14: " + phoneSays("dashboard.Phone"),
		"internal/dashboard/views.go:16: " + phoneSays("dashboard.Frame.phone"),
		"internal/dashboard/watch/watch.go:5: " + phoneSays("dashboard.Phone"),
		"internal/dashboard/gone.go: phoneAllowed allows it, but it's gone; " + phoneAllowedFix,
		"internal/dashboard/views.go:Frame.kept: phoneAllowed allows it, but it keeps the rule; " + phoneAllowedFix,
		"internal/dashboard/names.go:21: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:22: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:23: " + cutSays(module+"/internal/status.DirName"),
		"internal/dashboard/names.go:24: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:25: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:26: " + unjudged("cells.Most"),
		"internal/dashboard/names.go:27: " + unjudged("len(marks) + cells.Most"),
		"internal/dashboard/names.go:28: " + unjudged("int(unsafe.Sizeof(room)) + cells.Most"),
		"internal/dashboard/names.go:37: " + cutSays(module+"/internal/status.DirName"),
		"internal/dashboard/names.go:keptCut: cutAllowed allows it, but it keeps the rule; " + cutAllowedFix,
		"internal/dashboard/names.go:goneCut: cutAllowed allows it, but it's gone; " + cutAllowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// flexFixture is a module of prose and status, whose cutters cut a name to a
// width; and the dashboard, with its breakpoint in a file of its chrome, a
// view that chooses by it, called and as a value, in functions allowed and
// not, and one allowed that keeps the rule, and names cut to constant widths,
// named, sums, bare and an array's length, and to one of a package the loader
// stubs, beside widths known only at run time, one of them through what the
// loader stubs, the most there is, and a function allowed; and the watch,
// which chooses by it too.
var flexFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/prose/prose.go": `package prose

func Cut(text string, cells int) string      { return text }
func CutShort(text string, cells int) string { return text }
`,
	"internal/status/status.go": `package status

import "example.com/fixture/internal/prose"

func DirName(dir string, cells int) string { return prose.Cut(dir, cells) }
`,
	"internal/dashboard/chrome.go": `package dashboard

type Frame struct{ Width int }

func (f Frame) phone() bool { return Phone(f.Width) }

func Phone(width int) bool { return width < 100 }

func (f Frame) title() int {
	if f.phone() {
		return 2
	}
	return 1
}
`,
	"internal/dashboard/views.go": `package dashboard

func (f Frame) allowed() bool { return f.phone() }

func (f Frame) kept() bool { return f.Width > 0 }

func (f Frame) runway() int {
	if f.phone() {
		return 14
	}
	return 24
}

var choose = Phone

func (f Frame) later() func() bool { return f.phone }
`,
	"internal/dashboard/names.go": `package dashboard

import (
	"math"
	"unsafe"

	"example.org/cells"

	"example.com/fixture/internal/prose"
	"example.com/fixture/internal/status"
)

const labelCells = 8

func allowedCut(name string) string { return prose.Cut(name, labelCells) }

func keptCut(name string) string { return name }

func cut(name, dir string, room int, marks [3]string) []string {
	return []string{
		prose.Cut(name, labelCells),
		prose.Cut(name, labelCells-1),
		status.DirName(dir, 12),
		(prose.Cut)(name, 3),
		prose.Cut(name, len(marks)),
		prose.Cut(name, cells.Most),
		prose.Cut(name, len(marks)+cells.Most),
		prose.Cut(name, int(unsafe.Sizeof(room))+cells.Most),
		prose.Cut(name, room),
		prose.Cut(name, max(room-labelCells, 1)),
		prose.Cut(name, max(room-cells.Width(name), 1)),
		status.DirName(dir, math.MaxInt),
		prose.CutShort(name, labelCells),
	}
}

var cutter = status.DirName
`,
	"internal/dashboard/watch/watch.go": `package watch

import "example.com/fixture/internal/dashboard"

func phone(width int) bool { return dashboard.Phone(width) }
`,
}
