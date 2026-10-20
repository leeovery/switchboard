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
// out by the frame's width against the phone's, or any width or height
// against a constant, or cut a name to a constant width, rather than lay its
// parts out by flex, without failing here.

// drawingPackages are the packages that draw the dashboard, by directory from
// the module's root: each lays its views out by flex. Every package keeps to
// the chrome's breakpoint.
var drawingPackages = []string{"internal/dashboard", "internal/dashboard/watch"}

// chrome are the functions of the page's chrome that choose by its one
// breakpoint, Phone, by path from the module's root, a colon and the
// function's name, a method's its receiver's type's then its own: the frame,
// and the title row, the status line, the view's heading row and the key
// line, which change at it. chromeSized is the breakpoint itself, which
// compares the width with the phone's. Each list only shrinks as the
// breakpoint does: an entry must still be there, and still choose by it.
var (
	chrome = []string{
		"internal/dashboard/frame.go:Frame.phone",
		"internal/dashboard/frame.go:Frame.top",
		"internal/dashboard/headingrow.go:Frame.headingPieces",
		"internal/dashboard/keyline.go:Frame.keysAt",
		"internal/dashboard/statusline.go:Frame.routerStatus",
		"internal/dashboard/title.go:Frame.tabsAt",
		"internal/dashboard/title.go:Frame.title",
		"internal/dashboard/title.go:Frame.titleRows",
	}
	chromeSized = []string{
		"internal/dashboard/frame.go:Phone",
	}
)

// phoneAllowed are the functions outside the chrome allowed to choose by
// Phone all the same, or to hand it on to a view to choose by, named as
// chrome names them, each under the slice of the fluid pass that moves it to
// its parts. The list only shrinks: an entry must still be there, and still
// use it.
var phoneAllowed = []string{
	// S5: the Switchboard's lanes.
	"internal/dashboard/sessionslanes.go:Frame.lanesOf",
	// S6: the Log's columns, Requests' and Events', and its heading, which
	// the heading row hands the phone's breakpoint.
	"internal/dashboard/logcolumns.go:logColumns",
	"internal/dashboard/logevents.go:Frame.eventColumns",
	"internal/dashboard/headingrow.go:Frame.tabHeading",
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
	// S14: USAGE's key, its parts spaced apart by the width.
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

// sizedAllowed are the functions of drawingPackages, outside the chrome,
// allowed to compare a width or a height with a constant all the same, named
// as chrome names them, each under the slice of the fluid pass that moves it
// to its parts. The list only shrinks: an entry must still be there, and
// still compare one.
var sizedAllowed = []string{
	// S6: the Log's cost kept where two cells are left.
	"internal/dashboard/logtable.go:costKept",
	// S7: the List's routing column.
	"internal/dashboard/sessionslist.go:Frame.listColumns",
	// S10: ROUTING's rows, routingRows, and SESSIONS' form by it.
	"internal/dashboard/overview_sessions.go:Frame.runningForm",
	"internal/dashboard/routing.go:Frame.routingSection",
	"internal/dashboard/routing.go:Frame.drawRouting",
	"internal/dashboard/routing.go:Frame.accountRows",
	"internal/dashboard/routing.go:besideRows",
	"internal/dashboard/traces.go:Frame.accountTraceCells",
	// S12: USAGE paged at short heights.
	"internal/dashboard/overview_usage.go:usageViews.paged",
	// S14: USAGE's matrix, matrixWidth, its names' column, roomyNames, its
	// accounts' labels, wideAccounts, and its words, fullWords; and what
	// sits beside it by the matrix.
	"internal/dashboard/graphsbeside.go:Frame.graphsBeside",
	"internal/dashboard/overview_usage.go:Frame.overviewBlock",
	"internal/dashboard/overview_usage.go:usageBlock.enlarges",
	"internal/dashboard/routing.go:Frame.routingBesideUsage",
	"internal/dashboard/usage.go:nameColumn",
	"internal/dashboard/usage.go:usageBlock.wordCells",
	"internal/dashboard/usage.go:usageBlock.matrixed",
	"internal/dashboard/usage.go:usageBlock.byAccount",
	"internal/dashboard/usage.go:usageBlock.labels",
	"internal/dashboard/usage.go:usageBlock.key",
	"internal/dashboard/usagewords.go:usageWords.pick",
	// S19 and S20: Days and Weeks scrolling whole under historyRows.
	"internal/dashboard/historypage.go:Frame.shortOfHistory",
	// S21: Tokens' footnote, left off short of leastPaged.
	"internal/dashboard/historytokens.go:Frame.tokensPlaced",
	// S22 and S23: a session's page, its turns' form, sessionWideFrom, and
	// its rows.
	"internal/dashboard/sessionlayout.go:Frame.sessionLayout",
	// S25: the Log's panel and the theme picker, cards of a kind; usage's
	// printout's header; and a bar's cells along it, a quotient's guard
	// rather than an arrangement.
	"internal/dashboard/logpanelcard.go:LogPanel.laidOut",
	"internal/dashboard/picker.go:PickerWidth",
	"internal/dashboard/printout.go:Printout.header",
	"internal/dashboard/bar.go:along",
}

// cutAllowed are the functions of drawingPackages allowed to cut a name to a
// constant width all the same, named as chrome names them, or the file, where
// a cutter is a value outside any function, each under the slice of the
// fluid pass that gives it its width from flex. The list only shrinks: an
// entry must still be there, and still give one.
var cutAllowed = []string{
	// S5: the lanes' calls, their columns laneCall's, and the Switchboard's
	// lines' window labels.
	"internal/dashboard/sessionslanes.go:lanesBoard.callRow",
	"internal/dashboard/switchboardlines.go:linesDrawn.window",
	// S6: the Log's sessions' directories and accounts.
	"internal/dashboard/logpage.go:sessionNamed",
	"internal/dashboard/logtable.go:sessionCell",
	// S7: the List's routing column, its words a value of a package-level
	// column.
	"internal/dashboard/sessionslist.go",
	// S11: SESSIONS' model families.
	"internal/dashboard/overview_sessions.go:Frame.wideRunning",
	// S15: Compare's BY MODEL names.
	"internal/dashboard/accountscompare.go:compared.byModel",
	// S16: the stacked Detail's window names and its LATELY's words.
	"internal/dashboard/accountsphone.go:stacked.usage",
	"internal/dashboard/accountsphone.go:stacked.whole",
	// S17: Detail's window names, its sessions' model families and how many
	// more, and its LATELY's words.
	"internal/dashboard/accountsdetail.go:detailDrawn.usage",
	"internal/dashboard/accountsdetail.go:detailDrawn.session",
	"internal/dashboard/accountsdetail.go:detailDrawn.sessions",
	"internal/dashboard/accountsdetail.go:detailDrawn.panel",
	// S19: Days' weeks' heads, and its strips' names, a cutter as a value.
	"internal/dashboard/historydays.go:Frame.daysWeeks",
	"internal/dashboard/historydays.go:Frame.daysStrips",
	// S22: a session's page, its accounts' stays and the account it's on.
	"internal/dashboard/sessionpage.go:sessionDrawn.account",
	"internal/dashboard/sessionpage.go:sessionDrawn.phoneOn",
	// S25: the routing card's choices, the Log's panel's sessions and a
	// card's items' names, cards of a kind, opened over the page.
	"internal/dashboard/routingcard.go:RoutingCard.choice",
	"internal/dashboard/logpanel.go:LogPanel.sessionWords",
	"internal/dashboard/logcard.go:cardItem.row",
}

// What breaks the rules: the phone's breakpoint, each by its package's name
// under the module's internal directory, then its type's, where it's a
// method, then its own; what cuts a name or a line to the cells it's given,
// line.go's entry points among them, as it owns every cut; and what a width
// or height is called, as a variable or a field.
var (
	breakpoints = []string{"dashboard.Phone", "dashboard.Frame.phone"}
	cutters     = []cutter{
		{pkg: "/internal/status", name: "DirName", width: 1},
		{pkg: "/internal/prose", name: "Cut", width: 1},
		{pkg: "/internal/prose", name: "CutShort", width: 1},
		{pkg: "/internal/prose", name: "CutFront", width: 1},
		{pkg: "github.com/charmbracelet/x/ansi", name: "Truncate", width: 1},
		{pkg: "/internal/dashboard", recv: "line", name: "fit"},
		{pkg: "/internal/dashboard", recv: "line", name: "fitWhole"},
		{pkg: "/internal/dashboard", recv: "line", name: "fitItems"},
		{pkg: "/internal/dashboard", recv: "line", name: "wrap"},
		{pkg: "/internal/dashboard", name: "firstFit"},
		{pkg: "/internal/dashboard", name: "fitParts"},
		{pkg: "/internal/dashboard", name: "wrapParts"},
		{pkg: "/internal/dashboard", name: "spread", width: 2},
	}
	sizeNames = []string{"Width", "Height", "width", "height", "room", "rows", "cols"}
)

// cutter is a function or method that cuts what it's given to a width: by
// its package's path, under the module's where it starts with a slash, or in
// full; its receiver's type, where it's a method; its name; and the index of
// the argument that's its width.
type cutter struct {
	pkg, recv, name string
	width           int
}

// fullName is the cutter's name as the type checker gives it, in the module
// named.
func (c cutter) fullName(module string) string {
	pkg := c.pkg
	if strings.HasPrefix(pkg, "/") {
		pkg = module + pkg
	}
	if c.recv != "" {
		return "(" + pkg + "." + c.recv + ")." + c.name
	}
	return pkg + "." + c.name
}

// whole is the one constant width a name may be given: the most there is, so
// it's cut to none, as a name is measured whole.
var whole = constant.MakeInt64(math.MaxInt)

// The fix each kind of breach gives.
const (
	phoneFix        = "declare the view's parts and let flex.Lay say where its arrangement shifts; Phone is the chrome's breakpoint alone"
	sizedFix        = "declare the view's parts and let flex.Lay say where its arrangement shifts"
	cutFix          = "give the name the width flex lays its column out at"
	phoneAllowedFix = "remove it from phoneAllowed, or from chrome, in internal/dashboard/guard_test.go"
	sizedAllowedFix = "remove it from sizedAllowed, or from chromeSized, in internal/dashboard/guard_test.go"
	cutAllowedFix   = "remove it from cutAllowed in internal/dashboard/guard_test.go"
)

// phoneSays, sizedSays and cutSays say how a use of what's named breaks the
// rule.
func phoneSays(name string) string {
	return name + " chooses by the phone's width outside the chrome; " + phoneFix
}

func sizedSays(compared string) string {
	return compared + " compares a width or a height with a constant; " + sizedFix
}

func cutSays(name string) string {
	return name + " is given a constant width; " + cutFix
}

func TestViewsLayTheirPartsOutByFlex(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, flexBreaches(t, root, sourcecheck.ModulePath(t, root), sourcecheck.Packages(t, root), drawingPackages, allowed{
		phone: slices.Concat(chrome, phoneAllowed),
		sized: slices.Concat(chromeSized, sizedAllowed),
		cut:   cutAllowed,
	}))
}

// allowed are the functions each of the guard's rules allows to break it.
type allowed struct {
	phone, sized, cut []string
}

// flexBreaches are where the non-test files of the packages of the module at
// root with the given path break the guard's rules, but in the functions
// allowed: those in every use the phone's breakpoint, called or not; and
// those in drawing compare a width or a height with a constant, as sized has
// it, or give a cutter a constant width, as cutBreaches has it. Then each
// allowed that's gone, or keeps its rule. One loader reads them all, once.
func flexBreaches(t *testing.T, root, module string, every, drawing []string, a allowed) []string {
	t.Helper()
	l := sourcecheck.NewLoader(t, root, module, every)
	used := make(map[string]sourcecheck.Ban)
	for _, name := range breakpoints {
		used[name] = sourcecheck.Ban{Says: phoneSays(name)}
	}
	names := sourcecheck.NamesIn(module)
	return slices.Concat(
		l.Check(sourcecheck.Rule{Names: names, Used: used}, every, sourcecheck.Allowance{Entries: a.phone, List: "phoneAllowed", Fix: phoneAllowedFix}),
		l.Check(sourcecheck.Rule{Names: names, Node: sized}, drawing, sourcecheck.Allowance{Entries: a.sized, List: "sizedAllowed", Fix: sizedAllowedFix}),
		cutBreaches(l, module, drawing, a.cut),
	)
}

// sized says how n compares a width or a height with a constant: an ordered
// comparison of a constant with what reads a whole number sizeNames names, a
// variable or a field, as f.Width < phoneUnder or room < 110, but with 0, as
// whether there's any, or with 1 as with 0, as room < 1 is room <= 0. A size
// held under another name, as cells, or worked out by a function, as a
// canvas's width(), passes unseen.
func sized(n ast.Node, info *types.Info) string {
	b, ok := n.(*ast.BinaryExpr)
	if !ok || !slices.Contains([]token.Token{token.LSS, token.LEQ, token.GTR, token.GEQ}, b.Op) {
		return ""
	}
	for i, sides := range [][2]ast.Expr{{b.X, b.Y}, {b.Y, b.X}} {
		if tv := info.Types[sides[0]]; tv.Value != nil && !withNone(tv.Value, b.Op, i == 0) && readsASize(sides[1], info) {
			return sizedSays(types.ExprString(b))
		}
	}
	return ""
}

// withNone reports whether comparing a whole number with the constant c, by
// op, the constant first where first says, asks whether there's any: c is 0,
// or 1 where the comparison is one with 0, as n < 1 or 1 <= n.
func withNone(c constant.Value, op token.Token, first bool) bool {
	if constant.Sign(c) == 0 {
		return true
	}
	if first {
		op = map[token.Token]token.Token{token.LSS: token.GTR, token.LEQ: token.GEQ, token.GTR: token.LSS, token.GEQ: token.LEQ}[op]
	}
	return constant.Compare(c, token.EQL, constant.MakeInt64(1)) && (op == token.LSS || op == token.GEQ)
}

// readsASize reports whether e reads a whole number sizeNames names, a
// variable or a field.
func readsASize(e ast.Expr, info *types.Info) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && slices.Contains(sizeNames, id.Name) {
			v, isVar := info.Uses[id].(*types.Var)
			found = found || isVar && isWhole(v.Type())
		}
		return !found
	})
	return found
}

// isWhole reports whether t is a whole number's type.
func isWhole(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

// cutBreaches are where the non-test files of the packages in dirs, as l
// loads them, give a cutter a constant width other than whole, or use one as
// a value, but in the functions allowed: each of cutters, and each function
// or method of those packages that hands a width it's given on to one
// unchanged, as widths' wrapped works them out. Then each allowed that's
// gone, or gives none.
func cutBreaches(l *sourcecheck.Loader, module string, dirs, allowed []string) []string {
	var files []sourcecheck.File
	for _, dir := range dirs {
		files = append(files, l.Load(dir)...)
	}
	w := widthsOf(files)
	given := make(map[string][]widthAt)
	// A cutter that's a method, as a line's, or a wrapper is judged only where
	// the type checker sees the call: a fit of what the loader stubs isn't a
	// line's.
	function := make(map[string]bool)
	for _, c := range cutters {
		name := c.fullName(module)
		given[name] = []widthAt{{arg: c.width}}
		function[name] = c.recv == ""
	}
	calls := make(map[string]sourcecheck.CallBan)
	for name, at := range w.wrapped(files, given) {
		calls[name] = sourcecheck.CallBan{Says: cutSays(name), Breaks: w.constantWidth(at), Seen: !function[name]}
	}
	return l.Check(sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Calls: calls}, dirs, sourcecheck.Allowance{
		Entries: allowed, List: "cutAllowed", Fix: cutAllowedFix,
	})
}

func TestTheFlexGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, flexFixture)
	module := "example.com/fixture"
	dirs := []string{"internal/dashboard", "internal/dashboard/watch"}
	got := flexBreaches(t, root, module, dirs, dirs, allowed{
		phone: []string{
			"internal/dashboard/chrome.go:Frame.phone",
			"internal/dashboard/chrome.go:Frame.title",
			"internal/dashboard/gone.go:gone",
			"internal/dashboard/views.go:Frame.allowed",
			"internal/dashboard/views.go:Frame.kept",
		},
		sized: []string{
			"internal/dashboard/chrome.go:Phone",
			"internal/dashboard/sizes.go:allowedSize",
			"internal/dashboard/sizes.go:keptSize",
		},
		cut: []string{
			"internal/dashboard/names.go:allowedCut",
			"internal/dashboard/names.go:keptCut",
			"internal/dashboard/names.go:goneCut",
		},
	})
	unjudged := func(width string) string {
		return module + "/internal/prose.Cut can't be judged, as the type of its argument " + width +
			" is lost to a package the loader stubs; give the guard a type it can see, as a variable declared with one"
	}
	want := []string{
		"internal/dashboard/chrome.go:17: " + phoneSays("dashboard.Frame.phone"),
		"internal/dashboard/views.go:8: " + phoneSays("dashboard.Frame.phone"),
		"internal/dashboard/views.go:14: " + phoneSays("dashboard.Phone"),
		"internal/dashboard/views.go:16: " + phoneSays("dashboard.Frame.phone"),
		"internal/dashboard/watch/watch.go:5: " + phoneSays("dashboard.Phone"),
		"internal/dashboard/gone.go:gone: phoneAllowed allows it, but it's gone; " + phoneAllowedFix,
		"internal/dashboard/views.go:Frame.kept: phoneAllowed allows it, but it keeps the rule; " + phoneAllowedFix,
		"internal/dashboard/sizes.go:13: " + sizedSays("width >= wideCells"),
		"internal/dashboard/sizes.go:14: " + sizedSays("110 > room"),
		"internal/dashboard/sizes.go:15: " + sizedSays("f.Width - 2 <= 80"),
		"internal/dashboard/sizes.go:16: " + sizedSays("f.Height > 24"),
		"internal/dashboard/sizes.go:20: " + sizedSays("width <= 1"),
		"internal/dashboard/sizes.go:keptSize: sizedAllowed allows it, but it keeps the rule; " + sizedAllowedFix,
		"internal/dashboard/columns.go:31: " + cutSays(module+"/internal/dashboard.drawCall"),
		"internal/dashboard/columns.go:32: " + cutSays(module+"/internal/dashboard.label"),
		"internal/dashboard/columns.go:33: " + cutSays("("+module+"/internal/dashboard.columns).name"),
		"internal/dashboard/columns.go:34: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/line.go:17: " + cutSays("("+module+"/internal/dashboard.line).fit"),
		"internal/dashboard/line.go:19: " + cutSays("("+module+"/internal/dashboard.line).fit"),
		"internal/dashboard/line.go:21: " + cutSays(module+"/internal/dashboard.firstFit"),
		"internal/dashboard/line.go:23: " + cutSays(module+"/internal/dashboard.spread"),
		"internal/dashboard/line.go:25: " + cutSays(module+"/internal/dashboard.fitted"),
		"internal/dashboard/names.go:26: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:27: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:28: " + cutSays(module+"/internal/status.DirName"),
		"internal/dashboard/names.go:29: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:30: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:31: " + unjudged("cells.Most"),
		"internal/dashboard/names.go:32: " + unjudged("len(marks) + cells.Most"),
		"internal/dashboard/names.go:33: " + unjudged("int(unsafe.Sizeof(room)) + cells.Most"),
		"internal/dashboard/names.go:38: " + cutSays(module+"/internal/prose.CutShort"),
		"internal/dashboard/names.go:39: " + cutSays(module+"/internal/prose.CutFront"),
		"internal/dashboard/names.go:40: " + cutSays("github.com/charmbracelet/x/ansi.Truncate"),
		"internal/dashboard/names.go:44: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:45: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:47: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:48: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:50: " + cutSays(module+"/internal/prose.Cut"),
		"internal/dashboard/names.go:56: " + cutSays(module+"/internal/status.DirName"),
		"internal/dashboard/names.go:keptCut: cutAllowed allows it, but it keeps the rule; " + cutAllowedFix,
		"internal/dashboard/names.go:goneCut: cutAllowed allows it, but it's gone; " + cutAllowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// flexFixture is a module of prose and status, whose cutters cut a name to a
// width; and the dashboard, with its breakpoint in its chrome, chosen by in a
// function of the chrome's and one that isn't; a view that chooses by it,
// called and as a value, in functions allowed and not, and one allowed that
// keeps the rule; sizes compared with constants, a sum's, either side, and
// with 1 as with another, beside sizes compared with 0, and with 1 as with
// 0, a count, a function's and a cell's; names cut to constant widths,
// named, sums, bare and an array's length, followed back a step to a local,
// a package-level variable and a literal's field, keyed or not, through a
// literal within it, and to one of a package the loader stubs, beside widths
// known only at run time, one of them through what the loader stubs, the
// most there is, locals and literals written after, by assignment, address
// and a method taking a pointer, one first given a variable, a constant two
// steps back, and a function allowed; cutters that hand their width on, a
// parameter or its field, called with constants, beside one that changes it
// first; line.go's cutters, a method and functions with their widths first
// and last, given constants and run-time widths, one as a value, one wrapped,
// beside a method of the same name on what the loader stubs; and the watch,
// which chooses by the breakpoint too.
var flexFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/prose/prose.go": `package prose

func Cut(text string, cells int) string      { return text }
func CutShort(text string, cells int) string { return text }
func CutFront(text string, cells int) string { return text }
`,
	"internal/status/status.go": `package status

import "example.com/fixture/internal/prose"

func DirName(dir string, cells int) string { return prose.Cut(dir, cells) }
`,
	"internal/dashboard/chrome.go": `package dashboard

type Frame struct{ Width, Height int }

func (f Frame) phone() bool { return Phone(f.Width) }

func Phone(width int) bool { return width < 100 }

func (f Frame) title() int {
	if f.phone() {
		return 2
	}
	return 1
}

func (f Frame) heading() int {
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
	"internal/dashboard/sizes.go": `package dashboard

const wideCells = 120

func allowedSize(width int) bool { return width < wideCells }

func keptSize(width int) bool { return width > 0 }

func measured() int { return 3 }

func sized(f Frame, width, room, cells, count int, rows []string) []bool {
	return []bool{
		width >= wideCells,
		110 > room,
		f.Width-2 <= 80,
		f.Height > 24,
		width > 0,
		room < 1,
		1 <= room,
		width <= 1,
		count < 3,
		cells < 8,
		len(rows) > 2,
		measured() > 4,
	}
}
`,
	"internal/dashboard/names.go": `package dashboard

import (
	"math"
	"unsafe"

	"example.org/cells"

	"github.com/charmbracelet/x/ansi"

	"example.com/fixture/internal/prose"
	"example.com/fixture/internal/status"
)

const labelCells = 8

func allowedCut(name string) string { return prose.Cut(name, labelCells) }

func keptCut(name string) string { return name }

func cut(name, dir string, room int, marks [3]string) []string {
	w, written, from, addressed := 8, 8, room, 8
	written++
	_ = &addressed
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
		prose.CutFront(name, labelCells),
		ansi.Truncate(name, labelCells, ""),
		ansi.Truncate(name, room, ""),
		prose.Cut(name, written),
		prose.Cut(name, from),
		prose.Cut(name, outer),
		prose.Cut(name, w),
		prose.Cut(name, twice),
		prose.Cut(name, laneColumns.dir),
		prose.Cut(name, unkeyed.account),
		prose.Cut(name, boardColumns.dir),
		prose.Cut(name, nested.inner.dir),
		prose.Cut(name, addressed),
		prose.Cut(name, pointed.dir),
	}
}

var cutter = status.DirName

var outer = labelCells

var twice = outer
`,
	"internal/dashboard/columns.go": `package dashboard

import (
	"example.com/fixture/internal/prose"
	"example.com/fixture/internal/status"
)

type columns struct{ dir, account int }

type holder struct{ inner columns }

var laneColumns = columns{dir: 11, account: labelCells}

var boardColumns = columns{11, 8}

var unkeyed = columns{7, 8}

var nested = holder{inner: columns{dir: 6}}

var pointed = columns{dir: 5}

func init() {
	boardColumns.dir = 9
	pointed.widen()
}

func (c *columns) widen() { c.dir++ }

func lanes(dir, name string, room int) []string {
	return []string{
		drawCall(dir, name, laneColumns),
		label(name, labelCells),
		columns{}.name(name, labelCells),
		prose.Cut(name, laneColumns.account),
		drawCall(dir, name, columns{dir: room}),
		label(name, room),
		changed(name, labelCells),
	}
}

func drawCall(dir, name string, k columns) string {
	return status.DirName(dir, k.dir) + from(name, k)
}

func from(name string, k columns) string { return prose.Cut(name, k.account) }

func label(name string, cells int) string { return columns{}.name(name, cells) }

func (c columns) name(name string, cells int) string { return prose.Cut(name, cells) }

func changed(name string, cells int) string {
	cells--
	return prose.Cut(name, cells)
}
`,
	"internal/dashboard/line.go": `package dashboard

import "example.org/cells"

type line []string

func (l line) fit(width int) line { return l }

func firstFit(width int, forms ...line) line { return forms[0].fit(width) }

func spread(left, right line, width int) line { return left.fit(width - len(right)) }

func fitted(l line, cells int) line { return l.fit(cells) }

func lines(name string, room int) []line {
	l := line{name}
	cut := l.fit
	return []line{
		l.fit(labelCells),
		l.fit(room),
		firstFit(labelCells, l),
		firstFit(room, l),
		spread(l, l, labelCells),
		spread(l, l, room),
		fitted(l, 4),
		cut(room),
		cells.Of(name).fit(labelCells),
	}
}
`,
	"internal/dashboard/watch/watch.go": `package watch

import "example.com/fixture/internal/dashboard"

func phone(width int) bool { return dashboard.Phone(width) }
`,
}
