package status

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// The forms of the reasons and the whys in which a placeholder stands for
// what varies, as the reference writes them.
const (
	placedID     = "<id>"
	placedWhy    = "<why>"
	placedIdle   = "<idle>"
	placedShare  = "<n>%"
	placedReason = "<reason>"
)

// reasonForms are every reason the router gives, a reason with a tail in the
// form the reference writes it.
func reasonForms() []string {
	return []string{
		ReasonNew, ReasonPinned, ReasonPoolPin, ReasonGlobalPin, ReasonMovedByPin, ReasonSticky, ReasonBound,
		ReasonUnsessioned, ReasonNoRoom, ReasonClient, ReasonBack, ReasonTokenLeftPool,
		ReasonMovedOff + placedID + " " + placedWhy,
		ReasonPinYields + placedID + " " + placedWhy,
		ReasonRescored + placedIdle + rescoredIdle,
		ReasonCompacted + placedID + compactedResets + placedShare + compactedLeft,
		PassingOver(placedReason, placedID),
	}
}

// whys are every why an account can't take a request.
var whys = []string{WhyLimit, WhyRefused, WhyNewToken, WhyLeftPool, WhyReserve, WhyNoRoom}

// The reasons and whys are stored for years, in the ledger, the events,
// state.json and /sessions, and read back, so the reference lists every one
// as fixed: a reason reworded in the code, or in the reference, fails here.
func TestTheReasonsAreTheOnesTheReferenceFixes(t *testing.T) {
	reference := readReference(t)
	tests := []struct {
		heading string
		want    []string
	}{
		{heading: "### The reasons", want: reasonForms()},
		{heading: "### The whys", want: whys},
	}
	for _, tt := range tests {
		t.Run(tt.heading, func(t *testing.T) {
			listed := firstCells(t, reference, tt.heading)
			if !sameSet(listed, tt.want) {
				t.Errorf("docs/reference.md's %q lists\n%q\nwant the code's\n%q", tt.heading, listed, tt.want)
			}
		})
	}
}

// The forms <idle> takes in the reference are the ones idled gives.
func TestTheIdleSpansTheReferenceGivesAreTheOnesIdledFiles(t *testing.T) {
	reference := readReference(t)
	rescored := ReasonRescored + placedIdle + rescoredIdle
	for _, span := range []string{idled(132 * time.Hour), idled(144 * time.Hour), idled(4*time.Hour + 57*time.Minute), idled(7 * time.Minute), idled(0)} {
		if !strings.Contains(rowOf(reference, rescored), "`"+span+"`") {
			t.Errorf("docs/reference.md's row of %q doesn't give %q as an <idle>", rescored, span)
		}
	}
}

// Every reason and why status declares, read from its source, is one the
// reference fixes, so one added without the reference listing it fails here:
// a whole reason, or the start of one with a tail, as "moved: " starts
// "moved: <id> <why>"; and a why, whole.
func TestEveryReasonAndWhyStatusDeclaresIsOneTheReferenceFixes(t *testing.T) {
	reference := readReference(t)
	reasons, whysListed := firstCells(t, reference, "### The reasons"), firstCells(t, reference, "### The whys")
	l, _ := reasonsLoader(t)
	for name, value := range declaredFrozen(t, l) {
		switch {
		case strings.HasPrefix(name, whyPrefix) && !slices.Contains(whysListed, value):
			t.Errorf("status declares %s, %q, which docs/reference.md's whys don't list", name, value)
		case strings.HasPrefix(name, reasonPrefix) && !slices.ContainsFunc(reasons, startedBy(value)):
			t.Errorf("status declares %s, %q, which docs/reference.md's reasons don't list", name, value)
		}
	}
}

// The router gives a reason or a why through status's constants, never in
// words of its own, which would escape the reference and its guard.
func TestTheRouterGivesNoReasonNorWhyButStatussOwn(t *testing.T) {
	l, module := reasonsLoader(t)
	frozen := slices.Collect(maps.Values(declaredFrozen(t, l)))
	rule := sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Node: func(n ast.Node, _ *types.Info) string {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return ""
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil || !slices.ContainsFunc(frozen, opens(text)) {
			return ""
		}
		return fmt.Sprintf("%q is a reason or a why of status's own: give it through its constant", text)
	}}
	sourcecheck.ReportEach(t, l.Check(rule, []string{routerDir}, sourcecheck.Allowance{
		Entries: routerPhrasesAllowed, List: "routerPhrasesAllowed",
		Fix: "remove it from the list, as the router's words no longer say any reason or why",
	}))
}

// routerPhrasesAllowed are the router's functions whose words are a reason's
// without being one: the log's "pinned".
var routerPhrasesAllowed = []string{"internal/router/control.go:Router.pinTo"}

const (
	statusDir    = "internal/status"
	routerDir    = "internal/router"
	reasonPrefix = "Reason"
	whyPrefix    = "Why"
)

// reasonsLoader loads status and the router from their source, the module
// with the path given.
func reasonsLoader(t *testing.T) (l *sourcecheck.Loader, module string) {
	t.Helper()
	root := sourcecheck.ModuleRoot(t)
	module = sourcecheck.ModulePath(t, root)
	return sourcecheck.NewLoader(t, root, module, []string{statusDir, routerDir}), module
}

// declaredFrozen are the string constants status declares whose names start
// Reason or Why, by name, as l reads them from its source.
func declaredFrozen(t *testing.T, l *sourcecheck.Loader) map[string]string {
	t.Helper()
	scope := l.Load(statusDir)[0].Pkg.Scope()
	frozen := make(map[string]string)
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if ok && (strings.HasPrefix(name, reasonPrefix) || strings.HasPrefix(name, whyPrefix)) && c.Val().Kind() == constant.String {
			frozen[name] = constant.StringVal(c.Val())
		}
	}
	return frozen
}

// startedBy reports of a reason's form whether it's the reason given, or,
// where that opens a reason with a tail, ending ": " or " ", starts with it.
func startedBy(reason string) func(form string) bool {
	opener := strings.HasSuffix(reason, " ")
	return func(form string) bool {
		return form == reason || opener && strings.HasPrefix(form, reason)
	}
}

// opens reports of a reason or a why whether text says it: is it, or, of one
// that opens a reason with a tail, starts with it.
func opens(text string) func(frozen string) bool {
	return func(frozen string) bool {
		return text == frozen || strings.HasSuffix(frozen, " ") && strings.HasPrefix(text, frozen)
	}
}

// readReference reads docs/reference.md, from the module's root.
func readReference(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sourcecheck.ModuleRoot(t), "docs", "reference.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// firstCells are the code in the first cell of each row of the table under
// the heading given in doc, up to the next heading.
func firstCells(t *testing.T, doc, heading string) []string {
	t.Helper()
	_, under, ok := strings.Cut(doc, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("docs/reference.md has no heading %q", heading)
	}
	var cells []string
	for line := range strings.Lines(under) {
		if strings.HasPrefix(line, "#") {
			break
		}
		if code, ok := strings.CutPrefix(line, "| `"); ok {
			cell, _, _ := strings.Cut(code, "` |")
			cells = append(cells, cell)
		}
	}
	return cells
}

// rowOf is the line of the table row in doc whose first cell is the code
// given: "" where there's none.
func rowOf(doc, code string) string {
	for line := range strings.Lines(doc) {
		if strings.HasPrefix(line, "| `"+code+"` |") {
			return line
		}
	}
	return ""
}

// sameSet reports whether a and b hold the same strings, in any order.
func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
