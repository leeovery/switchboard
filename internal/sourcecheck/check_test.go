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
		Names:   sourcecheck.NamesIn("example.com/fixture"),
		Fields:  []string{"data.Record", "data.Meta"},
		Read:    func(field string) string { return field + " reads raw fields; " + readFix },
		Used:    map[string]string{"data.Banned": "data.Banned is absorbed; " + usedFix},
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
	got := sourcecheck.Check(t, root, "example.com/fixture", fixtureRule("data.Record.Size", "data.Keyed"), []string{"internal/reader"}, fixtureAllowance(
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
// one embedding another and one wrapping them, a function absorbed, and
// functions and methods taking a guarded type; and a reader package with a
// file of breaches, a file and functions allowed to break the rule, a file
// and a function allowed that keep it, and a test file, which a check
// passes over. The reader imports a module the fixture doesn't have, as the
// source loader stubs it.
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

func (r Record) Size() int                  { return len(r.ID) }
func (r *Record) Tag() string               { return r.ID }
func Count(records []*Record) int           { return len(records) }
func Keyed(byID map[string]Record) int      { return len(byID) }
func Banned() string                        { return "" }
func Named(o Other) string                  { return o.Name }
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
	_ = banned() + strings.ToUpper(o.Name) + data.Named(o)
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
