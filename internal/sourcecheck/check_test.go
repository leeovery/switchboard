package sourcecheck_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// The fix each kind of breach gives in the fixture's rule.
const (
	readFix  = "read the view's fact"
	usedFix  = "say it as its owner does"
	takesFix = "read the view's fact, or allow the call"
	listFix  = "remove it from allowed"
	callFix  = "remove it from calls"
)

func fixtureRule(calls ...string) sourcecheck.Rule {
	return sourcecheck.Rule{
		Names:  sourcecheck.NamesIn("example.com/fixture"),
		Fields: []string{"data.Record", "data.Meta"},
		Read:   func(field string) string { return field + " reads raw fields; " + readFix },
		Used: map[string]sourcecheck.Ban{
			"data.Banned":     {Says: "data.Banned is absorbed; " + usedFix},
			"data.Scoped":     {Says: "data.Scoped is absorbed in other; " + usedFix, In: []string{"internal/other"}},
			"data.Other.Name": {Says: "data.Other.Name is absorbed in other; " + usedFix, In: []string{"internal/other"}},
		},
		Owners:  []string{"data"},
		Takes:   func(fn, field string) string { return fn + " takes " + field + "; " + takesFix },
		Allowed: calls,
	}
}

func fixtureAllowance(entries ...string) sourcecheck.Allowance {
	return sourcecheck.Allowance{Entries: entries, List: "allowed", Fix: listFix, Calls: "calls", Callers: "reader", CallFix: callFix}
}

func TestACheckFindsEachBreachButWhereItsAllowed(t *testing.T) {
	root := sourcecheck.WriteModule(t, checkFixture)
	dirs := []string{"internal/reader", "internal/other"}
	got := sourcecheck.Check(t, root, "example.com/fixture", fixtureRule("data.Record.Size", "data.Keyed"), dirs, fixtureAllowance(
		"internal/reader/allowed.go",
		"internal/reader/kept.go",
		"internal/reader/gone.go",
		"internal/reader/fns.go:allowedFn",
		"internal/reader/fns.go:keptFn",
		"internal/reader/fns.go:goneFn",
		"internal/reader/fns.go:holder.allowedMethod",
	))
	want := []string{
		"internal/reader/breaches.go:14: data.Record.ID reads raw fields; " + readFix,
		"internal/reader/breaches.go:15: data.Record.Note reads raw fields; " + readFix,
		"internal/reader/breaches.go:16: data.Record.Meta reads raw fields; " + readFix,
		"internal/reader/breaches.go:16: data.Meta.Note reads raw fields; " + readFix,
		"internal/reader/breaches.go:17: data.Banned is absorbed; " + usedFix,
		"internal/reader/breaches.go:18: data.Count takes data.Record; " + takesFix,
		"internal/reader/breaches.go:19: data.Record.Tag takes data.Record; " + takesFix,
		"internal/reader/breaches.go:24: data.Record.ID reads raw fields; " + readFix,
		"internal/reader/fns.go:13: data.Record.ID reads raw fields; " + readFix,
		"internal/other/other.go:5: data.Scoped is absorbed in other; " + usedFix,
		"internal/other/other.go:7: data.Other.Name is absorbed in other; " + usedFix,
		"internal/other/other.go:9: data.Other.Name is absorbed in other; " + usedFix,
		"internal/reader/kept.go: allowed allows it, but it keeps the rule; " + listFix,
		"internal/reader/gone.go: allowed allows it, but it's gone; " + listFix,
		"internal/reader/fns.go:keptFn: allowed allows it, but it keeps the rule; " + listFix,
		"internal/reader/fns.go:goneFn: allowed allows it, but it's gone; " + listFix,
		"data.Keyed: calls allows it, but no reader calls it; " + callFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// checkFixture is a module whose data package has the types a rule guards,
// one embedding another and one wrapping them, a function absorbed, one
// absorbed in another package alone, as is a field of a type it doesn't
// guard, and functions and methods taking a guarded type; a reader package
// with a file of breaches, a file and functions allowed to break the rule, a
// file and a function allowed that keep it, and a test file, which a check
// passes over; and that other package, using what's absorbed in it, the
// field read directly and through a type embedding its own. The reader imports a module the
// fixture doesn't have, as the source loader stubs it.
var checkFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

type Meta struct{ Note string }

type Record struct {
	ID string
	Meta
}

type Wrapper struct{ Record }

type Other struct{ Name string }

type Embeds struct{ Other }

func (r Record) Size() int             { return len(r.ID) }
func (r *Record) Tag() string          { return r.ID }
func Count(records []*Record) int      { return len(records) }
func Keyed(byID map[string]Record) int { return len(byID) }
func Banned() string                   { return "" }
func Scoped() string                   { return "" }
func Named(o Other) string             { return o.Name }
`,
	"internal/other/other.go": `package other

import "example.com/fixture/internal/data"

func scoped() string { return data.Scoped() }

func named(o data.Other) string { return o.Name }

func through(e data.Embeds) string { return e.Name }
`,
	"internal/reader/breaches.go": `package reader

import (
	"strings"

	"example.org/elsewhere"

	"example.com/fixture/internal/data"
)

var _ = elsewhere.Anything

func breaches(r data.Record, w data.Wrapper, o data.Other) string {
	id := r.ID
	note := w.Note
	_ = r.Meta.Note
	banned := data.Banned
	_ = data.Count([]*data.Record{&r})
	_ = r.Tag()
	_ = banned() + strings.ToUpper(o.Name) + data.Named(o) + data.Scoped()
	return id + note + string(rune(r.Size()))
}

var first = data.Record{}.ID
`,
	"internal/reader/fns.go": `package reader

import "example.com/fixture/internal/data"

type holder struct{ r data.Record }

func allowedFn(r data.Record) int { return data.Keyed(map[string]data.Record{r.ID: r}) }

func keptFn() int { return 0 }

func (h holder) allowedMethod() string { return h.r.ID }

func unallowed(r data.Record) string { return r.ID }
`,
	"internal/reader/allowed.go": `package reader

import "example.com/fixture/internal/data"

func allowed(r data.Record) string { return r.ID + data.Banned() }
`,
	"internal/reader/kept.go": `package reader

func kept() {}
`,
	"internal/reader/reader_test.go": `package reader

import "example.com/fixture/internal/data"

func tested(r data.Record) string { return r.ID }
`,
}

func TestACheckFindsEachTypeAndConstantNamedThatItsRuleBans(t *testing.T) {
	root := sourcecheck.WriteModule(t, typesNamedFixture)
	r := sourcecheck.Rule{Names: sourcecheck.NamesIn("example.com/fixture"), Used: map[string]sourcecheck.Ban{
		"data.Banned": {Says: "data.Banned is banned in reader; " + usedFix, In: []string{"internal/reader"}},
		"data.Limit":  {Says: "data.Limit is banned; " + usedFix},
	}}
	got := sourcecheck.Check(t, root, "example.com/fixture", r, []string{"internal/reader", "internal/other"}, fixtureAllowance())
	want := []string{
		"internal/reader/named.go:5: data.Banned is banned in reader; " + usedFix,
		"internal/reader/named.go:7: data.Banned is banned in reader; " + usedFix,
		"internal/reader/named.go:9: data.Limit is banned; " + usedFix,
		"internal/reader/named.go:11: data.Banned is banned in reader; " + usedFix,
		"internal/reader/named.go:12: data.Banned is banned in reader; " + usedFix,
		"internal/reader/named.go:16: data.Limit is banned; " + usedFix,
		"internal/other/other.go:7: data.Limit is banned; " + usedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// typesNamedFixture is a module whose data package declares a type and a
// constant a rule bans, beside a type, a constant and a function it
// doesn't; a reader package naming the banned type embedded, aliased, in a
// map's type and a variable's, and the constant, beside what isn't banned,
// a value of the banned type among it, made without naming it, and a test
// file naming them, which a check passes over; and another package, where
// the type is allowed and the constant isn't, naming both.
var typesNamedFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

type Banned struct{ ID string }

type Kept struct{ ID string }

const Limit, Free = 3, 4

func Make() Banned { return Banned{} }
`,
	"internal/reader/named.go": `package reader

import "example.com/fixture/internal/data"

type holds struct{ data.Banned }

type alias = data.Banned

var limit = data.Limit

func named(m map[string]data.Banned) int {
	var b data.Banned
	_ = data.Kept{ID: b.ID}
	made := data.Make()
	const Limit = 5
	return len(m) + data.Free + len(made.ID) + data.Limit + Limit
}
`,
	"internal/reader/reader_test.go": `package reader

import "example.com/fixture/internal/data"

var tested = data.Banned{ID: string(rune(data.Limit))}
`,
	"internal/other/other.go": `package other

import "example.com/fixture/internal/data"

var scoped data.Banned

var limited = data.Limit
`,
}

func TestACheckFindsABannedTypeByEachRouteToIt(t *testing.T) {
	root := sourcecheck.WriteModule(t, routesFixture)
	r := sourcecheck.Rule{
		Names:  sourcecheck.NamesIn("example.com/fixture"),
		Fields: []string{"data.Banned"},
		Read:   func(field string) string { return field + " reads raw fields; " + readFix },
		Used: map[string]sourcecheck.Ban{
			"data.Banned": {Says: "data.Banned is banned; " + usedFix},
			"data.Shared": {Says: "data.Shared is banned; " + usedFix},
		},
	}
	got := sourcecheck.Check(t, root, "example.com/fixture", r, []string{"internal/reader"}, fixtureAllowance())
	want := []string{
		"internal/reader/routes.go:8: data.Banned is banned; " + usedFix,
		"internal/reader/routes.go:8: data.Banned is banned; " + usedFix,
		"internal/reader/routes.go:9: data.Banned is banned; " + usedFix,
		"internal/reader/routes.go:10: data.Banned.ID reads raw fields; " + readFix,
		"internal/reader/routes.go:10: data.Banned.ID reads raw fields; " + readFix,
		"internal/reader/routes.go:10: data.Banned.ID reads raw fields; " + readFix,
		"internal/reader/routes.go:10: data.Shared is banned; " + usedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// routesFixture is a module whose data package declares a type and a
// variable a rule bans, beside a variable it doesn't; an elsewhere package,
// which no check reads, declaring an alias of the banned type, a type defined
// as it, another defined as that one, and a struct of its own alike but for
// its name; and a reader package naming each of elsewhere's, reading a field
// of each, and naming both variables.
var routesFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

type Banned struct{ ID string }

var Shared, Open = 7, 8
`,
	"internal/elsewhere/elsewhere.go": `package elsewhere

import "example.com/fixture/internal/data"

type Alias = data.Banned

type Over data.Banned

type Again Over

type Own struct{ ID string }
`,
	"internal/reader/routes.go": `package reader

import (
	"example.com/fixture/internal/data"
	"example.com/fixture/internal/elsewhere"
)

func routes(o elsewhere.Over, a elsewhere.Again, own elsewhere.Own) int {
	var al elsewhere.Alias
	return len(o.ID) + len(a.ID) + len(al.ID) + len(own.ID) + data.Shared + data.Open
}
`,
}

func TestACheckSeesRawDataTakenThroughFuncsChannelsAndTypeArguments(t *testing.T) {
	root := sourcecheck.WriteModule(t, shapesFixture)
	got := sourcecheck.Check(t, root, "example.com/fixture", fixtureRule(), []string{"internal/reader"}, fixtureAllowance())
	want := []string{
		"internal/reader/shapes.go:6: data.Each takes data.Record; " + takesFix,
		"internal/reader/shapes.go:7: data.Made takes data.Record; " + takesFix,
		"internal/reader/shapes.go:8: data.Fed takes data.Record; " + takesFix,
		"internal/reader/shapes.go:9: data.Seq takes data.Record; " + takesFix,
		"internal/reader/shapes.go:10: data.Seq2 takes data.Record; " + takesFix,
		"internal/reader/shapes.go:11: data.Boxed takes data.Record; " + takesFix,
		"internal/reader/shapes.go:12: data.Listed takes data.Record; " + takesFix,
		"internal/reader/shapes.go:13: data.Walked takes data.Record; " + takesFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// shapesFixture is a module whose data package has functions taking a
// guarded type through each shape that holds one: a func's parameter, and
// its result; a channel; a generic named type's type arguments, iter's
// among them, nested; and a recursive generic type. One takes recursive
// types holding none, and a reader calls each.
var shapesFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

import "iter"

type Record struct{ ID string }

type Box[T any] struct{ v T }

type List[T any] struct {
	next *List[T]
	v    T
}

type Visit func(Visit) bool

func Each(each func(Record))                  {}
func Made(made func() []Record)               {}
func Fed(records chan<- Record)               {}
func Seq(records iter.Seq[Record])            {}
func Seq2(records iter.Seq2[string, *Record]) {}
func Boxed(b Box[map[string]Record])          {}
func Listed(l *List[Record])                  {}
func Walked(walk func(Visit) Record)          {}
func Visited(v Visit, l *List[Box[string]])   {}
`,
	"internal/reader/shapes.go": `package reader

import "example.com/fixture/internal/data"

func shapes() {
	data.Each(nil)
	data.Made(nil)
	data.Fed(nil)
	data.Seq(nil)
	data.Seq2(nil)
	data.Boxed(data.Box[map[string]data.Record]{})
	data.Listed(nil)
	data.Walked(nil)
	data.Visited(nil, nil)
}
`,
}

// A check judges every file, whatever platform it's built for, each once:
// one built for this platform as it's built here, and one built for
// another alone as that platform builds it.
func TestACheckJudgesTheFilesBuiltForEveryPlatform(t *testing.T) {
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod":                              "module example.com/fixture\n",
		"internal/reader/here.go":             "package reader\n\nimport \"fmt\"\n\nfunc Here() string { return fmt.Sprintf(\"%v\", 1.5) }\n",
		"internal/reader/there_unix.go":       "//go:build unix\n\npackage reader\n\nimport \"fmt\"\n\nfunc There() string { return fmt.Sprintf(\"%v\", 2.5) }\n",
		"internal/reader/there_other.go":      "//go:build !unix\n\npackage reader\n\nimport \"fmt\"\n\nfunc There() string { return fmt.Sprintf(\"%v\", 2.5) }\n",
		"internal/reader/there_other_test.go": "//go:build !unix\n\npackage reader\n\nimport \"fmt\"\n\nfunc tested() string { return fmt.Sprintf(\"%v\", 3.5) }\n",
	})
	rule := sourcecheck.Rule{Names: sourcecheck.NamesIn("example.com/fixture"), Calls: map[string]sourcecheck.CallBan{"fmt.Sprintf": {Ban: sourcecheck.Ban{Says: "banned"}}}}
	got := sourcecheck.Check(t, root, "example.com/fixture", rule, []string{"internal/reader"}, fixtureAllowance())
	slices.Sort(got)
	want := []string{
		"internal/reader/here.go:5: banned",
		"internal/reader/there_other.go:7: banned",
		"internal/reader/there_unix.go:7: banned",
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestACheckSeesRawDataThroughNamedTypesOfThePackagesThatDeclareIt(t *testing.T) {
	root := sourcecheck.WriteModule(t, namedFixture)
	rule := fixtureRule()
	rule.Owners = append(rule.Owners, "store")
	got := sourcecheck.Check(t, root, "example.com/fixture", rule, []string{"internal/reader"}, fixtureAllowance())
	want := []string{
		"internal/reader/named.go:9: data.Listed takes data.Record; " + takesFix,
		"internal/reader/named.go:10: data.Indexed takes data.Record; " + takesFix,
		"internal/reader/named.go:11: data.Read takes data.Record; " + takesFix,
		"internal/reader/named.go:12: data.Accept takes data.Record; " + takesFix,
		"internal/reader/named.go:13: data.Batched takes data.Record; " + takesFix,
		"internal/reader/named.go:16: store.Kept takes data.Record; " + takesFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// namedFixture is a module whose data package declares a guarded type and
// named types holding it: a slice, a map of pointers, a func whose result is
// iter's sequence of it, a recursive func, and a type defined as another of
// them; and a recursive func holding none. Its functions each take one of
// them. store, an owner too, declares no guarded type, but a slice of one,
// which it's judged by its name, and a function taking a slice of one
// unnamed. A reader calls each.
var namedFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

import "iter"

type Record struct{ ID string }

type Records []Record

type Index map[string]*Record

type Reader func(from int) iter.Seq[Record]

type Visitor func(next Visitor, r Record) bool

type Batch Records

type Visit func(Visit) bool

func Listed(records Records) {}
func Indexed(index Index)    {}
func Read(read Reader)       {}
func Accept(visit Visitor)   {}
func Batched(batch *Batch)   {}
func Visited(visit Visit)    {}
`,
	"internal/store/store.go": `package store

import "example.com/fixture/internal/data"

type Rows []data.Record

func Saved(rows Rows)            {}
func Kept(records []data.Record) {}
`,
	"internal/reader/named.go": `package reader

import (
	"example.com/fixture/internal/data"
	"example.com/fixture/internal/store"
)

func named() {
	data.Listed(nil)
	data.Indexed(nil)
	data.Read(nil)
	data.Accept(nil)
	data.Batched(nil)
	data.Visited(nil)
	store.Saved(nil)
	store.Kept(nil)
}
`,
}

func TestADeclarationIsNamedByItsFunction(t *testing.T) {
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod": "module example.com/fixture\n",
		"internal/named/named.go": `package named

type plain struct{}

type generic[T any, U any] struct{}

type single[T any] struct{}

var declared = 1

func function() {}

func (plain) value()            {}
func (*plain) pointer()         {}
func (generic[T, U]) two()      {}
func (*single[T]) one()         {}
`,
	})
	files := sourcecheck.NewLoader(t, root, "example.com/fixture", []string{"internal/named"}).Load("internal/named")
	var got []string
	for _, f := range files {
		for _, decl := range f.AST.Decls {
			got = append(got, sourcecheck.DeclName(decl))
		}
	}
	want := []string{"", "", "", "", "function", "plain.value", "plain.pointer", "generic.two", "single.one"}
	if !slices.Equal(got, want) {
		t.Errorf("DeclName() named %q, want %q", got, want)
	}
}
