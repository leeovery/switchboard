package status_test

import (
	"cmp"
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a view can't work out a fact of a
// document at a moment for itself, rather than read the one moment status
// works it out in, without failing here.

// viewPackages are the packages that read documents to show them, by
// directory from the module's root: each reads the moment alone.
var viewPackages = []string{
	"internal/cli",
	"internal/dashboard",
	"internal/dashboard/watch",
	"internal/notify",
	"internal/views",
}

// readsAllowed are the files of viewPackages allowed to break the rule, by
// path from the module's root. The list only shrinks: a file it names must
// still be there, and still break the rule.
var readsAllowed = []string{
	// Producers, which build the documents moments are of.
	"internal/cli/source.go",
	// History's points, worked out of the readings the ledger keeps of
	// requests' answers, each a window as read: no document's, so of no
	// moment.
	"internal/views/points.go",
}

// callsAllowed are the functions and methods of status, quota and score
// that take a wire type, as their receiver or a parameter, that a view may
// call all the same, each by its name, as nameOf has it. Any other fails, as
// one added later would. The list only shrinks: one no view calls fails.
var callsAllowed = []string{
	// The moment of a document at a time, which every fact a view says is
	// read off; and that moment held, for every look at the one document at
	// the one time to share, known by the document it's of.
	"status.Document.At", "status.Document.HoldAt", "status.HeldMoment.Of",
	// The watch's read loop, which asks for the next read by the document
	// as it was read, when it was read, rather than at a moment.
	"status.Document.IncompleteAsRead", "status.Document.NextFetchAsRead", "status.Document.RereadAfterResetAsRead",
}

// momentsMade are the functions and methods of viewPackages that work out a
// document's moment, as worksOut names what does, each by its name, as nameOf
// has it: where a look starts, which reads the one moment it works out, or
// hands it on, for every view of it. Any other fails, as one added later
// would, so a look holding a moment works out no second of the same document
// at the same time. The list only shrinks: one that works none out fails.
var momentsMade = []string{
	// The frame's, which holds the moment of the document it draws for each
	// of its looks to share, else works it out for the one look asking.
	"dashboard.Frame.At", "dashboard.Frame.Moment",
	// The watch's: the moment of the message Update takes in, held for every
	// look it takes; a read's, as it lands, which show holds in its place;
	// whether a read is the router's, before the watch takes it in; the
	// moment Update holds, else worked out where no frame is asked for; and
	// each last look's, taken to a new look's moment, once for every account
	// it last read.
	"dashboard/watch.Model.Update", "dashboard/watch.Model.fetched", "dashboard/watch.fetchFrom",
	"dashboard/watch.Model.moment", "dashboard/watch.takenTo",
	// The commands', each of the one document it reads.
	"cli.app.pinning", "cli.app.printUsage", "cli.newStatusCommand",
}

// worksOut are what work out a document's moment.
var worksOut = []string{"status.Document.At", "status.Document.HoldAt", "status.Moment.At"}

// What breaks the rule, each by its package's name under the module's
// internal directory, then its type's, where it's a method, then its own.
var (
	// wireTypes are the types of the documents status and quota give, whose
	// fields hold the facts as read, for the moment to judge: a view reads
	// none of their fields.
	wireTypes = []string{
		"quota.ExtraUsage", "quota.Failure", "quota.Usage", "quota.Window",
		"status.Account", "status.Document", "status.Limit", "status.Pool", "status.PoolWindow",
		"status.Pressure", "status.Prime", "status.Rate", "status.Slot",
	}
	// judges are the functions that judge a window: a view calls none,
	// whether or not it takes a wire type.
	judges = []string{
		"quota.Window.ResetBy", "quota.Window.Span",
		"score.AllowanceOf", "score.AtReserve", "score.Available", "score.Current", "score.Elapsed", "score.EvenPace",
		"score.HeldByReserve", "score.LetGo", "score.PaceOf", "score.Project", "score.ProjectAt", "score.Reaches",
		"score.RecentRate", "score.Room", "score.Spent",
		"score.Policy.AsOf", "score.Policy.Choose", "score.Policy.HasLapsed", "score.Policy.Lapsed", "score.Policy.Pick",
		"score.Policy.PickPinned", "score.Policy.PressureOf",
		"status.PressureFrom", "status.Account.AsOf",
	}
	// absorbed are what status works a fact out with that the moment has:
	// a view calls none, but reads the moment's fact. What the moment
	// absorbed that nothing outside status called, status no longer gives.
	absorbed = []string{
		"status.Best", "status.ProbedBest",
		"status.Account.HeldBy", "status.Account.Shut", "status.Account.TokenRefused", "status.Account.WindowAt",
		"status.Document.Destination", "status.Document.Floor", "status.Document.Reserved", "status.Document.RunsOut",
		"status.Document.StateOf", "status.Document.Text", "status.Document.WindowStatesOf",
	}
)

// wirePackages are the packages, by name under the module's internal
// directory, whose functions and methods that take a wire type a view calls
// only where callsAllowed names them.
var wirePackages = []string{"quota", "score", "status"}

// The fix each kind of breach gives.
const (
	wireFix        = "read the fact off the moment the document is at, as Document.At has it"
	judgeFix       = "read the moment's verdict of the window, its WindowNow"
	absorbedFix    = "read the moment's fact, which it works out once"
	callFix        = "read the moment's fact, or name the call in callsAllowed in internal/status/guard_test.go, saying why a view may make it"
	allowedFix     = "remove it from readsAllowed in internal/status/guard_test.go"
	allowedCallFix = "remove it from callsAllowed in internal/status/guard_test.go"
	madeFix        = "hand on the moment the look holds, or name the function in momentsMade in internal/status/guard_test.go, saying why it works one out"
	allowedMadeFix = "remove it from momentsMade in internal/status/guard_test.go"
)

func TestViewsReadTheMomentAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, momentBreaches(t, root, sourcecheck.ModulePath(t, root), guarded{dirs: viewPackages, files: readsAllowed, calls: callsAllowed}))
}

func TestTheMomentGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, guardFixture)
	got := momentBreaches(t, root, "example.com/fixture", guarded{
		dirs:  []string{"internal/view"},
		files: []string{"internal/view/allowed.go", "internal/view/kept.go", "internal/view/gone.go"},
		calls: []string{"status.Document.At", "status.Account.Title", "status.Document.Gone"},
	})
	want := []string{
		"internal/view/absorbed.go:10: status.Document.StateOf works out what the moment has; " + absorbedFix,
		"internal/view/absorbed.go:11: status.Best works out what the moment has; " + absorbedFix,
		"internal/view/absorbed.go:12: status.Account.TokenRefused works out what the moment has; " + absorbedFix,
		"internal/view/fields.go:13: status.Document.Accounts reads the wire's fields; " + wireFix,
		"internal/view/fields.go:14: status.Account.ID reads the wire's fields; " + wireFix,
		"internal/view/fields.go:15: status.Account.Windows reads the wire's fields; " + wireFix,
		"internal/view/fields.go:15: quota.Window.Utilization reads the wire's fields; " + wireFix,
		"internal/view/fields.go:17: status.Account.Windows reads the wire's fields; " + wireFix,
		"internal/view/fields.go:20: status.Document.Accounts reads the wire's fields; " + wireFix,
		"internal/view/fields.go:21: status.Account.ID reads the wire's fields; " + wireFix,
		"internal/view/fields.go:21: status.Account.Windows reads the wire's fields; " + wireFix,
		"internal/view/fields.go:26: status.Document.Accounts reads the wire's fields; " + wireFix,
		"internal/view/judges.go:11: score.Current judges a window; " + judgeFix,
		"internal/view/judges.go:12: score.Policy.AsOf judges a window; " + judgeFix,
		"internal/view/judges.go:13: quota.Window.ResetBy judges a window; " + judgeFix,
		"internal/view/judges.go:14: score.Current judges a window; " + judgeFix,
		"internal/view/judges.go:15: quota.Window.Span judges a window; " + judgeFix,
		"internal/view/planted.go:10: status.Account.Fresh takes the wire's status.Account; " + callFix,
		"internal/view/planted.go:11: status.Sorted takes the wire's status.Account; " + callFix,
		"internal/view/planted.go:12: status.Keyed takes the wire's status.Account; " + callFix,
		"internal/view/kept.go: readsAllowed allows it, but it keeps the rule; " + allowedFix,
		"internal/view/gone.go: readsAllowed allows it, but it's gone; " + allowedFix,
		"status.Document.Gone: callsAllowed allows it, but no view calls it; " + allowedCallFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestALookWorksOutOneMoment(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, momentsMadeOutside(t, root, sourcecheck.ModulePath(t, root), viewPackages, momentsMade))
}

func TestTheMomentsMadeGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, madeFixture)
	got := momentsMadeOutside(t, root, "example.com/fixture", []string{"internal/view"}, []string{"view.Frame.At", "view.listed", "view.gone"})
	want := []string{
		"internal/view/lost.go:15: held.doc.At can't be judged, as the type of held.doc is lost to a package the loader stubs; give the guard a type it can see, as a variable declared with one",
		"internal/view/view.go:16: view.again works out a moment with status.Document.At; " + madeFix,
		"internal/view/view.go:17: view.again works out a moment with status.Moment.At; " + madeFix,
		"internal/view/view.go:21: view.inside works out a moment with status.Document.At; " + madeFix,
		"internal/view/view.go:24: a package-level declaration works out a moment with status.Document.At; " + madeFix,
		"view.gone: momentsMade allows it, but it works out none; " + allowedMadeFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// madeFixture is a module whose status works out a document's moment, and a
// view package of functions that work one out: one allowed to, at a frame's
// method; one that works out two, the second of the first; one that does so
// in a function literal; a package-level declaration; one allowed to that
// works out none; and one that may, through a document whose type the
// loader lost to a package it stubs, beside what may not.
var madeFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/status/status.go": `package status

import "time"

type Document struct{}

type Moment struct{}

type HeldMoment struct{}

func (d Document) At(now time.Time) Moment          { return Moment{} }
func (d Document) HoldAt(now time.Time) *HeldMoment { return nil }
func (m Moment) At(now time.Time) Moment            { return m }
`,
	"internal/view/view.go": `package view

import (
	"time"

	"example.com/fixture/internal/status"
)

type Frame struct{}

func (f Frame) At(doc status.Document, now time.Time) *status.HeldMoment { return doc.HoldAt(now) }

func listed(doc status.Document, now time.Time) status.Moment { return doc.At(now) }

func again(doc status.Document, now time.Time) status.Moment {
	m := doc.At(now)
	return m.At(now)
}

func inside(doc status.Document, now time.Time) func() status.Moment {
	return func() status.Moment { return doc.At(now) }
}

var made = status.Document{}.At(time.Time{})

func gone() {}
`,
	"internal/view/lost.go": `package view

import (
	"time"

	"example.org/tea"

	"example.com/fixture/internal/status"
)

type docMsg struct{ doc status.Document }

func lost(msg tea.Msg, now time.Time) []any {
	held, _ := msg.(docMsg)
	return []any{held.doc.At(now), msg.String()}
}
`,
}

// guardFixture is a module of the wire's types, the judges, what the moment
// absorbed, and functions and methods that take a wire type, as one added
// later would, by path from its root; and a view package with a file of each
// kind of breach, one that keeps the rule, calling what the allow-list
// allows, one the allow-list allows, and one it allows that keeps it.
var guardFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/quota/quota.go": `package quota

import "time"

type Window struct {
	Key         string
	Utilization float64
	ResetsAt    time.Time
}

func (w Window) Span() (time.Time, time.Duration, bool) { return time.Time{}, 0, false }
func (w Window) ResetBy(t time.Time) bool                 { return false }

type Usage struct{ Windows []Window }
`,
	"internal/score/score.go": `package score

import (
	"time"

	"example.com/fixture/internal/quota"
)

func Current(w quota.Window, now time.Time) quota.Window { return w }

type Policy struct{ Shared []string }

func (p Policy) AsOf(windows []quota.Window, now time.Time) []quota.Window { return windows }
func (p Policy) IsShared(key string) bool                                 { return false }
`,
	"internal/status/status.go": `package status

import (
	"time"

	"example.com/fixture/internal/quota"
)

type Document struct{ Accounts []Account }

type Account struct {
	ID string
	quota.Usage
}

type Event struct{ Account string }

type Moment struct{ Accounts []AccountNow }

type AccountNow struct {
	ID   string
	Held bool
}

func (d Document) At(now time.Time) Moment                  { return Moment{} }
func (d Document) StateOf(a Account, now time.Time) string    { return "" }
func (a Account) TokenRefused(now time.Time) bool             { return false }
func (a Account) Title() string                               { return a.ID }
func Best(accounts []Account) string                          { return "" }
func (d Document) Gone()                                      {}
func (a Account) Fresh(now time.Time) bool                    { return false }
func Sorted(accounts []*Account) []*Account                   { return accounts }
func Keyed(byID map[string]Account) int                       { return len(byID) }
`,
	"internal/view/clean.go": `package view

import (
	"time"

	"example.com/fixture/internal/score"
	"example.com/fixture/internal/status"
)

func clean(doc status.Document, e status.Event, now time.Time, policy score.Policy) (string, bool) {
	m := doc.At(now)
	held := len(m.Accounts) > 0 && m.Accounts[0].Held
	return e.Account + m.Accounts[0].ID, held || policy.IsShared("5h")
}

func titled(a status.Account) string { return a.Title() }
`,
	"internal/view/fields.go": `package view

import (
	"slices"

	"example.com/fixture/internal/status"
)

type holding struct{ status.Account }

func fields(doc status.Document, h holding) []string {
	var ids []string
	for _, a := range doc.Accounts {
		ids = append(ids, a.ID)
		_ = a.Windows[0].Utilization
	}
	if len(h.Windows) > 0 {
		ids = append(ids, "")
	}
	first := slices.Clone(doc.Accounts)[0]
	return append(ids, first.ID, string(rune(len(first.Windows))))
}

func counted(doc *status.Document) int {
	read := func() int {
		return len(doc.Accounts)
	}
	return read()
}
`,
	"internal/view/judges.go": `package view

import (
	"time"

	"example.com/fixture/internal/score"
	"example.com/fixture/internal/quota"
)

func judges(w quota.Window, policy score.Policy, now time.Time) bool {
	_ = score.Current(w, now)
	_ = policy.AsOf([]quota.Window{w}, now)
	reset := w.ResetBy
	current := score.Current
	_, _, ok := w.Span()
	return reset(now) && current(w, now) == w && ok
}
`,
	"internal/view/absorbed.go": `package view

import (
	"time"

	"example.com/fixture/internal/status"
)

func absorbed(doc status.Document, a status.Account, now time.Time) bool {
	state := doc.StateOf(a, now)
	best := status.Best(nil)
	return a.TokenRefused(now) && state == best
}
`,
	"internal/view/planted.go": `package view

import (
	"time"

	"example.com/fixture/internal/status"
)

func planted(a status.Account, accounts []*status.Account, byID map[string]status.Account, now time.Time) bool {
	fresh := a.Fresh(now)
	sorted := status.Sorted(accounts)
	return fresh && len(sorted) == status.Keyed(byID)
}
`,
	"internal/view/allowed.go": `package view

import "example.com/fixture/internal/status"

func allowed(doc status.Document) int { return len(doc.Accounts) }
`,
	"internal/view/kept.go": `package view

func kept() {}
`,
	"internal/view/view_test.go": `package view

import "example.com/fixture/internal/status"

func tested(doc status.Document) int { return len(doc.Accounts) }
`,
}

// guarded is what the guard reads, and what it lets be: the packages it
// reads, by directory from the module's root; the files allowed to break the
// rule, by path from the root; and the calls allowed that take a wire type.
type guarded struct {
	dirs, files, calls []string
}

// momentBreaches are where the non-test files of the packages g reads, of the
// module at root with the given path, break the rule, but in the files g
// allows: each read of a wire type's field, through any field it's embedded
// in; each use of a judge, called or not; each use of what the moment
// absorbed; and each use of any other function or method of wirePackages
// that takes a wire type, but the calls g allows. Then each file allowed
// that's gone, or keeps the rule, and each call allowed that none of the
// other files makes.
func momentBreaches(t *testing.T, root, module string, g guarded) []string {
	t.Helper()
	return sourcecheck.Check(t, root, module, newRule(module, g.calls), g.dirs, sourcecheck.Allowance{
		Entries: g.files, List: "readsAllowed", Fix: allowedFix,
		Calls: "callsAllowed", Callers: "view", CallFix: allowedCallFix,
	})
}

// newRule is the moment's rule, of the module with the given path, the calls
// given allowed: no wire type's field read; no judge used, nor what the
// moment absorbed; and no other function or method of wirePackages called
// that takes a wire type.
func newRule(module string, calls []string) sourcecheck.Rule {
	used := make(map[string]sourcecheck.Ban)
	for _, name := range judges {
		used[name] = sourcecheck.Ban{Says: name + " judges a window; " + judgeFix}
	}
	for _, name := range absorbed {
		used[name] = sourcecheck.Ban{Says: name + " works out what the moment has; " + absorbedFix}
	}
	return sourcecheck.Rule{
		Names:   sourcecheck.NamesIn(module),
		Fields:  wireTypes,
		Read:    func(field string) string { return field + " reads the wire's fields; " + wireFix },
		Used:    used,
		Owners:  wirePackages,
		Takes:   func(fn, wire string) string { return fn + " takes the wire's " + wire + "; " + callFix },
		Allowed: calls,
	}
}

// momentsMadeOutside are where the non-test files of the packages in dirs, of
// the module at root with the given path, work out a document's moment, as
// worksOut names what does, or may, through what the type checker lost the
// type of, but in the functions and methods made names: each use, called or
// not, in any other, a function literal's counting as its function's, or in
// a package-level declaration. Then each that made names that works none
// out.
func momentsMadeOutside(t *testing.T, root, module string, dirs, made []string) []string {
	t.Helper()
	names := sourcecheck.NamesIn(module)
	l := sourcecheck.NewLoader(t, root, module, dirs)
	var problems []string
	making := make(map[string]bool)
	for _, dir := range dirs {
		for _, f := range l.Load(dir) {
			for _, decl := range f.AST.Decls {
				who := "a package-level declaration"
				if fd, ok := decl.(*ast.FuncDecl); ok {
					who = names.Func(f.Info.Defs[fd.Name].(*types.Func))
				}
				for _, m := range momentsIn(f, decl, names, who) {
					making[who] = true
					if !slices.Contains(made, who) {
						problems = append(problems, f.At(m.At)+": "+m.Says)
					}
				}
			}
		}
	}
	for _, name := range made {
		if !making[name] {
			problems = append(problems, name+": momentsMade allows it, but it works out none; "+allowedMadeFix)
		}
	}
	return problems
}

// momentsIn are where the declaration in f, of who, works out a document's
// moment, saying with what of worksOut; and where it may, through what the
// type checker lost the type of, which the guard can't judge: in the order
// they come in it.
func momentsIn(f sourcecheck.File, decl ast.Decl, names sourcecheck.Names, who string) []sourcecheck.Breach {
	var found []sourcecheck.Breach
	ast.Inspect(decl, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if fn, ok := f.Info.Uses[id].(*types.Func); ok && slices.Contains(worksOut, names.Func(fn)) {
			found = append(found, sourcecheck.Breach{At: id.Pos(), Says: who + " works out a moment with " + names.Func(fn) + "; " + madeFix})
		}
		return true
	})
	for _, lost := range f.LostIn(decl) {
		if lost.Of != nil && slices.ContainsFunc(worksOut, func(name string) bool { return strings.HasSuffix(name, "."+lost.Name.Name) }) {
			found = append(found, sourcecheck.Breach{At: lost.Name.Pos(), Says: lost.Says()})
		}
	}
	slices.SortStableFunc(found, func(a, b sourcecheck.Breach) int { return cmp.Compare(a.At, b.At) })
	return found
}
