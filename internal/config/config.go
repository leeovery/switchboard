// Package config locates, parses and validates switchboard's config file.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/redact"
)

const (
	defaultListen   = "127.0.0.1:4747"
	defaultUpstream = "https://api.anthropic.com"
	// primaryReserve is the primary's reserve where the config doesn't give
	// one. The other accounts' is 0.
	primaryReserve = 0.1
)

// Example is a small valid config, for showing someone who doesn't have one yet.
const Example = `# One [[account]] per Claude subscription. Each account's token, made with
# claude setup-token, goes in a file of its own: <state dir>/tokens/<id>, as in
# ~/.local/state/switchboard/tokens/work.

[[account]]
id      = "work"  # for good: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use; else the first
reserve = 0.1     # optional: the share of every window the router leaves; 0.1 on the primary, else 0

[[account]]
id    = "personal"
label = "Personal"

# Optional: start the 5-hour windows on a staggered schedule over the day, in
# local time; an end before the start means past midnight.
# [prime]
# day = "08:00-23:00"
`

// Config is a validated config file with its defaults filled in.
type Config struct {
	Listen   string
	Upstream string
	Accounts Accounts
	// Prime says when priming starts the accounts' 5-hour windows.
	Prime         Prime
	Notifications Notifications
}

// Account is one Claude subscription.
type Account struct {
	// ID names the account for good, and names its token file.
	ID    string
	Label string
	// Primary is set on the primary account, the one the browser and the
	// Claude apps are signed into: the one the config marks, else the first.
	Primary bool
	// Reserve is the share of every window the router leaves unused on the
	// account: the config's, else 0.1 on the primary and 0 on the others.
	Reserve float64
}

// Accounts are the configured accounts, in the config file's order, which is
// their display order everywhere.
type Accounts []Account

// IDs lists the accounts' ids.
func (as Accounts) IDs() []string {
	ids := make([]string, len(as))
	for i, a := range as {
		ids[i] = a.ID
	}
	return ids
}

// Primary returns the primary account: the one marked primary, else the
// first. There must be an account.
func (as Accounts) Primary() Account {
	return as[as.primary()]
}

// primary is the index of the primary account: the one marked primary, else
// the first.
func (as Accounts) primary() int {
	if i := slices.IndexFunc(as, func(a Account) bool { return a.Primary }); i >= 0 {
		return i
	}
	return 0
}

// Prime says when priming starts the accounts' 5-hour windows.
type Prime struct {
	// Day is the part of each day the windows' resets are spread over: zero
	// when the config gives none.
	Day Day
}

// On reports whether the config turns priming on, by giving it a day.
func (p Prime) On() bool {
	return p.Day != Day{}
}

// Day is a part of each day, in local time: from Start until End, which is
// past midnight when it comes before Start. Every day has an end other than
// its start, so the zero Day is none.
type Day struct {
	// Start and End are times of day, as the time since midnight.
	Start, End time.Duration
}

// String gives the day as [prime] does, such as 08:00-23:00.
func (d Day) String() string {
	return timeOfDay(d.Start) + "-" + timeOfDay(d.End)
}

// timeOfDay gives the time since midnight as HH:MM.
func timeOfDay(since time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(since/time.Hour), int(since%time.Hour/time.Minute))
}

// Notifications says which desktop notifications the router posts.
type Notifications struct {
	// Limits tells of an account reaching a limit, and the sessions it moved.
	Limits bool `toml:"limits"`
	// Room tells of an account that has room again.
	Room bool `toml:"room"`
	// Warning is the share of a window's limit whose passing is told of, or 0
	// to tell of none.
	Warning float64 `toml:"warning"`
	// Moves tells of every other session move, such as after an idle hour or
	// by pin.
	Moves bool `toml:"moves"`
}

// defaultNotifications are those posted where the config doesn't say.
var defaultNotifications = Notifications{Limits: true, Room: true, Warning: 0.9}

// file is the config file as it's written, before its defaults are filled in.
type file struct {
	Listen        string        `toml:"listen"`
	Upstream      string        `toml:"upstream"`
	Accounts      []fileAccount `toml:"account"`
	Prime         filePrime     `toml:"prime"`
	Notifications Notifications `toml:"notifications"`
}

// fileAccount is an [[account]] table as it's written.
type fileAccount struct {
	ID      string `toml:"id"`
	Label   string `toml:"label"`
	Primary bool   `toml:"primary"`
	// Reserve is nil when the table doesn't give one.
	Reserve *float64 `toml:"reserve"`
}

// filePrime is the [prime] table as it's written.
type filePrime struct {
	Day string `toml:"day"`
}

// Load reads the config file at path, validates it and fills in its defaults.
// When the file doesn't exist, the error matches fs.ErrNotExist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	f, meta, err := decodeFile(path, data)
	if err != nil {
		return nil, err
	}
	return f.check(path, meta)
}

// decodeFile reads data as the text of the config file at path, with its
// defaults filled in where it gives none, failing when it isn't TOML or holds
// a value of the wrong type. The error hides anything that looks like a
// token.
func decodeFile(path string, data []byte) (file, toml.MetaData, error) {
	f, meta, err := decode(string(data))
	if err != nil {
		// Not wrapped: the decoder's text quotes the key it stopped at, which
		// can be a token pasted by mistake, as one without a value or given
		// twice would be.
		return file{}, toml.MetaData{}, fmt.Errorf("parse config %s: %s", path, redact.Text(err.Error()))
	}
	return f, meta, nil
}

// decode reads text as a config file's, with its defaults filled in where it
// gives none.
func decode(text string) (file, toml.MetaData, error) {
	f := file{Listen: defaultListen, Upstream: defaultUpstream, Notifications: defaultNotifications}
	meta, err := toml.Decode(text, &f)
	return f, meta, err
}

// check returns the Config the config file at path describes, as decoding it
// found it, or every problem with it together.
func (f file) check(path string, meta toml.MetaData) (*Config, error) {
	cfg, err := f.config(meta.Undecoded())
	if err != nil {
		return nil, fmt.Errorf("invalid config %s:\n%w", path, err)
	}
	return cfg, nil
}

// config returns the Config the file describes, with its defaults filled in,
// or every problem with it together, the keys decoding left unused among
// them.
func (f file) config(undecoded []toml.Key) (*Config, error) {
	day, dayErr := ParseDay(f.Prime.Day)
	err := errors.Join(
		checkKeys(undecoded),
		checkListen(f.Listen),
		checkUpstream(f.Upstream),
		checkAccounts(f.Accounts),
		dayErr,
		checkWarning(f.Notifications.Warning),
	)
	if err != nil {
		return nil, err
	}
	return &Config{
		Listen:        f.Listen,
		Upstream:      f.Upstream,
		Accounts:      resolve(f.Accounts),
		Prime:         Prime{Day: day},
		Notifications: f.Notifications,
	}, nil
}

// resolve returns the accounts as they're written, with their defaults filled
// in: the id for a label, the first account for the primary, and the
// primary's default reserve.
func resolve(written []fileAccount) Accounts {
	accounts := make(Accounts, len(written))
	for i, w := range written {
		accounts[i] = Account{ID: w.ID, Label: cmp.Or(w.Label, w.ID), Primary: w.Primary}
	}
	accounts[accounts.primary()].Primary = true
	for i, w := range written {
		accounts[i].Reserve = reserve(w.Reserve, accounts[i].Primary)
	}
	return accounts
}

// reserve is an account's reserve: the one given, else the primary's default,
// else none.
func reserve(given *float64, primary bool) float64 {
	switch {
	case given != nil:
		return *given
	case primary:
		return primaryReserve
	}
	return 0
}
