package router

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/tokens"
)

// tokenFiles keeps the accounts' tokens as their files hold them while the
// router runs, reading each file again whenever it looks: an account whose
// file holds another token goes out on that one, the one before still
// counting as the account's; one that had no usable token gains the one its
// file comes to hold; and one whose file holds none it can use, two looks in
// a row, has nothing to send on until it's back. A writer that empties a file
// before it writes the token leaves it so for a moment, which one look can
// catch.
type tokenFiles struct {
	accounts accounts
	read     func(id string) (tokens.Token, error)
	now      func() time.Time
	// kept hears that what the state file keeps of the accounts' tokens has
	// changed, and sendable that the accounts requests can go out on have.
	kept, sendable func()
	// replaced hears of each account, by id, that goes out on another token
	// from now on, and gained of each that has a usable token again, or for
	// the first time.
	replaced, gained func(id string)
}

// look reads every account's token file again, and takes up what each
// holds, logging each change, but never a token.
func (f *tokenFiles) look() {
	now := f.now()
	var kept, sendable bool
	for _, a := range f.accounts {
		token, err := f.read(a.ID)
		r := a.secret.reload(token, err, now)
		r.log(a.ID, err)
		if r.replaced {
			f.replaced(a.ID)
		}
		if r.gained {
			f.gained(a.ID)
		}
		kept = kept || r.replaced || r.gained || r.lost
		sendable = sendable || r.gained || r.lost
	}
	if kept {
		f.kept()
	}
	if sendable {
		f.sendable()
	}
}

// retire keeps the accounts' tokens in step with those configured, as the
// config file now makes them, as accounts' retire says: the tokens of one no
// longer configured count as the primary's it makes, until the router
// restarts to take the config up, and after.
func (f *tokenFiles) retire(configured config.Accounts) {
	if f.accounts.retire(configured, f.now()) {
		f.kept()
	}
}

// reloaded is what reading an account's token file again changed of it.
type reloaded struct {
	// replaced is set when the file holds another token than the one the
	// account had, which it goes out on from then on.
	replaced bool
	// gained is set when the account has a usable token and had none before,
	// and lost when it had one and has none now.
	gained, lost bool
	// missed is set when the file held no token the account can use, and the
	// account keeps the one it has unless the file holds none at the next
	// look either.
	missed bool
}

// log logs what reading the token file of the account with the given id
// again changed of it, err saying why it holds no token the account can use,
// when it doesn't.
func (r reloaded) log(id string, err error) {
	if r.replaced {
		logger.Info("token replaced; the one before still counts as the account's", "account", id)
	}
	switch {
	case r.gained:
		logger.Info("account has a usable token; requests can go out on it", "account", id)
	case r.lost:
		logger.Info("account has no usable token; nothing will go out on it until it's back", "account", id, "error", err)
	case r.missed:
		logger.Debug("token file holds no usable token; the account keeps its own unless the next look finds none either", "account", id, "error", err)
	}
}

// reload takes up what the account's token file holds as it's read again at
// now: token, or, when err says why, none the account can use, which an
// account with a usable token takes up only when the file held none the look
// before either. It returns what that changed.
func (s *secret) reload(token tokens.Token, err error, now time.Time) reloaded {
	s.mu.Lock()
	defer s.mu.Unlock()
	had := s.unusable == nil
	switch {
	case err == nil:
		s.unusable, s.missed = nil, false
		return reloaded{replaced: s.swap(token, now), gained: !had}
	case had && !s.missed:
		s.missed = true
		return reloaded{missed: true}
	}
	s.unusable, s.missed = err, false
	return reloaded{lost: had}
}
