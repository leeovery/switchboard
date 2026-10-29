package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

// primingScreen is the screen at the Priming step, Accounts passed, the
// router not running yet and the accounts working, its day 08:00-23:00
// typed in, as frame 7 has it, with the changes given.
func primingScreen(changes ...func(*Setup)) Setup {
	s := Setup{
		Now:     now,
		At:      SetupPriming,
		Passed:  []SetupStep{SetupAccounts},
		Status:  []setup.Part{{Standing: setup.PartDown, Words: "router not running yet"}, {Words: "3 accounts, all working"}},
		Priming: &SetupPrimingPage{Day: Field{Value: "08:00-23:00", Cursor: 11, Typing: true}},
	}
	for _, change := range changes {
		change(&s)
	}
	return s
}

func TestThePrimingPageSaysWhatPrimingIsForAndHoldsTheDay(t *testing.T) {
	rows := textOf(primingScreen().Draw(160, 37, Look{}))
	want := map[int]string{
		4:  "PRIMING  " + strings.Repeat("─", 151),
		6:  "  Priming starts each account's 5-hour window on a schedule, so their resets are spread through your day",
		7:  "  instead of all arriving at once. It's optional.",
		8:  "",
		9:  "  Your day  08:00-23:00               start and end, as HH:MM-HH:MM",
		10: "",
		11: "  Leave it empty to keep priming off.",
		12: "",
		36: blanks(132) + "enter next: Service   q quit",
	}
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
}

func TestThePrimingPagesInks(t *testing.T) {
	c := setupDrawn(primingScreen(), 160, 37)
	for _, check := range []struct{ what, why string }{
		{"its purpose", inksFrom(c, 2, 6, 7, secondaryInk)},
		{"its field's value, typed in", inksFrom(c, 12, 9, 11, typingInk)},
		{"its field's cursor", inksFrom(c, 23, 9, 1, caretInk)},
		{"how to leave priming off", inksFrom(c, 2, 11, 5, mutedInk)},
	} {
		if check.why != "" {
			t.Errorf("%s: %s", check.what, check.why)
		}
	}
}

func TestThePrimingPagesFieldGivenUpIsPlain(t *testing.T) {
	c := setupDrawn(primingScreen(func(s *Setup) { s.Priming.Day.Typing = false }), 160, 37)
	if why := inksFrom(c, 11, 9, 1, ink{}); why != "" {
		t.Errorf("the field given up, the cell before its value: %s", why)
	}
	if why := inksFrom(c, 12, 9, 11, secondaryInk); why != "" {
		t.Errorf("the field given up, its value: %s", why)
	}
}

func TestThePrimingPageBeforeItsDayIsReadHoldsNoField(t *testing.T) {
	rows := textOf(primingScreen(func(s *Setup) { s.Priming, s.UnderWay = nil, true }).Draw(160, 37, Look{}))
	if rows[6] == "" || rows[9] != "" || rows[11] != "" {
		t.Errorf("rows 6, 9 and 11 read %q, %q and %q; want what priming is for alone", rows[6], rows[9], rows[11])
	}
	if got, want := rows[36], blanks(154)+"q quit"; got != want {
		t.Errorf("the key line reads %q, want %q: enter waits for the day", got, want)
	}
}

func TestADayRefusedIsSaidUnderTheField(t *testing.T) {
	const refused = "must be two times of day, HH:MM, joined by -, such as 08:00-23:00"
	rows := textOf(primingScreen(func(s *Setup) {
		s.Priming.Day = Field{Value: "8-9", Cursor: 3, Typing: true}
		s.Priming.Refused = refused
	}).Draw(160, 37, Look{}))
	want := map[int]string{
		9:  "  Your day  8-9                       start and end, as HH:MM-HH:MM",
		10: blanks(12) + refused,
		11: "",
		12: "  Leave it empty to keep priming off.",
		36: blanks(132) + "enter next: Service   q quit",
	}
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
}

func TestAPrimingStepThatFailedSaysWhyInPlaceOfTheField(t *testing.T) {
	const why = "read the config: the config doesn't parse"
	rows := textOf(primingScreen(func(s *Setup) {
		s.Priming, s.Failure = &SetupPrimingPage{Failure: why}, why
	}).Draw(160, 37, Look{}))
	if got, want := rows[9], textsAt(160, map[int]string{2: "✗", 5: "Not set", 26: why}); got != want {
		t.Errorf("row 9 reads\n%q\nwant\n%q", got, want)
	}
	if rows[11] != "" {
		t.Errorf("row 11 reads %q, want nothing of the field's", rows[11])
	}
	if got, want := rows[36], blanks(154)+"q quit"; got != want {
		t.Errorf("the key line reads %q, want %q", got, want)
	}
}

// A page whose field is typed in, and 30 notes under it: 34 rows under its
// heading, scrolled at setup's least, 95×25, as TestAPageTallerThanItsRowsScrolls
// has it.
func TestThePageSaysWhatsOutOfViewWithoutKeysWhileItsFieldIsTypedIn(t *testing.T) {
	tests := []struct {
		name   string
		typing bool
		out    string
	}{
		{name: "typed in: no keys, as j and k type", typing: true, out: "▲ 3 more above   ·   ▼ 15 more below"},
		{name: "given up: j/k to scroll", typing: false, out: "▲ 3 more above   ·   ▼ 15 more below   ·   j/k to scroll"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := setupPage{name: "PRIMING", fields: []textField{dayField(func(f *textField) { f.field.Typing = tt.typing })}}
			for i := range 32 {
				page.notes = append(page.notes, line{{fmt.Sprintf("note %d", i+1), mutedInk}})
			}
			c := newCanvas(95, 25)
			page.draw(c, 4, 19, 95, 3)
			if got := rowOf(c, 22); got != tt.out {
				t.Errorf("row 22 reads %q, want %q", got, tt.out)
			}
		})
	}
}
