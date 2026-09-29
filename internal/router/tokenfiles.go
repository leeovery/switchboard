package router

import (
	"time"

	"github.com/leeovery/switchboard/internal/tokens"
)

// tokenFiles keeps the accounts' tokens as their files hold them while the
// router runs, reading each file again whenever it looks: an account whose
// file holds another token goes out on that one, the one before still
// counting as the account's; one that had no usable token gains the one its
// file comes to hold; and one whose file holds none it can use has nothing
// to send on until it's back.
type tokenFiles struct {
	accounts accounts
	read     func(id string) (tokens.Token, error)
	now      func() time.Time
	// kept hears that what the state file keeps of the accounts' tokens has
	// changed, and sendable that the accounts requests can go out on have.
	kept, sendable func()
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

// reloaded is what reading an account's token file again changed of it.
type reloaded struct {
	// replaced is set when the file holds another token than the one the
	// account had, which it goes out on from then on.
	replaced bool
	// gained is set when the account has a usable token and had none before,
	// and lost when it had one and has none now.
	gained, lost bool
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
	}
}

// reload takes up what the account's token file holds as it's read again at
// now: token, or, when err says why, none the account can use. It returns
// what that changed.
func (s *secret) reload(token tokens.Token, err error, now time.Time) reloaded {
	s.mu.Lock()
	defer s.mu.Unlock()
	had := s.unusable == nil
	s.unusable = err
	if err != nil {
		return reloaded{lost: had}
	}
	return reloaded{replaced: s.swap(token, now), gained: !had}
}
