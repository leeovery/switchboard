package views_test

import (
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so the code that draws a view, and
// the commands that print one, can't tell a view read through a day's digest
// from one read raw without failing here: they name none of the digests'
// types, functions or constants, nor read a field of one, but where the CLI
// wires its readers to the digests the views read, and the router to the
// views' Digests. The views read sessions alike either way; a drawing
// package that could tell might read them otherwise.

// digestsWired are the functions of drawingPackages allowed to name a
// digest's, by path from the module's root, a colon and the function's
// name, a method's its receiver's type's then its own: the CLI's readers,
// whose Follower rests sessions on the states of the version the views
// build; and the router it serves, which digests each day with the views'
// Digests. The list only shrinks: an entry must still be there, and still
// name one.
var digestsWired = []string{
	"internal/cli/readers.go:app.newStateReaders",
	"internal/cli/serve.go:app.startingFrom",
}

// digestsOwners are the packages, by name under the module's internal
// directory, whose digests' names a drawing package names none of: each
// they declare whose name begins Digest, a method's among them.
var digestsOwners = []string{"ledger", "views"}

// sessionsAsOf are the ledger's names, beside those that begin Digest, of a
// session as of a day, as a digest's state it rests on, the chain of parts
// behind that, and the lines after it give it, and of what it's asked to
// rest on. Each is one the ledger declares, or the guard fails, as a ban on
// a name that's gone passes whatever names it.
var sessionsAsOf = []string{
	"ledger.SessionBase", "ledger.SessionFrom", "ledger.SessionPart", "ledger.SessionsWhole",
	"ledger.Rest", "ledger.RestOnPage", "ledger.RestOnState", "ledger.RestToFold", "ledger.RestWhole",
}

// The fix each kind of breach gives.
const (
	digestFix       = "read the view, which reads alike through a digest or raw"
	wiredFix        = "remove it from digestsWired in internal/views/drawingdigests_test.go"
	sessionsAsOfFix = "remove it from sessionsAsOf in internal/views/drawingdigests_test.go"
)

func TestDrawingCantTellADigestFromRaw(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, digestBreaches(t, root, sourcecheck.ModulePath(t, root), drawingPackages, digestsWired, sessionsAsOf))
}

func TestTheDigestGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, digestsFixture)
	got := digestBreaches(t, root, "example.com/fixture", []string{"internal/cli", "internal/dashboard"}, []string{
		"internal/cli/readers.go:wired",
		"internal/cli/readers.go:kept",
		"internal/cli/readers.go:gone",
	}, []string{"ledger.SessionFrom", "ledger.SessionBase", "ledger.SessionGone"})
	want := []string{
		"internal/cli/named.go:9: ledger.SessionFrom is a digest's; " + digestFix,
		"internal/cli/named.go:12: ledger.DigestHead is a digest's; " + digestFix,
		"internal/cli/named.go:14: views.DigestVersion is a digest's; " + digestFix,
		"internal/cli/named.go:17: ledger.Ledger.DigestEnded is a digest's; " + digestFix,
		"internal/cli/named.go:19: ledger.SessionFrom.Base reads a digest's fields; " + digestFix,
		"internal/cli/named.go:19: ledger.SessionBase.Date reads a digest's fields; " + digestFix,
		"internal/dashboard/dashboard.go:8: views.Digests is a digest's; " + digestFix,
		"internal/dashboard/dashboard.go:10: views.DigestEvery is a digest's; " + digestFix,
		"internal/dashboard/dashboard.go:12: ledger.DigestHead is a digest's; " + digestFix,
		"internal/dashboard/dashboard.go:12: ledger.SessionBase is a digest's; " + digestFix,
		"internal/dashboard/dashboard.go:12: ledger.DigestHead.Day reads a digest's fields; " + digestFix,
		"internal/dashboard/dashboard.go:12: ledger.SessionBase.Date reads a digest's fields; " + digestFix,
		"internal/cli/readers.go:kept: digestsWired allows it, but it keeps the rule; " + wiredFix,
		"internal/cli/readers.go:gone: digestsWired allows it, but it's gone; " + wiredFix,
		"ledger.SessionGone: sessionsAsOf bans it, but ledger declares no such name; " + sessionsAsOfFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// digestsFixture is a module of the ledger, with a digest's types, its
// method that digests the days ended, a session as of a day, and a Follower
// giving sessions as of one without naming it; the views, with the
// version of their states, a variable and their Digests; a kit, no drawing
// package, with an alias of a digest's head and a type defined as a session
// as of a day; and two drawing packages: the CLI, with a file naming each
// and reading a session's fields as of a day it never names, a function
// allowed to name one, one allowed that names none, and a test file, which
// the guard passes over; and the dashboard, with the views' Digests and
// variable, and the kit's types, reading a field of each.
var digestsFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/ledger/ledger.go": `package ledger

import "time"

type DigestHead struct{ Day string }

type SessionBase struct{ Date string }

type SessionFrom struct{ Base *SessionBase }

type Ledger struct{}

type Follower struct{}

func (l *Ledger) DigestEnded(now time.Time) {}

func (f *Follower) Sessions(ids []string) map[string]SessionFrom { return nil }
`,
	"internal/views/views.go": `package views

const DigestVersion = 1

var DigestEvery = 2

type Digests struct{}
`,
	"internal/cli/named.go": `package cli

import (
	"example.com/fixture/internal/ledger"
	"example.com/fixture/internal/views"
)

type held struct {
	from ledger.SessionFrom
}

var head ledger.DigestHead

const version = views.DigestVersion

func named(l *ledger.Ledger, f *ledger.Follower) string {
	digest := l.DigestEnded
	_ = digest
	return f.Sessions(nil)["s"].Base.Date
}
`,
	"internal/cli/readers.go": `package cli

import "example.com/fixture/internal/views"

func wired() int { return views.DigestVersion }

func kept() int { return 0 }
`,
	"internal/cli/cli_test.go": `package cli

import "example.com/fixture/internal/ledger"

var tested ledger.SessionFrom
`,
	"internal/kit/kit.go": `package kit

import "example.com/fixture/internal/ledger"

type Head = ledger.DigestHead

type Base ledger.SessionBase
`,
	"internal/dashboard/dashboard.go": `package dashboard

import (
	"example.com/fixture/internal/kit"
	"example.com/fixture/internal/views"
)

var digests = views.Digests{}

var every = views.DigestEvery

func routes(h kit.Head, b kit.Base) string { return h.Day + b.Date }
`,
}

// digestBreaches are where the non-test files of the packages in dirs, of
// the module at root with the given path, name a digest's, but in the
// functions allowed: each type, function or constant of digestsOwners whose
// name begins Digest, or of asOf, named; and each field read of a struct
// among them, named or not. Then each function allowed that's gone, or
// names none, and each of asOf the ledger doesn't declare.
func digestBreaches(t *testing.T, root, module string, dirs, allowed, asOf []string) []string {
	t.Helper()
	loader := sourcecheck.NewLoader(t, root, module, nil)
	owners := make(map[string]*types.Package, len(digestsOwners))
	for _, owner := range digestsOwners {
		pkg, err := loader.Import(module + "/internal/" + owner)
		if err != nil {
			t.Fatal(err)
		}
		owners[owner] = pkg
	}
	var names, gone []string
	for _, name := range asOf {
		if lookup(owners["ledger"], name) == nil {
			gone = append(gone, name+": sessionsAsOf bans it, but ledger declares no such name; "+sessionsAsOfFix)
			continue
		}
		names = append(names, name)
	}
	for _, owner := range digestsOwners {
		names = append(names, digestsOf(owners[owner])...)
	}
	problems := sourcecheck.Check(t, root, module, digestRule(module, names, structsOf(owners, names)), dirs, sourcecheck.Allowance{
		Entries: allowed, List: "digestsWired", Fix: wiredFix,
	})
	return append(problems, gone...)
}

// digestsOf are the names, as Names names them, of what pkg declares that
// begins Digest: its types, functions, constants and variables, exported,
// and the methods of its exported types.
func digestsOf(pkg *types.Package) []string {
	var names []string
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		if strings.HasPrefix(name, "Digest") {
			names = append(names, pkg.Name()+"."+name)
		}
		if typ, ok := obj.(*types.TypeName); ok {
			names = append(names, methodsOf(pkg, typ)...)
		}
	}
	return names
}

// methodsOf are the names, as Names names them, of the exported methods of
// the type typ names, of pkg, that begin Digest.
func methodsOf(pkg *types.Package, typ *types.TypeName) []string {
	named, ok := typ.Type().(*types.Named)
	if !ok {
		return nil
	}
	var names []string
	for method := range named.Methods() {
		if method.Exported() && strings.HasPrefix(method.Name(), "Digest") {
			names = append(names, pkg.Name()+"."+typ.Name()+"."+method.Name())
		}
	}
	return names
}

// structsOf are those of names, as Names names them, each declared by one
// of owners, by its name, that name a struct type.
func structsOf(owners map[string]*types.Package, names []string) []string {
	var structs []string
	for _, name := range names {
		pkg, _, _ := strings.Cut(name, ".")
		if typ, ok := lookup(owners[pkg], name).(*types.TypeName); ok {
			if _, isStruct := typ.Type().Underlying().(*types.Struct); isStruct {
				structs = append(structs, name)
			}
		}
	}
	return structs
}

// digestRule is the rule that the drawing packages, of the module with the
// given path, name none of names, a digest's, nor read a field of structs,
// those of them that are structs.
func digestRule(module string, names, structs []string) sourcecheck.Rule {
	used := make(map[string]sourcecheck.Ban, len(names))
	for _, name := range names {
		used[name] = sourcecheck.Ban{Says: name + " is a digest's; " + digestFix}
	}
	return sourcecheck.Rule{
		Names:  sourcecheck.NamesIn(module),
		Fields: structs,
		Read:   func(field string) string { return field + " reads a digest's fields; " + digestFix },
		Used:   used,
	}
}
