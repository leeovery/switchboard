package setup

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// TokenState is how an account's token stands, as far as the review knows.
type TokenState int

const (
	// NoToken is an account whose token file holds no usable token.
	NoToken TokenState = iota
	// TokenRead is a token read from its file, not yet checked with the API.
	TokenRead
	// TokenWorking is a token the API takes.
	TokenWorking
	// TokenRefused is a token the API refuses, which is kept all the same.
	TokenRefused
	// TokenUnchecked is a token the API didn't answer for, which is kept.
	TokenUnchecked
)

// TokenStanding is an account's token, as the review found it.
type TokenStanding struct {
	State TokenState
	// Year is the token's year, as its record says: zero where there's no
	// token, or no record of when it was taken.
	Year status.TokenYear
	// Why is why the API didn't answer for it, with nothing shaped like a
	// token in it: "" but for TokenUnchecked.
	Why string
	// Organization is the id of the organization the API's answer named the
	// token's account in: "" where it named none, or wasn't asked.
	Organization string
}

// Working reports whether the account has a token the API hasn't refused:
// one read and not yet checked, or one it didn't answer for, counts.
func (t TokenStanding) Working() bool {
	return t.State != NoToken && t.State != TokenRefused
}

// Reviewed is an account as setup's review shows it, a row of its table.
type Reviewed struct {
	ID, Label string
	// Plan is the config's, one of config.Plans: "" where it doesn't say.
	Plan string
	// Reserve is the share of every window the router leaves unused on it.
	Reserve float64
	// Primary is set on the primary: the one the config marks, or the only
	// account. With two or more and none marked, none is.
	Primary bool
	Token   TokenStanding
}

// Review is every configured account, in the config's order, as read from
// the config and the token files, no token checked.
type Review struct {
	// At is when it was read, in the local time zone, which a token's days
	// left are counted in.
	At       time.Time
	Accounts []Reviewed
	// Unmarked is set where two accounts or more are configured and none is
	// marked the primary, which one must be.
	Unmarked bool
}

// Review reads every configured account and its token, checking none with
// the API, so the review shows at once: none on a first run.
func (s *Setup) Review() (Review, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return Review{}, err
	}
	review := Review{At: s.now()}
	cfg := draft.Config()
	if cfg == nil {
		return review, nil
	}
	review.Unmarked = len(cfg.Accounts) > 1 && !draft.MarksPrimary()
	for _, a := range cfg.Accounts {
		row, _ := s.reviewed(a, review.Unmarked)
		review.Accounts = append(review.Accounts, row)
	}
	return review, nil
}

// ReviewOf is the row of the account with the given id, its token checked
// with the API: working, refused or unchecked, each kept as it is. It writes
// nothing, and is safe to call for several accounts at once. It fails,
// matching config.ErrNotConfigured, when no account has the id, and with
// ctx's error once ctx is done.
func (s *Setup) ReviewOf(ctx context.Context, id string) (Reviewed, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return Reviewed{}, err
	}
	cfg := draft.Config()
	if cfg == nil {
		return Reviewed{}, config.NotConfigured(id)
	}
	i := slices.IndexFunc(cfg.Accounts, func(a config.Account) bool { return a.ID == id })
	if i < 0 {
		return Reviewed{}, config.NotConfigured(id)
	}
	row, token := s.reviewed(cfg.Accounts[i], len(cfg.Accounts) > 1 && !draft.MarksPrimary())
	if row.Token.State == NoToken {
		return row, nil
	}
	checked, err := s.Accounts.Check(ctx, token)
	switch {
	case errors.Is(err, accounts.ErrRefused):
		row.Token.State = TokenRefused
	case err != nil:
		return Reviewed{}, err
	case checked.Unchecked:
		row.Token.State, row.Token.Why = TokenUnchecked, checked.Why
	default:
		row.Token.State, row.Token.Organization = TokenWorking, checked.Organization
	}
	return row, nil
}

// reviewed is the account's row, its token read but unchecked, and the token
// read, for checking. unmarked says no account is marked the primary, of
// two or more.
func (s *Setup) reviewed(a config.Account, unmarked bool) (Reviewed, tokens.Token) {
	row := Reviewed{ID: a.ID, Label: a.Label, Plan: a.Plan, Reserve: a.Reserve, Primary: a.Primary && !unmarked}
	token, err := s.Accounts.Tokens.Read(a.ID)
	if err != nil {
		return row, tokens.Token{}
	}
	row.Token = TokenStanding{State: TokenRead, Year: s.yearOf(a.ID, token)}
	return row, token
}

// yearOf is the year of the account's token, as the record beside its file
// says, logging why at debug where there's a record it can't go by.
func (s *Setup) yearOf(id string, token tokens.Token) status.TokenYear {
	taken, err := s.Accounts.Tokens.Taken(id, token)
	switch {
	case errors.Is(err, tokens.ErrUnrecorded):
	case err != nil:
		logger.Debug("not going by the record of when the token was taken", "account", id, "error", err)
	default:
		return status.TokenYearFrom(taken)
	}
	return status.TokenYear{}
}

// now reads setup's clock, the registry's, else the system's, in the local
// time zone.
func (s *Setup) now() time.Time {
	if s.Accounts.Now == nil {
		return time.Now()
	}
	return s.Accounts.Now()
}

// Need is what an account is missing, which a note under the review says.
type Need int

const (
	// NeedPrimary is two accounts or more, none the primary.
	NeedPrimary Need = iota
	// NeedToken is an account without a working token.
	NeedToken
	// NeedNewToken is an account whose token's year ends within
	// status.TokenWarning days, or has ended.
	NeedNewToken
	// NeedPlan is an account whose plan isn't set.
	NeedPlan
)

// Note is a note under the review, of something missing and the key that
// sees to it.
type Note struct {
	Need Need
	// Account is the id of the account it's of, which leads it: "" for one of
	// no account alone.
	Account string
	// Says is what's missing, after the account's id; Press, how to see to
	// it, and Key the key that does.
	Says, Press, Key string
}

// Notes are what the review's accounts are missing: the primary first, then
// each kind of need in turn, by the accounts' order, so an account's token's
// year comes before its plan.
func (r Review) Notes() []Note {
	var notes []Note
	if r.Unmarked {
		notes = append(notes, Note{
			Need:  NeedPrimary,
			Says:  "No account is the primary, the one the browser and the Claude apps are signed in to.",
			Press: "Choose it, and press p to make it the primary.",
			Key:   "p",
		})
	}
	for _, need := range []Need{NeedToken, NeedNewToken, NeedPlan} {
		for _, a := range r.Accounts {
			if note, ok := r.note(a, need); ok {
				notes = append(notes, note)
			}
		}
	}
	return notes
}

// note is the note of the account's need, reporting whether it has it.
func (r Review) note(a Reviewed, need Need) (Note, bool) {
	newToken := "Press t to create a new one, approving it in the browser signed in to " + a.Label + "."
	year := a.Token.Year
	switch {
	case need == NeedToken && !a.Token.Working():
		return Note{Need: need, Account: a.ID, Says: "has no working token, so no requests are routed to it.", Press: newToken, Key: "t"}, true
	case need == NeedNewToken && a.Token.Working() && year.EndsWithin(r.At, status.TokenWarning):
		return Note{Need: need, Account: a.ID, Says: "has a token that " + yearEnding(year, r.At) + ".", Press: newToken, Key: "t"}, true
	case need == NeedPlan && a.Plan == "":
		return Note{Need: need, Account: a.ID, Says: "has no plan set, so History can't value its usage.", Press: "Press e to set one.", Key: "e"}, true
	}
	return Note{}, false
}

// yearEnding says when a token's year ends, at now, and the days left of it
// where it hasn't, such as "expires 17 Nov 2026: 21 days left".
func yearEnding(year status.TokenYear, now time.Time) string {
	ends := prose.Expires(now, year.GoodUntil, year.Up(now))
	if left, counted := year.DaysLeft(now); counted {
		ends += ": " + prose.DaysLeft(left)
	}
	return ends
}

// Part is the accounts' part of setup's status line: how many there are, and
// how many have working tokens, such as "3 accounts, 2 working", or "3
// accounts, all working".
func (r Review) Part() Part {
	working := 0
	for _, a := range r.Accounts {
		if a.Token.Working() {
			working++
		}
	}
	words := prose.Grouped(working) + " working"
	if working == len(r.Accounts) {
		words = "all working"
	}
	return Part{Standing: PartTold, Words: prose.Counted(len(r.Accounts), "account") + ", " + words}
}
