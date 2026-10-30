package screen

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/tokens"
)

// tokening is a harness of frame 3's accounts, started, their tokens
// checked, MakeToken making as given, t pressed on the row downs rows down.
func tokening(t *testing.T, making fakeMaking, downs int) *harness {
	t.Helper()
	h := accountsHarness(t, frame3Steps())
	h.steps.making = making
	h.start()
	for range downs {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("t")
	return h
}

// checkCard checks the card open is titled title and reads want, each row
// inside its line, the key line empty.
func checkCard(t *testing.T, h *harness, title string, want []string) {
	t.Helper()
	got, rows := h.card()
	if got != title {
		t.Fatalf("the card open is %q, want %q", got, title)
	}
	if !slices.Equal(rows, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if got := h.rows()[keyRow]; got != "" {
		t.Errorf("with the card open, the key line reads %q, want it empty: the card's keys are its own", got)
	}
}

// keysOf is the card's keys row, its keys ending where its content does.
func keysOf(keys string) string {
	return strings.Repeat(" ", 76-len([]rune(keys))) + keys
}

// t on an account's row makes its token, saved in its old one's place, the
// token card opening on setup-token starting; compared with the
// organizations of the accounts read, as setup has them.
func TestTMakesTheChosenAccountsTokenInTheTokenCard(t *testing.T) {
	steps := frame3Steps()
	steps.checked["work"] = setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}
	steps.checked["personal"] = setup.TokenStanding{State: setup.TokenWorking, Year: endingIn, Organization: "org-personal"}
	h := accountsHarness(t, steps)
	h.start()
	h.pressAndDeliver("down")
	h.pressAndDeliver("t")

	want := setup.TokenFor{Account: config.NewAccount{ID: "personal"}, Organizations: setup.Organizations{"work": "org-work", "personal": "org-personal"}}
	if got := h.steps.nextMake(t); !reflect.DeepEqual(got, want) {
		t.Errorf("MakeToken was asked %+v, want %+v", got, want)
	}
	checkCard(t, h, "NEW TOKEN · PERSONAL", []string{
		"",
		"  ●∙∙  Starting claude setup-token",
		"",
		"       Setup opens its sign-in page in the browser once it's ready.",
		"",
		keysOf("p paste a token   esc cancel"),
	})
}

// The card waits on the approval in the browser as the address opened, the
// time it's waited counted from when it opened, by the screen's clock: in a
// private window, as frame 2 has it; as it is, saying nothing more; and not
// opened, the address shown to paste, c copying it in o's place.
func TestTheTokenCardWaitsAsTheAddressOpened(t *testing.T) {
	asItIs := setup.Making{Step: setup.AddressReady, Address: signIn, Opened: setuptoken.OpenedInBrowser, For: "Personal"}
	unopened := setup.Making{Step: setup.AddressReady, Address: signIn, Opened: setuptoken.NotOpened, Browser: "Safari", For: "Personal", SignedIn: "Work"}
	waitedFor := "  ●∙∙  Waiting for you to approve it in the browser                     0:21"
	saves := "       Setup saves the token automatically. There's nothing to copy."
	tests := []struct {
		name  string
		ready setup.Making
		want  []string
	}{
		{name: "in a private window", ready: privately, want: []string{
			"", waitedFor, "",
			"       Your browser is signed in to Work, so setup opened a private Firefox",
			"       window. Sign in to Personal there, then approve.",
			"",
			"       Close any other private window first: they all share one sign-in.",
			"", saves, "",
			keysOf("o reopen window   p paste a token   esc cancel"),
		}},
		{name: "in the browser as it is", ready: asItIs, want: []string{
			"", waitedFor, "", saves, "",
			keysOf("o reopen window   p paste a token   esc cancel"),
		}},
		{name: "not opened", ready: unopened, want: []string{
			"", waitedFor, "",
			"       Your browser is signed in to Work, and setup couldn't open a private",
			"       Safari window. Open this address in one, sign in to Personal there,",
			"       then approve:",
			"",
			"       " + signIn.Reveal(),
			"", saves, "",
			keysOf("c copy the address   p paste a token   esc cancel"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: []setup.Making{tt.ready, approving}}, 1)
			h.hearAll(2)
			h.clock.now = start.Add(21 * time.Second)
			checkCard(t, h, "NEW TOKEN · PERSONAL", tt.want)
		})
	}
}

// o opens the address again, the way it opened, where it opened, a failure
// noted in the log, the card waiting on; c copies it, through the screen's
// one copied, where it didn't open, and nothing opens it again.
func TestOReopensTheAddressAndCCopiesItWhereItDidntOpen(t *testing.T) {
	t.Run("opened", func(t *testing.T) {
		log := logstest.Capture(t)
		h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}}, 1)
		h.hearAll(2)
		h.steps.reopenErr = errors.New("open the sign-in address in a private window of Firefox: exit status 1")
		if msgs := h.press("c"); len(msgs) > 0 {
			t.Errorf("c, where the address opened, sends %v, want nothing copied", msgs)
		}
		h.pressAndDeliver("o")
		if got := h.steps.reopens; len(got) != 1 || got[0].Address.Reveal() != signIn.Reveal() || got[0].Opened != setuptoken.OpenedPrivately {
			t.Errorf("Reopen was asked %+v, want the address as it opened", got)
		}
		if !log.Has("level=WARN", "couldn't open the sign-in address again", "exit status 1") {
			t.Errorf("the log reads\n%s\nwant why it couldn't open again", log)
		}
		if got := h.model.token.stage(); got != dashboard.TokenWaiting {
			t.Errorf("the card is at stage %d, want it waiting on", got)
		}
	})
	t.Run("not opened", func(t *testing.T) {
		unopened := privately
		unopened.Opened = setuptoken.NotOpened
		h := tokening(t, fakeMaking{steps: []setup.Making{unopened, approving}}, 1)
		h.hearAll(2)
		h.pressAndDeliver("o")
		if len(h.steps.reopens) > 0 {
			t.Errorf("o, where the address didn't open, had Reopen asked %+v", h.steps.reopens)
		}
		if msgs := h.press("c"); !clipboard(msgs, signIn.Reveal()) {
			t.Errorf("c sends %v, want the address copied", msgs)
		}
	})
}

// esc before setup-token says it's made the token stops it: the card says
// so, then closes once it's stopped, nothing saved nor read again; at its
// start, and as it waits on the approval.
func TestEscBeforeTheTokensMadeStopsItSavingNothing(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []setup.Making
	}{{name: "starting"}, {name: "waiting", steps: []setup.Making{privately, approving}}} {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: tt.steps}, 1)
			h.hearAll(len(tt.steps))
			reviews := h.steps.reviews
			h.pressAndDeliver("esc")
			checkCard(t, h, "NEW TOKEN · PERSONAL", []string{
				"",
				"  ●∙∙  Stopping claude setup-token",
				"",
				"       Nothing is saved, unless it had already created the token.",
				"",
			})
			h.hear()
			if h.model.token != nil {
				t.Fatalf("stopped, the token card is %+v, want it closed", h.model.token)
			}
			if h.steps.replaced["personal"] != 0 || h.steps.reviews != reviews {
				t.Errorf("stopped, personal's token was replaced %d times and the accounts read %d more; want nothing saved", h.steps.replaced["personal"], h.steps.reviews-reviews)
			}
		})
	}
}

// Once setup-token says it's made the token, the card checks and saves it:
// esc changes nothing it shows, nor stops the making, as the token lasts a
// year on the account; once it's saved, the card closes, the accounts are
// read again, and the row chosen stays on the account, its token a year
// from today.
func TestATokenMadeIsKeptThroughEsc(t *testing.T) {
	release, ended := make(chan struct{}), make(chan bool, 1)
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving, madeIt}, release: release, ended: ended}, 1)
	h.hearAll(3)
	checking := []string{
		"",
		"  ●∙∙  Checking the token and saving it",
		"",
		"       claude setup-token created it. Setup checks it with the API, then",
		"       saves it, a year from today.",
		"",
		"       It's saved however you leave setup now, as it lasts a year on the",
		"       account.",
		"",
	}
	checkCard(t, h, "NEW TOKEN · PERSONAL", checking)
	h.pressAndDeliver("esc")
	checkCard(t, h, "NEW TOKEN · PERSONAL", checking)

	close(release)
	h.hear()
	if h.model.token != nil {
		t.Fatalf("saved, the token card is %+v, want it closed", h.model.token)
	}
	if sent(ended) {
		t.Error("esc at Checking ended the making's context, want it live, as the token's kept")
	}
	if got := h.steps.replacedOf("personal"); got != 1 {
		t.Errorf("personal's token was replaced %d times, want once, esc or not", got)
	}
	if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal") || !strings.HasSuffix(got, "expires 27 Oct 2027  ·  365 days left") {
		t.Errorf("personal's row reads %q, want it chosen, its token a year from today", got)
	}
}

// sent is what was sent on ch, which must hold it already.
func sent[T any](ch <-chan T) T {
	select {
	case v := <-ch:
		return v
	default:
		panic("nothing on the channel, want what was sent before")
	}
}

// esc as a making waits its turn among setup's writing actions stops its
// wait: the card closes, the making cancelled, never begun.
func TestEscAsAMakingWaitsItsTurnStopsIt(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	// A write holds the turn, as one that hangs would.
	h.model.jobs.writing <- struct{}{}
	defer func() { <-h.model.jobs.writing }()
	h.pressAndDeliver("t")
	h.pressAndDeliver("esc")
	if got := h.model.token.stage(); got != dashboard.TokenStopping {
		t.Fatalf("esc as the making waits its turn, the card is at stage %d, want it stopping", got)
	}
	h.hear()
	if h.model.token != nil {
		t.Errorf("the making stopped as it waited its turn, the card is %+v, want it closed", h.model.token)
	}
	select {
	case f := <-h.steps.asked:
		t.Errorf("MakeToken was asked %+v, want nothing made", f)
	default:
	}
}

// While any account's token is still being checked, t and the add card's
// enter wait, doing nothing, so a token made is compared with every
// organization the run will know; once the checks are in, each makes the
// token, compared with them all.
func TestATokensMakingWaitsForTheAccountsChecks(t *testing.T) {
	tests := []struct {
		name string
		// press presses the keys that make the token, from the first row.
		press func(h *harness)
		want  setup.TokenFor
	}{
		{name: "t", press: func(h *harness) { h.pressAndDeliver("t") }, want: setup.TokenFor{Account: config.NewAccount{ID: "work"}}},
		{
			name: "the add card's enter",
			press: func(h *harness) {
				if h.model.add == nil {
					for range 3 {
						h.pressAndDeliver("down")
					}
					h.pressAndDeliver("enter")
					h.typeIn("team")
				}
				h.pressAndDeliver("enter")
			},
			want: setup.TokenFor{Account: config.NewAccount{ID: "team"}, Add: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := frame3Steps()
			steps.checked["work"] = setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}
			steps.checked["personal"] = setup.TokenStanding{State: setup.TokenWorking, Year: endingIn, Organization: "org-personal"}
			h := accountsHarness(t, steps)
			var checks []tea.Msg
			for pending := h.run(h.model.Init()); len(pending) > 0; pending = pending[1:] {
				if _, ok := pending[0].(checkedMsg); ok {
					checks = append(checks, pending[0])
					continue
				}
				pending = append(pending, h.update(pending[0])...)
			}
			if len(checks) != 3 {
				t.Fatalf("held back %d checks, want every account's 3", len(checks))
			}

			tt.press(h)
			if h.model.token != nil || len(h.steps.adds) != 0 {
				t.Fatalf("with the checks out, the token card is %+v, accounts checked to add %v; want nothing begun", h.model.token, h.steps.adds)
			}
			h.deliver(checks...)
			tt.press(h)
			want := tt.want
			want.Organizations = setup.Organizations{"work": "org-work", "personal": "org-personal"}
			if got := h.steps.nextMake(t); !reflect.DeepEqual(got, want) {
				t.Errorf("MakeToken was asked %+v, want %+v", got, want)
			}
		})
	}
}

// Once a saved token's accounts are read again, whatever came of it, the
// account it was saved for is forgotten: a read that fails leaves the row
// chosen where it is, and none after moves it there.
func TestARereadThatFailsForgetsTheTokensAccount(t *testing.T) {
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, ends: &saved}, 1)
	h.hearAll(4)
	h.steps.reviewErr = errors.New("read the config: permission denied")
	wait := h.waits[0]
	h.waits = h.waits[1:]
	var reread []tea.Msg
	for _, msg := range h.run(wait) {
		reread = append(reread, h.update(msg)...)
	}
	h.pressAndDeliver("down")
	h.deliver(reread...)
	if h.model.madeFor != "" || h.model.chosen != 2 {
		t.Fatalf("the read again failed, the token's account is %q, row %d chosen; want it forgotten, side chosen still", h.model.madeFor, h.model.chosen)
	}
	h.steps.reviewErr = nil
	m, cmd := h.model.askAgain()
	h.model = m
	h.deliver(h.run(cmd)...)
	if h.model.chosen != 2 {
		t.Errorf("a later read chose row %d, want side's still", h.model.chosen)
	}
}

// q, at any stage of the card, does nothing, the making carrying on; ctrl+c
// leaves, as it always does.
func TestQDoesNothingAtTheTokenCardAndCtrlCLeaves(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []setup.Making
	}{{name: "waiting", steps: []setup.Making{privately, approving}}, {name: "the token made", steps: []setup.Making{privately, approving, madeIt}}} {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: tt.steps}, 1)
			h.hearAll(len(tt.steps))
			if quits(h.press("q")) || h.model.token == nil || h.model.token.stopping {
				t.Errorf("q quit, or left the card %+v; want it doing nothing", h.model.token)
			}
			if !quits(h.press("ctrl+c")) {
				t.Error("ctrl+c didn't quit with the token card open")
			}
		})
	}
}

// A making that saves nothing says why in the card, in its own words, laid
// out as frame 10's: setup-token's error, what it showed last; a token the
// API refuses; no Claude Code; another account's subscription, named; and a
// token made but unread, all setup-token showed, the token in it, enter
// making no other. enter tries again, making the token afresh for the same
// account; esc closes the card, nothing saved.
func TestAMakingThatSavedNothingSaysWhy(t *testing.T) {
	tests := []struct {
		name    string
		tokened setup.Tokened
		want    []string
		again   bool
	}{
		{
			name:    "setup-token's error",
			tokened: setup.Tokened{Result: setup.TokenNotMade, Shown: []string{" OAuth error: Request failed with status code 400", " Press Enter to retry."}},
			want: []string{
				"",
				"  ✗    The token wasn't created",
				"       claude setup-token stopped with an error. What it showed:",
				"",
				"       │ OAuth error: Request failed with status code 400",
				"       │ Press Enter to retry.",
				"",
				"       Try again, or paste a token you made with claude setup-token.",
				"",
				keysOf("enter try again   p paste a token   esc cancel"),
			},
			again: true,
		},
		{
			name:    "refused",
			tokened: setup.Tokened{Result: setup.TokenRefused},
			want: []string{
				"",
				"  ✗    The token was refused",
				"       The API refused the token, so it isn't saved.",
				"",
				"       Try again, or paste a token you made with claude setup-token.",
				"",
				keysOf("enter try again   p paste a token   esc cancel"),
			},
			again: true,
		},
		{
			name:    "no Claude Code",
			tokened: setup.Tokened{Result: setup.NoClaudeCode, Why: "can't find claude: it isn't on PATH"},
			want: []string{
				"",
				"  ✗    There's no Claude Code to create the token with",
				"       can't find claude: it isn't on PATH",
				"",
				"       Install Claude Code, then try again, or paste a token you made with",
				"       claude setup-token.",
				"",
				keysOf("enter try again   p paste a token   esc cancel"),
			},
			again: true,
		},
		{
			name:    "the same account",
			tokened: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"},
			want: []string{
				"",
				"  ✗    The token is for work's subscription",
				"       work has that subscription already, so the token isn't saved.",
				"",
				"       Try again, signed in to the other subscription, or paste a token you",
				"       made with claude setup-token.",
				"",
				keysOf("enter try again   p paste a token   esc cancel"),
			},
			again: true,
		},
		{
			name:    "unread",
			tokened: setup.Tokened{Result: setup.TokenUnread, Shown: []string{" Your OAuth token (valid for 1 year):", " test-token-personal-made"}},
			want: []string{
				"",
				"  !    The token was created, but setup couldn't read it",
				"       It lasts a year on the account, so copy it from what claude",
				"       setup-token showed:",
				"",
				"       │ Your OAuth token (valid for 1 year):",
				"       │ test-token-personal-made",
				"",
				"       Then press p to paste it.",
				"",
				keysOf("p paste a token   esc cancel"),
			},
		},
		{
			name:    "not saved",
			tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{"test-token-personal-made"}},
			want: []string{
				"",
				"  !    The token was created, but setup couldn't save it",
				"       write the token file: permission denied",
				"",
				"       │ test-token-personal-made",
				"",
				"       It lasts a year on the account, so copy it, then press p to paste it",
				"       once that's put right.",
				"",
				keysOf("p paste a token   esc cancel"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &tt.tokened}, 1)
			h.steps.nextMake(t)
			h.hearAll(3)
			checkCard(t, h, "NEW TOKEN · PERSONAL", tt.want)
			if h.steps.replaced["personal"] != 0 {
				t.Errorf("personal's token was replaced, want nothing saved")
			}

			h.pressAndDeliver("enter")
			if tt.again {
				if got := h.steps.nextMake(t); got.Account.ID != "personal" || got.Add {
					t.Errorf("enter made the token for %+v, want personal's again", got)
				}
				if got := h.model.token.stage(); got != dashboard.TokenStarting {
					t.Errorf("enter left the card at stage %d, want setup-token starting again", got)
				}
				h.hearAll(3)
			} else if got := h.model.token.stage(); got != dashboard.TokenUnsaved {
				t.Errorf("enter moved the card to stage %d, want it making no other token", got)
			}
			h.pressAndDeliver("esc")
			if h.model.token != nil {
				t.Errorf("esc left the card %+v open", h.model.token)
			}
		})
	}
}

// On a first run, the account added has its token made, then, once it's
// saved, the card closes, the accounts are read again with it, its row
// chosen, its token a year from today, and the page's line says what to add,
// no account left to wait for.
func TestATokenMadeAddsTheAccount(t *testing.T) {
	h := firstRun(t)
	h.steps.making = fakeMaking{steps: []setup.Making{{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Work"}, approving, madeIt, checkIt}, ends: &saved}
	h.typeIn("work")
	h.pressAndDeliver("tab")
	h.typeIn("Work")
	h.pressAndDeliver("enter")
	h.hearAll(5)
	if h.model.token != nil || h.model.add != nil {
		t.Fatalf("saved, the cards are %+v and %+v, want both closed", h.model.token, h.model.add)
	}
	rows := h.rows()
	if want := "  Add each Claude subscription you want switchboard to route across."; rows[firstUnderHeading] != want {
		t.Errorf("the page's line reads %q, want %q", rows[firstUnderHeading], want)
	}
	if got := rows[firstRow]; !strings.HasPrefix(got, "▌ 1  work ◆") || !strings.HasSuffix(got, "expires 27 Oct 2027  ·  365 days left") {
		t.Errorf("the first row reads %q, want work, chosen, its token a year from today", got)
	}
	if got := rows[keyRow]; !strings.Contains(got, "enter next: Priming") {
		t.Errorf("the key line reads %q, want enter moving on, an account added", got)
	}
}

// On a run again, an account added from + add an account has its row chosen
// once its token's saved, never + add an account, chosen as the card opened.
func TestTheRowChosenMovesToTheAccountAdded(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, ends: &saved}
	h.start()
	for range 3 {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	h.typeIn("team")
	h.pressAndDeliver("enter")
	if got := h.steps.nextMake(t); got.Account.ID != "team" || !got.Add {
		t.Fatalf("MakeToken was asked %+v, want team's token, to add it", got)
	}
	h.hearAll(5)
	if review := h.model.status.review; review == nil || len(review.Accounts) != 4 || h.model.chosen != 3 {
		t.Fatalf("saved, the review is %+v, row %d chosen; want team added, its row chosen", review, h.model.chosen)
	}
	if got := h.rows()[firstRow+6]; !strings.HasPrefix(got, "▌ 4  team") {
		t.Errorf("team's row reads %q, want it chosen", got)
	}
}

// A making that fails, as where the token can't be saved, fails the step,
// not saved, the card closed.
func TestAMakingThatFailsFailsTheStep(t *testing.T) {
	errNotSaved := errors.New("save the token: write the token file: no space left on device")
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, err: errNotSaved}, 1)
	h.hearAll(5)
	if h.model.token != nil {
		t.Errorf("failed, the token card is %+v, want it closed", h.model.token)
	}
	if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNotSaved}) {
		t.Errorf("setup would be left %+v, want failing at Accounts", got)
	}
	if got, want := h.rows()[firstUnderHeading], factRowOf("✗", setup.Said{Label: "Not saved", Detail: errNotSaved.Error()}); got != want {
		t.Errorf("the page's row reads\n%q\nwant\n%q", got, want)
	}
}

// What a making not the card's tells is dropped: a step, or how it ended,
// of one before, or of one once the card's closed.
func TestWhatAnotherMakingTellsIsDropped(t *testing.T) {
	h := tokening(t, fakeMaking{steps: []setup.Making{privately}}, 1)
	h.hear()
	stale := h.model.token.told.making - 1
	h.deliver(makingMsg{making: stale, at: madeIt}, tokenedMsg{making: stale, tokened: saved})
	if got := h.model.token.stage(); got != dashboard.TokenWaiting || h.model.rereading {
		t.Errorf("told of another making, the card is at stage %d, rereading %t; want it waiting, as it was", got, h.model.rereading)
	}
	h.pressAndDeliver("esc")
	h.hear()
	h.deliver(tokenedMsg{making: stale + 1, tokened: saved})
	if h.model.token != nil || h.model.rereading {
		t.Errorf("told of the card's making once it closed, the card is %+v, rereading %t; want it dropped", h.model.token, h.model.rereading)
	}
}

// While the token card is open, no page key acts, and a paste goes nowhere,
// as the card has no field.
func TestTheTokenCardTakesEveryKey(t *testing.T) {
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}}, 1)
	h.hearAll(2)
	for _, k := range []string{"e", "r", "t", "down", "pgdown"} {
		h.pressAndDeliver(k)
	}
	h.deliver(tea.PasteMsg{Content: "test-token-personal"})
	if h.model.paste != nil || len(h.steps.pastes) != 0 {
		t.Errorf("a paste at the token card opened %+v, pasted %v; want it going nowhere", h.model.paste, h.steps.pastes)
	}
	if h.model.edit != nil || h.model.remove != nil || h.model.chosen != 1 || h.model.scroll != 0 || len(h.steps.edits) != 0 {
		t.Errorf("with the token card open, a page key acted: cards %v %v, row %d chosen, scrolled %d", h.model.edit, h.model.remove, h.model.chosen, h.model.scroll)
	}
	if h.model.token.told.making != 1 {
		t.Errorf("t with the token card open began making %d, want the one making alone", h.model.token.told.making)
	}
}

// However setup's screen is left as a token's made, Run returns only once
// the making has ended: ctrl+c, or the screen's context ending, before
// setup-token says it's made the token, stop it, nothing saved; once it has,
// the token is saved, as it lasts a year on the account, before Run returns,
// or, unread, given back by Run, for its caller to show.
func TestRunReturnsOnceATokensMakingHasEndedItsTokenKept(t *testing.T) {
	waiting, made := []setup.Making{privately, approving}, []setup.Making{privately, approving, madeIt}
	unread := setup.Tokened{Result: setup.TokenUnread, Why: "setup-token showed an error", Shown: []string{"Your OAuth token (valid for 1 year):", "test-token-work"}}
	heldThrough := setup.Tokened{Result: setup.TokenSaved, InterruptHeld: true}
	tests := []struct {
		name  string
		steps []setup.Making
		ctrlC bool
		// kept is how the making ends once it's made, saved where it's nil;
		// saved is how many times work's token is saved, unsaved what Run
		// gives back, and interrupted whether it says an interrupt was held.
		kept        *setup.Tokened
		saved       int
		unsaved     *setup.Tokened
		interrupted bool
	}{
		{name: "ctrl+c before the token's made", steps: waiting, ctrlC: true},
		{name: "the context ending before the token's made", steps: waiting},
		{name: "ctrl+c once the token's made", steps: made, ctrlC: true, saved: 1},
		{name: "the context ending once the token's made", steps: made, saved: 1},
		{name: "ctrl+c once the token's made, which is unread", steps: made, ctrlC: true, kept: &unread, unsaved: &unread},
		{name: "ctrl+c once the token's made, an interrupt held as it's saved", steps: made, ctrlC: true, kept: &heldThrough, saved: 1, interrupted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := newFakeSteps()
			reported, saving := make(chan struct{}), make(chan struct{})
			steps.making = fakeMaking{steps: tt.steps, reported: reported, saving: saving, kept: tt.kept}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reviewed := make(chan struct{})
			var once sync.Once
			term := terminal{keys: "t", ready: reviewed, saw: func(msg tea.Msg) {
				if _, ok := msg.(reviewMsg); ok {
					once.Do(func() { close(reviewed) })
				}
			}}
			if tt.ctrlC {
				term.then, term.thenReady = "\x03", reported
			}
			type ran struct {
				left Left
				err  error
			}
			returned := make(chan ran, 1)
			go func() {
				left, _, err := runIn(t, ctx, Config{Steps: steps, Step: dashboard.SetupAccounts}, term)
				returned <- ran{left: left, err: err}
			}()

			select {
			case <-reported:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, t hadn't made work's token")
			}
			if !tt.ctrlC {
				cancel()
			}
			if slices.Contains(tt.steps, madeIt) {
				select {
				case <-returned:
					t.Fatal("run() returned while the token made was being saved, want it to wait till it's saved")
				case <-time.After(overlap):
				}
			}
			close(saving)
			var got ran
			select {
			case got = <-returned:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, run() hadn't returned")
			}
			if got.left.Ending != Paused || got.left.Err != nil || (tt.ctrlC && got.err != nil) || (!tt.ctrlC && !errors.Is(got.err, context.Canceled)) {
				t.Errorf("run() = %+v, %v; want setup paused, by ctrl+c or the context's ending", got.left, got.err)
			}
			if !reflect.DeepEqual(got.left.Unsaved, tt.unsaved) || got.left.Interrupted != tt.interrupted {
				t.Errorf("run() gives back the making as %+v, interrupted: %t; want %+v, interrupted: %t", got.left.Unsaved, got.left.Interrupted, tt.unsaved, tt.interrupted)
			}
			if got := steps.replacedOf("work"); got != tt.saved {
				t.Errorf("work's token was replaced %d times, want %d", got, tt.saved)
			}
		})
	}
}

// An interrupt held as a token made is saved, as a pane closed at Checking,
// which Bubble Tea doesn't watch, would send, has the screen leave once the
// token's saved, nothing pressed, Run saying an interrupt was held.
func TestTheScreenLeavesOnceATokenHeldThroughAnInterruptIsSaved(t *testing.T) {
	steps := newFakeSteps()
	heldThrough := setup.Tokened{Result: setup.TokenSaved, InterruptHeld: true}
	steps.making = fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, ends: &heldThrough}
	reviewed := make(chan struct{})
	var once sync.Once
	term := terminal{keys: "t", ready: reviewed, saw: func(msg tea.Msg) {
		if _, ok := msg.(reviewMsg); ok {
			once.Do(func() { close(reviewed) })
		}
	}}
	type ran struct {
		left Left
		err  error
	}
	returned := make(chan ran, 1)
	go func() {
		left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupAccounts}, term)
		returned <- ran{left: left, err: err}
	}()
	var got ran
	select {
	case got = <-returned:
	case <-time.After(time.Minute):
		t.Fatal("a minute on, the screen was up still, want it left once the token held through an interrupt was saved")
	}
	if got.err != nil || got.left != (Left{Ending: Paused, Interrupted: true}) {
		t.Errorf("run() = %+v, %v; want setup paused, an interrupt held", got.left, got.err)
	}
	if got := steps.replacedOf("work"); got != 1 {
		t.Errorf("work's token was replaced %d times, want once, before the screen left", got)
	}
}

// endsWithin is how long a test waits on a context it holds to end, where it
// wrongly would: a cancel can reach it on a goroutine of its own, as
// context.AfterFunc runs one, a moment after what cancelled it returns.
const endsWithin = 500 * time.Millisecond

// endsSoon reports whether ctx ends within endsWithin.
func endsSoon(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(endsWithin):
		return false
	}
}

// heldOnceMade is setup-token, as setup's TokenMaker: it has the Opener it's
// given open the sign-in address, reporting it, then its wait on the
// approval, then that it's made the token, closing made; then holds till
// released, telling ended whether what it reads under ends within
// endsWithin, and gives token, or, where unread is set, fails as setup-token
// does where it can't read the token off its screen.
type heldOnceMade struct {
	token          string
	unread         bool
	made, released chan struct{}
	ended          chan bool
}

func (m heldOnceMade) Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error) {
	opening, err := s.Opener.Open(ctx, signIn)
	s.Report(setuptoken.Event{Step: setuptoken.StepAddress, Address: signIn, Opening: opening, Unopened: err})
	s.Report(setuptoken.Event{Step: setuptoken.StepWaiting})
	s.Report(setuptoken.Event{Step: setuptoken.StepMade})
	close(m.made)
	<-m.released
	m.ended <- endsSoon(ctx)
	if m.unread {
		return tokens.Token{}, unreadScreen{}
	}
	return tokens.Parse(m.token)
}

// unreadScreen is setup-token's own failure, as a *setuptoken.ScreenError
// is, its screen showing the token made, which Make couldn't read.
type unreadScreen struct{}

func (unreadScreen) Error() string { return "setup-token showed a token Make couldn't read" }

func (unreadScreen) Screen() string {
	return "Your OAuth token (valid for 1 year):\n\ntest-token-work\n"
}

// makingsGiven are steps that tell given the context each token's making is
// begun under.
type makingsGiven struct {
	Steps
	given chan context.Context
}

func (s makingsGiven) MakeToken(ctx context.Context, f setup.TokenFor, report func(setup.Making)) (setup.Tokened, error) {
	s.given <- ctx
	return s.Steps.MakeToken(ctx, f, report)
}

// Through setup's own steps, ctrl+c once setup-token says it's made the
// token ends the screen's making, but not what setup-token is read under,
// which reads on: Run returns once the token's saved, or, unread, gives it
// back, for its caller to show.
func TestCtrlCOnceATokensMadeLeavesItReadOn(t *testing.T) {
	unread := setup.Tokened{Result: setup.TokenUnread, Why: unreadScreen{}.Error(), Shown: []string{"Your OAuth token (valid for 1 year):", "test-token-work"}}
	tests := []struct {
		name    string
		unread  bool
		unsaved *setup.Tokened
	}{
		{name: "shown whole: saved"},
		{name: "unread: given back by Run", unread: true, unsaved: &unread},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := newRegistered(t, threeAccounts)
			s := steps.setup
			maker := heldOnceMade{token: "test-token-work", unread: tt.unread, made: make(chan struct{}), released: make(chan struct{}), ended: make(chan bool, 1)}
			s.Accounts.Prober = func(string) accounts.Prober { return tokenAPI{} }
			s.TokenMaker, s.Browser, s.PrivateWindow = maker, privateWindow{}, privateWindow{}
			s.Interrupt = func(ctx context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(ctx) }
			told := makingsGiven{Steps: steps, given: make(chan context.Context, 1)}
			reviewed := make(chan struct{})
			var once sync.Once
			term := terminal{keys: "t", ready: reviewed, then: "\x03", thenReady: maker.made, saw: func(msg tea.Msg) {
				if _, ok := msg.(reviewMsg); ok {
					once.Do(func() { close(reviewed) })
				}
			}}
			type ran struct {
				left Left
				err  error
			}
			returned := make(chan ran, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: told, Step: dashboard.SetupAccounts}, term)
				returned <- ran{left: left, err: err}
			}()

			var screens context.Context
			select {
			case screens = <-told.given:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, t hadn't made work's token")
			}
			select {
			case <-screens.Done():
			case <-time.After(time.Minute):
				t.Fatal("a minute on, ctrl+c hadn't ended the screen's making")
			}
			close(maker.released)
			if <-maker.ended {
				t.Error("ctrl+c ended what setup-token was read under, want it read on, the token made")
			}
			var got ran
			select {
			case got = <-returned:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, run() hadn't returned")
			}
			if got.err != nil || got.left.Ending != Paused || !reflect.DeepEqual(got.left.Unsaved, tt.unsaved) {
				t.Errorf("run() = %+v, %v; want setup paused, the making given back as %+v", got.left, got.err, tt.unsaved)
			}
			token, err := s.Accounts.Tokens.Read("work")
			if saved := err == nil && token.Reveal() == maker.token; saved == tt.unread {
				t.Errorf("work's token file: %v, saved: %t; want it saved: %t", err, saved, !tt.unread)
			}
		})
	}
}

// madeAs is setup-token, as setup's TokenMaker, making the token it is: it
// has the Opener it's given open the sign-in address, reporting it, then
// reports its wait on the approval, then that it's made the token, and
// gives it.
type madeAs string

func (token madeAs) Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error) {
	opening, err := s.Opener.Open(ctx, signIn)
	s.Report(setuptoken.Event{Step: setuptoken.StepAddress, Address: signIn, Opening: opening, Unopened: err})
	s.Report(setuptoken.Event{Step: setuptoken.StepWaiting})
	s.Report(setuptoken.Event{Step: setuptoken.StepMade})
	return tokens.Parse(string(token))
}

// privateWindow opens nothing, answering that it opened the address in a
// private Firefox window.
type privateWindow struct{}

func (privateWindow) Open(context.Context, setuptoken.Address) (setuptoken.Opening, error) {
	return setuptoken.Opening{Opened: setuptoken.OpenedPrivately, Browser: "Firefox"}, nil
}

// refusedByAPI is the token the API refuses.
const refusedByAPI = "test-token-refused"

// tokenAPI checks a token as the API does: it refuses refusedByAPI, and
// names each other test-token-<name>'s subscription org-<name>.
type tokenAPI struct{}

func (tokenAPI) Probe(_ context.Context, token string) (quota.Probe, error) {
	if token == refusedByAPI {
		return quota.Probe{}, refusedProbe{}
	}
	return quota.Probe{Organization: "org-" + strings.TrimPrefix(token, "test-token-")}, nil
}

// refusedProbe is the API refusing a token.
type refusedProbe struct{}

func (refusedProbe) Error() string { return "HTTP 401 · Invalid bearer token" }

func (refusedProbe) Refused() bool { return true }

// Through setup's own steps, setup-token faked: t on side makes its token,
// which, checked with the API, is saved with its year from today, the row
// showing it; refused, or another account's subscription, it isn't saved,
// the card saying why.
func TestATokenMadeThroughSetupIsSavedOnlyOnceItsChecked(t *testing.T) {
	tests := []struct {
		name  string
		token string
		// says is the card's headline, where the token isn't saved.
		says string
	}{
		{name: "saved", token: "test-token-side"},
		{name: "refused", token: refusedByAPI, says: "The token was refused"},
		{name: "another account's subscription", token: "test-token-work", says: "The token is for work's subscription"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := newRegistered(t, threeAccounts)
			s := steps.setup
			for _, id := range []string{"work", "personal"} {
				token, _ := tokens.Parse("test-token-" + id)
				if err := s.Accounts.Tokens.Write(id, token, start); err != nil {
					t.Fatal(err)
				}
			}
			s.Accounts.Prober = func(string) accounts.Prober { return tokenAPI{} }
			s.TokenMaker, s.Browser, s.PrivateWindow = madeAs(tt.token), privateWindow{}, privateWindow{}
			s.Interrupt = func(ctx context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(ctx) }
			h := registeredHarness(t, steps)
			h.pressAndDeliver("down")
			h.pressAndDeliver("down")
			h.pressAndDeliver("t")
			h.hearAll(5)

			token, err := s.Accounts.Tokens.Read("side")
			if tt.says != "" {
				_, rows := h.card()
				if !slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, tt.says) }) {
					t.Errorf("the card reads\n%s\nwant it saying %q", strings.Join(rows, "\n"), tt.says)
				}
				if !errors.Is(err, tokens.ErrMissing) {
					t.Errorf("side's token file: %v, want none, nothing saved", err)
				}
				return
			}
			if h.model.token != nil {
				t.Fatalf("saved, the token card is %+v, want it closed", h.model.token)
			}
			if err != nil || token.Reveal() != tt.token {
				t.Fatalf("side's token file: %v, want it holding the token made", err)
			}
			if taken, err := s.Accounts.Tokens.Taken("side", token); err != nil || !taken.Equal(start) {
				t.Errorf("side's token was taken %v (%v), want today, %v, so it lasts a year from today", taken, err, start)
			}
			rows := h.rows()
			if i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "▌ 3  side") }); i < 0 || !strings.HasSuffix(rows[i], "expires 27 Oct 2027  ·  365 days left") {
				t.Errorf("the page reads\n%s\nwant side's row chosen, its token a year from today", strings.Join(rows, "\n"))
			}
		})
	}
}
