package setup_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// frameClock is frame 3's clock, Tue 27 Oct 17:42, in a zone where it's 28
// Oct already in UTC, so days left counted in UTC come out a day short.
var frameClock = time.Date(2026, 10, 27, 17, 42, 0, 0, time.FixedZone("test", -8*60*60))

// personalUntil is when personal's token's year is up: 17 Nov in the clock's
// zone as in UTC, 21 days on from the clock's date, 20 from UTC's.
var personalUntil = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// The tokens the fake API answers for each way.
const (
	workToken     = "test-token-work"
	personalToken = "test-token-personal"
	sideToken     = refusedToken
	// downToken is answered with a 5xx, and slowToken not within the
	// prober's own time limit.
	downToken = "test-token-down"
	slowToken = "test-token-slow"
)

// allTokens are every token the review's tests give, none of which may show
// in what it returns or logs.
var allTokens = []string{workToken, personalToken, sideToken, downToken, slowToken}

// frameConfig is frame 3's accounts: work, the primary, with a 5% reserve;
// personal, its plan not set; and side.
const frameConfig = `[[account]]
id      = "work"
label   = "Work"
primary = true
plan    = "max20x"
reserve = 0.05

[[account]]
id    = "personal"
label = "Personal"

[[account]]
id    = "side"
label = "Side"
plan  = "pro"
`

// checkingAPI answers for each token as the review's tests have it, safely
// asked from several goroutines at once, quoting the token in each failure,
// as a careless one might.
type checkingAPI struct {
	mu    sync.Mutex
	asked []string
}

func (a *checkingAPI) proberAt(string) accounts.Prober {
	return a
}

func (a *checkingAPI) Probe(_ context.Context, token string) (quota.Probe, error) {
	a.mu.Lock()
	a.asked = append(a.asked, token)
	a.mu.Unlock()
	switch token {
	case refusedToken:
		return quota.Probe{}, refusal{}
	case downToken:
		return quota.Probe{}, fmt.Errorf("HTTP 529 · Overloaded, for %s", token)
	case slowToken:
		return quota.Probe{}, fmt.Errorf("probe with %s: %w", token, context.DeadlineExceeded)
	}
	return quota.Probe{Organization: "org-" + strings.TrimPrefix(token, "test-token-")}, nil
}

func (a *checkingAPI) askedOf() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.asked)
}

// reviewing is setup in the world, its clock frame 3's, its API checkingAPI.
func (w *world) reviewing(t *testing.T, api *checkingAPI) *setup.Setup {
	t.Helper()
	s := w.setup(t, &user{})
	s.Accounts.Prober = api.proberAt
	s.Accounts.Now = func() time.Time { return frameClock }
	return s
}

// frame3 is the world as frame 3 has it: work's token of a date not
// recorded, personal's in its last 30 days, and side's one the API refuses.
func frame3(t *testing.T) *world {
	t.Helper()
	w := newWorld(t)
	w.writeConfig(t, frameConfig)
	w.writeToken(t, "work", workToken)
	w.writeTokenTaken(t, "personal", personalToken, personalUntil.Add(-claude.SetupTokenLifetime))
	w.writeToken(t, "side", sideToken)
	return w
}

// writeTokenTaken writes the account's token, with the record that it was
// taken at taken.
func (w *world) writeTokenTaken(t *testing.T, id, secret string, taken time.Time) {
	t.Helper()
	token, err := tokens.Parse(secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.tokens().Write(id, token, taken); err != nil {
		t.Fatal(err)
	}
}

// personalYear is personal's token's year, as its record gives it.
var personalYear = status.TokenYearFrom(personalUntil.Add(-claude.SetupTokenLifetime))

func TestReviewOfFrame3(t *testing.T) {
	w := frame3(t)
	api := &checkingAPI{}
	s := w.reviewing(t, api)
	before := w.snapshot(t)

	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	read := []setup.Reviewed{
		{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: setup.TokenStanding{State: setup.TokenRead}},
		{ID: "personal", Label: "Personal", Token: setup.TokenStanding{State: setup.TokenRead, Year: personalYear}},
		{ID: "side", Label: "Side", Plan: "pro", Token: setup.TokenStanding{State: setup.TokenRead}},
	}
	checkRows(t, "the review", review.Accounts, read)
	if !review.At.Equal(frameClock) || review.At.Location() != frameClock.Location() {
		t.Errorf("the review is at %v, want setup's clock, %v, in its zone", review.At, frameClock)
	}
	if review.Unmarked {
		t.Error("the review says no account is the primary, but work is")
	}
	if asked := api.askedOf(); len(asked) != 0 {
		t.Errorf("the review asked the API about %d tokens, want none: it shows at once", len(asked))
	}
	healthy := setup.RouterStanding{State: setup.RouterHealthy}
	checkLine(t, healthy, review, setup.Part{Standing: setup.PartUp, Words: "router up, healthy"}, setup.Part{Standing: setup.PartTold, Words: "3 accounts, checking…"})

	want := []setup.Reviewed{
		{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}},
		{ID: "personal", Label: "Personal", Token: setup.TokenStanding{State: setup.TokenWorking, Year: personalYear, Organization: "org-personal"}},
		{ID: "side", Label: "Side", Plan: "pro", Token: setup.TokenStanding{State: setup.TokenNotTaken}},
	}
	var checked []setup.Reviewed
	for _, a := range review.Accounts {
		row, err := s.ReviewOf(t.Context(), a.ID)
		if err != nil {
			t.Fatalf("ReviewOf(%s): %v", a.ID, err)
		}
		checked = append(checked, row)
	}
	checkRows(t, "the checked rows", checked, want)
	w.checkUnchanged(t, before)

	review.Accounts = checked
	notes := []setup.Note{
		{Need: setup.NeedToken, Account: "side", Says: "has no working token, so no requests are routed to it.", Press: "Press t to create a new one, approving it in the browser signed in to Side.", Key: "t"},
		{Need: setup.NeedNewToken, Account: "personal", Says: "has a token that expires 17 Nov 2026: 21 days left.", Press: "Press t to create a new one, approving it in the browser signed in to Personal.", Key: "t"},
		{Need: setup.NeedPlan, Account: "personal", Says: "has no plan set, so History can't value its usage.", Press: "Press e to set one.", Key: "e"},
	}
	if got := review.Notes(); !slices.Equal(got, notes) {
		t.Errorf("the notes are\n%+v\nwant\n%+v", got, notes)
	}
	checkLine(t, healthy, review, setup.Part{Standing: setup.PartUp, Words: "router up, healthy"}, setup.Part{Standing: setup.PartTold, Words: "3 accounts, 2 working"})
}

// checkLine checks the status line of the router and the review is want.
func checkLine(t *testing.T, rt setup.RouterStanding, review setup.Review, want ...setup.Part) {
	t.Helper()
	if got := setup.StatusLine(rt, review); !slices.Equal(got, want) {
		t.Errorf("the status line is %+v, want %+v", got, want)
	}
}

func checkRows(t *testing.T, what string, got, want []setup.Reviewed) {
	t.Helper()
	if !slices.EqualFunc(got, want, sameRow) {
		t.Errorf("%s are\n%+v\nwant\n%+v", what, got, want)
	}
}

// sameRow reports whether two rows are alike, their tokens' years to the
// instant.
func sameRow(a, b setup.Reviewed) bool {
	ay, by := a.Token.Year, b.Token.Year
	a.Token.Year, b.Token.Year = status.TokenYear{}, status.TokenYear{}
	return a == b && ay.Taken.Equal(by.Taken) && ay.GoodUntil.Equal(by.GoodUntil)
}

func TestReviewOfAFirstRun(t *testing.T) {
	w := newWorld(t)
	s := w.reviewing(t, &checkingAPI{})
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Accounts) != 0 || review.Unmarked {
		t.Errorf("a first run's review is %+v, want no accounts", review)
	}
	if notes := review.Notes(); len(notes) != 0 {
		t.Errorf("a first run's notes are %+v, want none", notes)
	}
	line := setup.StatusLine(setup.RouterStanding{State: setup.RouterDown}, review)
	if want := []setup.Part{{Standing: setup.PartDown, Words: "nothing set up yet"}}; !slices.Equal(line, want) {
		t.Errorf("a first run's status line is %+v, want %+v", line, want)
	}
	if _, err := s.ReviewOf(t.Context(), "work"); !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("ReviewOf an account on a first run: %v, want %v", err, config.ErrNotConfigured)
	}
}

func TestReviewsNotes(t *testing.T) {
	// lastDay is a token's year that's up on the clock's date, later in it.
	lastDay := status.TokenYear{GoodUntil: frameClock.Add(5 * time.Hour)}
	gone := status.TokenYear{GoodUntil: frameClock.Add(-time.Hour)}
	ample := status.TokenYear{GoodUntil: frameClock.AddDate(0, 0, 31)}
	working := func(year status.TokenYear) setup.TokenStanding {
		return setup.TokenStanding{State: setup.TokenWorking, Year: year}
	}
	newToken := "Press t to create a new one, approving it in the browser signed in to Work."
	tests := []struct {
		name  string
		token setup.TokenStanding
		want  []setup.Note
	}{
		{"no token", setup.TokenStanding{}, []setup.Note{{Need: setup.NeedToken, Account: "work", Says: "has no working token, so no requests are routed to it.", Press: newToken, Key: "t"}}},
		{"refused", setup.TokenStanding{State: setup.TokenNotTaken, Year: gone}, []setup.Note{{Need: setup.NeedToken, Account: "work", Says: "has no working token, so no requests are routed to it.", Press: newToken, Key: "t"}}},
		{"unreadable", setup.TokenStanding{State: setup.TokenUnreadable, Why: "the token file <path> isn't a file: replace it with one holding the token"}, []setup.Note{{Need: setup.NeedToken, Account: "work", Says: "has a token setup can't read: the token file <path> isn't a file: replace it with one holding the token", Press: "Put that right, or press t to create a new one, approving it in the browser signed in to Work.", Key: "t"}}},
		{"unchecked, kept", setup.TokenStanding{State: setup.TokenUnchecked, Why: "HTTP 529"}, nil},
		{"unchecked, in its last 30 days", setup.TokenStanding{State: setup.TokenUnchecked, Why: "HTTP 529", Year: lastDay}, []setup.Note{{Need: setup.NeedNewToken, Account: "work", Says: "has a token that expires 27 Oct 2026: last day.", Press: newToken, Key: "t"}}},
		{"working, its date unknown", working(status.TokenYear{}), nil},
		{"31 days left", working(ample), nil},
		{"30 days left", working(status.TokenYear{GoodUntil: frameClock.AddDate(0, 0, 30)}), []setup.Note{{Need: setup.NeedNewToken, Account: "work", Says: "has a token that expires 26 Nov 2026: 30 days left.", Press: newToken, Key: "t"}}},
		{"its last day", working(lastDay), []setup.Note{{Need: setup.NeedNewToken, Account: "work", Says: "has a token that expires 27 Oct 2026: last day.", Press: newToken, Key: "t"}}},
		{"read, its year up", setup.TokenStanding{State: setup.TokenRead, Year: gone}, []setup.Note{{Need: setup.NeedNewToken, Account: "work", Says: "has a token that expired 27 Oct 2026.", Press: newToken, Key: "t"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review := setup.Review{At: frameClock, Accounts: []setup.Reviewed{{ID: "work", Label: "Work", Plan: "pro", Primary: true, Token: tt.token}}}
			if got := review.Notes(); !slices.Equal(got, tt.want) {
				t.Errorf("the notes are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestReviewsNotesByAccount(t *testing.T) {
	ending := setup.TokenStanding{State: setup.TokenWorking, Year: status.TokenYear{GoodUntil: frameClock.AddDate(0, 0, 21)}}
	review := setup.Review{At: frameClock, Unmarked: true, Accounts: []setup.Reviewed{
		{ID: "plain", Label: "Plain", Token: setup.TokenStanding{State: setup.TokenWorking}},
		{ID: "work", Label: "Work", Token: ending},
		{ID: "personal", Label: "Personal", Plan: "pro", Token: ending},
		{ID: "side", Label: "Side", Plan: "pro", Token: setup.TokenStanding{State: setup.TokenNotTaken}},
	}}
	var got []string
	for _, note := range review.Notes() {
		got = append(got, fmt.Sprintf("%s %d", note.Account, note.Need))
	}
	want := []string{
		fmt.Sprintf(" %d", setup.NeedPrimary),
		fmt.Sprintf("side %d", setup.NeedToken),
		fmt.Sprintf("work %d", setup.NeedNewToken),
		fmt.Sprintf("work %d", setup.NeedPlan),
		fmt.Sprintf("personal %d", setup.NeedNewToken),
		fmt.Sprintf("plain %d", setup.NeedPlan),
	}
	if !slices.Equal(got, want) {
		t.Errorf("the notes go %q, want %q: the primary's first, then each account's together, by its most pressing", got, want)
	}
}

func TestReviewOfAnUnreadableToken(t *testing.T) {
	tests := []struct {
		name  string
		place func(w *world)
		// state is how the token stands, and why says, in part, why.
		state setup.TokenState
		why   string
	}{
		{"others can read it", func(w *world) { writeFile(t, w.tokens().Path("work"), workToken+"\n", 0o644) }, setup.TokenUnreadable, "chmod 600"},
		{"a directory", func(w *world) { mkdir(t, w.tokens().Path("work")) }, setup.TokenUnreadable, "isn't a file"},
		{"empty", func(w *world) { writeFile(t, w.tokens().Path("work"), "", 0o600) }, setup.NoToken, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.writeConfig(t, "[[account]]\nid = \"work\"\n")
			tt.place(w)
			api := &checkingAPI{}
			s := w.reviewing(t, api)
			review, err := s.Review()
			if err != nil {
				t.Fatal(err)
			}
			row, err := s.ReviewOf(t.Context(), "work")
			if err != nil {
				t.Fatal(err)
			}
			for _, got := range []setup.TokenStanding{review.Accounts[0].Token, row.Token} {
				if got.State != tt.state || !strings.Contains(got.Why, tt.why) || (tt.why == "") != (got.Why == "") {
					t.Errorf("the token stands %d, why %q, want %d, why with %q", got.State, got.Why, tt.state, tt.why)
				}
			}
			if row.Token.Working() {
				t.Error("a token that can't be read is working")
			}
			if asked := api.askedOf(); len(asked) != 0 {
				t.Errorf("asked the API about %d tokens, want none, with none read", len(asked))
			}
		})
	}
}

func TestReviewsNoPrimaryOfTwo(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\nplan = \"pro\"\n\n[[account]]\nid = \"personal\"\nplan = \"pro\"\n")
	w.writeToken(t, "work", workToken)
	w.writeToken(t, "personal", personalToken)
	s := w.reviewing(t, &checkingAPI{})
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	if !review.Unmarked {
		t.Error("the review doesn't say no account is the primary, of two")
	}
	for _, a := range review.Accounts {
		if a.Primary {
			t.Errorf("%s is the primary, but the config marks none of two", a.ID)
		}
		if a.Label != a.ID {
			t.Errorf("%s is labelled %q, want its id, as it has no label", a.ID, a.Label)
		}
	}
	want := []setup.Note{{
		Need:  setup.NeedPrimary,
		Says:  "No account is the primary, the one the browser and the Claude apps are signed in to.",
		Press: "Choose it, and press p to make it the primary.",
		Key:   "p",
	}}
	if got := review.Notes(); !slices.Equal(got, want) {
		t.Errorf("the notes are\n%+v\nwant\n%+v", got, want)
	}
	row, err := s.ReviewOf(t.Context(), "work")
	if err != nil || row.Primary {
		t.Errorf("ReviewOf(work) is %+v (%v), want it not the primary", row, err)
	}
}

func TestReviewsTheOnlyAccountAsThePrimary(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\nplan = \"pro\"\n")
	review, err := w.reviewing(t, &checkingAPI{}).Review()
	if err != nil {
		t.Fatal(err)
	}
	if review.Unmarked || len(review.Accounts) != 1 || !review.Accounts[0].Primary {
		t.Errorf("the review of one account none marks is %+v, want it the primary", review)
	}
	if got := review.Notes(); len(got) != 1 || got[0].Need != setup.NeedToken {
		t.Errorf("the notes are %+v, want work's missing token's alone", got)
	}
}

func TestReviewOfChecksTheToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  setup.TokenState
		// why is what the API not answering says.
		why string
	}{
		{"working", workToken, setup.TokenWorking, ""},
		{"refused", sideToken, setup.TokenNotTaken, ""},
		{"a 5xx", downToken, setup.TokenUnchecked, "HTTP 529 · Overloaded, for [redacted]"},
		{"a timeout", slowToken, setup.TokenUnchecked, "probe with [redacted]: context deadline exceeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.writeConfig(t, "[[account]]\nid = \"work\"\n")
			w.writeToken(t, "work", tt.token)
			before := w.snapshot(t)
			row, err := w.reviewing(t, &checkingAPI{}).ReviewOf(t.Context(), "work")
			if err != nil {
				t.Fatal(err)
			}
			if row.Token.State != tt.want || row.Token.Why != tt.why {
				t.Errorf("the token stands %d, why %q, want %d, why %q", row.Token.State, row.Token.Why, tt.want, tt.why)
			}
			w.checkUnchanged(t, before)
			w.checkToken(t, "work", tt.token)
		})
	}
}

func TestReviewOfAnAccountWithoutAToken(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\n")
	api := &checkingAPI{}
	row, err := w.reviewing(t, api).ReviewOf(t.Context(), "work")
	if err != nil || row.Token != (setup.TokenStanding{}) {
		t.Errorf("ReviewOf(work) is %+v (%v), want no token", row, err)
	}
	if asked := api.askedOf(); len(asked) != 0 {
		t.Errorf("asked the API about %d tokens, want none, with none to ask about", len(asked))
	}
}

func TestReviewOfFails(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	if _, err := s.ReviewOf(t.Context(), "elsewhere"); !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("ReviewOf an account not configured: %v, want %v", err, config.ErrNotConfigured)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ReviewOf(ctx, "work"); !errors.Is(err, context.Canceled) {
		t.Errorf("ReviewOf once its context is done: %v, want %v", err, context.Canceled)
	}
}

func TestReviewOfManyAtOnce(t *testing.T) {
	w := frame3(t)
	s := w.reviewing(t, &checkingAPI{})
	ids := []string{"work", "personal", "side"}
	want := make(map[string]setup.TokenState)
	for _, id := range ids {
		row, err := s.ReviewOf(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		want[id] = row.Token.State
	}
	const rounds = 8
	var wg sync.WaitGroup
	errs := make(chan error, rounds*len(ids))
	for range rounds {
		for _, id := range ids {
			wg.Go(func() {
				row, err := s.ReviewOf(t.Context(), id)
				switch {
				case err != nil:
					errs <- fmt.Errorf("%s: %w", id, err)
				case row.ID != id || row.Token.State != want[id]:
					errs <- fmt.Errorf("%s: got %+v, want its token %d", id, row, want[id])
				}
			})
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestAccountsPart(t *testing.T) {
	with := func(states ...setup.TokenState) setup.Review {
		var review setup.Review
		for _, state := range states {
			review.Accounts = append(review.Accounts, setup.Reviewed{Token: setup.TokenStanding{State: state}})
		}
		return review
	}
	tests := []struct {
		name   string
		review setup.Review
		want   string
	}{
		{"one, working", with(setup.TokenWorking), "1 account, working"},
		{"one, unchecked", with(setup.TokenUnchecked), "1 account, working"},
		{"one, refused", with(setup.TokenNotTaken), "1 account, none working"},
		{"one, checking", with(setup.TokenRead), "1 account, checking…"},
		{"all working or unchecked", with(setup.TokenWorking, setup.TokenWorking, setup.TokenUnchecked), "3 accounts, all working"},
		{"one refused", with(setup.TokenWorking, setup.TokenWorking, setup.TokenNotTaken), "3 accounts, 2 working"},
		{"one still checking", with(setup.TokenWorking, setup.TokenRead, setup.TokenNotTaken), "3 accounts, checking…"},
		{"none working", with(setup.NoToken, setup.TokenNotTaken, setup.TokenUnreadable), "3 accounts, none working"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			down := setup.RouterStanding{}
			checkLine(t, down, tt.review, setup.Part{Standing: setup.PartDown, Words: "router not running yet"}, setup.Part{Standing: setup.PartTold, Words: tt.want})
		})
	}
}

func TestStatusLine(t *testing.T) {
	two := setup.Review{Accounts: []setup.Reviewed{{ID: "work", Token: setup.TokenStanding{State: setup.TokenWorking}}, {ID: "side"}}}
	accountsPart := setup.Part{Standing: setup.PartTold, Words: "2 accounts, 1 working"}
	tests := []struct {
		name   string
		router setup.RouterStanding
		review setup.Review
		want   []setup.Part
	}{
		{"healthy", setup.RouterStanding{State: setup.RouterHealthy}, two, []setup.Part{{Standing: setup.PartUp, Words: "router up, healthy"}, accountsPart}},
		{"unhealthy", setup.RouterStanding{State: setup.RouterUnhealthy, Reason: "failed 5 of 6 requests"}, two, []setup.Part{{Standing: setup.PartWarning, Words: "router up, unhealthy: failed 5 of 6 requests"}, accountsPart}},
		{"not running", setup.RouterStanding{}, two, []setup.Part{{Standing: setup.PartDown, Words: "router not running yet"}, accountsPart}},
		{"up, with no account", setup.RouterStanding{State: setup.RouterHealthy}, setup.Review{}, []setup.Part{{Standing: setup.PartUp, Words: "router up, healthy"}}},
		{"nothing", setup.RouterStanding{}, setup.Review{}, []setup.Part{{Standing: setup.PartDown, Words: "nothing set up yet"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkLine(t, tt.router, tt.review, tt.want...)
		})
	}
}

func TestStanding(t *testing.T) {
	healthy := setup.RouterStanding{State: setup.RouterHealthy}
	down := setup.RouterStanding{State: setup.RouterDown}
	tests := []struct {
		name string
		// arrange sets the world up before setup is made in it.
		arrange func(t *testing.T, w *world)
		want    setup.RouterStanding
	}{
		{"not running", func(*testing.T, *world) {}, down},
		{"healthy", func(t *testing.T, w *world) { w.installService(t, w.switchboard) }, healthy},
		{"unhealthy", func(t *testing.T, w *world) {
			w.launchd.unhealthy = "failed 5 of 6 requests"
			w.installService(t, w.switchboard)
		}, setup.RouterStanding{State: setup.RouterUnhealthy, Reason: "failed 5 of 6 requests"}},
		{"the installed plist's router, its state elsewhere than this shell's", func(t *testing.T, w *world) {
			w.installService(t, w.switchboard)
			w.env["XDG_STATE_HOME"] = filepath.Join(w.root, "elsewhere")
		}, healthy},
		{"the installed plist's router before this shell's, both answering", func(t *testing.T, w *world) {
			w.installService(t, w.switchboard)
			w.launchd.unhealthy = "failed 5 of 6 requests"
			w.env["XDG_STATE_HOME"] = filepath.Join(w.root, "elsewhere")
			w.launchd.byHand, w.launchd.byHandAt = 4300, filepath.Join(w.root, "elsewhere", "switchboard")
		}, setup.RouterStanding{State: setup.RouterUnhealthy, Reason: "failed 5 of 6 requests"}},
		{"this shell's router, with no plist", func(_ *testing.T, w *world) { byHandHere(w) }, healthy},
		{"this shell's router, the plist unreadable", func(t *testing.T, w *world) {
			writeFile(t, w.plist(), "not a plist", 0o644)
			byHandHere(w)
		}, healthy},
		{"slower than 2s, within launch's wait", func(t *testing.T, w *world) {
			w.installService(t, w.switchboard)
			w.launchd.slow = launch.AskTimeout - time.Second
		}, healthy},
		{"slower than launch waits", func(t *testing.T, w *world) {
			w.installService(t, w.switchboard)
			w.launchd.slow = launch.AskTimeout + time.Second
		}, down},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			tt.arrange(t, w)
			got, err := w.reviewing(t, &checkingAPI{}).Standing(t.Context())
			if err != nil || got != tt.want {
				t.Errorf("Standing is %+v (%v), want %+v", got, err, tt.want)
			}
			if ran := w.launchd.ran(); len(ran) != 0 {
				t.Errorf("Standing ran launchctl %q, want nothing: it only asks the router", ran)
			}
		})
	}
}

// byHandHere has a router run by hand answer where this shell keeps its
// state, as no plist puts it.
func byHandHere(w *world) {
	w.launchd.byHand = 4300
	w.launchd.byHandAt = w.state
}

func TestStandingOnceItsLimitPasses(t *testing.T) {
	w := newWorld(t)
	w.installService(t, w.switchboard)
	w.launchd.stuck = true
	s := w.reviewing(t, &checkingAPI{})
	s.WaitForRouter(50 * time.Millisecond)
	if got, err := s.Standing(t.Context()); err != nil || got != (setup.RouterStanding{State: setup.RouterDown}) {
		t.Errorf("Standing of a router that never answers is %+v (%v), want it down, and no error: the limit passing is setup's, not the caller's", got, err)
	}
}

func TestStandingOnceItsContextIsDone(t *testing.T) {
	w := newWorld(t)
	w.installService(t, w.switchboard)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := w.reviewing(t, &checkingAPI{}).Standing(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Standing once its context is done is %+v (%v), want %v", got, err, context.Canceled)
	}
}

// lockedBuffer is a buffer written from several goroutines at once.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestReviewTellsNoToken(t *testing.T) {
	logged := &lockedBuffer{}
	logs.Init(logs.Options{Level: slog.LevelDebug, Mirror: logged})
	t.Cleanup(func() { logs.Close(0) })

	w := frame3(t)
	w.writeConfig(t, frameConfig+"\n[[account]]\nid = \"down\"\n\n[[account]]\nid = \"slow\"\n\n[[account]]\nid = \"loose\"\n")
	// down's token is unchecked, in its last 30 days, so its note tells its
	// year; loose's, others can read, so its note tells why it can't be read.
	w.writeTokenTaken(t, "down", downToken, personalUntil.Add(-claude.SetupTokenLifetime))
	w.writeToken(t, "slow", slowToken)
	writeFile(t, w.tokens().Path("loose"), slowToken+"\n", 0o644)
	writeFile(t, w.tokens().Path("work")+".taken", "taken "+workToken+"\n", 0o600)
	s := w.reviewing(t, &checkingAPI{})

	var told []string
	review, err := s.Review()
	if err != nil {
		t.Fatal(err)
	}
	told = append(told, fmt.Sprintf("%+v", review))
	var checked []setup.Reviewed
	for _, id := range []string{"work", "personal", "side", "down", "slow", "loose", "elsewhere"} {
		row, err := s.ReviewOf(t.Context(), id)
		told = append(told, fmt.Sprintf("%+v %v", row, err))
		if err == nil {
			checked = append(checked, row)
		}
	}
	review.Accounts = checked
	notes := review.Notes()
	told = append(told, fmt.Sprintf("%+v", notes), fmt.Sprintf("%+v", setup.StatusLine(setup.RouterStanding{}, review)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.ReviewOf(ctx, "down")
	told = append(told, fmt.Sprint(err), logged.String())
	for _, said := range told {
		for _, token := range allTokens {
			if strings.Contains(said, token) {
				t.Errorf("the review told a token, %s, in %q", token, said)
			}
		}
	}
	if !strings.Contains(logged.String(), "not going by the record of when the token was taken") {
		t.Errorf("the log has no line of work's record it can't go by, so it wasn't seen:\n%s", logged.String())
	}
	if !slices.ContainsFunc(notes, func(n setup.Note) bool { return n.Account == "down" && n.Need == setup.NeedNewToken }) ||
		!slices.ContainsFunc(notes, func(n setup.Note) bool {
			return n.Account == "loose" && strings.HasSuffix(n.Says, "chmod 600 "+w.tokens().Path("loose"))
		}) {
		t.Errorf("the notes are %+v, want down's year among them, and why loose's token can't be read, ending in the command to copy", notes)
	}
}
