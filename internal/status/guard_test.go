package status_test

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
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
	// Milestone 5's views, which stage 12 deletes, and what they alone read.
	"internal/dashboard/cords.go",
	"internal/dashboard/log.go",
	"internal/dashboard/panel.go",
	"internal/dashboard/plain.go",
	"internal/dashboard/recent_log.go",
	"internal/dashboard/seats.go",
	"internal/dashboard/sessions.go",
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
		"score.Policy.AsOf", "score.Policy.HasLapsed", "score.Policy.Lapsed", "score.Policy.Pick", "score.Policy.PressureOf",
		"status.PressureFrom", "status.Account.AsOf",
	}
	// absorbed are what status works a fact out with that the moment has:
	// a view calls none, but reads the moment's fact. What the moment
	// absorbed that nothing outside status called, status no longer gives.
	absorbed = []string{
		"status.Best",
		"status.Account.HeldBy", "status.Account.Shut", "status.Account.TokenRefused", "status.Account.WindowAt",
		"status.Document.Destination", "status.Document.Floor", "status.Document.Reserved", "status.Document.RunsOut",
		"status.Document.StateOf", "status.Document.WindowStatesOf",
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
	root := moduleRoot(t)
	reportEach(t, momentBreaches(t, root, modulePath(t, root), guarded{dirs: viewPackages, files: readsAllowed, calls: callsAllowed}))
}

func TestTheMomentGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := writeModule(t, guardFixture)
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
	root := moduleRoot(t)
	reportEach(t, momentsMadeOutside(t, root, modulePath(t, root), viewPackages, momentsMade))
}

func TestTheMomentsMadeGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := writeModule(t, madeFixture)
	got := momentsMadeOutside(t, root, "example.com/fixture", []string{"internal/view"}, []string{"view.Frame.At", "view.listed", "view.gone"})
	want := []string{
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
// in a function literal; a package-level declaration; and one allowed to
// that works out none.
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
	r := newRule(module, g.calls)
	l := newSourceLoader(t, root, module, g.dirs)
	var problems, broken []string
	called := make(map[string]bool)
	for _, dir := range g.dirs {
		for _, f := range l.load(dir) {
			found, allowedCalls := r.breaches(f)
			if slices.Contains(g.files, f.path) {
				if len(found) > 0 {
					broken = append(broken, f.path)
				}
				continue
			}
			problems = append(problems, found...)
			for _, call := range allowedCalls {
				called[call] = true
			}
		}
	}
	for _, file := range g.files {
		switch _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); {
		case errors.Is(err, os.ErrNotExist):
			problems = append(problems, file+": readsAllowed allows it, but it's gone; "+allowedFix)
		case err != nil:
			t.Fatal(err)
		case !slices.Contains(broken, file):
			problems = append(problems, file+": readsAllowed allows it, but it keeps the rule; "+allowedFix)
		}
	}
	for _, call := range g.calls {
		if !called[call] {
			problems = append(problems, call+": callsAllowed allows it, but no view calls it; "+allowedCallFix)
		}
	}
	return problems
}

// momentsMadeOutside are where the non-test files of the packages in dirs, of
// the module at root with the given path, work out a document's moment, as
// worksOut names what does, but in the functions and methods made names:
// each use, called or not, in any other, a function literal's counting as
// its function's, or in a package-level declaration. Then each that made
// names that works none out.
func momentsMadeOutside(t *testing.T, root, module string, dirs, made []string) []string {
	t.Helper()
	r := newRule(module, nil)
	l := newSourceLoader(t, root, module, dirs)
	var problems []string
	making := make(map[string]bool)
	for _, dir := range dirs {
		for _, f := range l.load(dir) {
			for _, decl := range f.ast.Decls {
				who := "a package-level declaration"
				if fd, ok := decl.(*ast.FuncDecl); ok {
					who = r.nameOf(f.info.Defs[fd.Name].(*types.Func))
				}
				for _, u := range f.momentsIn(decl, r) {
					making[who] = true
					if !slices.Contains(made, who) {
						problems = append(problems, f.at(u.at)+": "+who+" works out a moment with "+u.name+"; "+madeFix)
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

// use is where a file uses a function, by its name, as nameOf has it.
type use struct {
	at   token.Pos
	name string
}

// momentsIn are where the declaration in f works out a document's moment, in
// the order they come in it, each with what of worksOut it uses.
func (f checkedFile) momentsIn(decl ast.Decl, r rule) []use {
	var uses []use
	ast.Inspect(decl, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if fn, ok := f.info.Uses[id].(*types.Func); ok && slices.Contains(worksOut, r.nameOf(fn)) {
			uses = append(uses, use{at: id.Pos(), name: r.nameOf(fn)})
		}
		return true
	})
	return uses
}

// rule is what breaks the moment's rule, each named in full, by its package's
// import path, as qualified names it.
type rule struct {
	wire, judges, absorbed, calls []string
	// internal is the import path of the module's internal directory, which
	// the names go by.
	internal string
}

func newRule(module string, calls []string) rule {
	return rule{wire: wireTypes, judges: judges, absorbed: absorbed, calls: calls, internal: module + "/internal/"}
}

// breaches are where the type-checked file f breaks the rule, in the order
// they come in it, and the calls it makes that the rule allows.
func (r rule) breaches(f checkedFile) (problems, allowed []string) {
	type breach struct {
		at   token.Pos
		says string
	}
	var found []breach
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if sel, ok := f.info.Selections[n]; ok && sel.Kind() == types.FieldVal {
				if read := r.wireRead(sel); read != "" {
					found = append(found, breach{n.Sel.Pos(), read + " reads the wire's fields; " + wireFix})
				}
			}
		case *ast.Ident:
			fn, ok := f.info.Uses[n].(*types.Func)
			if !ok {
				break
			}
			switch name, says := r.called(fn); {
			case says != "":
				found = append(found, breach{n.Pos(), says})
			case slices.Contains(r.calls, name):
				allowed = append(allowed, name)
			}
		}
		return true
	})
	slices.SortStableFunc(found, func(x, y breach) int { return cmp.Compare(x.at, y.at) })
	for _, b := range found {
		problems = append(problems, f.at(b.at)+": "+b.says)
	}
	return problems, allowed
}

// called names the function fn, as nameOf has it, and says how using it
// breaks the rule, "" where it doesn't: it judges a window; it works out
// what the moment has; or it's of wirePackages and takes a wire type, and
// the rule doesn't allow it.
func (r rule) called(fn *types.Func) (name, says string) {
	name = r.nameOf(fn)
	pkg, _, _ := strings.Cut(name, ".")
	switch {
	case slices.Contains(r.judges, name):
		return name, name + " judges a window; " + judgeFix
	case slices.Contains(r.absorbed, name):
		return name, name + " works out what the moment has; " + absorbedFix
	case !slices.Contains(wirePackages, pkg) || slices.Contains(r.calls, name):
		return name, ""
	}
	if wire := r.takes(fn); wire != "" {
		return name, name + " takes the wire's " + wire + "; " + callFix
	}
	return name, ""
}

// takes names the first wire type the function fn takes, as its receiver or
// a parameter, as wireIn finds it: "" where it takes none.
func (r rule) takes(fn *types.Func) string {
	sig := fn.Signature()
	var taken []types.Type
	if recv := sig.Recv(); recv != nil {
		taken = append(taken, recv.Type())
	}
	for v := range sig.Params().Variables() {
		taken = append(taken, v.Type())
	}
	for _, t := range taken {
		if wire := r.wireIn(t); wire != "" {
			return wire
		}
	}
	return ""
}

// wireIn names the wire type t is, or holds through pointers, slices, arrays
// and maps, as "status.Account": "" where it's none.
func (r rule) wireIn(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return r.wireIn(t.Elem())
	case *types.Slice:
		return r.wireIn(t.Elem())
	case *types.Array:
		return r.wireIn(t.Elem())
	case *types.Map:
		return cmp.Or(r.wireIn(t.Key()), r.wireIn(t.Elem()))
	case *types.Named:
		if name := r.typeName(t); slices.Contains(r.wire, name) {
			return name
		}
	}
	return ""
}

// wireRead names the wire type's field sel reads, as "status.Account.ID": the
// field it selects, of the first wire type the selection passes through, from
// the type it selects from through each field embedded on the way; "" where
// it passes through none.
func (r rule) wireRead(sel *types.Selection) string {
	t := sel.Recv()
	for _, index := range sel.Index() {
		named, s := structOf(t)
		if s == nil {
			return ""
		}
		if name := r.typeName(named); slices.Contains(r.wire, name) {
			return name + "." + sel.Obj().Name()
		}
		t = s.Field(index).Type()
	}
	return ""
}

// structOf is the struct t is, or points to, and the type it's named by,
// where it is: nil where it's no struct.
func structOf(t types.Type) (*types.Named, *types.Struct) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, _ := types.Unalias(t).(*types.Named)
	s, _ := t.Underlying().(*types.Struct)
	return named, s
}

// typeName is the named type's name, by its package's under the module's
// internal directory, as "status.Account": "" of a type of no package there.
func (r rule) typeName(named *types.Named) string {
	if named == nil {
		return ""
	}
	obj := named.Obj()
	if obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), r.internal) {
		return ""
	}
	return strings.TrimPrefix(obj.Pkg().Path(), r.internal) + "." + obj.Name()
}

// nameOf is the function's name, by its package's under the module's
// internal directory, then, of a method, its receiver's type's, as
// "status.Document.RoutingOf": "" of one of no package there.
func (r rule) nameOf(fn *types.Func) string {
	if fn.Pkg() == nil || !strings.HasPrefix(fn.Pkg().Path(), r.internal) {
		return ""
	}
	name := strings.TrimPrefix(fn.Pkg().Path(), r.internal) + "."
	if recv := fn.Signature().Recv(); recv != nil {
		named, _ := structOf(recv.Type())
		if named == nil {
			return ""
		}
		name += named.Obj().Name() + "."
	}
	return name + fn.Name()
}

// checkedFile is one of a package's non-test files, parsed and type-checked.
type checkedFile struct {
	// path is the file's path from the module's root, with slashes.
	path string
	fset *token.FileSet
	ast  *ast.File
	info *types.Info
}

// at is where pos is in the file: its path and line.
func (f checkedFile) at(pos token.Pos) string {
	return fmt.Sprintf("%s:%d", f.path, f.fset.Position(pos).Line)
}

// sourceLoader type-checks packages from their source, without the go
// command, which no test may run: the module's, from its root, and the
// standard library's, from GOROOT, built as this platform builds them,
// without cgo. Any other package, a module the module requires, is stubbed:
// an empty package, as its source is where only the go command finds it.
// Nothing of the wire's types comes through one.
type sourceLoader struct {
	t            *testing.T
	fset         *token.FileSet
	root, module string
	ctx          build.Context
	// read are the directories, from the module's root, whose packages are
	// read whole: every function body type-checked, and what each expression
	// is kept. Of the rest, their declarations alone are.
	read    []string
	pkgs    map[string]*types.Package
	checked map[string][]checkedFile
}

func newSourceLoader(t *testing.T, root, module string, read []string) *sourceLoader {
	t.Helper()
	ctx := build.Default
	ctx.CgoEnabled = false
	if ctx.GOROOT == "" {
		t.Fatal("no GOROOT, whose source the standard library's types are read from")
	}
	return &sourceLoader{
		t: t, fset: token.NewFileSet(), root: root, module: module, ctx: ctx, read: read,
		pkgs: map[string]*types.Package{"unsafe": types.Unsafe}, checked: make(map[string][]checkedFile),
	}
}

// load is the type-checked files of the package in dir, from the module's
// root, read whole.
func (l *sourceLoader) load(dir string) []checkedFile {
	l.t.Helper()
	if _, err := l.Import(path.Join(l.module, dir)); err != nil {
		l.t.Fatal(err)
	}
	files := l.checked[dir]
	if len(files) == 0 {
		l.t.Fatalf("%s has no Go files to read: the guard reads nothing of it", dir)
	}
	return files
}

func (l *sourceLoader) Import(importPath string) (*types.Package, error) {
	return l.ImportFrom(importPath, l.root, 0)
}

// ImportFrom is the package at importPath, imported from the package in dir,
// type-checked from its source, once.
func (l *sourceLoader) ImportFrom(importPath, dir string, _ types.ImportMode) (*types.Package, error) {
	if pkg, ok := l.pkgs[importPath]; ok {
		return pkg, nil
	}
	src, std, ok := l.dirOf(importPath, dir)
	if !ok {
		pkg := types.NewPackage(importPath, stubName(importPath))
		pkg.MarkComplete()
		l.pkgs[importPath] = pkg
		return pkg, nil
	}
	pkg, err := l.check(importPath, src, std)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", importPath, err)
	}
	l.pkgs[importPath] = pkg
	return pkg, nil
}

// dirOf is the directory of the source of the package at importPath, imported
// from the package in dir, and whether it's the standard library's: one of
// the module's, from its root; or one of the standard library's, or one it
// vendors, imported from it. It reports false of any other.
func (l *sourceLoader) dirOf(importPath, dir string) (string, bool, bool) {
	if rel, ok := strings.CutPrefix(importPath, l.module); ok && (rel == "" || rel[0] == '/') {
		return filepath.Join(l.root, filepath.FromSlash(rel)), false, true
	}
	goSrc := filepath.Join(l.ctx.GOROOT, "src")
	var candidates []string
	if within(dir, goSrc) {
		candidates = append(candidates, filepath.Join(goSrc, "vendor", filepath.FromSlash(importPath)))
	}
	if first, _, _ := strings.Cut(importPath, "/"); !strings.Contains(first, ".") {
		candidates = append(candidates, filepath.Join(goSrc, filepath.FromSlash(importPath)))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, true, true
		}
	}
	return "", false, false
}

// within reports whether dir is base or one of its subdirectories.
func within(dir, base string) bool {
	rel, err := filepath.Rel(base, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stubName is the name a stubbed package goes by: the last part of its import
// path, but for a major version, as "v2", the part before it.
func stubName(importPath string) string {
	parts := strings.Split(importPath, "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && majorVersion.MatchString(name) {
		name = parts[len(parts)-2]
	}
	return strings.ReplaceAll(name, "-", "")
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// check type-checks the package at importPath, its source in dir, as built
// for this platform: of a directory read whole, its function bodies too,
// keeping what each of its files' expressions is; of the rest, its
// declarations alone. A package of the module type-checks past any error,
// as what it uses of a stubbed package is unknown; the standard library's
// type-checks clean, or the guard reads it wrong.
func (l *sourceLoader) check(importPath, dir string, std bool) (*types.Package, error) {
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(l.root, dir)
	rel = filepath.ToSlash(rel)
	whole := !std && slices.Contains(l.read, rel)
	files := make([]*ast.File, len(bp.GoFiles))
	for i, name := range bp.GoFiles {
		if files[i], err = parser.ParseFile(l.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution); err != nil {
			return nil, err
		}
	}
	var typeErrs []error
	conf := types.Config{
		Importer:         l,
		IgnoreFuncBodies: !whole,
		Error:            func(err error) { typeErrs = append(typeErrs, err) },
	}
	info := &types.Info{Selections: make(map[*ast.SelectorExpr]*types.Selection), Uses: make(map[*ast.Ident]types.Object), Defs: make(map[*ast.Ident]types.Object)}
	pkg, _ := conf.Check(importPath, l.fset, files, info)
	if std && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	if whole {
		for i, name := range bp.GoFiles {
			l.checked[rel] = append(l.checked[rel], checkedFile{path: path.Join(rel, name), fset: l.fset, ast: files[i], info: info})
		}
	}
	return pkg, nil
}

// modulePath is the path of the module at root, as its go.mod declares it.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(p), `"`)
		}
	}
	t.Fatal("go.mod declares no module")
	return ""
}

// moduleRoot is the module's root: the directory above this package's, where
// go test runs its tests, that holds go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod in any directory above the package's")
		}
		dir = parent
	}
}

// writeModule writes files, by path from the root, into a new directory, and
// returns it.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func reportEach(t *testing.T, problems []string) {
	t.Helper()
	for _, problem := range problems {
		t.Error(problem)
	}
}
