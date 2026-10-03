package dashboard_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// themes are the rows of a picker listing the built-ins, the default pair's
// badges on them, and a broken file.
var themes = []dashboard.PickerRow{
	{Name: "amber"},
	{Name: "broken", Problem: "bad colour"},
	{Name: "exchange"},
	{Name: "nord", Badge: "● dark"},
	{Name: "terminal"},
	{Name: "tokyo-night"},
	{Name: "tokyo-night-day", Badge: "● light"},
}

func TestPickerWidth(t *testing.T) {
	for width, want := range map[int]int{200: 30, 60: 30, 59: 24, 24: 24, 23: 0, 0: 0} {
		if got := dashboard.PickerWidth(width); got != want {
			t.Errorf("PickerWidth(%d) = %d, want %d", width, got, want)
		}
	}
}

func TestPickerFits(t *testing.T) {
	tests := []struct {
		width, height int
		want          bool
	}{
		{width: 160, height: 34, want: true},
		{width: 24, height: 10, want: true},
		{width: 23, height: 34},
		{width: 160, height: 9},
	}
	for _, tt := range tests {
		if got := dashboard.PickerFits(tt.width, tt.height); got != tt.want {
			t.Errorf("PickerFits(%d, %d) = %v, want %v", tt.width, tt.height, got, tt.want)
		}
	}
}

func TestThePickerIsDrawnOverTheRightOfTheScreen(t *testing.T) {
	screen := blankScreen(80, 16, "view")
	p := dashboard.Picker{Rows: themes, Cursor: 3}

	got := text(p.Over(screen, 80, dashboard.Screen(builtin(t, "nord"))))

	want := []string{
		"view                                              │",
		"view                                              │ Themes",
		"view                                              │",
		"view                                              │   amber",
		"view                                              │   broken        ⚠ bad colour",
		"view                                              │   exchange",
		"view                                              │ ▌ nord                ● dark",
		"view                                              │   terminal",
		"view                                              │   tokyo-night",
		"view                                              │   tokyo-night-day    ● light",
		"view                                              │",
		"view                                              │ ────────────────────────────",
		"view                                              │ ⏎    set theme",
		"view                                              │ d    set as dark",
		"view                                              │ l    set as light",
		"view                                              │ esc  close",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the screen reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestThePickersQuestionTakesThePlaceOfItsKeys(t *testing.T) {
	p := dashboard.Picker{Rows: themes, Cursor: 0, Clearing: "amber"}

	got := strings.Join(text(p.Over(blankScreen(80, 16, ""), 80, dashboard.Screen(builtin(t, "nord")))), "\n")

	for _, want := range []string{"│ clear amber?  y / n", "│ y    confirm", "│ n    cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("the screen reads\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "set theme") {
		t.Errorf("the screen reads\n%s\nwant the question's answers in place of the keys", got)
	}
}

func TestThePickersQuestionCutsTheThemeItNamesToStayWhole(t *testing.T) {
	p := dashboard.Picker{Rows: themes, Clearing: "a-theme-with-a-very-long-name"}

	got := strings.Join(text(p.Over(blankScreen(80, 16, ""), 80, dashboard.Screen(builtin(t, "nord")))), "\n")

	if want := "│ clear a-theme-with-…?  y / n"; !strings.Contains(got, want) {
		t.Errorf("the screen reads\n%s\nwant %q", got, want)
	}
}

func TestThePickerNotesWhatWentWrong(t *testing.T) {
	p := dashboard.Picker{Rows: themes, Note: "couldn't keep the theme"}

	got := strings.Join(text(p.Over(blankScreen(80, 16, ""), 80, dashboard.Screen(builtin(t, "nord")))), "\n")

	if !strings.Contains(got, "│ ⚠ couldn't keep the theme") || !strings.Contains(got, "set theme") {
		t.Errorf("the screen reads\n%s\nwant the note over the keys", got)
	}
}

func TestThePickerScrollsToTheCursor(t *testing.T) {
	var rows []dashboard.PickerRow
	for i := range 20 {
		rows = append(rows, dashboard.PickerRow{Name: fmt.Sprintf("theme-%02d", i)})
	}
	for _, cursor := range []int{0, 5, 12, 19} {
		got := strings.Join(text(dashboard.Picker{Rows: rows, Cursor: cursor}.Over(blankScreen(80, 14, ""), 80, dashboard.Screen(builtin(t, "nord")))), "\n")
		if want := fmt.Sprintf("▌ theme-%02d", cursor); !strings.Contains(got, want) {
			t.Errorf("with the cursor on row %d, the screen reads\n%s\nwant its theme in sight", cursor, got)
		}
	}
}

func TestThePickerCutsANameToFitItsBadgeAndProblem(t *testing.T) {
	rows := []dashboard.PickerRow{
		{Name: "a-theme-with-a-very-long-name", Badge: "● light"},
		{Name: "another-long-broken-theme", Problem: "missing tokens"},
		{Name: "a-long-broken-chosen-theme", Problem: "not found", Badge: "● dark"},
	}

	got := text(dashboard.Picker{Rows: rows, Cursor: 0}.Over(blankScreen(60, 12, ""), 60, dashboard.Screen(builtin(t, "nord"))))

	for i, want := range map[int]string{
		3: "│ ▌ a-theme-with-a-ve… ● light",
		4: "│   another-long-broken-the… ⚠",
		5: "│   a-long-broken-ch… ⚠ ● dark",
	} {
		if !strings.HasSuffix(got[i], want) {
			t.Errorf("row %d reads %q, want it to end %q", i, got[i], want)
		}
	}
}

func TestThePickerPaintsItsSelectedRow(t *testing.T) {
	p := dashboard.Picker{Rows: themes, Cursor: 3}
	lines := p.Over(blankScreen(80, 16, ""), 80, dashboard.Screen(builtin(t, "nord")))

	// Nord's bg.selection, #434C5E.
	if selected := "48;2;67;76;94"; !strings.Contains(lines[6], selected) || strings.Contains(lines[7], selected) {
		t.Errorf("the selected row, and the one under it, read\n%q\n%q\nwant the selected one alone on bg.selection", lines[6], lines[7])
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 80 {
			t.Errorf("line %d is %d cells wide, want the screen's 80", i, got)
		}
	}
}

// blankScreen is a screen of height lines, each width cells wide, saying
// what at its left.
func blankScreen(width, height int, what string) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = what + strings.Repeat(" ", width-len(what))
	}
	return lines
}

// text is lines as text alone, their escapes stripped and their trailing
// spaces trimmed.
func text(lines []string) []string {
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = strings.TrimRight(ansi.Strip(l), " ")
	}
	return plain
}
