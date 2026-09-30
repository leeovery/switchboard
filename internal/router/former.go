package router

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/tokens"
)

// formerFor is how long a token an account had still counts as the
// account's once it's replaced, or its account removed, when it counts as
// the primary's: sessions started before carry on sending it, and every
// session holds the primary's.
const formerFor = 7 * 24 * time.Hour

// formerToken is a token an account had before its current one, as the
// SHA-256 hash of the token, in hex, never the token, and when it was
// replaced.
type formerToken struct {
	SHA256     string    `json:"sha256"`
	ReplacedAt time.Time `json:"replaced_at"`
	// From is the id of the account it was a token of, when that account left
	// the config and this one, the primary, took its tokens up: "" for one of
	// this account's own.
	From string `json:"from,omitempty"`
}

// counts reports whether the token still counts as its account's at now: it
// was replaced less than formerFor before.
func (f formerToken) counts(now time.Time) bool {
	return now.Sub(f.ReplacedAt) < formerFor
}

func (f formerToken) equal(g formerToken) bool {
	return f.SHA256 == g.SHA256 && f.ReplacedAt.Equal(g.ReplacedAt) && f.From == g.From
}

// savedTokens is what the state file keeps of an account's tokens, never a
// token itself: the SHA-256 hash of the one the router held last, which tells
// the router as it next starts whether the token was replaced while it was
// away, and the account's former tokens.
type savedTokens struct {
	SHA256 string        `json:"sha256,omitempty"`
	Former []formerToken `json:"former,omitempty"`
}

func (s savedTokens) equal(t savedTokens) bool {
	return s.SHA256 == t.SHA256 && slices.EqualFunc(s.Former, t.Former, formerToken.equal)
}

// empty reports whether there's nothing to keep.
func (s savedTokens) empty() bool {
	return s.SHA256 == "" && len(s.Former) == 0
}

// hash returns the SHA-256 hash of token, in hex: all the router keeps of a
// token it doesn't hold.
func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// replace has the account go out on token from now on, keeping the token it
// replaces among its former tokens, replaced at now. It reports whether the
// account held another token: a request refused at the same time may have
// replaced it with token already.
func (s *secret) replace(token tokens.Token, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.swap(token, now)
}

// swap has the account hold token from now on, keeping the one it held
// before, if it held another, among its former tokens, replaced at now. It
// reports whether it replaced one. s.mu must be held.
func (s *secret) swap(token tokens.Token, now time.Time) bool {
	s.token = token
	was := s.held
	s.held = hash(token.Reveal())
	if was == "" || was == s.held {
		return false
	}
	s.former = append(s.former, formerToken{SHA256: was, ReplacedAt: now.UTC()})
	return true
}

// adopt has the tokens of the account from, removed from the config, as the
// state file keeps them, count as this account's former tokens: the one the
// router held last as replaced at now, and those it had before as they were,
// as take says. It reports whether it adopted any.
func (s *secret) adopt(from string, removed savedTokens, now time.Time) bool {
	adopted := append(slices.Clone(removed.Former), formerToken{SHA256: removed.SHA256, ReplacedAt: now.UTC()})
	for i := range adopted {
		adopted[i].From = from
	}
	return s.take(adopted, now)
}

// take takes tokens up among the account's former tokens, those that still
// count at now, but for any the account has already, and reports whether it
// took any.
func (s *secret) take(tokens []formerToken, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	took := false
	for _, f := range tokens {
		if f.SHA256 == "" || !f.counts(now) || s.knows(f.SHA256) {
			continue
		}
		s.former = append(s.former, f)
		took = true
	}
	return took
}

// knows reports whether sum is the hash of the token the account holds, or
// of one it had before. s.mu must be held.
func (s *secret) knows(sum string) bool {
	return sum == s.held || slices.ContainsFunc(s.former, func(f formerToken) bool { return f.SHA256 == sum })
}

// giveBack gives up the former tokens the account took up from the account
// from, and returns them, as that account's own.
func (s *secret) giveBack(from string) []formerToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	var back, kept []formerToken
	for _, f := range s.former {
		if f.From != from {
			kept = append(kept, f)
			continue
		}
		f.From = ""
		back = append(back, f)
	}
	s.former = kept
	return back
}

// retire marks the account as the config no longer configures it, and
// returns its tokens as they stand, for the primary to take up. It reports
// false when the account was retired already: the primary takes its tokens
// up once, as they stood as it left the config, and none it comes to have
// after.
func (s *secret) retire() (savedTokens, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retired {
		return savedTokens{}, false
	}
	s.retired = true
	return savedTokens{SHA256: s.held, Former: slices.Clone(s.former)}, true
}

// unretire marks the account as the config configures it again, its tokens
// its own, and reports whether it was retired.
func (s *secret) unretire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.retired
	s.retired = false
	return was
}

// was reports whether the token whose hash is sum is one the account had
// before, which still counts as the account's at now. Each comparison takes as
// long however much of the hash matches.
func (s *secret) was(sum string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.former, func(f formerToken) bool {
		return f.counts(now) && subtle.ConstantTimeCompare([]byte(f.SHA256), []byte(sum)) == 1
	})
}

// kept is what the state file keeps of the account's tokens: none while it's
// retired, as the primary's keep them.
func (s *secret) kept() savedTokens {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retired {
		return savedTokens{}
	}
	return savedTokens{SHA256: s.held, Former: slices.Clone(s.former)}
}

// recall takes in what the state file kept of the account's tokens, as the
// router starts at now: its former tokens that still count, and the token the
// router held last, as replaced now, when the account's file has held another
// since. It reports whether that happened. Of an account without a usable
// token now, it can't tell: the token the account comes to have is compared
// with that one instead, as it's taken up.
func (s *secret) recall(saved savedTokens, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.former = slices.DeleteFunc(slices.Clone(saved.Former), func(f formerToken) bool { return !f.counts(now) })
	if s.held == "" {
		s.held = saved.SHA256
		return false
	}
	replaced := saved.SHA256 != "" && saved.SHA256 != s.held
	if replaced {
		s.former = append(s.former, formerToken{SHA256: saved.SHA256, ReplacedAt: now.UTC()})
	}
	return replaced
}

// forget forgets the former tokens that no longer count at now, and reports
// whether there were any.
func (s *secret) forget(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := len(s.former)
	s.former = slices.DeleteFunc(s.former, func(f formerToken) bool { return !f.counts(now) })
	return len(s.former) < held
}

// kept is what the state file keeps of the accounts' tokens, by id: each
// with anything to keep has its own, whether it has a usable token now or
// not.
func (as accounts) kept() map[string]savedTokens {
	kept := make(map[string]savedTokens)
	for _, a := range as {
		if saved := a.secret.kept(); !saved.empty() {
			kept[a.ID] = saved
		}
	}
	return kept
}

// recall takes in what the state file kept of the accounts' tokens, as the
// router starts at now, noting in the log each account whose token was
// replaced while the router was away. Each configured account takes back the
// tokens the primary took up from it as it left the config, and the primary
// adopts those of accounts no longer configured, as adopt says. It reports
// whether what's to be kept now differs from what was.
func (as accounts) recall(saved map[string]savedTokens, now time.Time) bool {
	for _, a := range as {
		if a.secret.recall(saved[a.ID], now) {
			logger.Info("token replaced while the router was away; the one before still counts as the account's", "account", a.ID)
		}
	}
	for _, a := range as {
		if as.reclaim(a, now) {
			logger.Info("account configured again; its tokens are its own again", "account", a.ID)
		}
	}
	if primary, ok := as.primary(as.configured()); ok {
		for _, id := range slices.Sorted(maps.Keys(saved)) {
			if !as.includes(id) {
				adopt(primary, id, saved[id], now)
			}
		}
	}
	return !maps.EqualFunc(as.kept(), saved, savedTokens.equal)
}

// retire keeps the accounts' tokens in step with the accounts configured, as
// the config file now makes them, and reports whether that changed what's
// kept of them. The router takes a config up only as it restarts, which can
// be hours coming, so the tokens of an account the config no longer
// configures count as the primary's it makes at once, as leave says, and a
// session started with one stays routed; one the config configures again is
// an account like any other again. With none configured, as before the
// config file has changed, nothing changes.
func (as accounts) retire(configured config.Accounts, now time.Time) bool {
	if len(configured) == 0 {
		return false
	}
	ids := configured.IDs()
	changed := false
	for _, a := range as {
		if slices.Contains(ids, a.ID) {
			changed = as.restore(a, now) || changed
		} else {
			changed = as.leave(configured, a, now) || changed
		}
	}
	return changed
}

// leave has the primary the accounts configured make adopt the tokens of a,
// which they're without, as they stand now, as adopt says: once, as a leaves
// the config, as a token a comes to have after isn't the primary's. It
// reports whether a left, which it doesn't while the primary isn't among the
// accounts, to leave its tokens to the primary as the router next starts.
func (as accounts) leave(configured config.Accounts, a account, now time.Time) bool {
	primary, ok := as.primary(configured)
	if !ok {
		return false
	}
	tokens, left := a.secret.retire()
	if left {
		adopt(primary, a.ID, tokens, now)
	}
	return left
}

// restore has a, which the config configures again, take back the tokens
// the primary took up from it as it left, and reports whether it had left.
func (as accounts) restore(a account, now time.Time) bool {
	if !a.secret.unretire() {
		return false
	}
	as.reclaim(a, now)
	logger.Info("account configured again; its tokens are its own again", "account", a.ID)
	return true
}

// reclaim has a take back, as its own, the tokens any account took up from
// it as it left the config, and reports whether any had.
func (as accounts) reclaim(a account, now time.Time) bool {
	reclaimed := false
	for _, b := range as {
		if back := b.secret.giveBack(a.ID); len(back) > 0 {
			a.secret.take(back, now)
			reclaimed = true
		}
	}
	return reclaimed
}

// adopt has primary take up the tokens of the account with the given id,
// removed from the config, as the state file keeps them, as its former
// tokens, the one the router held last replaced at now, for a week: a
// session started with one carries on sending it, and is routed as the
// primary's. It logs the account whose tokens the primary adopted.
func adopt(primary account, id string, tokens savedTokens, now time.Time) {
	if primary.secret.adopt(id, tokens, now) {
		logger.Info("account no longer configured; its tokens count as the primary's for a week", "account", id, "primary", primary.ID)
	}
}

// primary returns the account the accounts configured make the primary, when
// it's among these.
func (as accounts) primary(configured config.Accounts) (account, bool) {
	if len(configured) == 0 {
		return account{}, false
	}
	return as.byID(configured.Primary().ID)
}

// forget forgets the accounts' former tokens that no longer count at now, and
// reports whether there were any.
func (as accounts) forget(now time.Time) bool {
	forgot := false
	for _, a := range as {
		forgot = a.secret.forget(now) || forgot
	}
	return forgot
}
