package sourcecheck_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// lostFix is how a guard says to give it what the loader lost.
const lostFix = "give the guard a type it can see, as a variable declared with one"

// lost says how what, whose type the loader lost as of says, breaks a rule
// that can't judge it.
func lost(what, of string) string {
	return what + " can't be judged, as " + of + " is lost to a package the loader stubs; " + lostFix
}

// lostRule is the fixture's rule, its calls banned too.
func lostRule() sourcecheck.Rule {
	r := fixtureRule()
	r.Calls = callsRule().Calls
	return r
}

func TestACheckFailsWhatItCantJudge(t *testing.T) {
	root := sourcecheck.WriteModule(t, lostFixture)
	got := sourcecheck.Check(t, root, "example.com/fixture", lostRule(), []string{"internal/drawn"}, fixtureAllowance())
	want := []string{
		"internal/drawn/lost.go:24: " + lost("held.record.ID", "the type of held.record"),
		"internal/drawn/lost.go:25: " + lost("held.record.Note", "the type of held.record"),
		"internal/drawn/lost.go:26: " + lost("held.record.Size", "the type of held.record"),
		"internal/drawn/lost.go:27: " + lost("held.store.Put", "the type of held.store"),
		"internal/drawn/lost.go:28: " + lost("held.record.Banned", "the type of held.record"),
		"internal/drawn/lost.go:29: " + lost("tick.at.Format", "the type of tick.at"),
		"internal/drawn/lost.go:30: " + lost("data.Count", "the type of data"),
		"internal/drawn/lost.go:31: " + lost("fmt.Sprint", "the type of its argument style.Width()"),
		"internal/drawn/lost.go:32: " + lost("(time.Time).Format", "the type of its argument tea.Layout"),
		"internal/drawn/lost.go:33: " + lost("Cut", "its type"),
		"internal/drawn/lost.go:34: " + worded("fmt.Sprint"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A ban on a method that judges only what it sees passes over the method's
// name selected from what the loader lost; one that doesn't fails it.
func TestABanOnWhatItSeesPassesOverWhatsLost(t *testing.T) {
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod": "module example.com/fixture\n",
		"internal/drawn/drawn.go": `package drawn

import (
	"time"

	"example.org/tea"
)

func said(key tea.KeyMsg, took time.Duration) []string {
	return []string{key.String(), took.String()}
}
`,
	})
	tests := []struct {
		name string
		seen bool
		want []string
	}{
		{name: "judging what it sees", seen: true, want: []string{"internal/drawn/drawn.go:10: banned"}},
		{name: "failing what's lost", want: []string{
			"internal/drawn/drawn.go:10: " + lost("key.String", "the type of key"),
			"internal/drawn/drawn.go:10: banned",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := sourcecheck.Rule{Names: sourcecheck.NamesIn("example.com/fixture"), Calls: map[string]sourcecheck.CallBan{
				"(time.Duration).String": {Ban: sourcecheck.Ban{Says: "banned"}, Seen: tt.seen},
			}}
			got := sourcecheck.Check(t, root, "example.com/fixture", rule, []string{"internal/drawn"}, fixtureAllowance())
			if !slices.Equal(got, tt.want) {
				t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// lostFixture is a module whose data package has the types a rule guards,
// one with a field no other package reads, a function it bans, and a
// function, a method and an interface's method taking a guarded type; and a
// drawn package, which takes a stubbed package's values, so the type checker
// loses the type of what's worked out of them. It reads, through what it
// lost, a guarded type's field, one embedded, the method and the
// interface's taking it, a function banned, and a time's layout; it calls
// the function taking a guarded type in a key of a literal of a stubbed
// type, which the type checker passes over; it gives fmt, and a time's
// layout, a value whose type it lost, and fmt such a value beside a float;
// and it calls a function banned through a stubbed package it dot-imports.
// Beside these, it selects from what it lost what no rule bans, and the
// field no other package reads, keys a literal of a stubbed type by name,
// gives a time's layout what it lost where the layout isn't, and selects
// what's banned in another package alone, all of which keep the rule.
var lostFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/data/data.go": `package data

type Meta struct{ Note string }

type Record struct {
	ID     string
	secret string
	Meta
}

type Store interface{ Put(r Record) }

func (r Record) Size() int      { return len(r.ID) }
func Count(records []Record) int { return len(records) }
func Banned() string            { return "" }
func Scoped() string            { return "" }
`,
	"internal/drawn/lost.go": `package drawn

import (
	"fmt"
	"time"

	. "example.org/cells"
	"example.org/tea"

	"example.com/fixture/internal/data"
)

type heldMsg struct {
	record data.Record
	store  data.Store
}

type tickMsg struct{ at time.Time }

func lost(msg tea.Msg, style tea.Style) []any {
	held, _ := msg.(heldMsg)
	tick := msg.(tickMsg)
	return []any{
		held.record.ID,
		held.record.Note,
		held.record.Size(),
		held.store.Put,
		held.record.Banned,
		tick.at.Format("15:04"),
		tea.Keyed{data.Count(nil): "drawn"},
		fmt.Sprint(style.Width()),
		time.Now().Format(tea.Layout),
		Cut("text", 4),
		fmt.Sprint(style.Width(), 1.5),
		style.Render(held.record),
		held.record.secret,
		tea.Command{Use: "drawn"},
		time.Now().AppendFormat(style.Bytes(), time.RFC3339),
		held.record.Scoped,
		fmt.Sprint("drawn"),
	}
}
`,
}
