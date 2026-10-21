package sourcecheck_test

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// wordFix is the fix the fixture's rule gives a call that words a value.
const wordFix = "word it as its owner does"

// worded says how a call of the function named breaks the fixture's rule,
// and valued how using it as a value does, where the rule says so apart.
func worded(name string) string { return name + " words a value of its own; " + wordFix }

func valued(name string) string { return name + " is used as a value; " + wordFix }

// callsRule is the fixture's rule: no time worded by a constant layout but a
// date key's or a wire stamp's; no value of time.Duration, a float or
// money.Picodollars given to fmt; no float worded by strconv, whatever it's
// given, and its use as a value said as such; and no cut of a stubbed
// package's, in drawn alone.
func callsRule() sourcecheck.Rule {
	keys := []string{time.DateOnly, time.RFC3339}
	operand := sourcecheck.OperandOf("time.Duration", "float32", "float64", "example.com/fixture/internal/money.Picodollars")
	return sourcecheck.Rule{
		Names: sourcecheck.NamesIn("example.com/fixture"),
		Calls: map[string]sourcecheck.CallBan{
			"(time.Time).Format":       {Says: worded("(time.Time).Format"), Breaks: sourcecheck.ConstantOtherThan(0, keys...)},
			"(time.Time).AppendFormat": {Says: worded("(time.Time).AppendFormat"), Breaks: sourcecheck.ConstantOtherThan(1, keys...)},
			"fmt.Sprint":               {Says: worded("fmt.Sprint"), Breaks: operand},
			"fmt.Sprintf":              {Says: worded("fmt.Sprintf"), Breaks: operand},
			"strconv.FormatFloat":      {Says: worded("strconv.FormatFloat"), AsValue: valued("strconv.FormatFloat")},
			"example.org/cells.Cut":    {Says: worded("example.org/cells.Cut"), In: []string{"internal/drawn"}},
		},
	}
}

func TestACheckJudgesEachCallByWhatItsGiven(t *testing.T) {
	root := sourcecheck.WriteModule(t, callsFixture)
	got := sourcecheck.Check(t, root, "example.com/fixture", callsRule(), []string{"internal/drawn", "internal/words"}, fixtureAllowance(
		"internal/drawn/fit.go:fit",
		"internal/drawn/fit.go:fitted",
	))
	want := []string{
		"internal/drawn/layouts.go:9: " + worded("(time.Time).Format"),
		"internal/drawn/layouts.go:10: " + worded("(time.Time).Format"),
		"internal/drawn/layouts.go:16: " + worded("(time.Time).AppendFormat"),
		"internal/drawn/layouts.go:17: " + worded("(time.Time).Format"),
		"internal/drawn/layouts.go:19: " + worded("(time.Time).Format"),
		"internal/drawn/operands.go:17: " + worded("fmt.Sprintf"),
		"internal/drawn/operands.go:18: " + worded("fmt.Sprintf"),
		"internal/drawn/operands.go:19: " + worded("fmt.Sprint"),
		"internal/drawn/operands.go:20: " + worded("fmt.Sprintf"),
		"internal/drawn/operands.go:21: " + worded("fmt.Sprint"),
		"internal/drawn/operands.go:22: " + worded("fmt.Sprint"),
		"internal/drawn/values.go:12: " + worded("(time.Time).Format"),
		"internal/drawn/values.go:13: " + worded("fmt.Sprint"),
		"internal/drawn/values.go:16: " + worded("strconv.FormatFloat"),
		"internal/drawn/values.go:17: " + valued("strconv.FormatFloat"),
		"internal/drawn/values.go:18: " + worded("example.org/cells.Cut"),
		"internal/words/words.go:10: " + worded("strconv.FormatFloat"),
		"internal/drawn/fit.go:fitted: allowed allows it, but it keeps the rule; " + listFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// callsFixture is a module of a type of money and two packages. drawn has a
// file of times worded by layouts: constant, named, known only at run time,
// and allowed, through the method, its method expression and parentheses; a
// file of what's given to fmt: each banned type, a type defined as a float,
// an untyped constant, types allowed, and a function the rule doesn't name; a
// file of what the rule names used as values, a call it bans whatever it's
// given, and a stubbed package's; and a file of functions allowed to break
// the rule, one of which keeps it. words cuts with the stubbed package,
// banned in drawn alone, and words a float with strconv.
var callsFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/money/money.go": `package money

type Picodollars int64

func (p Picodollars) Dollars() string { return "" }
`,
	"internal/drawn/layouts.go": `package drawn

import "time"

const clock = "15:04"

func layouts(t time.Time, layout string) []any {
	return []any{
		t.Format("Mon 2 Jan"),
		t.Format(clock),
		t.Format(layout),
		t.Format(layout + clock),
		t.Format(time.DateOnly),
		t.Format("2006-01-02"),
		t.AppendFormat(nil, time.RFC3339),
		t.AppendFormat(nil, clock),
		time.Time.Format(t, clock),
		time.Time.Format(t, time.DateOnly),
		(t.Format)(clock), (t.Format)(time.DateOnly),
	}
}
`,
	"internal/drawn/operands.go": `package drawn

import (
	"fmt"
	"time"

	"example.com/fixture/internal/money"
)

type share float64

func operands(d time.Duration, f32 float32, f64 float64, p money.Picodollars, s share, n int, t time.Time) []any {
	return []any{
		fmt.Sprintf("%d %s %v", n, "worded", t),
		fmt.Sprintf("%s", p.Dollars()),
		fmt.Errorf("took %v", d),
		fmt.Sprintf("took %v", d),
		fmt.Sprintf("%.1f", f32),
		fmt.Sprint(f64),
		fmt.Sprintf("%d", p),
		fmt.Sprint(s),
		fmt.Sprint(1.5),
	}
}
`,
	"internal/drawn/values.go": `package drawn

import (
	"fmt"
	"strconv"
	"time"

	"example.org/cells"
)

func values(t time.Time, f float64, text string) []any {
	format := t.Format
	sprint := fmt.Sprint
	return []any{
		format(time.DateOnly), sprint(1),
		strconv.FormatFloat(f, 'f', 1, 64),
		strconv.FormatFloat,
		cells.Cut(text, 4),
		strconv.Itoa(1),
	}
}
`,
	"internal/drawn/fit.go": `package drawn

import "example.org/cells"

func fit(text string) any { return cells.Cut(text, 4) }

func fitted(text string) string { return text }
`,
	"internal/words/words.go": `package words

import (
	"strconv"

	"example.org/cells"
)

func words(f float64, text string) []any {
	return []any{cells.Cut(text, 4), strconv.FormatFloat(f, 'f', 1, 64)}
}
`,
}

// Callee names each function called, a method by its receiver's type, a
// generic type's by its type parameters, and gives its arguments as Breaks
// judges them: of a method expression, less the receiver; of what's no
// function, as a conversion, no name.
func TestCalleeNamesWhatsCalledAndItsArguments(t *testing.T) {
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod": "module example.com/fixture\n",
		"internal/reader/reader.go": `package reader

type T struct{}

type B[V any] struct{}

func (T) m(a, b int) int { return a + b }

func (B[V]) g(a int) int { return a }

func f(a int) int { return a }

func h[V any](a V) V { return a }

func calls(t T, b B[int]) []int { return []int{f(1), t.m(2, 3), T.m(t, 4, 5), int(6), b.g(7), h(8)} }
`,
	})
	l := sourcecheck.NewLoader(t, root, "example.com/fixture", []string{"internal/reader"})
	var got []string
	for _, file := range l.Load("internal/reader") {
		ast.Inspect(file.AST, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				name, args := sourcecheck.Callee(call, file.Info)
				got = append(got, fmt.Sprintf("%q %d", name, len(args)))
			}
			return true
		})
	}
	want := []string{
		`"example.com/fixture/internal/reader.f" 1`,
		`"(example.com/fixture/internal/reader.T).m" 2`,
		`"(example.com/fixture/internal/reader.T).m" 2`,
		`"" 1`,
		`"(example.com/fixture/internal/reader.B[V]).g" 1`,
		`"example.com/fixture/internal/reader.h" 1`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
