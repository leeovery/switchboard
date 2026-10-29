package setup_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/status"
)

func TestATokenIsToldAsTheReviewsTokenColumnHasIt(t *testing.T) {
	known := status.TokenYear{GoodUntil: personalUntil}
	tests := []struct {
		name  string
		token setup.TokenStanding
		told  setup.TokenTold
		words string
	}{
		{"working, its year known: by its year", setup.TokenStanding{State: setup.TokenWorking, Year: known}, setup.ToldByYear, ""},
		{"working, its year unknown", setup.TokenStanding{State: setup.TokenWorking}, setup.ToldUndated, "working · expiry date unknown"},
		{"refused", setup.TokenStanding{State: setup.TokenNotTaken, Year: known}, setup.ToldRefused, "rejected: revoked or expired"},
		{"read, its check running", setup.TokenStanding{State: setup.TokenRead, Year: known}, setup.ToldInWords, "checking…"},
		{"the API didn't answer", setup.TokenStanding{State: setup.TokenUnchecked, Year: known, Why: "HTTP 529"}, setup.ToldInWords, "unchecked"},
		{"no token", setup.TokenStanding{}, setup.ToldInWords, "no token"},
		{"a token file setup can't read", setup.TokenStanding{State: setup.TokenUnreadable, Why: "chmod 600"}, setup.ToldInWords, "can't read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if told, words := tt.token.Told(); told != tt.told || words != tt.words {
				t.Errorf("the token is told %d, %q; want %d, %q", told, words, tt.told, tt.words)
			}
		})
	}
}

// checkedAll is the review with every account checked with ReviewOf, each
// answer folded in as Checked has it.
func checkedAll(t *testing.T, s *setup.Setup, review setup.Review) setup.Review {
	t.Helper()
	for _, a := range review.Accounts {
		row, err := s.ReviewOf(t.Context(), a.ID)
		review = review.Checked(a.ID, row, err)
	}
	return review
}

func TestCheckedFoldsEachAnswerIntoTheReview(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	read, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	checked := checkedAll(t, s, read)

	want := []setup.Reviewed{
		{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}},
		{ID: "personal", Label: "Personal", Token: setup.TokenStanding{State: setup.TokenWorking, Year: personalYear, Organization: "org-personal"}},
		{ID: "side", Label: "Side", Plan: "pro", Token: setup.TokenStanding{State: setup.TokenNotTaken}},
	}
	checkRows(t, "the checked review's rows", checked.Accounts, want)
	checkLine(t, setup.RouterStanding{State: setup.RouterHealthy}, checked,
		setup.Part{Standing: setup.PartUp, Words: "router up, healthy"}, setup.Part{Standing: setup.PartTold, Words: "3 accounts, 2 working"})
	for _, a := range read.Accounts {
		if a.Token.State != setup.TokenRead {
			t.Errorf("folding the answers changed the review read: %s's token stands %d, want it read", a.ID, a.Token.State)
		}
	}
}

func TestACheckThatFailedLeavesTheTokenUncheckedNeverChecking(t *testing.T) {
	w := frame3(t)
	w.writeConfig(t, frameConfig+"\n[[account]]\nid = \"spare\"\nplan = \"pro\"\n")
	s := w.reviewing(t, &checkingAPI{})
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	failed := errors.New("read the config: sk-ant-oat01-test-token-work is in the way")
	review = review.Checked("work", setup.Reviewed{}, failed).Checked("spare", setup.Reviewed{}, failed)
	work, spare := review.Accounts[0].Token, review.Accounts[3].Token
	if work.State != setup.TokenUnchecked || !strings.Contains(work.Why, "read the config: [redacted] is in the way") || strings.Contains(work.Why, "sk-ant-") {
		t.Errorf("work's token stands %d, why %q; want it unchecked, kept, saying why, with no token in it", work.State, work.Why)
	}
	if spare.State != setup.NoToken {
		t.Errorf("spare's token stands %d, want no token still: a failed check reads none", spare.State)
	}

	row, err := s.ReviewOf(t.Context(), "personal")
	if err != nil {
		t.Fatal(err)
	}
	review = review.Checked("personal", row, nil).Checked("personal", setup.Reviewed{}, failed)
	if got := review.Accounts[1].Token.State; got != setup.TokenWorking {
		t.Errorf("a failure after personal's check leaves it %d, want it working, as checked", got)
	}
}

func TestAnAnswerAskedOnceTheReviewWasReadIsOfTheTokenThere(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	w.writeToken(t, "side", personalToken)
	row, err := s.ReviewOf(t.Context(), "side")
	if err != nil {
		t.Fatal(err)
	}
	if got := review.Checked("side", row, nil).Accounts[2].Token.State; got != setup.TokenWorking {
		t.Errorf("side's token replaced before it was checked, its row stands %d, want the token there as checked, working", got)
	}
	if got := review.Checked("gone", row, nil); !slicesSameRows(got.Accounts, review.Accounts) {
		t.Errorf("an answer of an account the review doesn't have changes it to %+v", got.Accounts)
	}
}

func TestAnAnswerAskedBeforeTheReviewWasReadIsKeptOfTheSameTokenAlone(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	var asked []setup.Reviewed
	for _, id := range []string{"work", "side"} {
		row, err := s.ReviewOf(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		asked = append(asked, row)
	}
	w.writeToken(t, "side", personalToken)
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	if got := review.Keeping(setup.Review{Accounts: asked[:1]}).Accounts[0].Token.State; got != setup.TokenWorking {
		t.Errorf("work's answer, its token the same, leaves it %d, want it working, as checked", got)
	}
	if got := review.Keeping(setup.Review{Accounts: asked[1:]}).Accounts[2].Token.State; got != setup.TokenRead {
		t.Errorf("side's answer, of its old token, leaves its new one %d, want it read, to be checked", got)
	}
}

// slicesSameRows reports whether two reviews' rows are alike, as sameRow
// has it.
func slicesSameRows(a, b []setup.Reviewed) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameRow(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestKeepingKeepsEachTokenCheckedWhileItsTheSame(t *testing.T) {
	w := frame3(t)
	api := &checkingAPI{}
	s := w.reviewing(t, api)
	first, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	checked := checkedAll(t, s, first)
	w.writeToken(t, "side", personalToken)

	again, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	kept := again.Keeping(checked)
	for i, want := range []setup.TokenState{setup.TokenWorking, setup.TokenWorking, setup.TokenRead} {
		if got := kept.Accounts[i].Token.State; got != want {
			t.Errorf("read again, %s's token stands %d, want %d", kept.Accounts[i].ID, got, want)
		}
	}
	if got := kept.Accounts[0].Token.Organization; got != "org-work" {
		t.Errorf("work's token kept names organization %q, want org-work, as checked", got)
	}
	if again.Accounts[0].Token.State != setup.TokenRead {
		t.Error("keeping changed the review read again: want it as read")
	}
	if asked := len(api.askedOf()); asked != 3 {
		t.Errorf("the API was asked %d times, want 3: keeping asks nothing", asked)
	}
}

func TestKeepingTakesEachTokensYearAsReadAfresh(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	first, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	checked := checkedAll(t, s, first)
	w.writeTokenTaken(t, "work", workToken, personalUntil.Add(-claude.SetupTokenLifetime))

	again, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	work := again.Keeping(checked).Accounts[0].Token
	if work.State != setup.TokenWorking || work.Organization != "org-work" {
		t.Errorf("work's token kept stands %d, naming %q, want working, naming org-work, as checked", work.State, work.Organization)
	}
	if !work.Year.GoodUntil.Equal(personalUntil) {
		t.Errorf("work's token kept is good until %s, want %s, as its record says since", work.Year.GoodUntil, personalUntil)
	}
}

func TestKeepingKeepsWhatTheAPIAnsweredAlone(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	read, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	side, err := s.ReviewOf(t.Context(), "side")
	if err != nil {
		t.Fatal(err)
	}
	earlier := read.Checked("work", setup.Reviewed{}, errors.New("HTTP 529")).Checked("side", side, nil)

	again, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	kept := again.Keeping(earlier)
	if got := kept.Accounts[0].Token.State; got != setup.TokenRead {
		t.Errorf("work's token, unchecked before, stands %d read again, want it read, to be checked again", got)
	}
	if got := kept.Accounts[2].Token.State; got != setup.TokenNotTaken {
		t.Errorf("side's token, refused before, stands %d read again, want it refused, as the API answered", got)
	}
}

func TestAnAnswerOfATokenStillReadLeavesItUnchecked(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	read, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Checked("work", read.Accounts[0], nil).Accounts[0].Token.State; got != setup.TokenUnchecked {
		t.Errorf("an answer of work's token still read leaves it %d, want it unchecked, so it isn't checked again for good", got)
	}
}

func TestAnAccountsPlanIsNamedAsThePriceTableNamesIt(t *testing.T) {
	tests := []struct{ plan, want string }{{"max20x", "Max 20x"}, {"pro", "Pro"}, {"", ""}, {"unknown", ""}}
	for _, tt := range tests {
		if got := (setup.Reviewed{Plan: tt.plan}).PlanName(); got != tt.want {
			t.Errorf("plan %q is named %q, want %q", tt.plan, got, tt.want)
		}
	}
}
