package router

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/tokens"
)

// account is a configured account, and its token.
type account struct {
	config.Account
	// secret holds the account's token, which every copy of the account
	// shares.
	secret *secret
}

// secret is an account's token as the router holds it: read from its file as
// the router starts, again every so often while it runs, and whenever the
// upstream refuses it, so it can change while requests on the account are
// under way. Once it's replaced, it's still the account's for formerFor, as a
// former token.
type secret struct {
	mu sync.Mutex
	// token is the account's token. While its file holds none the account can
	// use, it's the one the account had last, which a request chosen for the
	// account just before goes out on; it's zero for an account that hasn't
	// had one since the router started.
	token tokens.Token
	// held is the SHA-256 hash of the token the account had last, which the
	// state file keeps: token's, or, while the account hasn't had one since
	// the router started, the one the state file kept, which the token it
	// comes to have is compared with. "" is neither.
	held string
	// unusable says why the account has no usable token, and so nothing to
	// send on, or is nil while it has one.
	unusable error
	// missed is set while the account has a usable token, but its file held
	// none as it was last looked at: the account keeps its token unless the
	// file holds none at the next look either.
	missed bool
	// foreign is held's hash where a probe found that token another
	// subscription's: nothing goes out on it, and the state file keeps the
	// finding, until a probe finds it the account's own, or the account takes
	// up another, which is its own till a probe finds otherwise, as any token
	// taken up is. "" is none.
	foreign string
	// former are the tokens the account had before this one, in the order
	// they were replaced, and those it took up from accounts removed from the
	// config.
	former []formerToken
	// retired is set once the config no longer configures the account, and
	// the primary has taken its tokens up, as they stood then: the state file
	// keeps them as the primary's, and none of the account's own, until the
	// config configures it again.
	retired bool
	// taken is when the token whose hash is takenOf was taken, as its record
	// said as the token was last read from its file: zero where it didn't.
	// It counts only while that's the token held, and is read again with
	// every read of the token, so a token read a moment before its record
	// lands has its date by the next read. untaken is why the record didn't
	// say, where there was one, as last logged.
	taken   time.Time
	takenOf string
	untaken string
}

// TakenFunc reads when switchboard took token as the account's with the
// given id, as tokens.Store's Taken does, failing where that isn't known,
// with tokens.ErrUnrecorded where there's no record.
type TakenFunc func(id string, token tokens.Token) (time.Time, error)

// unrecorded knows of no token's taking.
func unrecorded(string, tokens.Token) (time.Time, error) {
	return time.Time{}, tokens.ErrUnrecorded
}

// takenNote is when a token read from its account's file was taken, as its
// record said: the token's hash, the time, zero where it isn't known, and why
// it isn't, where there's a record that can't be gone by.
type takenNote struct {
	sum string
	at  time.Time
	why string
}

// takenOf reads when token, just read from the account's file, was taken, as
// taken reads it: before the account's lock is taken, as it reads a file, for
// what takes the token up to note under the same hold.
func (a account) takenOf(token tokens.Token, taken TakenFunc) takenNote {
	n := takenNote{sum: hash(token.Reveal())}
	at, err := taken(a.ID, token)
	switch {
	case err == nil:
		n.at = at
	case !errors.Is(err, tokens.ErrUnrecorded):
		n.why = err.Error()
	}
	return n
}

// note holds n, where it's of the token the account holds, and returns why
// its record can't be gone by, where that's news: "" where it isn't, so why
// is logged once until it changes, as the files are read every few seconds.
// s.mu must be held.
func (s *secret) note(n takenNote) string {
	if n.sum != s.held {
		return ""
	}
	news := n.why != "" && n.why != s.untaken
	s.takenOf, s.taken, s.untaken = n.sum, n.at, n.why
	if !news {
		return ""
	}
	return n.why
}

// noteHeld holds n, as note does, taking the account's lock.
func (s *secret) noteHeld(n takenNote) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.note(n)
}

// logUntaken logs why the account's record can't be gone by, at debug,
// where there's news of it.
func (a account) logUntaken(why string) {
	if why != "" {
		logger.Debug("not going by the record of when the token was taken", "account", a.ID, "error", why)
	}
}

// tokenTaken returns when the token the account holds was taken, and the
// token's hash, reporting false where when isn't known, as where it was noted
// of another.
func (s *secret) tokenTaken() (taken time.Time, of string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.taken.IsZero() || s.takenOf != s.held {
		return time.Time{}, "", false
	}
	return s.taken, s.held, true
}

func (s *secret) get() tokens.Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// usable reports whether the account has a usable token of its own.
func (s *secret) usable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unusable == nil && s.foreign == ""
}

// holding reports whether the account's file holds a usable token, its own
// or another subscription's.
func (s *secret) holding() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unusable == nil
}

// lacks says why the account has no usable token of its own: why its file
// holds none, or, while it holds one, whether that's another subscription's.
// It's nil and false while it has one.
func (s *secret) lacks() (unusable error, foreign bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unusable, s.unusable == nil && s.foreign != ""
}

// disown notes whether the token whose hash is sum is another subscription's,
// as a probe on it found, where it's the token the account holds: a probe
// that ends once the account has taken up another says nothing of that one.
// It reports whether that changed what's noted: a token found its own
// changes it only where it was found another's.
func (s *secret) disown(sum string, foreign bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sum != s.held || (s.foreign == sum) == foreign {
		return false
	}
	s.foreign = ""
	if foreign {
		s.foreign = sum
	}
	return true
}

// sends reports whether token is the usable token of the account's own it
// holds, which requests go out on. The comparison takes as long however much
// of the token matches, so response times can't give it away a byte at a
// time.
func (s *secret) sends(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unusable == nil && s.foreign == "" && subtle.ConstantTimeCompare([]byte(s.token.Reveal()), []byte(token)) == 1
}

// disowns reports whether sum is the hash of the token the account holds
// that a probe found another subscription's, whether its file holds it still
// or not. The comparison takes as long however much of the hash matches.
func (s *secret) disowns(sum string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.foreign != "" && subtle.ConstantTimeCompare([]byte(s.foreign), []byte(sum)) == 1
}

// hasToken reports whether the account has a usable token of its own, which
// requests can go out on.
func (a account) hasToken() bool {
	return a.secret.usable()
}

// holdsToken reports whether the account has a usable token, its own or
// another subscription's: a client sending it is the account's, and is
// routed, whether or not anything can go out on the account.
func (a account) holdsToken() bool {
	return a.secret.holding()
}

// problem says why the account has no usable token of its own, or is "" while
// it has one.
func (a account) problem() string {
	problem, _ := a.lacks()
	return problem
}

// lacks says why the account has no usable token of its own, and whether
// that's as the one it holds is another subscription's: "" and false while it
// has one.
func (a account) lacks() (problem string, foreign bool) {
	unusable, foreign := a.secret.lacks()
	switch {
	case unusable != nil:
		return unusable.Error(), false
	case foreign:
		return prose.ForeignTokenFix(a.ID), true
	}
	return "", false
}

// token returns the account's token as the router holds it now: the one it
// had last, while it has none it can use. The account must have had one.
func (a account) token() tokens.Token {
	return a.secret.get()
}

// accounts are the configured accounts, in the order they're shown.
type accounts []account

// resolve reads each configured account's token, as read reads it, and when
// it was taken, as taken reads that, noting why for each account without a
// usable one.
func resolve(configured []config.Account, read func(id string) (tokens.Token, error), taken TakenFunc) accounts {
	resolved := make(accounts, len(configured))
	for i, c := range configured {
		token, err := read(c.ID)
		s := &secret{token: token, unusable: err}
		resolved[i] = account{Account: c, secret: s}
		if err == nil {
			s.held = hash(token.Reveal())
			resolved[i].logUntaken(s.noteHeld(resolved[i].takenOf(token, taken)))
		}
	}
	return resolved
}

// byID returns the account with the given id.
func (as accounts) byID(id string) (account, bool) {
	i := slices.IndexFunc(as, func(a account) bool { return a.ID == id })
	if i < 0 {
		return account{}, false
	}
	return as[i], true
}

// idNow returns the id the account named by the id given has now: the id
// given, where there's an account of it, else the id of the one that had it
// before switchboard made ids, as formerly gives it, where one did, else the
// id given. A nil formerly knows of none.
func (as accounts) idNow(formerly func(id string) (string, bool), id string) string {
	if _, ok := as.byID(id); ok || id == "" || formerly == nil {
		return id
	}
	if now, ok := formerly(id); ok {
		return now
	}
	return id
}

// sender is who a request's token says sent it: the account it's routed as,
// and, where the token is one of an account that has left the config, the id
// of that account, as the token is its own or was taken up from it, "" where
// it's none's.
type sender struct {
	account
	removed string
}

// byToken returns the account a client sending token is, as sender has it.
func (as accounts) byToken(token string, now time.Time) (account, bool) {
	s, ok := as.sender(token, now)
	return s.account, ok
}

// sender returns who a client sending token is: the first account that holds
// it, usable and its own, as sends says, while the config configures it; else
// the first a probe found holding it another subscription's, as disowns says,
// so its client's requests are routed, never passed through on it; else the
// first that had it until it was replaced, or took it up from an account that
// left the config, while the account has a usable token and the one before
// still counts as the account's at now, as a client started before carries on
// sending it; else the first that left the config holding it, usable and its
// own, as its client's requests were routed before it left. So a client of an
// account that has left the config is the account's that took its tokens up
// from the look that found it gone, while that one has a usable token and the
// token still counts, and is routed as the account's own otherwise, while it
// holds it. Each comparison takes as long however much of the token matches,
// so response times can't give a token away a byte at a time.
func (as accounts) sender(token string, now time.Time) (sender, bool) {
	if token == "" {
		return sender{}, false
	}
	sum := hash(token)
	for _, match := range []func(account) (removed string, ok bool){
		func(a account) (string, bool) { return "", !a.secret.left() && a.secret.sends(token) },
		func(a account) (string, bool) {
			if !a.secret.disowns(sum) {
				return "", false
			}
			return a.removedAs(""), true
		},
		func(a account) (string, bool) {
			if !a.holdsToken() {
				return "", false
			}
			f, ok := a.secret.formerOf(sum, now)
			return a.removedAs(f.From), ok
		},
		func(a account) (string, bool) { return a.ID, a.secret.left() && a.secret.sends(token) },
	} {
		for _, a := range as {
			if removed, ok := match(a); ok {
				return sender{account: a, removed: removed}, true
			}
		}
	}
	return sender{}, false
}

// removedAs returns the id of the account that left the config whose token a
// token of a's is, from being the account a took it up from, "" for none:
// from, else a where a has left the config itself, else none.
func (a account) removedAs(from string) string {
	switch {
	case from != "":
		return from
	case a.secret.left():
		return a.ID
	}
	return ""
}

// sendable returns the accounts with a token of their own, which requests can
// go out on.
func (as accounts) sendable() accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !a.hasToken() })
}

// only returns the accounts with the ids given, in order.
func (as accounts) only(ids []string) accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !slices.Contains(ids, a.ID) })
}

// includes reports whether an account with the given id is configured.
func (as accounts) includes(id string) bool {
	_, ok := as.byID(id)
	return ok
}

// configured returns the accounts as the config gives them.
func (as accounts) configured() config.Accounts {
	configured := make(config.Accounts, len(as))
	for i, a := range as {
		configured[i] = a.Account
	}
	return configured
}

// bearer returns the token a request's Authorization header carries, or ""
// when it carries none.
func bearer(h http.Header) string {
	scheme, token, ok := strings.Cut(h.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
