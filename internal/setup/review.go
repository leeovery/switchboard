package setup

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// TokenState is how an account's token stands, as far as the review knows.
type TokenState int

const (
	// NoToken is an account without a token: its token file is missing, or
	// holds nothing.
	NoToken TokenState = iota
	// TokenUnreadable is a token file there that can't be read, as one others
	// can read, or one holding more than a token.
	TokenUnreadable
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
	// token read, or no record of when it was taken.
	Year status.TokenYear
	// Why is why its file can't be read, or why the API didn't answer for
	// it, with nothing shaped like a token in it: "" but for TokenUnreadable
	// and TokenUnchecked.
	Why string
	// Organization is the id of the organization the API's answer named the
	// token's account in: "" where it named none, or wasn't asked.
	Organization string
}

// Working reports whether the account has a token read that the API hasn't
// refused: one not yet checked, or one it didn't answer for, counts.
func (t TokenStanding) Working() bool {
	return t.State == TokenRead || t.State == TokenWorking || t.State == TokenUnchecked
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
	if row.Token.State != TokenRead {
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
	switch {
	case errors.Is(err, tokens.ErrMissing):
		return row, tokens.Token{}
	case err != nil:
		row.Token = TokenStanding{State: TokenUnreadable, Why: redact.Text(err.Error())}
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

// Need is what an account is missing, which a note under the review says,
// the most pressing first.
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
// each account's notes together, its token's before its plan's. Accounts go
// by their most pressing need, so those without a working token come first,
// and then in the config's order.
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
	var each [][]Note
	for _, a := range r.Accounts {
		if its := r.notesOf(a); len(its) > 0 {
			each = append(each, its)
		}
	}
	slices.SortStableFunc(each, func(a, b []Note) int { return cmp.Compare(a[0].Need, b[0].Need) })
	return append(notes, slices.Concat(each...)...)
}

// notesOf are the account's notes, its token's first.
func (r Review) notesOf(a Reviewed) []Note {
	approving := "create a new one, approving it in the browser signed in to " + a.Label + "."
	newToken := Note{Need: NeedToken, Account: a.ID, Press: "Press t to " + approving, Key: "t"}
	var notes []Note
	switch year := a.Token.Year; {
	case a.Token.State == TokenUnreadable:
		// No full stop: the why ends in its own advice, often a command to
		// copy, such as chmod 600 and the file's path.
		newToken.Says, newToken.Press = "has a token setup can't read: "+a.Token.Why, "Put that right, or press t to "+approving
		notes = append(notes, newToken)
	case !a.Token.Working():
		newToken.Says = "has no working token, so no requests are routed to it."
		notes = append(notes, newToken)
	case year.EndsWithin(r.At, status.TokenWarning):
		newToken.Need, newToken.Says = NeedNewToken, "has a token that "+yearEnding(year, r.At)+"."
		notes = append(notes, newToken)
	}
	if a.Plan == "" {
		notes = append(notes, Note{Need: NeedPlan, Account: a.ID, Says: "has no plan set, so History can't value its usage.", Press: "Press e to set one.", Key: "e"})
	}
	return notes
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

// part is the accounts' part of setup's status line, of one account or
// more: how many there are, and how many have working tokens, as working
// says, such as "3 accounts, 2 working".
func (r Review) part() Part {
	return Part{Standing: PartTold, Words: prose.Counted(len(r.Accounts), "account") + ", " + r.working()}
}

// working says how many of the accounts have working tokens: "checking…"
// while any is read but not yet checked; else "all working", or "working"
// of one; "none working"; or how many, as "2 working".
func (r Review) working() string {
	if slices.ContainsFunc(r.Accounts, func(a Reviewed) bool { return a.Token.State == TokenRead }) {
		return "checking…"
	}
	working := 0
	for _, a := range r.Accounts {
		if a.Token.Working() {
			working++
		}
	}
	switch {
	case working == 0:
		return "none working"
	case working < len(r.Accounts):
		return prose.Grouped(working) + " working"
	case working == 1:
		return "working"
	}
	return "all working"
}
