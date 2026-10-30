package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
)

// shellLine is switchboard's line for zsh, as the shell step writes it.
const shellLine = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`

// notFirst is what the claude link step finds where zsh runs Claude Code's
// own claude, and the user leaves switchboard's line to add by hand.
func notFirst() *setup.LinkFound {
	return &setup.LinkFound{
		Made: setup.LinkCreated,
		Said: []setup.Said{{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}},
		Shell: setup.ShellFound{
			Situation: setup.OfferLine,
			Said:      setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "runs Claude Code's own claude, ~/.local/bin/claude, so its sessions aren't routed"},
			Line:      shellLine,
			Where:     "~/.zshrc",
		},
	}
}

// doneScreen is the screen at Done, every step before it passed through, as
// frame 5 has it, with the changes given.
func doneScreen(changes ...func(*Setup)) Setup {
	working := setup.TokenStanding{State: setup.TokenWorking}
	review := setup.Review{Accounts: []setup.Reviewed{
		{ID: "work", Primary: true, Reserve: 0.05, Token: working}, {ID: "personal", Token: working}, {ID: "side", Token: working},
	}}
	day := config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}
	serviced := setup.Serviced{Said: []setup.Said{serviceInstalled, serviceUpdated, routerHealthy}, Router: &router.Health{OK: true}}
	skill := setup.Said{Mark: setup.Done, Label: "Up to date", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	return setupScreen(append([]func(*Setup){func(s *Setup) {
		s.At, s.Passed = SetupDone, SetupSteps()[:SetupDone]
		s.Summary = setup.Summary{At: setup.StepDone, Review: &review, Day: &day, Service: &serviced, Link: notFirst(), Skill: &skill}
	}}, changes...)...)
}

func TestDoneSaysEachStepsLineThenWhatsNext(t *testing.T) {
	rows := textOf(doneScreen().Draw(160, 37, Look{}))
	want := map[int]string{
		4:  "SWITCHBOARD IS SET UP  " + strings.Repeat("─", 137),
		6:  textsAt(160, map[int]string{2: "✓", 5: "Accounts", 26: "3, all working · primary: work, with 5% reserved for the Claude apps"}),
		8:  textsAt(160, map[int]string{2: "✓", 5: "Priming", 26: "on, 08:00–23:00"}),
		10: textsAt(160, map[int]string{2: "✓", 5: "Service", 26: "installed and updated · router healthy"}),
		12: textsAt(160, map[int]string{2: "!", 5: "claude link", 26: "created, but it isn't first on your PATH. Add this line to ~/.zshrc:"}),
		13: textsAt(160, map[int]string{26: shellLine}),
		15: textsAt(160, map[int]string{2: "✓", 5: "Skill", 26: "up to date"}),
		18: "NEXT  " + strings.Repeat("─", 154),
		20: textsAt(160, map[int]string{2: "claude", 26: "start Claude Code as usual: its sessions route through switchboard"}),
		22: textsAt(160, map[int]string{2: "switchboard", 26: "open the dashboard: every account's windows, sessions and history"}),
		36: "c copy the line" + blanks(112) + "enter open the dashboard   q quit",
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

func TestDonesInks(t *testing.T) {
	c := setupDrawn(doneScreen(), 160, 37)
	for _, tt := range []struct {
		what    string
		x, y, n int
		ink     ink
	}{
		{what: "the line to add, in accent.mode", x: 26, y: 13, n: len(shellLine), ink: accentInk},
		{what: "claude, bold accent.mode", x: 2, y: 20, n: len("claude"), ink: nameInk},
		{what: "what claude does, text.secondary", x: 26, y: 20, n: len("start Claude Code"), ink: secondaryInk},
		{what: "switchboard, bold accent.mode", x: 2, y: 22, n: len("switchboard"), ink: nameInk},
		{what: "the link's mark, bold accent.attention", x: 2, y: 12, n: 1, ink: alertInk},
	} {
		if says := inksFrom(c, tt.x, tt.y, tt.n, tt.ink); says != "" {
			t.Errorf("%s: %s", tt.what, says)
		}
	}
}

func TestDoneOffersTheLineToCopyOnlyWhereTheLinkWantsIt(t *testing.T) {
	const dir = "$HOME/.local/share/switchboard/bin"
	tests := []struct {
		name string
		link *setup.LinkFound
		// want is what's offered, and key the key line's left, "" for none.
		want, key string
	}{
		{name: "not first on PATH: the line", link: notFirst(), want: shellLine, key: "c copy the line"},
		{name: "a shell setup doesn't ask: the directory", link: &setup.LinkFound{Made: setup.LinkCreated, Shell: setup.ShellFound{Situation: setup.UnknownShell, Dir: dir}},
			want: dir, key: "c copy the directory"},
		{name: "done: none", link: &setup.LinkFound{Made: setup.LinkCreated, Shell: setup.ShellFound{Situation: setup.Through}}},
		{name: "passed through: none", link: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := doneScreen(func(s *Setup) { s.Link = tt.link })
			if got := s.Copies("c"); got != tt.want {
				t.Errorf("Done offers %q to copy at c, want %q", got, tt.want)
			}
			if got := s.Copies("d"); got != "" {
				t.Errorf("Done offers %q to copy at d, want nothing", got)
			}
			const right = "enter open the dashboard   q quit"
			want := tt.key + blanks(160-len(tt.key)-len(right)) + right
			if got := textOf(s.Draw(160, 37, Look{}))[36]; got != want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestOnlyDoneOffersALineToCopy(t *testing.T) {
	s := doneScreen(func(s *Setup) { s.At, s.Passed = SetupSkill, SetupSteps()[:SetupSkill] })
	if got := s.Copies("c"); got != "" {
		t.Errorf("the Skill page offers %q to copy, want nothing", got)
	}
}

func TestDonesLinkRowPassedThroughIsItsNameAlone(t *testing.T) {
	rows := textOf(doneScreen(func(s *Setup) { s.Link = nil }).Draw(160, 37, Look{}))
	if got, want := rows[12], textsAt(160, map[int]string{2: "✓", 5: "claude link"}); got != want {
		t.Errorf("row 12 reads %q, want %q: the link passed through, its name alone", got, want)
	}
	if got, want := rows[14], textsAt(160, map[int]string{2: "✓", 5: "Skill", 26: "up to date"}); got != want {
		t.Errorf("row 14 reads %q, want the Skill's line, %q, with no line to add above it", got, want)
	}
}

func TestDonesLineToAddWrapsUnderItsFactNeverCut(t *testing.T) {
	const width = 95
	c := setupDrawn(doneScreen(), width, 60)
	detail := factParts(width)[factDetail]
	var got []string
	for y := 13; ; y++ {
		row := []rune(rowOf(c, y))
		if len(row) <= detail.At {
			break
		}
		if lead := string(row[:detail.At]); strings.TrimSpace(lead) != "" {
			t.Errorf("row %d reads %q before the detail's column, want it blank", y, lead)
		}
		got = append(got, string(row[detail.At:]))
	}
	if len(got) < 2 {
		t.Fatalf("at %d columns, the line takes %d rows, want it wrapped onto more than one", width, len(got))
	}
	if fault := wrapFault(got, shellLine, detail.Size); fault != "" {
		t.Errorf("the line wraps to\n%s\nwant it as copied, %q, wrapped at its words: %s", strings.Join(got, "\n"), shellLine, fault)
	}
	for _, row := range got {
		if ansi.StringWidth(row) > detail.Size {
			t.Errorf("the line's row %q is wider than the detail's %d cells", row, detail.Size)
		}
	}
}

func TestDonesLineToAddBreaksAWordWiderThanItsColumn(t *testing.T) {
	long := strings.Repeat("x", 100)
	link := notFirst()
	link.Shell.Line = long
	c := setupDrawn(doneScreen(func(s *Setup) { s.Link = link }), 95, 60)
	var got string
	for y := 13; y < 16; y++ {
		got += strings.TrimSpace(rowOf(c, y))
	}
	if got != long {
		t.Errorf("a line of a word wider than its column reads %q, want it whole, broken across rows", got)
	}
}

// wrapFault says where rows aren't text wrapped width cells wide at its
// words: each row the next of text's characters as they are, spaces and
// all; each break where spaces fall, which it drops, or after a hyphen, as
// ansi.Wrap breaks; and no row's next word one that would have fitted on
// it. It's "" where they are.
func wrapFault(rows []string, text string, width int) string {
	rest := text
	for i, row := range rows {
		if i > 0 {
			trimmed := strings.TrimLeft(rest, " ")
			prev := rows[i-1]
			if trimmed == rest && !strings.HasSuffix(prev, "-") {
				return fmt.Sprintf("row %d breaks mid-word, after %q", i, prev)
			}
			next, _, _ := strings.Cut(trimmed, " ")
			if ansi.StringWidth(prev)+1+ansi.StringWidth(next) <= width {
				return fmt.Sprintf("row %d's first word, %q, would have fitted after %q", i, next, prev)
			}
			rest = trimmed
		}
		var ok bool
		if rest, ok = strings.CutPrefix(rest, row); !ok {
			return fmt.Sprintf("row %d, %q, isn't the line's next characters, %q", i, row, rest)
		}
	}
	if rest != "" {
		return fmt.Sprintf("the rows end before %q", rest)
	}
	return ""
}

func TestDoneHoldsItsPageAtTheLeastSize(t *testing.T) {
	const width, height = 95, 25
	top := textOf(doneScreen().Draw(width, height, Look{}))
	if top[0] == tooSmall {
		t.Fatalf("at %d×%d setup says %q, want Done drawn", width, height, tooSmall)
	}
	bottom := textOf(doneScreen(func(s *Setup) { s.Scroll = s.Scrolling(width, height).Most }).Draw(width, height, Look{}))
	shown := append(slices.Clone(top[4:height-2]), bottom[4:height-2]...)
	for _, want := range []string{
		"SWITCHBOARD IS SET UP  " + strings.Repeat("─", width-23),
		textsAt(width, map[int]string{2: "✓", 5: "Accounts", 26: "3, all working · primary: work, with 5% reserved for the Claude apps"}),
		textsAt(width, map[int]string{2: "✓", 5: "Priming", 26: "on, 08:00–23:00"}),
		textsAt(width, map[int]string{2: "✓", 5: "Service", 26: "installed and updated · router healthy"}),
		textsAt(width, map[int]string{2: "!", 5: "claude link", 26: "created, but it isn't first on your PATH. Add this line to ~/.zshrc:"}),
		textsAt(width, map[int]string{2: "✓", 5: "Skill", 26: "up to date"}),
		"NEXT  " + strings.Repeat("─", width-6),
		textsAt(width, map[int]string{2: "claude", 26: "start Claude Code as usual: its sessions route through switchboard"}),
		textsAt(width, map[int]string{2: "switchboard", 26: "open the dashboard: every account's windows, sessions and history"}),
	} {
		if !slices.Contains(shown, want) {
			t.Errorf("at %d×%d, scrolled top and bottom, Done never shows the row\n%q\nin\n%s", width, height, want, strings.Join(shown, "\n"))
		}
	}
	for y, row := range append(top, bottom...) {
		if w := ansi.StringWidth(row); w > width {
			t.Errorf("row %d is %d cells, wider than the terminal", y, w)
		}
	}
}
