package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestThePrintoutRunsFromItsHeaderToItsKeyLine(t *testing.T) {
	for _, by := range []By{ByWindow, ByAccount} {
		for _, width := range []int{160, 120, 80, 52} {
			rows := printed(trio(), width, by)
			if !strings.HasPrefix(rows[0], " SWITCHBOARD ") {
				t.Errorf("by %d at %d columns, the printout starts %q, want its header", by, width, rows[0])
			}
			if last := rows[len(rows)-1]; !strings.Contains(last, "cap: work at 95%") {
				t.Errorf("by %d at %d columns, the printout ends %q, want its key line", by, width, last)
			}
		}
	}
}

func TestTheHeaderSaysWhenTheDocumentWasRead(t *testing.T) {
	doc := trio(func(d *status.Document) { d.GeneratedAt = on(1, 14, 41).Add(58 * time.Second).UTC() })
	if got, want := colAt(printed(doc, 160, ByWindow)[0], 15, 16), "Thu 1 Oct  14:41"; got != want {
		t.Errorf("the header reads %q, want %q: when the document was read, not when it's printed", got, want)
	}
}

func TestTheHeaderTakesOneLineFrom120Columns(t *testing.T) {
	says := "● healthy  ·  6 sessions  ·  new sessions → ▲ side"
	one := printed(trio(), 120, ByWindow)
	if got := one[0]; !strings.HasSuffix(got, says) || len([]rune(got)) != 120 {
		t.Errorf("at 120 columns the header reads %q, want its status ending at the last column", got)
	}
	if one[1] != "" || one[2] != "5-HOUR" {
		t.Errorf("at 120 columns, the header is followed by %q, want a blank line, then the first window", one[1:3])
	}
	two := printed(trio(), 119, ByWindow)
	if got, want := two[:5], []string{" SWITCHBOARD   Thu 1 Oct  14:42", "", says, "", "5-HOUR"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("at 119 columns the printout starts %q, want %q: its status on a line of its own", got, want)
	}
}

func TestTheHeadersStatusShowsTheModeOnlyWhilePinned(t *testing.T) {
	if got := printed(trio(), 160, ByWindow)[0]; strings.Contains(got, "auto") {
		t.Errorf("on auto, the header reads %q, want no mode", got)
	}
	if got := printed(trio(pinnedTo("personal")), 160, ByWindow)[0]; !strings.Contains(got, "  ·  pinned to personal  ·  ") {
		t.Errorf("pinned, the header reads %q, want the pin", got)
	}
}

func TestTheHeadersStatusShortensWhereItDoesntFit(t *testing.T) {
	if got, want := printed(trio(), 52, ByWindow)[2], "● healthy  ·  6 sessions  ·  new sessions → ▲ side"; got != want {
		t.Errorf("at 52 columns, the status reads %q, want it in full, as it fits", got)
	}
	if got, want := printed(trio(), 48, ByWindow)[2], "● healthy · 6 sessions · new → ▲ side"; got != want {
		t.Errorf("at 48 columns, the status reads %q, want %q, as a phone has it", got, want)
	}
}

func TestTheHeaderLeavesOffWholePartsBeforeCuttingAny(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{width: 31, want: " SWITCHBOARD   Thu 1 Oct  14:42"},
		{width: 30, want: " SWITCHBOARD   14:42"},
		{width: 20, want: " SWITCHBOARD   14:42"},
		{width: 19, want: "14:42"},
		{width: 5, want: "14:42"},
		{width: 4, want: "14:…"},
	}
	for _, tt := range tests {
		if got := printed(trio(), tt.width, ByWindow)[0]; got != tt.want {
			t.Errorf("at %d columns, the header reads %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestByAccountLeavesABlankLineUnderTheHeader(t *testing.T) {
	rows := printed(trio(), 160, ByAccount)
	if rows[1] != "" || !strings.HasPrefix(rows[2], "1 work") {
		t.Errorf("by account, the printout starts %q, want the header, a blank line, then work's block", rows[:3])
	}
}

func TestThePrintoutFitsEveryWidth(t *testing.T) {
	docs := map[string]status.Document{
		"three accounts": trio(pinnedTo("personal", "side")),
		"nothing at all": {},
	}
	for name, doc := range docs {
		for _, by := range []By{ByWindow, ByAccount} {
			for width := 1; width <= 220; width++ {
				for i, r := range printed(doc, width, by) {
					if got := ansi.StringWidth(r); got > width {
						t.Fatalf("%s by %d at %d columns, row %d is %d cells wide: %q", name, by, width, i, got, r)
					}
				}
			}
		}
	}
}

func TestAPrintoutWithoutColourTellsStateByGlyphsAndBold(t *testing.T) {
	rows := Printout{Width: 160, Look: NoColour()}.Draw(trio().At(printNow, score.Policy{}))
	for i, r := range rows {
		if strings.Contains(r, "38;") || strings.Contains(r, "48;") {
			t.Fatalf("row %d is drawn %q, want no colour", i, r)
		}
	}
	if got, want := colAt(ansi.Strip(rows[4]), 16, 16), "████████┃██████╎"; got != want {
		t.Errorf("work's 5-hour bar reads %q, want %q: its fill in blocks, its marks over it", got, want)
	}
	if !strings.Contains(rows[4], "\x1b[1mwork") {
		t.Errorf("work's row is drawn %q, want its name bold", rows[4])
	}
}
