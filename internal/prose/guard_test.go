package prose_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a package can't word a plain
// value for itself, rather than through prose, where each form has one owner
// and is changed once, without failing here: a time by a layout of its own, a
// span, a float or money by fmt, a float by strconv, or text cut short.

// tellingPackages are the packages that tell people what they show, by
// directory from the module's root: each words a time, a span, a float and
// money through prose. strconv's floats and ansi's cuts are prose's in every
// package.
var tellingPackages = []string{
	"internal/cli",
	"internal/dashboard",
	"internal/dashboard/watch",
	"internal/notify",
	"internal/router",
	"internal/status",
	"internal/views",
}

// prosePackage is prose's directory, from the module's root: the rule's one
// owner, which it doesn't read.
const prosePackage = "internal/prose"

// wordingAllowed are the files allowed to break the rule, by path from the
// module's root, and their functions, by path, a colon and the function's
// name, a method's its receiver's type's then its own. The list only
// shrinks: an entry must still be there, and still break the rule.
var wordingAllowed = []string{
	// Cells cut, not words: a drawn screen's rows cut short, their styles
	// kept, for the picker to lay over their right; and the capture
	// harness's frame, for the keys pressed to lay over its foot.
	"internal/capture/live.go:live.keycaps",
	"internal/dashboard/picker.go:Picker.Over",
	// An error's text: the price the config gives, as it gives it.
	"internal/config/prices.go:checkPrice",
	// A name, not a span: a window of whole hours, called by its length as
	// "5-hour" wherever a window's named, its hours a count.
	"internal/status/moment.go:NameOfWindow",
}

// What breaks the rule, each by its full name, its package's path then, of a
// method, its receiver's type's, as (*types.Func).FullName has it.
var (
	// keys are the layouts a time may be formatted by outside prose: a date,
	// as the ledger's days are filed and keyed by, and a stamp on the wire.
	keys = []string{time.DateOnly, time.RFC3339}
	// plainTypes are the plain values prose words: no fmt printer is given
	// one, nor money.
	plainTypes = []string{"time.Duration", "float32", "float64"}
	// printers are fmt's functions that word text to show. Errorf isn't
	// one: its text is an error's.
	printers = []string{
		"fmt.Append", "fmt.Appendf", "fmt.Appendln",
		"fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln",
		"fmt.Print", "fmt.Printf", "fmt.Println",
		"fmt.Sprint", "fmt.Sprintf", "fmt.Sprintln",
	}
	// floatWorders are strconv's, which word a float whatever they're given.
	floatWorders = []string{"strconv.AppendFloat", "strconv.FormatFloat"}
	// cutters are ansi's, which cut text short whatever they're given.
	cutters = []string{"github.com/charmbracelet/x/ansi.Truncate", "github.com/charmbracelet/x/ansi.TruncateLeft"}
)

// money is the module's money, by its path under the module's, which words
// itself by its own methods.
const money = "/internal/ledger.Picodollars"

// The fix each kind of breach gives.
const (
	layoutFix  = "word it with prose's times, adding the form there where prose has none"
	operandFix = "word it with prose's spans or figures, or with Picodollars' own methods"
	floatFix   = "word it with prose's figures"
	cutFix     = "cut it with prose.Cut, CutShort or CutFront"
	allowedFix = "remove it from wordingAllowed in internal/prose/guard_test.go"
)

// layoutSays, operandSays, floatSays and cutSays say how a use of the
// function named breaks the rule, each of its kind of breach.
func layoutSays(name string) string {
	return name + " words a time by a layout of its own; " + layoutFix
}

func operandSays(name string) string {
	return name + " is given a span, a float or money to word; " + operandFix
}

func floatSays(name string) string {
	return name + " words a float; " + floatFix
}

func cutSays(name string) string {
	return name + " cuts text short; " + cutFix
}

func TestPlainValuesAreWordedByProseAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, wordingBreaches(t, root, sourcecheck.ModulePath(t, root), wordingAllowed))
}

// wordingBreaches are where the non-test files of the packages of the module
// at root with the given path, but prose's, break the rule, but in the files
// and functions allowed: in tellingPackages, each time worded by a constant
// layout but a key, and each fmt printer given a plain value or money; and
// in every package, each float worded by strconv, and each text cut by
// ansi. Each is found used as a value too, as what it's given then can't be
// judged. Then each file or function allowed that's gone, or keeps the rule.
func wordingBreaches(t *testing.T, root, module string, allowed []string) []string {
	t.Helper()
	dirs := slices.DeleteFunc(sourcecheck.Packages(t, root), func(dir string) bool { return dir == prosePackage })
	return sourcecheck.Check(t, root, module, wordingRule(module), dirs, sourcecheck.Allowance{
		Entries: allowed, List: "wordingAllowed", Fix: allowedFix,
	})
}

// wordingRule is prose's rule, of the module with the given path.
func wordingRule(module string) sourcecheck.Rule {
	calls := map[string]sourcecheck.CallBan{
		"(time.Time).Format":       layoutBan("(time.Time).Format", 0),
		"(time.Time).AppendFormat": layoutBan("(time.Time).AppendFormat", 1),
	}
	operand := sourcecheck.OperandOf(append(slices.Clone(plainTypes), module+money)...)
	for _, name := range printers {
		calls[name] = sourcecheck.CallBan{Says: operandSays(name), In: tellingPackages, Breaks: operand}
	}
	for _, name := range floatWorders {
		calls[name] = sourcecheck.CallBan{Says: floatSays(name)}
	}
	for _, name := range cutters {
		calls[name] = sourcecheck.CallBan{Says: cutSays(name)}
	}
	return sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Calls: calls}
}

// layoutBan bans the time formatter named, in tellingPackages, from a
// constant layout but a key, its layout the argument at index arg.
func layoutBan(name string, arg int) sourcecheck.CallBan {
	return sourcecheck.CallBan{Says: layoutSays(name), In: tellingPackages, Breaks: sourcecheck.ConstantOtherThan(arg, keys...)}
}

func TestTheWordingGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, wordingFixture)
	got := wordingBreaches(t, root, "example.com/fixture", []string{
		"internal/dashboard/allowed.go:fit",
		"internal/dashboard/allowed.go:kept",
	})
	want := []string{
		"internal/config/config.go:10: " + floatSays("strconv.FormatFloat"),
		"internal/dashboard/times.go:7: " + layoutSays("(time.Time).Format"),
		"internal/dashboard/times.go:8: " + layoutSays("(time.Time).AppendFormat"),
		"internal/dashboard/values.go:14: " + operandSays("fmt.Fprintf"),
		"internal/dashboard/values.go:16: " + operandSays("fmt.Sprintf"),
		"internal/dashboard/values.go:17: " + operandSays("fmt.Sprint"),
		"internal/dashboard/values.go:18: " + operandSays("fmt.Appendf"),
		"internal/dashboard/values.go:21: " + floatSays("strconv.FormatFloat"),
		"internal/dashboard/values.go:22: " + floatSays("strconv.AppendFloat"),
		"internal/dashboard/values.go:23: " + cutSays("github.com/charmbracelet/x/ansi.Truncate"),
		"internal/dashboard/values.go:24: " + cutSays("github.com/charmbracelet/x/ansi.TruncateLeft"),
		"internal/dashboard/values.go:25: " + operandSays("fmt.Sprint"),
		"internal/dashboard/allowed.go:kept: wordingAllowed allows it, but it keeps the rule; " + allowedFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// wordingFixture is a module of money and prose, which words plain values
// as it likes; a telling package, the dashboard, with a file of times worded
// by layouts, of keys and not, constant and not, and a file of values worded
// by fmt, strconv and ansi, of each type the rule bans and of types it
// doesn't, by fmt.Errorf too, and fmt used as a value; a file of functions
// allowed to break the rule, one of which keeps it, and a test file; and
// config, a package that tells nothing, where a float worded by strconv
// alone breaks the rule.
var wordingFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/ledger/ledger.go": `package ledger

type Picodollars int64

func (p Picodollars) Dollars() string { return "" }
`,
	"internal/prose/prose.go": `package prose

import (
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func Words(now time.Time, f float64, text string) string {
	return now.Format("15:04") + strconv.FormatFloat(f, 'f', 1, 64) + ansi.Truncate(text, 4, "")
}
`,
	"internal/dashboard/times.go": `package dashboard

import "time"

func times(now time.Time, layout string) []any {
	return []any{
		now.Format("15:04"),
		now.AppendFormat(nil, "Mon 2 Jan"),
		now.Format(time.DateOnly),
		now.Format(time.RFC3339),
		now.Format(layout),
	}
}
`,
	"internal/dashboard/values.go": `package dashboard

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"
	"example.com/fixture/internal/ledger"
)

func values(w io.Writer, took time.Duration, share float32, rate float64, worth ledger.Picodollars, n int) []any {
	fmt.Fprintf(w, "%.0f%%", share)
	return []any{
		fmt.Sprintf("took %v", took),
		fmt.Sprint(rate),
		fmt.Appendf(nil, "%d", worth),
		fmt.Sprintf("%d for %s", n, worth.Dollars()),
		fmt.Errorf("took %v", took),
		strconv.FormatFloat(rate, 'f', 1, 64),
		strconv.AppendFloat(nil, rate, 'f', 1, 64),
		ansi.Truncate("text", 2, ""),
		ansi.TruncateLeft("text", 2, ""),
		fmt.Sprint,
	}
}
`,
	"internal/dashboard/allowed.go": `package dashboard

import "github.com/charmbracelet/x/ansi"

func fit(text string) string { return ansi.Truncate(text, 4, "") }

func kept(text string) string { return text }
`,
	"internal/dashboard/dashboard_test.go": `package dashboard

import "fmt"

func tested(rate float64) string { return fmt.Sprint(rate) }
`,
	"internal/config/config.go": `package config

import (
	"fmt"
	"strconv"
	"time"
)

func said(now time.Time, took time.Duration, price float64) []any {
	return []any{now.Format("15:04"), fmt.Sprint(took), strconv.FormatFloat(price, 'g', -1, 64)}
}
`,
}
