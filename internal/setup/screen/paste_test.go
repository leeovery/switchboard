package screen

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/tokens"
)

// pasteCardOpened is the paste card as it opens, its field empty, typed in.
var pasteCardOpened = []string{
	"",
	"       Paste the token you made with claude setup-token",
	"       Setup checks it with the API, then saves it, a year from today.",
	"",
	"       Token" + strings.Repeat(" ", 31) + "shown as dots",
	"",
	"       A token copied across lines is joined up, the card's edges left out.",
	"",
	keysOf("enter save   esc cancel"),
}

// notMade is a making that ended as setup-token's error, saving nothing.
var notMade = setup.Tokened{Result: setup.TokenNotMade, Shown: []string{"OAuth error: Request failed with status code 400"}}

// pastingFor is frame 3's accounts, t pressed on personal, its making ending
// as ends has it, then p pressed at the card that says so.
func pastingFor(t *testing.T, ends setup.Tokened) *harness {
	t.Helper()
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &ends}, 1)
	h.steps.nextMake(t)
	h.hearAll(3)
	h.pressAndDeliver("p")
	return h
}

// checkPasteRow checks the paste card's row, from its field's, reads want.
func checkPasteRow(t *testing.T, h *harness, below int, want string) {
	t.Helper()
	_, rows := h.card()
	if len(rows) <= 4+below || rows[4+below] != want {
		t.Errorf("the card reads\n%s\nwant its row %d under the field\n%q", strings.Join(rows, "\n"), below, want)
	}
}

// p at every card that ends a making and offers it opens the paste card
// for the same account, in the token card's place, its field empty: after
// setup-token's error, a token refused, another account's, no Claude Code,
// a token made but unread, or not saved.
func TestPOpensThePasteCardFromEachEndThatOffersIt(t *testing.T) {
	ends := map[string]setup.Tokened{
		"setup-token's error": notMade,
		"refused":             {Result: setup.TokenRefused},
		"another account's":   {Result: setup.TokenSameAccount, SameAs: "work"},
		"no Claude Code":      {Result: setup.NoClaudeCode, Why: "can't find claude"},
		"unread":              {Result: setup.TokenUnread, Shown: []string{"test-token-personal-made"}},
		"not saved":           {Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{"test-token-personal-made"}},
		"not saved, unread":   {Result: setup.TokenNotSaved, Why: "read the token: setup-token stopped"},
	}
	for name, ended := range ends {
		t.Run(name, func(t *testing.T) {
			h := pastingFor(t, ended)
			if h.model.token != nil || h.model.paste == nil {
				t.Fatalf("p left the token card open: %t, the paste card %s; want the paste card in its place", h.model.token != nil, pasteSaid(h.model.paste))
			}
			if got := h.model.paste.f; got.Account != (config.NewAccount{ID: "personal"}) || got.Add {
				t.Errorf("the paste card saves the token for %s, adding it: %t; want personal's, in its place, as the token card did", got.Account.ID, got.Add)
			}
			checkCard(t, h, "PASTE A TOKEN · PERSONAL", pasteCardOpened)
		})
	}
}

// p before setup-token says it's made the token stops it, as esc does, the
// card saying so, and once it has stopped, the paste card opens in its
// place, nothing saved; at its start, and as it waits on the approval.
func TestPBeforeTheTokensMadeStopsItThenOpensThePasteCard(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []setup.Making
	}{{name: "starting"}, {name: "waiting", steps: []setup.Making{privately, approving}}} {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: tt.steps}, 1)
			h.hearAll(len(tt.steps))
			h.pressAndDeliver("p")
			checkCard(t, h, "NEW TOKEN · PERSONAL", []string{
				"",
				"  ●∙∙  Stopping claude setup-token",
				"",
				"       Nothing is saved, unless it had already created the token.",
				"",
			})
			// The making ends, cancelled, only once its context has.
			h.hear()
			if h.model.token != nil || h.model.paste == nil || h.model.paste.f.Account.ID != "personal" {
				t.Fatalf("stopped, the token card is open: %t, the paste card %s; want personal's paste card alone", h.model.token != nil, pasteSaid(h.model.paste))
			}
			checkCard(t, h, "PASTE A TOKEN · PERSONAL", pasteCardOpened)
			if got := h.steps.replacedOf("personal"); got != 0 {
				t.Errorf("personal's token was replaced %d times, want nothing saved of the making", got)
			}
		})
	}
}

// A making p stopped that had made the token, as a stop that lands as it
// does, keeps it: the token's saved, as a made one is, and no paste card
// opens.
func TestATokenMadeAsPStopsItIsSavedAndNothingPasted(t *testing.T) {
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &saved}, 1)
	h.hearAll(2)
	h.pressAndDeliver("p")
	h.hear()
	if h.model.token != nil || h.model.paste != nil {
		t.Errorf("the token saved, the token card is open: %t, the paste card %s; want both closed", h.model.token != nil, pasteSaid(h.model.paste))
	}
	if got := h.steps.replacedOf("personal"); got != 1 {
		t.Errorf("personal's token was replaced %d times, want the token made saved", got)
	}
}

// The paste card's field draws a dot for each glyph pasted or typed, never
// the token, however it's pasted, its line breaks left out, as the field's.
func TestThePasteCardDrawsDotsNeverTheToken(t *testing.T) {
	h := pastingFor(t, notMade)
	const start, rest = "sk-ant-oat01-test-token-personal-", "0123456789"
	h.deliver(tea.PasteMsg{Content: "│ " + start + "\n│ " + rest + "\n"})
	h.typeIn("ab")
	h.pressAndDeliver("backspace")
	value := "│ " + start + "│ " + rest + "a"
	if got := h.model.paste.token.value(); got != value {
		t.Fatalf("the field holds %q, want %q, the line breaks left out", got, value)
	}
	checkPasteRow(t, h, 0, "       Token     …"+strings.Repeat("•", 21)+"    shown as dots")
	screen := strings.Join(h.rows(), "\n")
	for _, part := range []string{"sk-ant", "test-token", rest} {
		if strings.Contains(screen, part) {
			t.Errorf("the screen reads\n%s\nwant nothing of %q pasted", screen, part)
		}
	}
}

// enter saves the token pasted, as the field holds it, with PasteToken,
// compared with the accounts read: the card checks it, taking no key, its
// spinner moving; once it's saved, the card closes, the accounts are read
// again, and the account's row is chosen, its token a year from today. A
// token in an account's place is the account's; one for an account being
// added adds it.
func TestEnterSavesTheTokenPasted(t *testing.T) {
	t.Run("in an account's place", func(t *testing.T) {
		steps := frame3Steps()
		steps.checked["work"] = setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}
		h := accountsHarness(t, steps)
		h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &notMade}
		h.start()
		h.pressAndDeliver("down")
		h.pressAndDeliver("t")
		h.hearAll(3)
		h.pressAndDeliver("p")
		h.typeIn("test-token-pasted")
		timers := len(h.timers)

		answered := h.press("enter")
		if !h.model.paste.checking {
			t.Fatal("enter left the card unchecked, want it checking the token")
		}
		if row := h.cardRow("●∙∙  Checking the token and saving it"); row == "" {
			_, rows := h.card()
			t.Errorf("checking, the card reads\n%s\nwant it saying so", strings.Join(rows, "\n"))
		}
		if !slices.ContainsFunc(h.timers[timers:], func(tm timer) bool { _, ok := tm.msg.(spinMsg); return ok }) {
			t.Error("checking, no spinner move was armed")
		}
		for _, k := range []string{"x", "esc", "enter"} {
			h.pressAndDeliver(k)
		}
		if h.model.paste == nil || h.model.paste.token.value() != "test-token-pasted" || len(h.steps.pastes) != 1 {
			t.Fatalf("keys while it's checked changed the card to %s, pasted %d times; want none taken", pasteSaid(h.model.paste), len(h.steps.pastes))
		}
		h.deliver(answered...)

		want := fakePaste{f: setup.TokenFor{Account: config.NewAccount{ID: "personal"}, Organizations: setup.Organizations{"work": "org-work"}}, pasted: "test-token-pasted"}
		if got := h.steps.pastes[0]; !reflect.DeepEqual(got, want) {
			t.Errorf("PasteToken was asked %s, want %s", pastesSaid([]fakePaste{got}), pastesSaid([]fakePaste{want}))
		}
		if h.model.paste != nil {
			t.Fatalf("saved, the paste card is %s, want it closed", pasteSaid(h.model.paste))
		}
		if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal") || !strings.HasSuffix(got, "expires 27 Oct 2027  ·  365 days left") {
			t.Errorf("personal's row reads %q, want it chosen, its token a year from today", got)
		}
	})
	t.Run("adding the account", func(t *testing.T) {
		h := accountsHarness(t, frame3Steps())
		h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &notMade}
		h.start()
		for range 3 {
			h.pressAndDeliver("down")
		}
		h.pressAndDeliver("enter")
		h.typeIn("team")
		h.pressAndDeliver("enter")
		h.hearAll(3)
		h.pressAndDeliver("p")
		checkCard(t, h, "PASTE A TOKEN · TEAM", pasteCardOpened)
		h.typeIn("test-token-team")
		h.pressAndDeliver("enter")

		if got := h.steps.pastes; len(got) != 1 || got[0].f.Account.ID != "team" || !got[0].f.Add {
			t.Fatalf("PasteToken was asked %s, want team's token, to add it", pastesSaid(got))
		}
		if review := h.model.status.review; review == nil || len(review.Accounts) != 4 || h.model.chosen != 3 {
			t.Fatalf("saved, the review is %+v, row %d chosen; want team added, its row chosen", review, h.model.chosen)
		}
		if got := h.rows()[firstRow+6]; !strings.HasPrefix(got, "▌ 4  team") || !strings.HasSuffix(got, "expires 27 Oct 2027  ·  365 days left") {
			t.Errorf("team's row reads %q, want it chosen, its token a year from today", got)
		}
	})
}

// A paste setup saves nothing of says why under the field, in setup's
// words, the field emptied, the card open to paste again, till the field
// changes, a paste again holding the second alone: what's pasted no token,
// a token the API refuses, or doesn't answer for, and another account's.
func TestAPasteSavingNothingSaysWhyUnderTheField(t *testing.T) {
	tests := []struct {
		name string
		ends setup.Tokened
		want []string
	}{
		{
			name: "no token",
			ends: setup.Tokened{Result: setup.TokenNotGiven, Why: "what's pasted holds more than the token: paste the token alone, as claude setup-token showed it"},
			want: []string{"                 what's pasted holds more than the token: paste the token", "                 alone, as claude setup-token showed it"},
		},
		{
			name: "refused",
			ends: setup.Tokened{Result: setup.TokenRefused, Why: "the API refused the token (HTTP 401 · Invalid bearer token), so it isn't saved: check it was copied whole"},
			want: []string{"                 the API refused the token (HTTP 401 · Invalid bearer", "                 token), so it isn't saved: check it was copied whole"},
		},
		{
			name: "unanswered",
			ends: setup.Tokened{Result: setup.TokenUnanswered, Why: "the API didn't answer (HTTP 529 · overloaded), so it isn't saved: paste it again in a moment"},
			want: []string{"                 the API didn't answer (HTTP 529 · overloaded), so it isn't", "                 saved: paste it again in a moment"},
		},
		{
			name: "another account's",
			ends: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"},
			want: []string{"                 work has that subscription already, so the token isn't", "                 saved"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := pastingFor(t, notMade)
			h.steps.pasteEnds = &tt.ends
			h.typeIn("test-token-pasted")
			h.pressAndDeliver("enter")
			if h.model.paste == nil || h.model.paste.checking {
				t.Fatalf("refused, the paste card is %s, want it open, checked", pasteSaid(h.model.paste))
			}
			for i, line := range tt.want {
				checkPasteRow(t, h, 1+i, line)
			}
			if got := h.steps.replacedOf("personal"); got != 0 {
				t.Errorf("personal's token was replaced %d times, want nothing saved", got)
			}
			if got := h.model.paste.token.value(); got != "" {
				t.Errorf("refused, the field holds %d bytes, want it emptied, to paste again", len(got))
			}
			h.deliver(tea.PasteMsg{Content: "test-token-again"})
			checkPasteRow(t, h, 1, "")
			h.steps.pasteEnds = nil
			h.pressAndDeliver("enter")
			if got := h.steps.pastes; len(got) != 2 || got[1].pasted != "test-token-again" {
				t.Errorf("pasted again, PasteToken was asked %s, want the second paste alone", pastesSaid(got))
			}
		})
	}
}

// A paste that fails, as where the token can't be written, fails the step,
// not saved, the card closed.
func TestAPasteThatFailsFailsTheStep(t *testing.T) {
	errNotSaved := errors.New("write the token file: no space left on device")
	h := pastingFor(t, notMade)
	h.steps.pasteErr = errNotSaved
	h.typeIn("test-token-pasted")
	h.pressAndDeliver("enter")
	if h.model.paste != nil {
		t.Errorf("failed, the paste card is %s, want it closed", pasteSaid(h.model.paste))
	}
	if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNotSaved}) {
		t.Errorf("setup would be left %+v, want failing at Accounts", got)
	}
}

// While any account's token is still being checked, the paste card's enter
// waits, doing nothing, so the token's compared with every organization the
// run will know; once the checks are in, it saves the token, compared with
// them all.
func TestThePasteCardsEnterWaitsForTheAccountsChecks(t *testing.T) {
	steps := frame3Steps()
	steps.checked["work"] = setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}
	h := accountsHarness(t, steps)
	checks := h.held()
	h.model = h.model.pasting(setup.TokenFor{Account: config.NewAccount{ID: "personal"}}, nil)
	h.typeIn("test-token-pasted")
	h.pressAndDeliver("enter")
	if h.model.paste.checking || len(h.steps.pastes) != 0 {
		t.Fatalf("with the checks out, the card is checking: %t, pasted %s; want nothing begun", h.model.paste.checking, pastesSaid(h.steps.pastes))
	}
	h.deliver(checks...)
	h.pressAndDeliver("enter")
	if got := h.steps.pastes; len(got) != 1 || !reflect.DeepEqual(got[0].f.Organizations, setup.Organizations{"work": "org-work"}) {
		t.Errorf("PasteToken was asked %s, want the token compared with work's organization", pastesSaid(got))
	}
}

// esc closes the paste card, saving nothing, with no token card behind it:
// an account being added isn't added.
func TestEscClosesThePasteCardSavingNothing(t *testing.T) {
	h := firstRun(t)
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &notMade}
	h.typeIn("work")
	h.pressAndDeliver("enter")
	h.hearAll(3)
	h.pressAndDeliver("p")
	h.typeIn("test-token-work")
	h.pressAndDeliver("esc")
	if h.model.carded() {
		t.Errorf("esc left a card open: add %t, token %t, paste %s", h.model.add != nil, h.model.token != nil, pasteSaid(h.model.paste))
	}
	if review := h.model.status.review; len(h.steps.pastes) != 0 || review == nil || len(review.Accounts) != 0 {
		t.Errorf("esc pasted %s, the review is %+v; want nothing saved, no account added", pastesSaid(h.steps.pastes), review)
	}
}

// pastedStart and pastedRest are a setup token's two parts, as the unread
// card breaks it, a placeholder.
const (
	pastedStart = "sk-ant-oat01-test-token-side-"
	pastedRest  = "0123456789abcdef"
)

// Through setup's own steps: a token pasted is checked with the API, then
// saved with its year from today, the row showing it, taken whole where it
// was copied across the card's lines, its gutter among it; one the API
// refuses isn't saved, nor one setup can't tell from the words round it,
// the card saying why, never what's pasted, which neither the screen nor
// the log shows.
func TestAPastedTokenThroughSetupIsSavedOnlyOnceItsChecked(t *testing.T) {
	tests := []struct {
		name, paste string
		// saved is the token saved, "" where none is, the card saying says.
		saved, says string
	}{
		{name: "saved", paste: "test-token-side", saved: "test-token-side"},
		{name: "across the card's lines", paste: "│ " + pastedStart + "\n│ " + pastedRest + "\n", saved: pastedStart + pastedRest},
		{name: "refused", paste: refusedByAPI, says: "the API refused the token (HTTP 401 · Invalid bearer"},
		{name: "words round it", paste: "Then press p: " + pastedStart + pastedRest, says: "what's pasted holds more than the token: paste the token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			steps := newRegistered(t, threeAccounts)
			s := steps.setup
			s.Accounts.Prober = func(string) accounts.Prober { return tokenAPI{} }
			h := registeredHarness(t, steps)
			h.model = h.model.pasting(setup.TokenFor{Account: config.NewAccount{ID: "side"}}, nil)
			h.deliver(tea.PasteMsg{Content: tt.paste})
			h.pressAndDeliver("enter")

			token, err := s.Accounts.Tokens.Read("side")
			if tt.saved == "" {
				if row := h.cardRow("               " + tt.says); row == "" {
					_, rows := h.card()
					t.Errorf("the card reads\n%s\nwant it saying %q", strings.Join(rows, "\n"), tt.says)
				}
				if !errors.Is(err, tokens.ErrMissing) {
					t.Errorf("side's token file: %v, want none, nothing saved", err)
				}
				for _, part := range []string{pastedStart, pastedRest, refusedByAPI} {
					if screen := strings.Join(h.rows(), "\n"); strings.Contains(screen, part) {
						t.Errorf("the screen reads\n%s\nwant nothing of what's pasted", screen)
					}
					if strings.Contains(log.String(), part) {
						t.Errorf("the log reads\n%s\nwant nothing of what's pasted", log)
					}
				}
				return
			}
			if h.model.paste != nil {
				t.Fatalf("saved, the paste card is %s, want it closed", pasteSaid(h.model.paste))
			}
			if err != nil || token.Reveal() != tt.saved {
				t.Fatalf("side's token file: %v, want it holding the token pasted", err)
			}
			if taken, err := s.Accounts.Tokens.Taken("side", token); err != nil || !taken.Equal(start) {
				t.Errorf("side's token was taken %v (%v), want today, %v, so it lasts a year from today", taken, err, start)
			}
			rows := h.rows()
			if i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "▌ 3  side") }); i < 0 || !strings.HasSuffix(rows[i], "expires 27 Oct 2027  ·  365 days left") {
				t.Errorf("the page reads\n%s\nwant side's row chosen, its token a year from today", strings.Join(rows, "\n"))
			}
			if strings.Contains(log.String(), tt.saved) {
				t.Errorf("the log reads\n%s\nwant nothing of the token", log)
			}
		})
	}
}

// The paste card's field takes what a page's does, the card its own keys:
// q, j and k type into it, and no page key acts.
func TestThePasteCardTakesEveryKey(t *testing.T) {
	h := pastingFor(t, notMade)
	for _, k := range []string{"q", "j", "k", "e", "r", "t", "p", "pgdown"} {
		h.pressAndDeliver(k)
	}
	if got := h.model.paste.token.value(); got != "qjkertp" {
		t.Errorf("the field holds %q, want every key that types typed into it", got)
	}
	if h.model.edit != nil || h.model.remove != nil || h.model.token != nil || h.model.chosen != 1 || h.model.page().At != dashboard.SetupAccounts {
		t.Errorf("with the paste card open, a page key acted: edit %t, remove %t, token %t, row %d chosen", h.model.edit != nil, h.model.remove != nil, h.model.token != nil, h.model.chosen)
	}
}

// pasteSaid says what the paste card holds, for a test's failure: whether
// it's open, whose token it saves, how many bytes its field holds, and
// whether it's checking, never what's in it.
func pasteSaid(c *pasteCard) string {
	if c == nil {
		return "closed"
	}
	return fmt.Sprintf("open on %s, %d bytes in its field, checking: %t", c.f.Account.ID, len(c.token.value()), c.checking)
}

// pastesSaid says what PasteToken was asked, for a test's failure: each
// paste's account, whether it's added, how many bytes were pasted, and the
// organizations it's compared with, never what was pasted.
func pastesSaid(pastes []fakePaste) string {
	said := make([]string, len(pastes))
	for i, p := range pastes {
		said[i] = fmt.Sprintf("%s (adding: %t, %d bytes, compared with %v)", p.f.Account.ID, p.f.Add, len(p.pasted), p.f.Organizations)
	}
	return "[" + strings.Join(said, "; ") + "]"
}
