package views_test

import (
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so the files that draw Sessions
// can't build for themselves what the watch builds of Sessions once a look,
// nor the dashboard work out again who sits where on its Switchboard, without
// failing here.

// sessionsFiles are the dashboard's files that draw Sessions, by pattern from
// the module's root, as path.Match has it: each draws what the watch built of
// Sessions at the look, as SessionsShown holds it. A pattern no file matches
// fails, as a file renamed past it would pass whatever it called.
var sessionsFiles = []string{"internal/dashboard/sessions*.go", "internal/dashboard/switchboard*.go"}

// builtForSessions are what the watch builds of Sessions once a look: who
// runs where, the calls, who sits where and the stubs hanging there, and the
// List. The files that draw Sessions call none, though the rest of the
// dashboard may read who runs where. Each is one views declares, or the
// guard fails, as a ban on a name that's gone passes whatever they call.
var builtForSessions = []string{"views.CallsOf", "views.ListSessions", "views.Patch", "views.RunningOf", "views.StubsOf"}

// sessionsAbsorbed are what milestone 5's Sessions worked out in the
// dashboard that the patch keeps in the watch now, each with its owner: the
// order its calls kept from look to look, and the moves that re-patched them
// till a listing showed them done. The dashboard declares none of them again.
var sessionsAbsorbed = map[string]string{
	"dashboard.Order":              "views.Patch",
	"dashboard.Frame.OrderOf":      "views.Patch",
	"dashboard.Traffic.repatching": "views.Patch",
}

// The fix each kind of breach gives.
const (
	sessionsFilesFix    = "name the files that draw Sessions in sessionsFiles in internal/views/sessionsguard_test.go"
	builtForSessionsFix = "remove it from builtForSessions in internal/views/sessionsguard_test.go"
)

// builtForSessionsSays says how using what the watch builds of Sessions in a
// file that draws it breaks the rule.
func builtForSessionsSays(name string) string {
	return name + " is built in the watch once a look; draw what SessionsShown holds"
}

func TestSessionsDrawWhatTheWatchBuilt(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, sessionsBreaches(t, root, sourcecheck.ModulePath(t, root), sessionsFiles, builtForSessions, sessionsAbsorbed))
}

func TestTheSessionsGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, sessionsFixture)
	got := sessionsBreaches(t, root, "example.com/fixture",
		[]string{"internal/dashboard/sessions*.go", "internal/dashboard/switchboard*.go", "internal/dashboard/lanes*.go"},
		[]string{"views.RunningOf", "views.Patch", "views.ListSessions", "views.Gone", "views.Patched.Gone"},
		map[string]string{
			"dashboard.Order": "views.Patch", "dashboard.Frame.OrderOf": "views.Patch",
			"dashboard.Traffic.repatching": "views.Patch", "dashboard.Frame.kept": "views.Patch",
		})
	want := []string{
		"internal/dashboard/sessions.go:9: " + builtForSessionsSays("views.RunningOf"),
		"internal/dashboard/sessions.go:9: " + builtForSessionsSays("views.Patch"),
		"internal/dashboard/switchboard.go:5: " + builtForSessionsSays("views.ListSessions"),
		"internal/dashboard/lanes*.go: sessionsFiles names it, but no file matches it; " + sessionsFilesFix,
		"internal/dashboard: " + absorbedSays("dashboard.Frame.OrderOf", "views.Patch"),
		"internal/dashboard: " + absorbedSays("dashboard.Order", "views.Patch"),
		"internal/dashboard: " + absorbedSays("dashboard.Traffic.repatching", "views.Patch"),
		"views.Gone: builtForSessions bans it, but views declares no such function; " + builtForSessionsFix,
		"views.Patched.Gone: builtForSessions bans it, but views declares no such function; " + builtForSessionsFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// sessionsFixture is a module of views, which builds who runs where and who
// sits where, and lists the sessions; and the dashboard, whose Sessions
// files build them for themselves, called and as a value, but in a test; and
// whose Overview reads who runs where, as it may, beside the order milestone
// 5's Sessions kept and the moves that re-patched it, declared again.
var sessionsFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/views/views.go": `package views

type Running struct{ Session string }

type Patched struct{}

func RunningOf() []Running    { return nil }
func Patch(p Patched) Patched { return p }
func ListSessions() []string  { return nil }
`,
	"internal/dashboard/sessions.go": `package dashboard

import "example.com/fixture/internal/views"

type Frame struct{ patched views.Patched }

func (f Frame) sessions() int {
	return len(
		views.RunningOf()) + len(views.Patch(f.patched).String())
}

func (f Frame) String() string { return "" }
`,
	"internal/dashboard/switchboard.go": `package dashboard

import "example.com/fixture/internal/views"

var listed = views.ListSessions
`,
	"internal/dashboard/sessions_test.go": `package dashboard

import "example.com/fixture/internal/views"

var tested = views.RunningOf
`,
	"internal/dashboard/overview.go": `package dashboard

import "example.com/fixture/internal/views"

type Order []string

type Traffic struct{}

func (t Traffic) repatching() {}

func (f Frame) OrderOf() Order { return nil }

var running = views.RunningOf
`,
}

// sessionsBreaches are where the non-test files that draw Sessions, as files'
// patterns match them, of the module at root with the given path, use what
// built names the watch building, called or not; then each pattern no file
// matches; each of absorbed the dashboard declares again; and each of built
// views doesn't declare.
func sessionsBreaches(t *testing.T, root, module string, files, built []string, absorbed map[string]string) []string {
	t.Helper()
	used := make(map[string]sourcecheck.Ban, len(built))
	for _, name := range built {
		used[name] = sourcecheck.Ban{Says: builtForSessionsSays(name)}
	}
	r := sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Used: used}
	dirs := slices.Compact(slices.Sorted(func(yield func(string) bool) {
		for _, p := range files {
			if !yield(path.Dir(p)) {
				return
			}
		}
	}))
	l := sourcecheck.NewLoader(t, root, module, dirs)
	var problems []string
	matched := make(map[string]bool)
	for _, dir := range dirs {
		for _, f := range l.Load(dir) {
			i := slices.IndexFunc(files, func(p string) bool { ok, _ := path.Match(p, f.Path); return ok })
			if i < 0 {
				continue
			}
			matched[files[i]] = true
			found, _ := r.Breaches(f)
			for _, b := range found {
				problems = append(problems, f.At(b.At)+": "+b.Says)
			}
		}
	}
	for _, p := range files {
		if !matched[p] {
			problems = append(problems, p+": sessionsFiles names it, but no file matches it; "+sessionsFilesFix)
		}
	}
	dashboard := imported(t, root, module, "dashboard")
	for _, name := range slices.Sorted(maps.Keys(absorbed)) {
		if lookup(dashboard, name) != nil {
			problems = append(problems, "internal/dashboard: "+absorbedSays(name, absorbed[name]))
		}
	}
	return append(problems, builtGone(t, root, module, built, "builtForSessions", builtForSessionsFix)...)
}
