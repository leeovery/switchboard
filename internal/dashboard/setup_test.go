package dashboard

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

// installed is what the skill step says of a skill it wrote.
var installed = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}

// setupScreen is the screen at the Skill step, the four before it passed
// through, the router up and the accounts working, as frame 9 has it, with
// the changes given.
func setupScreen(changes ...func(*Setup)) Setup {
	said := installed
	s := Setup{
		Now:    now,
		Step:   SetupSkill,
		Passed: []SetupStep{SetupAccounts, SetupPriming, SetupService, SetupLink},
		Status: []SetupPart{{Standing: SetupUp, Words: "router up, healthy"}, {Words: "3 accounts, all working"}},
		Skill:  &said,
	}
	for _, change := range changes {
		change(&s)
	}
	return s
}

// inkAt is the ink of the cell at x along row y.
func inkAt(c *canvas, x, y int) ink {
	return c.cells[y][x].ink
}

func TestSetupsTitleRowNamesItsStepsAsTabs(t *testing.T) {
	c := setupScreen().page(160, 37)
	want := " SWITCHBOARD   setup     ✓ Accounts   ✓ Priming   ✓ Service   ✓ claude link   5 Skill   Done " + blanks(47) + "Mon 28 Sep  13:12:00"
	if got := rowOf(c, 0); got != want {
		t.Errorf("the title row reads\n%q\nwant\n%q: setup's name, its steps a cell apart, Done unnumbered, and the dashboard's clock", got, want)
	}
	if got := inkAt(c, 15, 0); got != titleInk {
		t.Errorf("setup's name is in %+v, want bold text.primary", got)
	}
}

func TestSetupsTabsTellEachStepsState(t *testing.T) {
	tickedOn := boldPositiveInk
	tickedOn.on = shownTabInk.on
	tests := []struct {
		name   string
		step   SetupStep
		passed []SetupStep
		// at is the tab's first cell, and want its words, then the inks of
		// its first cell and of its number's.
		at           int
		want         string
		ink, numeral ink
	}{
		{name: "the step showing, picked out", step: SetupPriming, at: 37, want: " 2 Priming ", ink: shownTabInk, numeral: shownTabInk},
		{name: "a step passed through, muted, ticked in bold state.positive", step: SetupPriming, passed: []SetupStep{SetupAccounts}, at: 24, want: " ✓ Accounts ", ink: mutedInk, numeral: boldPositiveInk},
		{name: "a step to come, subtle", step: SetupPriming, at: 49, want: " 3 Service ", ink: tabInk, numeral: tabInk},
		{name: "the step showing and passed through, picked out and ticked", step: SetupPriming, passed: []SetupStep{SetupPriming}, at: 37, want: " ✓ Priming ", ink: shownTabInk, numeral: tickedOn},
		{name: "Done, with no number", step: SetupPriming, at: 87, want: " Done ", ink: tabInk, numeral: tabInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupScreen(func(s *Setup) { s.Step, s.Passed = tt.step, tt.passed }).page(160, 37)
			row := []rune(rowOf(c, 0))
			if got := string(row[tt.at : tt.at+len([]rune(tt.want))]); got != tt.want {
				t.Errorf("the tab reads %q, want %q", got, tt.want)
			}
			if got := inkAt(c, tt.at, 0); got != tt.ink {
				t.Errorf("the tab is in %+v, want %+v", got, tt.ink)
			}
			if got := inkAt(c, tt.at+1, 0); got != tt.numeral {
				t.Errorf("its number is in %+v, want %+v", got, tt.numeral)
			}
		})
	}
}

func TestSetupsStatusLineSaysItsPartsApart(t *testing.T) {
	tests := []struct {
		name   string
		status []SetupPart
		want   string
		// inks are the inks of the cells at their columns.
		inks map[int]ink
	}{
		{name: "a part that's up, then one told alone, a faint dot between", status: []SetupPart{{Standing: SetupUp, Words: "router up, healthy"}, {Words: "3 accounts, 2 working"}},
			want: "● router up, healthy  ·  3 accounts, 2 working",
			inks: map[int]ink{0: positiveInk, 2: secondaryInk, 20: faintInk, 22: faintInk, 25: mutedInk}},
		{name: "a part not up yet, alone", status: []SetupPart{{Standing: SetupDown, Words: "nothing set up yet"}},
			want: "○ nothing set up yet",
			inks: map[int]ink{0: dimInk, 2: secondaryInk}},
		{name: "no parts, nothing", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupScreen(func(s *Setup) { s.Status = tt.status }).page(160, 37)
			if got := rowOf(c, 2); got != tt.want {
				t.Errorf("the status line reads %q, want %q", got, tt.want)
			}
			for x, want := range tt.inks {
				if got := inkAt(c, x, 2); got != want {
					t.Errorf("column %d is in %+v, want %+v", x, got, want)
				}
			}
		})
	}
}

func TestAFactsRowSetsItsMarkLabelAndDetailInColumns(t *testing.T) {
	tests := []struct {
		mark  setup.Mark
		glyph string
		ink   ink
	}{
		{mark: setup.Done, glyph: "✓", ink: boldPositiveInk},
		{mark: setup.Warning, glyph: "!", ink: alertInk},
		{mark: setup.Failed, glyph: "✗", ink: exhaustedInk},
	}
	for _, tt := range tests {
		t.Run(tt.glyph, func(t *testing.T) {
			c := newCanvas(160, 3)
			next := fact(c, 0, 160, setup.Said{Mark: tt.mark, Label: "Router", Detail: "healthy · 127.0.0.1:4747"})
			if got, want := rowOf(c, 0), textsAt(160, map[int]string{2: tt.glyph, 5: "Router", 26: "healthy · 127.0.0.1:4747"}); got != want {
				t.Errorf("the row reads %q, want %q", got, want)
			}
			for x, want := range map[int]ink{2: tt.ink, 5: secondaryInk, 26: mutedInk} {
				if got := inkAt(c, x, 0); got != want {
					t.Errorf("column %d is in %+v, want %+v", x, got, want)
				}
			}
			if next != 1 {
				t.Errorf("the row after it is %d, want 1", next)
			}
		})
	}
}

func TestAFactsDetailWrapsUnderItself(t *testing.T) {
	c := newCanvas(40, 3)
	next := fact(c, 0, 40, setup.Said{Mark: setup.Done, Label: "Link", Detail: "made and first on your PATH"})
	if got, want := []string{rowOf(c, 0), rowOf(c, 1)}, []string{textsAt(40, map[int]string{2: "✓", 5: "Link", 26: "made and first"}), blanks(26) + "on your PATH"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the fact reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if next != 2 {
		t.Errorf("the row after it is %d, want 2", next)
	}
}

func TestSetupsKeyLineHasThePagesKeysLeftAndOnwardRight(t *testing.T) {
	page := setupPage{
		keys:   []Key{{Key: "c", Does: "copy the line"}, {Key: "d", Does: "copy the directory"}, {Key: "r", Does: "check again"}},
		onward: onward("leave it for now"),
	}
	right := "enter leave it for now   q quit"
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "the page's keys from the first column, three apart; enter and q quit ending at the last", width: 100,
			want: "c copy the line   d copy the directory   r check again" + blanks(15) + right},
		{name: "the page's keys that don't fit whole left off, keeping three clear of the right", width: 87,
			want: "c copy the line   d copy the directory" + blanks(18) + right},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(tt.width, 1)
			page.keyLine(c, 0, tt.width)
			if got := rowOf(c, 0); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
			if got := inkAt(c, 0, 0); got != keyInk {
				t.Errorf("a key is in %+v, want bold accent.key", got)
			}
			if got := inkAt(c, 1, 0); got != mutedInk {
				t.Errorf("its word is in %+v, want muted", got)
			}
		})
	}
}

// The least: the title row's name and tabs whole, two cells clear of the
// edge, 95 columns; and Done's rows under the chrome, 25.
func TestSetupBelowItsLeastSaysToMakeTheTerminalLarger(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		small         bool
	}{
		{name: "at its least", width: 95, height: 25},
		{name: "a column narrower", width: 94, height: 25, small: true},
		{name: "a row shorter", width: 95, height: 24, small: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := textOf(setupScreen().Draw(tt.width, tt.height, Look{}))
			if got := rows[0] == tooSmall && strings.Join(rows[1:], "") == ""; got != tt.small {
				t.Errorf("at %d×%d the screen reads\n%s\nwant it to say only %q: %t", tt.width, tt.height, strings.Join(rows, "\n"), tooSmall, tt.small)
			}
			if !tt.small && rows[tt.height-1] == "" {
				t.Errorf("at %d×%d the last row is blank, want the key line", tt.width, tt.height)
			}
		})
	}
}

func TestTheSkillPageSaysWhatTheSkillIsForAndWhatTheStepDid(t *testing.T) {
	rows := textOf(setupScreen().Draw(160, 37, Look{}))
	want := map[int]string{
		4:  "SKILL  " + strings.Repeat("─", 153),
		6:  "  The skill tells Claude about switchboard, so it can answer questions about your accounts, usage and sessions.",
		8:  textsAt(160, map[int]string{2: "✓", 5: "Installed", 26: "~/.claude/skills/switchboard/SKILL.md"}),
		36: blanks(139) + "enter finish   q quit",
	}
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
	for y := range rows {
		if _, said := want[y]; !said && y > 2 && rows[y] != "" {
			t.Errorf("row %d reads %q, want it blank", y, rows[y])
		}
	}
}

func TestTheSkillPageBeforeTheStepHasDoneItSaysNoFact(t *testing.T) {
	rows := textOf(setupScreen(func(s *Setup) { s.Skill = nil }).Draw(160, 37, Look{}))
	if rows[8] != "" {
		t.Errorf("row 8 reads %q, want it blank until the step has said what it did", rows[8])
	}
}
