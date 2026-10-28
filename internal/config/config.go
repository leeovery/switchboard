// Package config locates, parses and validates switchboard's config file.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	defaultListen   = "127.0.0.1:4747"
	defaultUpstream = "https://api.anthropic.com"
)

// How long the readings history and the request ledger's lines are kept
// where the config doesn't say.
const (
	// DefaultHistoryKeep keeps the readings history 400 days, a year with
	// room to spare.
	DefaultHistoryKeep = 400 * 24 * time.Hour
	// DefaultLedgerKeep keeps the request ledger's lines 400 days, a year
	// with room to spare.
	DefaultLedgerKeep = 400 * 24 * time.Hour
)

// Example is a small valid config, for showing someone who doesn't have one yet.
const Example = `# One [[account]] per Claude subscription. Each account's token, made with
# claude setup-token, goes in a file of its own: <state dir>/tokens/<id>, as in
# ~/.local/state/switchboard/tokens/work.

[[account]]
id      = "work"  # for good: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use; else the first
# reserve = 0.1   # optional, on any account: the share of every window the router leaves; else 0

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
	// WhenRouterDown is what claude does when the router isn't running, can't
	// be routed through, or can't be found: the config's, else Stop. A router
	// that answers unhealthy is routed through all the same.
	WhenRouterDown RouterDown
	// WeekStarts is the day the dashboard's calendar weeks start on: the
	// config's, else Monday.
	WeekStarts time.Weekday
	Accounts   Accounts
	// Prime says when priming starts the accounts' 5-hour windows.
	Prime         Prime
	Notifications Notifications
	History       History
	Ledger        Ledger
	Usage         Usage
	// Prices are the config's prices in place of those switchboard carries.
	Prices Prices
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
	// account: the config's, else 0.
	Reserve float64
	// Plan is the plan the account's subscription is on, one of Plans: ""
	// where the config doesn't say, as switchboard can't read it.
	Plan string
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

// InAnotherCase returns the account whose id is the one given but for its
// case, which macOS takes for the same token file, when there's one.
func (as Accounts) InAnotherCase(id string) (Account, bool) {
	i := slices.IndexFunc(as, func(a Account) bool { return a.ID != id && strings.EqualFold(a.ID, id) })
	if i < 0 {
		return Account{}, false
	}
	return as[i], true
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
	return prose.ClockOf(d.Start) + "-" + prose.ClockOf(d.End)
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

// History says how long the readings history is kept.
type History struct {
	// Keep is how long a day's file of the history is kept once its day has
	// ended: a whole number of days, or dayfile.Forever, which keeps every
	// day's.
	Keep time.Duration
}

// Ledger says how long the request ledger's lines are kept.
type Ledger struct {
	// Keep is how long a day's file of the ledger's lines is kept once its
	// day has ended: a whole number of days, or dayfile.Forever, which keeps
	// every day's.
	Keep time.Duration
}

// Usage says how usage and the dashboard lay out the accounts' windows.
type Usage struct {
	// By is the layout they open with: the config's, else ByWindow.
	By Layout
}

// Layout is how the accounts' windows are laid out, as [usage] by and usage
// --by name it.
type Layout string

const (
	// ByWindow compares the accounts a window at a time.
	ByWindow Layout = "window"
	// ByAccount puts each account's windows together.
	ByAccount Layout = "account"
)

// ParseLayout reads a layout by its name, window or account, in any case,
// reporting false for any other text.
func ParseLayout(given string) (Layout, bool) {
	for _, layout := range []Layout{ByWindow, ByAccount} {
		if strings.EqualFold(given, string(layout)) {
			return layout, true
		}
	}
	return "", false
}

// RouterDown is what claude does when the router isn't running, or isn't
// healthy, as when_router_down names it.
type RouterDown string

const (
	// Stop starts nothing, saying why and how to go on.
	Stop RouterDown = "stop"
	// Direct starts Claude Code connected directly to the API, on an
	// account's token.
	Direct RouterDown = "direct"
)

// ParseRouterDown reads what claude does when the router is down by its name,
// stop or direct, in any case, reporting false for any other text.
func ParseRouterDown(given string) (RouterDown, bool) {
	for _, down := range []RouterDown{Stop, Direct} {
		if strings.EqualFold(given, string(down)) {
			return down, true
		}
	}
	return "", false
}

// file is the config file as it's written, before its defaults are filled in.
type file struct {
	Listen   string `toml:"listen"`
	Upstream string `toml:"upstream"`
	// WhenRouterDown is nil when the file doesn't give it.
	WhenRouterDown *string       `toml:"when_router_down"`
	WeekStarts     string        `toml:"week_starts"`
	Accounts       []fileAccount `toml:"account"`
	Prime          filePrime     `toml:"prime"`
	Notifications  Notifications `toml:"notifications"`
	History        fileKeep      `toml:"history"`
	Ledger         fileKeep      `toml:"ledger"`
	Usage          fileUsage     `toml:"usage"`
	Prices         Prices        `toml:"prices"`
}

// fileAccount is an [[account]] table as it's written.
type fileAccount struct {
	ID      string  `toml:"id"`
	Label   string  `toml:"label"`
	Primary bool    `toml:"primary"`
	Reserve float64 `toml:"reserve"`
	Plan    string  `toml:"plan"`
}

// shownAs is the label the account is shown by: its own, else its id.
func (a fileAccount) shownAs() string {
	return cmp.Or(a.Label, a.ID)
}

// filePrime is the [prime] table as it's written.
type filePrime struct {
	Day string `toml:"day"`
}

// fileKeep is a table that says how long something is kept, [history] or
// [ledger], as it's written.
type fileKeep struct {
	// Keep is nil when the table doesn't give one.
	Keep *string `toml:"keep"`
}

// fileUsage is the [usage] table as it's written.
type fileUsage struct {
	// By is nil when the table doesn't give one.
	By *string `toml:"by"`
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
		return nil, invalid(path, err)
	}
	return cfg, nil
}

// invalid is the error of the config file at path, which err says is wrong.
func invalid(path string, err error) error {
	return fmt.Errorf("invalid config %s:\n%w", path, err)
}

// config returns the Config the file describes, with its defaults filled in,
// or every problem with it together, the keys decoding left unused among
// them.
func (f file) config(undecoded []toml.Key) (*Config, error) {
	weekStarts, weekErr := parseWeekStart(f.WeekStarts)
	day, dayErr := ParseDay(f.Prime.Day)
	historyKeep, historyErr := parseKeep("history.keep", f.History.Keep, DefaultHistoryKeep)
	ledgerKeep, ledgerErr := parseKeep("ledger.keep", f.Ledger.Keep, DefaultLedgerKeep)
	by, byErr := parseUsageBy(f.Usage.By)
	down, downErr := parseRouterDown(f.WhenRouterDown)
	err := errors.Join(
		checkKeys(undecoded),
		checkListen(f.Listen),
		checkUpstream(f.Upstream),
		downErr,
		weekErr,
		checkAccounts(f.Accounts),
		dayErr,
		checkWarning(f.Notifications.Warning),
		historyErr,
		ledgerErr,
		byErr,
		checkPrices(f.Prices),
	)
	if err != nil {
		return nil, err
	}
	return &Config{
		Listen:         f.Listen,
		Upstream:       f.Upstream,
		WhenRouterDown: down,
		WeekStarts:     weekStarts,
		Accounts:       resolve(f.Accounts),
		Prime:          Prime{Day: day},
		Notifications:  f.Notifications,
		History:        History{Keep: historyKeep},
		Ledger:         Ledger{Keep: ledgerKeep},
		Usage:          Usage{By: by},
		Prices:         f.Prices,
	}, nil
}

// resolve returns the accounts as they're written, with their defaults filled
// in: the id for a label, and the first account for the primary.
func resolve(written []fileAccount) Accounts {
	accounts := make(Accounts, len(written))
	for i, w := range written {
		accounts[i] = Account{ID: w.ID, Label: w.shownAs(), Primary: w.Primary, Reserve: w.Reserve, Plan: w.Plan}
	}
	accounts[accounts.primary()].Primary = true
	return accounts
}
