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

// The header is one line where what it says, as read, fits: the badge's
// line, its day counted at the widest, 32 cells; two; and the status in
// full as the moment has it, 50. So from 84 columns here.
func TestTheHeaderTakesOneLineWhereWhatItSaysFits(t *testing.T) {
	says := "● healthy  ·  6 sessions  ·  new sessions → ▲ side"
	for _, width := range []int{84, 120} {
		one := printed(trio(), width, ByWindow)
		if got := one[0]; !strings.HasSuffix(got, says) || len([]rune(got)) != width {
			t.Errorf("at %d columns the header reads %q, want its status ending at the last column", width, got)
		}
		if one[1] != "" || !strings.HasPrefix(strings.TrimSpace(one[2]), "WEEK") {
			t.Errorf("at %d columns, the header is followed by %q, want a blank line, then the windows' headings", width, one[1:3])
		}
	}
	two := printed(trio(), 83, ByWindow)
	if got, want := two[:3], []string{" SWITCHBOARD   Thu 1 Oct  14:42", "", says}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("at 83 columns the printout starts %q, want %q: its status on a line of its own", got, want)
	}
}

// At its widest, every part of the status said, a pin to every account,
// whatever their names, unhealthy, a restart due, the header switches to
// one line once as the width grows, and on one line says every part it says
// on two, the restart's reason, free text, alone cut.
func TestTheHeaderOnOneLineNeverSaysLessThanOnTwo(t *testing.T) {
	labelled := func(work, personal, side string) status.Document {
		label := func(id, l string) func(*status.Document) {
			return account(id, func(a *status.Account) { a.Label = l })
		}
		return trio(label("work", work), label("personal", personal), label("side", side), pinnedTo("work", "personal", "side"), func(d *status.Document) {
			d.Router.Healthy = false
			d.Restart = status.Restart{Reason: "config changed", Since: printNow.UTC()}
		})
	}
	tests := []struct {
		name  string
		doc   status.Document
		parts []string
	}{
		{name: "the frames' names", doc: labelled("work", "personal", "side"),
			parts: []string{"● unhealthy", "restart due (config changed)", "pinned to work, personal and side", "6 sessions", "new sessions → ▲ side"}},
		{name: "names of 24, 23 and 4", doc: labelled("a-very-long-account-name", "another-long-account-id", "side"),
			parts: []string{"● unhealthy", "restart due (config changed)", "pinned to a-very-long-account-name, another-long-account-id and side", "6 sessions", "new sessions → ▲ side"}},
		{name: "probed past a router that didn't answer", doc: trio(func(d *status.Document) {
			d.Source, d.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}
		}), parts: []string{"○ probed · router unhealthy: no answer within 500ms"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switches, was := 0, false
			for width := 40; width <= 220; width++ {
				rows := printed(tt.doc, width, ByWindow)
				oneLine := strings.HasPrefix(strings.TrimSpace(rows[2]), "WEEK")
				if oneLine != was {
					switches, was = switches+1, oneLine
				}
				if !oneLine {
					continue
				}
				for _, part := range tt.parts {
					if !strings.Contains(rows[0], part) {
						t.Errorf("at %d columns, on one line, the header reads %q, want %q in it, as two lines say it", width, rows[0], part)
					}
				}
			}
			if switches != 1 || !was {
				t.Errorf("the header switched form %d times from 40 columns to 220, ending on one line: %v; want once, to one line", switches, was)
			}
		})
	}
}

// The printout is printed once, so the header's form follows what it says:
// each document's switches to one line once as the width grows, and a
// longer status, pinned to every account, unhealthy, a restart due, where
// it fits whole, further on.
func TestTheHeadersFormFollowsWhatItSays(t *testing.T) {
	longer := trio(pinnedTo("work", "personal", "side"), func(d *status.Document) {
		d.Router.Healthy = false
		d.Restart = status.Restart{Reason: "config changed", Since: printNow.UTC()}
	})
	from := func(doc status.Document) int {
		switches, at, was := 0, 0, false
		for width := 40; width <= 220; width++ {
			if oneLine := strings.HasPrefix(strings.TrimSpace(printed(doc, width, ByWindow)[2]), "WEEK"); oneLine != was {
				switches, at, was = switches+1, width, oneLine
			}
		}
		if switches != 1 || !was {
			t.Errorf("the header switched form %d times from 40 columns to 220, ending on one line: %v; want once, to one line", switches, was)
		}
		return at
	}
	if short, long := from(trio()), from(longer); long <= short {
		t.Errorf("the longer status takes one line from %d columns, the sample's from %d; want it further on", long, short)
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
