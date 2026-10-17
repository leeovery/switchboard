package ledger_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so nothing reads a line's JSON but
// what writes lines as the ledger holds them, without failing here: the
// follower gives today's lines without it, as it holds them, so what reads
// one of theirs would read nothing, and what reads one of a Reader's is the
// verb that passes lines through.

// jsonAllowed are the functions allowed to read a line's JSON, by path from
// the module's root, a colon and the function's name, a method's its
// receiver's type's then its own: each reads lines a Reader gives. The list
// only shrinks: an entry must still be there, and still read it.
var jsonAllowed = []string{
	// The requests verb's JSON, which passes lines through as the ledger
	// filed them.
	"internal/cli/requests.go:writeLines",
}

// heldJSONField is the field a line's JSON is held in, as sourcecheck names it.
const heldJSONField = "ledger.Held.JSON"

// The fix each kind of breach gives.
const (
	jsonSays       = heldJSONField + " is the JSON a Reader's line was written as, which the follower's lines don't hold; read what the line reads as, or name the function in jsonAllowed in internal/ledger/guard_test.go, saying why it writes lines as the ledger holds them"
	jsonAllowedFix = "remove it from jsonAllowed in internal/ledger/guard_test.go"
)

func TestALinesJSONIsReadByTheVerbThatWritesItAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, jsonBreaches(t, root, sourcecheck.ModulePath(t, root), sourcecheck.Packages(t, root), jsonAllowed))
}

func TestTheJSONGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, jsonFixture)
	got := jsonBreaches(t, root, "example.com/fixture", sourcecheck.Packages(t, root), []string{
		"internal/cli/requests.go:writeLines",
		"internal/cli/requests.go:kept",
		"internal/cli/requests.go:gone",
	})
	want := []string{
		"internal/ledger/ledger.go:12: " + jsonSays,
		"internal/views/views.go:7: " + jsonSays,
		"internal/views/views.go:15: " + jsonSays,
		"internal/cli/requests.go:kept: jsonAllowed allows it, but it keeps the rule; " + jsonAllowedFix,
		"internal/cli/requests.go:gone: jsonAllowed allows it, but it's gone; " + jsonAllowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// jsonBreaches are where the non-test files of the packages in dirs, of the
// module at root with the given path, read a line's JSON, through any field
// it's embedded in, but in the functions allowed. Then each function allowed
// that's gone, or reads none.
func jsonBreaches(t *testing.T, root, module string, dirs, allowed []string) []string {
	t.Helper()
	rule := sourcecheck.Rule{
		Names: sourcecheck.NamesIn(module),
		Used:  map[string]sourcecheck.Ban{heldJSONField: {Says: jsonSays}},
	}
	return sourcecheck.Check(t, root, module, rule, dirs, sourcecheck.Allowance{Entries: allowed, List: "jsonAllowed", Fix: jsonAllowedFix})
}

// jsonFixture is a module of the ledger, which holds a line's JSON, setting
// it, which keeps the rule, and reading it, which doesn't; views, which reads
// it of a line and through a type it's embedded in; and the CLI, with the
// function allowed, one allowed that reads none, and a test file reading it.
var jsonFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/ledger/ledger.go": `package ledger

type Line struct{ Session string }

type Held struct {
	Line
	JSON []byte
}

func heldIn(data []byte) Held { return Held{JSON: data} }

func sized(h Held) int { return len(h.JSON) }
`,
	"internal/views/views.go": `package views

import "example.com/fixture/internal/ledger"

func read(lines []ledger.Held) (n int) {
	for _, h := range lines {
		n += len(h.JSON)
	}
	return n
}

type told struct{ ledger.Held }

func (t told) raw() []byte {
	return t.JSON
}

func session(h ledger.Held) string { return h.Session }
`,
	"internal/cli/requests.go": `package cli

import "example.com/fixture/internal/ledger"

func writeLines(lines []ledger.Held) (out []byte) {
	for _, h := range lines {
		out = append(out, h.JSON...)
	}
	return out
}

func kept(h ledger.Held) string { return h.Session }
`,
	"internal/cli/requests_test.go": `package cli

import "example.com/fixture/internal/ledger"

func tested(h ledger.Held) []byte { return h.JSON }
`,
}
