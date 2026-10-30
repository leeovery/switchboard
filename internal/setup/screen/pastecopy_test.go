package screen

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// madeWhole is a setup token as setup-token makes one, 108 characters, a
// placeholder, its hyphens breaking it where the card wraps it.
var madeWhole = "sk-ant-oat01-" + strings.Repeat("Ab3_x-Z9q", 11)[:93] + "AA"

// pastedThroughSetup are steps that answer as fakeSteps does, but for
// PasteToken, which setup's own answers, its accounts a config file and a
// state directory of the test's, its check of a token the test's tokenAPI.
type pastedThroughSetup struct {
	*fakeSteps
	setup *setup.Setup
}

func (s pastedThroughSetup) PasteToken(ctx context.Context, f setup.TokenFor, pasted string) (setup.Tokened, error) {
	return s.setup.PasteToken(ctx, f, pasted)
}

// cardOfMadeToken is frame 3's accounts, notes under their table, on a
// terminal of the size given, t pressed on personal, its making ending as
// ends has it, the card saying so, its token in what it shows; and the steps,
// which save a pasted token as setup does.
func cardOfMadeToken(t *testing.T, width, height int, ends setup.Tokened) (*harness, pastedThroughSetup) {
	t.Helper()
	fake := frame3Steps()
	fake.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &ends}
	real := newRegistered(t, threeAccounts).setup
	real.Accounts.Prober = func(string) accounts.Prober { return tokenAPI{} }
	steps := pastedThroughSetup{fakeSteps: fake, setup: real}
	h := accountsHarness(t, fake)
	cfg := h.config(dashboard.SetupAccounts)
	cfg.Steps = steps
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: width, Height: height})
	h.start()
	h.pressAndDeliver("down")
	h.pressAndDeliver("t")
	h.steps.nextMake(t)
	h.hearAll(3)
	return h, steps
}

// screenRows are the screen as it's drawn, without colour, each row padded
// with blanks to its width, as a terminal holds it.
func screenRows(h *harness, width int) [][]rune {
	var rows [][]rune
	for row := range strings.SplitSeq(ansi.Strip(h.model.View().Content), "\n") {
		r := []rune(row)
		rows = append(rows, append(r, []rune(strings.Repeat(" ", max(width-len(r), 0)))...))
	}
	return rows
}

// tokenRunes are what a setup token is made of.
func tokenRune(r rune) bool {
	return r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// tokenAt is where the card shows a token, broken across rows: its first
// row and its last; the column it starts on, after the gutter, on each,
// and the one after its last character, on its last; and the columns of the
// card's left edge, its gutter and its right edge.
type tokenAt struct {
	first, last, from, to int
	left, gutter, right   int
}

// locate is where rows show token, as the card shows it after its gutter,
// failing the test where they don't show it whole. It says where, and how
// much, never what's there.
func locate(t *testing.T, rows [][]rune, token string) tokenAt {
	t.Helper()
	at := tokenAt{first: -1}
	for y, row := range rows {
		if i := strings.Index(string(row), "sk-ant-"); i >= 0 {
			at.first, at.from = y, len([]rune(string(row)[:i]))
			break
		}
	}
	if at.first < 0 {
		t.Fatal("no row shows the token's start")
	}
	seen := ""
	for y := at.first; y < len(rows) && seen != token; y++ {
		end := at.from
		for end < len(rows[y]) && tokenRune(rows[y][end]) {
			end++
		}
		seen += string(rows[y][at.from:end])
		at.last, at.to = y, end
		if !strings.HasPrefix(token, seen) {
			t.Fatalf("row %d, from column %d, doesn't go on with the token's %d characters before it", y, at.from, len(seen)-(end-at.from))
		}
	}
	if seen != token {
		t.Fatalf("the rows show %d of the token's %d characters", len(seen), len(token))
	}
	row := rows[at.first]
	at.gutter = at.from - 2
	at.left = at.gutter - 1
	for at.left >= 0 && row[at.left] != '│' {
		at.left--
	}
	at.right = at.from
	for at.right < len(row) && row[at.right] != '│' {
		at.right++
	}
	if row[at.gutter] != '│' || at.left < 0 || at.right == len(row) {
		t.Fatalf("row %d doesn't show the card's edges and its gutter round the token: left %d, gutter %d, right %d", at.first, at.left, at.gutter, at.right)
	}
	return at
}

// drag is how a terminal copies rows over a token: a line at a time, from
// its first character to its last, the first row from there to its end, the
// rows between whole and the last from its start, or, whole, every row it's
// on, whole; or, where block is set, the same columns of every row it's on,
// from a to b, as block has them. Each row's trailing blanks are kept, as
// some terminals keep them, or trimmed, as others do.
type drag struct {
	name           string
	whole, trimmed bool
	block          func(at tokenAt) (a, b int)
}

// copyOff is what d copies of rows over the token at.
func (d drag) copyOff(rows [][]rune, at tokenAt) string {
	var lines []string
	for y := at.first; y <= at.last; y++ {
		line := rows[y]
		switch {
		case d.block != nil:
			a, b := d.block(at)
			line = line[a:b]
		case d.whole:
		case y == at.first && y == at.last:
			line = line[at.from:at.to]
		case y == at.first:
			line = line[at.from:]
		case y == at.last:
			line = line[:at.to]
		}
		text := string(line)
		if d.trimmed {
			text = strings.TrimRight(text, " ")
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

// drags are the ways a test copies the token off its card: a line at a
// time, from its first character to its last, or its rows whole; and as a
// block, inside the card, from its gutter to its right edge, from edge to
// edge, and from just after its gutter; each trailing blanks kept and
// trimmed.
func drags() []drag {
	ways := []drag{
		{name: "start to end"},
		{name: "whole rows", whole: true},
		{name: "a block inside the card", block: func(at tokenAt) (int, int) { return at.left + 1, at.right }},
		{name: "a block from the gutter to the right edge", block: func(at tokenAt) (int, int) { return at.gutter, at.right + 1 }},
		{name: "a block from edge to edge", block: func(at tokenAt) (int, int) { return at.left, at.right + 1 }},
		{name: "a block from just after the gutter", block: func(at tokenAt) (int, int) { return at.gutter + 1, at.right }},
	}
	var all []drag
	for _, d := range ways {
		trimmed := d
		trimmed.name, trimmed.trimmed = d.name+", trimmed", true
		all = append(all, d, trimmed)
	}
	return all
}

// A token made but unsaved, copied off its card, is taken whole by the
// paste card, whatever the page shows either side of the card, and whatever
// the terminal's size: a line at a time, as a terminal copies by default,
// from its first character to its last, or its rows whole; or a block,
// inside the card, from its gutter to its right edge, from edge to edge, or
// from just after its gutter; trailing blanks kept or trimmed; through the
// paste card's field, which leaves the line breaks out, and given to setup
// with them. A copy holding the token
// twice is refused. On the phone's 73×61, setup says to make the terminal
// larger, so there's no card to copy from.
func TestATokenCopiedOffItsCardIsTakenWhole(t *testing.T) {
	ends := map[string]setup.Tokened{
		"unread":    {Result: setup.TokenUnread, Shown: []string{"Your OAuth token (valid for 1 year):", madeWhole, "Store this token securely. You won't be able to see it again."}},
		"not saved": {Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{madeWhole}},
	}
	sizes := []Size{{Width: 160, Height: 37}, {Width: 160, Height: 40}, {Width: 140, Height: 37}, {Width: 120, Height: 37}, {Width: 120, Height: 40}, {Width: 100, Height: 37}}
	for name, ended := range ends {
		for _, size := range sizes {
			for _, d := range drags() {
				t.Run(fmt.Sprintf("%s, %d×%d, %s", name, size.Width, size.Height, d.name), func(t *testing.T) {
					h, steps := cardOfMadeToken(t, size.Width, size.Height, ended)
					rows := screenRows(h, size.Width)
					clip := d.copyOff(rows, locate(t, rows, madeWhole))

					got, err := steps.setup.PasteToken(t.Context(), setup.TokenFor{Account: config.NewAccount{ID: "side"}}, clip)
					if err != nil || got.Result != setup.TokenSaved {
						t.Errorf("given to setup, its line breaks in it, the copy was %v (%v, %q), want it saved", got.Result, err, got.Why)
					} else if token, err := steps.setup.Accounts.Tokens.Read("side"); err != nil || token.Reveal() != madeWhole {
						t.Errorf("given to setup, its line breaks in it, side's token file: %v, holding the token: %t; want it holding the token whole", err, err == nil && token.Reveal() == madeWhole)
					}

					h.pressAndDeliver("p")
					h.deliver(tea.PasteMsg{Content: clip})
					h.pressAndDeliver("enter")
					if h.model.paste != nil {
						_, rows := h.card()
						t.Fatalf("pasted, the card reads\n%s\nwant the token taken, the card closed", strings.Join(rows, "\n"))
					}
					if token, err := steps.setup.Accounts.Tokens.Read("personal"); err != nil || token.Reveal() != madeWhole {
						t.Errorf("pasted, personal's token file: %v, holding the token: %t; want it holding the token whole", err, err == nil && token.Reveal() == madeWhole)
					}
				})
			}
		}
	}
	t.Run("twice", func(t *testing.T) {
		h, steps := cardOfMadeToken(t, 120, 37, ends["unread"])
		rows := screenRows(h, 120)
		clip := drag{trimmed: true}.copyOff(rows, locate(t, rows, madeWhole))
		h.pressAndDeliver("p")
		h.deliver(tea.PasteMsg{Content: clip + "\n" + clip})
		h.pressAndDeliver("enter")
		if h.model.paste == nil || h.model.paste.why == "" {
			t.Fatalf("the token pasted twice, the paste card is %s, saying why: %t; want it refused", pasteSaid(h.model.paste), h.model.paste != nil && h.model.paste.why != "")
		}
		if _, err := steps.setup.Accounts.Tokens.Read("personal"); err == nil {
			t.Error("the token pasted twice, personal's token file holds a token, want nothing saved")
		}
	})
	t.Run("on the phone", func(t *testing.T) {
		h, _ := cardOfMadeToken(t, 73, 61, ends["unread"])
		if got := h.rows()[0]; got != "Make the terminal larger." {
			t.Errorf("at 73×61, the screen's first row reads %q, want it saying to make the terminal larger", got)
		}
	})
}
