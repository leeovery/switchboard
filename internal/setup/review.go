package setup

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
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
	// TokenNotTaken is a token the API refuses, which is kept all the same.
	TokenNotTaken
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
	// sum is the token's hash, as tokens.Token.Sum has it, which tells the
	// token read from another read later: "" where none was read.
	sum string
}

// Of is the standing, of the token given: its hash kept, never the token,
// as what tells it from a token read later, which Keeping goes by.
func (t TokenStanding) Of(token tokens.Token) TokenStanding {
	t.sum = token.Sum()
	return t
}

// answered reports whether the API answered for the token: it took it, or
// refused it.
func (t TokenStanding) answered() bool {
	return t.State == TokenWorking || t.State == TokenNotTaken
}

// Working reports whether the account has a token read that the API hasn't
// refused: one not yet checked, or one it didn't answer for, counts.
func (t TokenStanding) Working() bool {
	return t.State == TokenRead || t.State == TokenWorking || t.State == TokenUnchecked
}

// TokenTold is how the review's TOKEN column tells a token.
type TokenTold int

const (
	// ToldByYear is a working token whose year is known, told by its year.
	ToldByYear TokenTold = iota
	// ToldUndated is a working token whose year isn't known.
	ToldUndated
	// ToldRefused is a token the API refuses.
	ToldRefused
	// ToldInWords is a token told by its words alone: one being checked,
	// unchecked, missing or unreadable.
	ToldInWords
)

// checking is what's said of a token read but not yet checked, and of the
// accounts while any is.
const checking = "checking…"

// Told is how the review's TOKEN column tells the token, and its words: none
// for one told by its year.
func (t TokenStanding) Told() (TokenTold, string) {
	switch t.State {
	case TokenWorking:
		if t.Year.Known() {
			return ToldByYear, ""
		}
		return ToldUndated, "working · expiry date unknown"
	case TokenNotTaken:
		return ToldRefused, "rejected: revoked or expired"
	case TokenRead:
		return ToldInWords, checking
	case TokenUnchecked:
		return ToldInWords, "unchecked"
	case TokenUnreadable:
		return ToldInWords, "can't read"
	}
	return ToldInWords, "no token"
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

// PlanName is the account's plan as the price table names it, as PlanName
// has it.
func (r Reviewed) PlanName() string {
	return PlanName(r.Plan)
}

// PlanName is the plan, one of config.Plans, as the price table names it,
// such as "Max 20x": "" for "", none, or one the table doesn't know.
func PlanName(plan string) string {
	if p, ok := ledger.Pricing.Plan(plan); ok {
		return p.Name
	}
	return ""
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
		row.Token.State = TokenNotTaken
	case err != nil:
		return Reviewed{}, err
	case checked.Unchecked:
		row.Token.State, row.Token.Why = TokenUnchecked, checked.Why
	default:
		row.Token.State, row.Token.Organization = TokenWorking, checked.Organization
	}
	return row, nil
}

// Checked is the review with what ReviewOf answered of the account id, asked
// once the review was read, folded in, where the review read its token and
// hasn't checked it: the token as checked, read since where it's been
// replaced; or, where ReviewOf failed, or answered with the token unchecked
// still, kept unchecked, saying why where it failed. An answer of an account
// the review doesn't have changes nothing. One asked before the review was
// read is Keeping's to fold, as its token may since have been replaced.
func (r Review) Checked(id string, row Reviewed, err error) Review {
	i := slices.IndexFunc(r.Accounts, func(a Reviewed) bool { return a.ID == id })
	if i < 0 || r.Accounts[i].Token.State != TokenRead {
		return r
	}
	token := row.Token
	switch {
	case err != nil:
		token = r.Accounts[i].Token
		token.State, token.Why = TokenUnchecked, redact.Text(err.Error())
	case token.State == TokenRead:
		token.State = TokenUnchecked
	}
	r.Accounts = slices.Clone(r.Accounts)
	r.Accounts[i].Token = token
	return r
}

// Keeping is the review, read afresh, with each token it read and hasn't
// checked that earlier has the API's answer for, the same token, as the API
// answered: working, its organization named, or refused. Its year is as
// read afresh, as the record of when it was taken may have changed. A token
// earlier has unchecked is checked again.
func (r Review) Keeping(earlier Review) Review {
	r.Accounts = slices.Clone(r.Accounts)
	for i, a := range r.Accounts {
		j := slices.IndexFunc(earlier.Accounts, func(e Reviewed) bool { return e.ID == a.ID })
		if a.Token.State != TokenRead || j < 0 {
			continue
		}
		if was := earlier.Accounts[j].Token; was.answered() && was.sum == a.Token.sum {
			kept := &r.Accounts[i].Token
			kept.State, kept.Organization, kept.Why = was.State, was.Organization, was.Why
		}
	}
	return r
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
	row.Token = TokenStanding{State: TokenRead, Year: s.yearOf(a.ID, token)}.Of(token)
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
		return checking
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
