package screen

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
	switchstatus "github.com/leeovery/switchboard/internal/status"
)

// The rows of the Accounts page at 160×37: the first under its heading; the
// first account's, each after it two apart; and the first note's.
const (
	firstUnderHeading = 6
	firstRow          = 10
	firstNotes        = 19
)

// readToken is a token read, not yet checked, and its year, 21 days left of
// it at start.
var (
	readToken = setup.TokenStanding{State: setup.TokenRead}
	endingIn  = switchstatus.TokenYear{Taken: start.AddDate(-1, 0, 21), GoodUntil: start.AddDate(0, 0, 21)}
)

// frame3Steps are the steps as frame 3 has them: work, the primary, its
// token working, its year unknown; personal, no plan set, its token working,
// 21 days left of its year; and side, its token refused; each read, then
// checked.
func frame3Steps() *fakeSteps {
	steps := newFakeSteps()
	steps.review = setup.Review{At: start, Accounts: []setup.Reviewed{
		{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: readToken},
		{ID: "personal", Label: "Personal", Token: setup.TokenStanding{State: setup.TokenRead, Year: endingIn}},
		{ID: "side", Label: "Side", Plan: "pro", Token: readToken},
	}}
	steps.checked = map[string]setup.TokenStanding{
		"work":     {State: setup.TokenWorking},
		"personal": {State: setup.TokenWorking, Year: endingIn},
		"side":     {State: setup.TokenNotTaken},
	}
	return steps
}

// accountsHarness is a harness opening on Accounts, its steps as given.
func accountsHarness(t *testing.T, steps *fakeSteps) *harness {
	t.Helper()
	return harnessOf(t, steps, dashboard.SetupAccounts)
}

// harnessOf is a harness opening on step, its steps as given, in a terminal
// 160 cells wide and 37 tall. It hasn't started.
func harnessOf(t *testing.T, steps *fakeSteps, step dashboard.SetupStep) *harness {
	t.Helper()
	h := newHarness(t, step)
	h.steps = steps
	h.model = New(t.Context(), h.config(step))
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	return h
}

// held starts the model, delivering every message its first commands send
// back but the checks', which it returns, undelivered.
func (h *harness) held() []tea.Msg {
	h.t.Helper()
	var checks []tea.Msg
	msgs := h.run(h.model.Init())
	for len(msgs) > 0 {
		msg := msgs[0]
		msgs = msgs[1:]
		if _, ok := msg.(checkedMsg); ok {
			checks = append(checks, msg)
			continue
		}
		msgs = append(msgs, h.update(msg)...)
	}
	return checks
}

func TestEveryAccountIsCheckedAtOnceAsTheScreenStarts(t *testing.T) {
	for _, step := range []dashboard.SetupStep{dashboard.SetupAccounts, dashboard.SetupSkill} {
		t.Run(step.Title(), func(t *testing.T) {
			h := harnessOf(t, frame3Steps(), step)
			checks := h.held()
			if got := h.model.status.checking; !slices.Equal(got, []string{"work", "personal", "side"}) {
				t.Errorf("before any check answers, the accounts being checked are %v, want every one", got)
			}
			if got := len(checks); got != 3 {
				t.Errorf("starting checks %d accounts, want 3", got)
			}
		})
	}
}

func TestTheStatusLineLeavesCheckingAsEachCheckAnswers(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	checks := h.held()
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, checking…"; got != want {
		t.Errorf("before any check answers, the status line reads %q, want %q", got, want)
	}
	h.deliver(checks[2])
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, checking…"; got != want {
		t.Errorf("side's check answered, the status line reads %q, want %q, the others still checking", got, want)
	}
	if got := h.rows()[firstRow+4]; !strings.Contains(got, "✗ rejected: revoked or expired") {
		t.Errorf("side's row reads %q, want its token refused, as its check answered", got)
	}
	h.deliver(checks[:2]...)
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, 2 working"; got != want {
		t.Errorf("every check answered, the status line reads %q, want %q", got, want)
	}
	if got := h.steps.checks; len(got) != 3 {
		t.Errorf("ReviewOf was asked of %v, want each account once", got)
	}
}

func TestACheckThatFailsLeavesItsRowUnchecked(t *testing.T) {
	log := logstest.Capture(t)
	steps := frame3Steps()
	steps.checkErr = errors.New("read the config: it doesn't parse")
	h := accountsHarness(t, steps)
	h.start()
	for i, a := range []string{"work", "personal", "side"} {
		if got := h.rows()[firstRow+2*i]; !strings.HasSuffix(got, "unchecked") || strings.Contains(got, "checking") {
			t.Errorf("%s's row reads %q, want its token unchecked, never checking for good", a, got)
		}
	}
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, all working"; got != want {
		t.Errorf("the status line reads %q, want %q: a token kept unchecked counts as working", got, want)
	}
	if !log.Has("level=WARN", "couldn't check", "account=side", "it doesn't parse") {
		t.Errorf("the log reads\n%s\nwant why each check failed", log)
	}
}

func TestAskingAgainKeepsEachTokenAsChecked(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	var cmd tea.Cmd
	h.model, cmd = h.model.askAgain()
	h.deliver(h.run(cmd)...)
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, 2 working"; got != want {
		t.Errorf("asked again, the status line reads %q, want %q, each token kept as checked", got, want)
	}
	if got := h.steps.checks; len(got) != 3 {
		t.Errorf("asked again, ReviewOf was asked of %v, want each account once, none again", got)
	}
}

func TestAnAnswerToACheckOfAReviewReadBeforeIsKeptOfTheSameTokenAlone(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	checks := h.held()
	var cmd tea.Cmd
	h.model, cmd = h.model.askAgain()
	for _, msg := range h.run(cmd) {
		if _, ok := msg.(checkedMsg); !ok {
			h.deliver(msg)
		}
	}
	h.deliver(checks[:2]...)
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, checking…"; got != want {
		t.Errorf("two answers to checks of the accounts read before, the status line reads %q, want %q", got, want)
	}
	if got := h.rows()[firstRow]; !strings.HasSuffix(got, "working · expiry date unknown") {
		t.Errorf("work's row reads %q, want its token as checked, the same as read since", got)
	}
	failed := checks[2].(checkedMsg)
	failed.row, failed.err = setup.Reviewed{}, errors.New("read the config: it doesn't parse")
	h.deliver(failed)
	if got := h.rows()[firstRow+4]; strings.Contains(got, "unchecked") {
		t.Errorf("side's row reads %q, want a failure of a check of the accounts read before dropped", got)
	}
	if got := h.steps.checks; len(got) != 4 || got[3] != "side" {
		t.Errorf("ReviewOf was asked of %v, want side again, its token read since unchecked", got)
	}
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, 2 working"; got != want {
		t.Errorf("side checked again, the status line reads %q, want %q", got, want)
	}
}

func TestATokenReplacedIsCheckedAgain(t *testing.T) {
	working := setup.TokenStanding{State: setup.TokenWorking}
	t.Run("asked again once it was checked", func(t *testing.T) {
		steps := frame3Steps()
		h := accountsHarness(t, steps)
		h.start()
		steps.replace("side")
		steps.checked["side"] = working
		var cmd tea.Cmd
		h.model, cmd = h.model.askAgain()
		h.deliver(h.run(cmd)...)
		if got := steps.checks; !slices.Equal(got, []string{"work", "personal", "side", "side"}) {
			t.Errorf("ReviewOf was asked of %v, want side again alone, its token replaced", got)
		}
		if got := h.rows()[firstRow+4]; !strings.HasSuffix(got, "working · expiry date unknown") {
			t.Errorf("side's row reads %q, want its new token as checked, working", got)
		}
	})
	t.Run("its check of the token before answering after", func(t *testing.T) {
		steps := frame3Steps()
		h := accountsHarness(t, steps)
		checks := h.held()
		steps.replace("side")
		steps.checked["side"] = working
		var cmd tea.Cmd
		h.model, cmd = h.model.askAgain()
		h.deliver(h.run(cmd)...)
		h.deliver(checks...)
		if got := steps.checks; !slices.Equal(got, []string{"work", "personal", "side", "side"}) {
			t.Errorf("ReviewOf was asked of %v, want side again alone, its answer of the token before dropped", got)
		}
		if got := h.rows()[firstRow+4]; !strings.HasSuffix(got, "working · expiry date unknown") {
			t.Errorf("side's row reads %q, want its new token as checked, never the old one's refusal", got)
		}
	})
}

func TestAnUncheckedTokenIsCheckedAgainOnAskingAgain(t *testing.T) {
	steps := frame3Steps()
	steps.checkErr = errors.New("HTTP 529")
	h := accountsHarness(t, steps)
	h.start()
	steps.checkErr = nil
	var cmd tea.Cmd
	h.model, cmd = h.model.askAgain()
	h.deliver(h.run(cmd)...)
	if got := len(steps.checks); got != 6 {
		t.Errorf("ReviewOf was asked of %v, want each account again, as none was answered for", steps.checks)
	}
	if got := h.rows()[firstRow+4]; !strings.Contains(got, "✗ rejected: revoked or expired") {
		t.Errorf("side's row reads %q, want its token refused, as checked again", got)
	}
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, 2 working"; got != want {
		t.Errorf("the status line reads %q, want %q", got, want)
	}
}

func TestAnAnswerOfATokenStillReadEndsItsCheckUnchecked(t *testing.T) {
	steps := frame3Steps()
	steps.checked["side"] = readToken
	h := accountsHarness(t, steps)
	h.start()
	if got := steps.checks; len(got) != 3 {
		t.Errorf("ReviewOf was asked of %v, want each account once: an answer still read checks no more", got)
	}
	if got := h.rows()[firstRow+4]; !strings.HasSuffix(got, "unchecked") {
		t.Errorf("side's row reads %q, want its token unchecked, never checking for good", got)
	}
}

func TestTheChosenRowIsKeptAmongTheAccountsReadAgain(t *testing.T) {
	steps := frame3Steps()
	h := accountsHarness(t, steps)
	h.start()
	for range 3 {
		h.pressAndDeliver("down")
	}
	steps.review.Accounts = steps.review.Accounts[:2]
	var cmd tea.Cmd
	h.model, cmd = h.model.askAgain()
	h.deliver(h.run(cmd)...)
	if got := h.model.chosen; got != 2 {
		t.Errorf("read again with an account fewer, row %d is chosen, want 2, + add an account", got)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupAccounts {
		t.Errorf("enter on + add an account shows %s, want nothing done yet", got.Title())
	}
}

// reviewTillCancelled is setup's steps whose ReviewOf holds on until its
// context is cancelled, saying when each has started and ended.
type reviewTillCancelled struct {
	*fakeSteps
	started, ended chan string
}

func (s *reviewTillCancelled) ReviewOf(ctx context.Context, id string) (setup.Reviewed, error) {
	s.started <- id
	<-ctx.Done()
	s.ended <- id
	return setup.Reviewed{}, fmt.Errorf("check %s: %w", id, ctx.Err())
}

func TestQWhileChecksRunLeavesOnceEveryCheckHasEnded(t *testing.T) {
	steps := &reviewTillCancelled{fakeSteps: frame3Steps(), started: make(chan string, 3), ended: make(chan string, 3)}
	all := make(chan struct{})
	go func() {
		for range 3 {
			<-steps.started
		}
		close(all)
	}()
	type ran struct {
		left Left
		err  error
	}
	returned := make(chan ran, 1)
	go func() {
		left, _, err := runUntil(t, Config{Steps: steps}, terminal{keys: "q", ready: all})
		returned <- ran{left: left, err: err}
	}()
	select {
	case got := <-returned:
		if got.err != nil || got.left != (Left{Ending: Paused}) {
			t.Errorf("run() = %+v, %v; want setup paused, no error", got.left, got.err)
		}
	case <-time.After(time.Minute):
		t.Fatal("a minute on, run() hasn't returned: want every check running at once, then q leaving")
	}
	if len(steps.ended) != 3 {
		t.Errorf("run() returned with %d of 3 checks ended, want every one", len(steps.ended))
	}
}

func TestChoosingARow(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	steps := []struct {
		key    string
		chosen int
	}{
		{"up", 0}, {"down", 1}, {"j", 2}, {"j", 3}, {"down", 3}, {"k", 2}, {"up", 1},
	}
	for _, step := range steps {
		h.pressAndDeliver(step.key)
		if h.model.chosen != step.chosen {
			t.Errorf("after %s, row %d is chosen, want %d", step.key, h.model.chosen, step.chosen)
		}
	}
	if got := h.model.scroll; got != 0 {
		t.Errorf("j and k scrolled the page to %d, want them to choose alone", got)
	}
}

func TestTheKeyLineFollowsTheRowChosen(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	if got, want := h.rows()[keyRow], "↑↓ select   t new token   e edit   r remove   p make primary"+strings.Repeat(" ", 72)+"enter next: Priming   q quit"; got != want {
		t.Errorf("with an account chosen, the key line reads\n%q\nwant\n%q", got, want)
	}
	for range 3 {
		h.pressAndDeliver("down")
	}
	if got, want := h.rows()[keyRow], "↑↓ select"+strings.Repeat(" ", 122)+"enter add an account   q quit"; got != want {
		t.Errorf("with + add an account chosen, the key line reads\n%q\nwant\n%q", got, want)
	}
}

func TestEnterAtAccounts(t *testing.T) {
	tests := []struct {
		name string
		// unmarked has two accounts, none the primary; down is how many rows
		// down to choose.
		unmarked bool
		down     int
		want     dashboard.SetupStep
	}{
		{name: "an account chosen: on to Priming", want: dashboard.SetupPriming},
		{name: "+ add an account chosen: nothing yet", down: 3, want: dashboard.SetupAccounts},
		{name: "none the primary: enter waits for one", unmarked: true, want: dashboard.SetupAccounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := frame3Steps()
			if tt.unmarked {
				steps.review.Unmarked, steps.review.Accounts[0].Primary = true, false
			}
			h := accountsHarness(t, steps)
			h.start()
			for range tt.down {
				h.pressAndDeliver("down")
			}
			h.pressAndDeliver("enter")
			if got := h.model.page().Step; got != tt.want {
				t.Errorf("enter shows %s, want %s", got.Title(), tt.want.Title())
			}
			if len(steps.set) > 0 {
				t.Errorf("enter set %v, want nothing changed", steps.set)
			}
		})
	}
}

func TestWhereNoneIsThePrimaryTheKeyLineHasNoEnter(t *testing.T) {
	steps := frame3Steps()
	steps.review.Unmarked, steps.review.Accounts[0].Primary = true, false
	h := accountsHarness(t, steps)
	h.start()
	if got := h.rows()[keyRow]; !strings.HasSuffix(got, "   q quit") || strings.Contains(got, "enter") {
		t.Errorf("the key line reads %q, want q quit alone at its right: enter waits for a primary", got)
	}
	if got := h.rows()[firstNotes]; got != "  No account is the primary, the one the browser and the Claude apps are signed in to." {
		t.Errorf("the first note reads %q, want that none is the primary", got)
	}
}

func TestEnterWaitsForTheAccountsToBeRead(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	for _, msg := range h.run(h.model.Init()) {
		if _, ok := msg.(reviewMsg); !ok {
			h.deliver(msg)
		}
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupAccounts {
		t.Errorf("enter before the accounts are read shows %s, want Accounts", got.Title())
	}
	if got := h.rows()[keyRow]; strings.TrimLeft(got, " ") != "q quit" {
		t.Errorf("before the accounts are read, the key line reads %q, want q quit alone", got)
	}
}

func TestAccountsThatCantBeReadStopSetupThere(t *testing.T) {
	steps := frame3Steps()
	steps.reviewErr = errNoConfig
	h := accountsHarness(t, steps)
	h.start()
	if got, want := h.rows()[firstUnderHeading], factRowOf("✗", setup.Said{Label: "Not read", Detail: errNoConfig.Error()}); got != want {
		t.Errorf("the page's row reads\n%q\nwant\n%q", got, want)
	}
	if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNoConfig}) {
		t.Errorf("setup would be left %+v, want failing at Accounts", got)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupAccounts {
		t.Errorf("enter shows %s, want setup stopped at Accounts", got.Title())
	}
}

// manyAccounts are the steps of n accounts, each working, the first the
// primary, each with a plan.
func manyAccounts(n int) *fakeSteps {
	steps := newFakeSteps()
	steps.review.Accounts = nil
	for i := range n {
		id := fmt.Sprintf("acct%02d", i+1)
		steps.review.Accounts = append(steps.review.Accounts, setup.Reviewed{ID: id, Label: "Label " + id, Plan: "pro", Primary: i == 0, Token: setup.TokenStanding{State: setup.TokenWorking}})
	}
	return steps
}

func TestManyAccountsScrollTheChosenRowKeptInViewTheHeadsKept(t *testing.T) {
	h := accountsHarness(t, manyAccounts(12))
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.start()
	for i := range 12 {
		h.pressAndDeliver("down")
		rows := h.rows()
		want := fmt.Sprintf("▌ %-2d  acct%02d", i+2, i+2)
		if i == 11 {
			want = "▌ +   add an account"
		}
		if !slices.ContainsFunc(rows[6:23], func(row string) bool { return strings.HasPrefix(row, want) }) {
			t.Fatalf("with row %d chosen, at 95×25, the page reads\n%s\nwant the row chosen, %q, in view", i+2, strings.Join(rows, "\n"), want)
		}
	}
	rows := h.rows()
	if !strings.HasPrefix(rows[6], "      ACCOUNT") {
		t.Errorf("scrolled into the table's rows, the page's first row reads %q, want the table's heads, kept", rows[6])
	}
	out := rows[22]
	if !strings.HasPrefix(out, "▲ ") || strings.Contains(out, "j/k") {
		t.Errorf("the line saying what's out of view reads %q, want it saying what's above, naming no keys: ↑↓ choose here", out)
	}
	for range 13 {
		h.pressAndDeliver("up")
	}
	if rows := h.rows(); h.model.scroll != 0 || !strings.HasPrefix(rows[6], "  Each token was checked") || !strings.HasPrefix(rows[10], "▌ 1   acct01") {
		t.Errorf("back at the first row, scrolled %d, the page reads\n%s\nwant it scrolled to the top, what's over the table showing", h.model.scroll, strings.Join(rows[6:11], "\n"))
	}
}

func TestPgDnScrollsTheAccountsPageToItsNotes(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.start()
	h.pressAndDeliver("pgdown")
	if h.model.scroll == 0 {
		t.Errorf("PgDn leaves the page scrolled 0, want it scrolled on to the notes")
	}
	if h.model.chosen != 0 {
		t.Errorf("PgDn chose row %d, want the row chosen as it was", h.model.chosen)
	}
}
