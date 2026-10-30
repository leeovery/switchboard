package dashboard

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

// What the claude link step says, as frames 11 and 12 have it.
var (
	linkMade     = setup.Said{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}
	shellOwn     = setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "runs Claude Code's own claude, ~/.local/bin/claude, so its sessions aren't routed"}
	shellThrough = setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it"}
	hookOn       = setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "Claude Code says so when a session doesn't start through switchboard · ~/.claude/settings.json"}
)

// binDir is switchboard's bin directory, as setup writes it to put first on
// PATH by hand; and addTheLine and orTheDir are what setup says of the line
// and the directory, as frame 11 has them.
const (
	binDir     = "$HOME/.local/share/switchboard/bin"
	addTheLine = "Add this line to your shell's startup file yourself, after anything else there that changes PATH:"
	orTheDir   = "Or, where you keep your PATH another way, put this directory first on it:"
)

// linkFound is what the claude link step finds where the shell leaves the
// situation given, its row as said says.
func linkFound(situation setup.Situation, said setup.Said) *setup.LinkFound {
	return &setup.LinkFound{
		Made:  setup.LinkCreated,
		Said:  []setup.Said{linkMade},
		Shell: setup.ShellFound{Situation: situation, Said: said, Line: shellLine, Where: "~/.zshrc", Dir: binDir},
	}
}

// linkScreen is the screen at the claude link step, the three before it
// passed through, zsh running Claude Code's own claude, its line left to
// add by hand, as frame 11 has it, with the changes given.
func linkScreen(changes ...func(*Setup)) Setup {
	return setupScreen(append([]func(*Setup){func(s *Setup) {
		s.At, s.Passed, s.Skill = SetupLink, []SetupStep{SetupAccounts, SetupPriming, SetupService}, nil
		s.Link = linkFound(setup.OfferLine, shellOwn)
	}}, changes...)...)
}

// through is the shell running switchboard's claude, the step ended, the
// session hook installed, as frame 12 has it.
func through(s *Setup) {
	s.Link = linkFound(setup.Through, shellThrough)
	s.Linking.Ended, s.Linking.Hook = true, &hookOn
}

// askingFirst is the step asking the shell, nothing found yet.
func askingFirst(s *Setup) {
	s.Link, s.Linking.Asking, s.UnderWay = nil, true, true
}

// checkLinkRows checks the screen's rows from its heading down, each in
// want reading as it says, and every other blank.
func checkLinkRows(t *testing.T, rows []string, want map[int]string) {
	t.Helper()
	want[4] = "CLAUDE LINK  " + strings.Repeat("─", 147)
	want[6] = "  " + linkPurpose
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

func TestByHandTheLinkPageSaysHowToPutItRight(t *testing.T) {
	checkLinkRows(t, textOf(linkScreen().Draw(160, 37, Look{})), map[int]string{
		8:  textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail}),
		10: textsAt(160, map[int]string{2: "!", 5: "Your shell", 26: shellOwn.Detail}),
		13: "  " + addTheLine,
		15: "      " + shellLine,
		17: "  " + orTheDir,
		19: "      " + binDir,
		21: "  " + pressR,
		36: "c copy the line   d copy the directory   r check again" + blanks(75) + "enter leave it for now   q quit",
	})
}

func TestTheLinkPagesByHandInks(t *testing.T) {
	c := setupDrawn(linkScreen(), 160, 37)
	for _, tt := range []struct {
		what    string
		x, y, n int
		ink     ink
	}{
		{what: "what the claude command is for, text.secondary", x: 2, y: 6, n: len(linkPurpose), ink: secondaryInk},
		{what: "the shell's mark, bold accent.attention", x: 2, y: 10, n: 1, ink: alertInk},
		{what: "how to add the line, text.secondary", x: 2, y: 13, n: len(addTheLine), ink: secondaryInk},
		{what: "the line, accent.mode", x: 6, y: 15, n: len(shellLine), ink: accentInk},
		{what: "how to put the directory first, text.secondary", x: 2, y: 17, n: len(orTheDir), ink: secondaryInk},
		{what: "the directory, accent.mode", x: 6, y: 19, n: len(binDir), ink: accentInk},
		{what: "to press r, muted", x: 2, y: 21, n: len(pressR), ink: mutedInk},
	} {
		if says := inksFrom(c, tt.x, tt.y, tt.n, tt.ink); says != "" {
			t.Errorf("%s: %s", tt.what, says)
		}
	}
}

func TestTheShellRunningSwitchboardsClaudeTheLinkPageSaysSoAndTheSessionHook(t *testing.T) {
	checkLinkRows(t, textOf(linkScreen(through).Draw(160, 37, Look{})), map[int]string{
		8:  textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail}),
		10: textsAt(160, map[int]string{2: "✓", 5: "Your shell", 26: shellThrough.Detail}),
		12: textsAt(160, map[int]string{2: "✓", 5: "Session hook", 26: hookOn.Detail}),
		36: blanks(134) + "enter next: Skill   q quit",
	})
}

func TestWhileItAsksTheShellTheLinkPageSaysSo(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Setup)
		want   map[int]string
	}{
		{name: "nothing found yet: the asking alone", change: askingFirst, want: map[int]string{
			8: "  ●∙∙  " + linkAsking,
		}},
		{name: "asking again: under the link's row, in the shell's place", change: func(s *Setup) { s.Linking.Asking, s.UnderWay = true, true }, want: map[int]string{
			8:  textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail}),
			10: "  ●∙∙  " + linkAsking,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := linkScreen(tt.change)
			tt.want[36] = blanks(154) + "q quit"
			checkLinkRows(t, textOf(s.Draw(160, 37, Look{})), tt.want)
			if _, spins := s.SpinDelay(); !spins {
				t.Error("the spinner doesn't move while the shell is asked")
			}
			for _, key := range []string{"c", "d"} {
				if got := s.Copies(key); got != "" {
					t.Errorf("Copies(%s) = %q while the shell is asked, want nothing", key, got)
				}
			}
		})
	}
}

func TestTheLinkPageShowsTheRemedyInSetupsWords(t *testing.T) {
	tests := []struct {
		name      string
		situation setup.Situation
		rows      map[int]string
		keys      string
		c, d      string
	}{
		{name: "the line not taking effect: moved, no line to copy", situation: setup.NotTaking, keys: "d copy the directory   r check again", d: binDir,
			rows: map[int]string{13: "  Move switchboard's line to the end of ~/.zshrc, or what changes PATH after it to before it.", 15: "  " + orTheDir, 17: "      " + binDir, 19: "  " + pressR}},
		{name: "another switchboard's line: put in its place", situation: setup.OtherLine, keys: "c copy the line   d copy the directory   r check again", c: shellLine, d: binDir,
			rows: map[int]string{13: "  Put this line in its place:", 15: "      " + shellLine, 17: "  " + orTheDir, 19: "      " + binDir, 21: "  " + pressR}},
		{name: "asking again alone", situation: setup.InFront, keys: "r check again",
			rows: map[int]string{13: "  " + pressR}},
		{name: "a shell setup doesn't ask: the directory alone, which r can't check", situation: setup.UnknownShell, keys: "d copy the directory", d: binDir,
			rows: map[int]string{13: "  Put this directory first on PATH, wherever your shell sets it:", 15: "      " + binDir}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := linkScreen(func(s *Setup) { s.Link = linkFound(tt.situation, shellOwn) })
			tt.rows[8] = textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail})
			tt.rows[10] = textsAt(160, map[int]string{2: "!", 5: "Your shell", 26: shellOwn.Detail})
			const right = "enter leave it for now   q quit"
			tt.rows[36] = tt.keys + blanks(160-len(tt.keys)-len(right)) + right
			checkLinkRows(t, textOf(s.Draw(160, 37, Look{})), tt.rows)
			for key, want := range map[string]string{"c": tt.c, "d": tt.d, "r": ""} {
				if got := s.Copies(key); got != want {
					t.Errorf("Copies(%s) = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestOnceTheStepHasEndedByHandItsLinesAreGoneTheSessionHookUnderTheShell(t *testing.T) {
	notInstalled := setup.Said{Mark: setup.Warning, Label: "Session hook", Detail: "setup carries on without it: settings.json doesn't parse"}
	tests := []struct {
		name   string
		change func(*Setup)
		want   map[int]string
	}{
		{name: "the hook installing: under way", change: func(s *Setup) { s.Linking.Ended, s.UnderWay = true, true }, want: map[int]string{
			36: blanks(154) + "q quit",
		}},
		{name: "the hook installed", change: func(s *Setup) { s.Linking.Ended, s.Linking.Hook = true, &hookOn }, want: map[int]string{
			12: textsAt(160, map[int]string{2: "✓", 5: "Session hook", 26: hookOn.Detail}),
			36: blanks(134) + "enter next: Skill   q quit",
		}},
		{name: "the hook not installed", change: func(s *Setup) { s.Linking.Ended, s.Linking.Hook = true, &notInstalled }, want: map[int]string{
			12: textsAt(160, map[int]string{2: "!", 5: "Session hook", 26: notInstalled.Detail}),
			36: blanks(134) + "enter next: Skill   q quit",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := linkScreen(tt.change)
			tt.want[8] = textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail})
			tt.want[10] = textsAt(160, map[int]string{2: "!", 5: "Your shell", 26: shellOwn.Detail})
			checkLinkRows(t, textOf(s.Draw(160, 37, Look{})), tt.want)
			for _, key := range []string{"c", "d"} {
				if got := s.Copies(key); got != "" {
					t.Errorf("Copies(%s) = %q once the step has ended, want nothing", key, got)
				}
			}
		})
	}
}

func TestUnreachedTheLinkPageSaysWhatSetupSaidAndGoesOn(t *testing.T) {
	blocked := setup.Said{Mark: setup.Warning, Label: "Link", Detail: "not made: it isn't a link, so it's left alone"}
	unasked := setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "not asked, as the claude link isn't made"}
	s := linkScreen(func(s *Setup) {
		s.Link = &setup.LinkFound{Said: []setup.Said{blocked}, Shell: setup.ShellFound{Situation: setup.Unreached, Said: unasked}}
	})
	checkLinkRows(t, textOf(s.Draw(160, 37, Look{})), map[int]string{
		8:  textsAt(160, map[int]string{2: "!", 5: "Link", 26: blocked.Detail}),
		10: textsAt(160, map[int]string{2: "!", 5: "Your shell", 26: unasked.Detail}),
		36: blanks(134) + "enter next: Skill   q quit",
	})
}

func TestTheLinkStepFailedItsPageSaysWhy(t *testing.T) {
	const why = "make switchboard's bin directory: permission denied"
	tests := []struct {
		name string
		link *setup.LinkFound
		want map[int]string
	}{
		{name: "nothing found", want: map[int]string{8: textsAt(160, map[int]string{2: "✗", 5: "Not set up", 26: why})}},
		{name: "asking again: the link's row kept", link: linkFound(setup.OfferLine, shellOwn), want: map[int]string{
			8:  textsAt(160, map[int]string{2: "✓", 5: "Link", 26: linkMade.Detail}),
			10: textsAt(160, map[int]string{2: "✗", 5: "Not set up", 26: why}),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := linkScreen(func(s *Setup) { s.Link, s.Failure = tt.link, why })
			tt.want[36] = blanks(154) + "q quit"
			checkLinkRows(t, textOf(s.Draw(160, 37, Look{})), tt.want)
			if got := s.Copies("c"); got != "" {
				t.Errorf("Copies(c) = %q at a failed step, want nothing", got)
			}
		})
	}
}

func TestAtSetupsLeastTheLinkPagesLineWrapsWholeAndThePageScrolls(t *testing.T) {
	s := linkScreen()
	laid := s.shown(Look{}).content(95)
	var put []string
	for y := range len(laid.whole.cells) {
		row := rowOf(laid.whole, y)
		if strings.HasPrefix(row, "      ") && !strings.HasPrefix(row, "       ") {
			put = append(put, strings.TrimPrefix(row, "      "))
		}
		if len([]rune(row)) > 95 {
			t.Errorf("row %d is %d cells wide, past 95: %q", y, len([]rune(row)), row)
		}
	}
	if got := strings.Join(put, ""); strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(shellLine+binDir, " ", "") {
		t.Errorf("what's to put in place reads, joined,\n%q\nwant the line and the directory whole,\n%q", got, shellLine+binDir)
	}
	if len(put) < 3 {
		t.Errorf("what's to put in place takes %d rows at 95 columns, want the line wrapped", len(put))
	}
	if got := s.Scrolling(95, 25); got.Most == 0 {
		t.Errorf("at 95×25, the page scrolls %+v, want it taller than its rows, so it scrolls", got)
	}
}
