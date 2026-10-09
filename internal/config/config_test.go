package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
)

func TestLoad(t *testing.T) {
	path := writeConfig(t, `
listen      = "[::1]:9000"
upstream    = "http://127.0.0.1:8080"
week_starts = "Sunday"

[[account]]
id      = "work"
label   = "Work"
reserve = 0.05
plan    = "max20x"

[[account]]
id      = "personal"
label   = "Personal"
primary = true
reserve = 0.2

[[account]]
id   = "side"
plan = "pro"

[prime]
day = "07:30-22:45"

[notifications]
limits  = false
room    = true
warning = 0.75
moves   = true

[history]
keep = "30d"

[ledger]
keep = "120d"

[prices.plans]
max5x = 90
pro   = 0

[prices.models.claude-opus-5-5]
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4

[prices.models.claude-test-1]
output = 12.5
`)
	want := &config.Config{
		Listen:     "[::1]:9000",
		Upstream:   "http://127.0.0.1:8080",
		WeekStarts: time.Sunday,
		Accounts: []config.Account{
			{ID: "work", Label: "Work", Reserve: 0.05, Plan: "max20x"},
			{ID: "personal", Label: "Personal", Primary: true, Reserve: 0.2},
			{ID: "side", Label: "side", Plan: "pro"},
		},
		Prime:         config.Prime{Day: config.Day{Start: 7*time.Hour + 30*time.Minute, End: 22*time.Hour + 45*time.Minute}},
		Notifications: config.Notifications{Room: true, Warning: 0.75, Moves: true},
		History:       config.History{Keep: 30 * 24 * time.Hour},
		Ledger:        config.Ledger{Keep: 120 * 24 * time.Hour},
		Prices: config.Prices{
			Plans: map[string]float64{"max5x": 90, "pro": 0},
			Models: map[string]config.ModelPrices{
				"claude-opus-5-5": {Input: new(3.2), Output: new(16.0), CacheRead: new(0.16), CacheWrite5m: new(4.0), CacheWrite1h: new(6.4)},
				"claude-test-1":   {Output: new(12.5)},
			},
		},
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	path := writeConfig(t, `
[[account]]
id = "work"

[[account]]
id    = "personal"
label = "Personal"
`)
	want := &config.Config{
		Listen:     "127.0.0.1:4747",
		Upstream:   "https://api.anthropic.com",
		WeekStarts: time.Monday,
		Accounts: []config.Account{
			{ID: "work", Label: "work", Primary: true},
			{ID: "personal", Label: "Personal"},
		},
		Notifications: config.Notifications{Limits: true, Room: true, Warning: 0.9},
		History:       config.History{Keep: 400 * 24 * time.Hour},
		Ledger:        config.Ledger{Keep: 400 * 24 * time.Hour},
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
	if got.Prime.On() {
		t.Errorf("Load() primes on %+v, want priming off without a day", got.Prime.Day)
	}
}

func TestLoadThePrimary(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   []config.Account
	}{
		{
			name:   "the first, without one marked",
			config: accountTOML("work") + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side"}},
		},
		{
			name:   "the one marked",
			config: accountTOML("work") + accountTOML("side") + "primary = true\n",
			want:   []config.Account{{ID: "work", Label: "work"}, {ID: "side", Label: "side", Primary: true}},
		},
		{
			name:   "the first, with the others marked not to be",
			config: accountTOML("work") + "primary = false\n" + accountTOML("side") + "primary = false\n",
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.config))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !slices.Equal(cfg.Accounts, tt.want) {
				t.Errorf("Load() accounts = %+v, want %+v", cfg.Accounts, tt.want)
			}
		})
	}
}

func TestLoadTheReserves(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   []config.Account
	}{
		{
			name:   "none without one given, on the primary as on the others",
			config: accountTOML("work") + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side"}},
		},
		{
			name:   "the primary's, given",
			config: accountTOML("work") + "reserve = 0.25\n" + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.25}, {ID: "side", Label: "side"}},
		},
		{
			name:   "another account's, given",
			config: accountTOML("work") + accountTOML("side") + "reserve = 0.05\n",
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side", Reserve: 0.05}},
		},
		{
			name:   "another account's, given, with the primary marked",
			config: accountTOML("work") + "reserve = 0.05\n" + accountTOML("side") + "primary = true\n",
			want:   []config.Account{{ID: "work", Label: "work", Reserve: 0.05}, {ID: "side", Label: "side", Primary: true}},
		},
		{
			name:   "each account's, given",
			config: accountTOML("work") + "reserve = 0.1\n" + accountTOML("side") + "reserve = 0.2\n",
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.1}, {ID: "side", Label: "side", Reserve: 0.2}},
		},
		{
			name:   "none, given as 0",
			config: accountTOML("work") + "reserve = 0\n" + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.config))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !slices.Equal(cfg.Accounts, tt.want) {
				t.Errorf("Load() accounts = %+v, want %+v", cfg.Accounts, tt.want)
			}
		})
	}
}

func TestLoadThePrimingDay(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   config.Day
	}{
		{name: "none without the table", want: config.Day{}},
		{name: "none with the table empty", config: "[prime]\n", want: config.Day{}},
		{name: "within a day", config: "[prime]\nday = \"08:00-23:00\"\n", want: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}},
		{name: "past midnight", config: "[prime]\nday = \"22:15-02:30\"\n", want: config.Day{Start: 22*time.Hour + 15*time.Minute, End: 2*time.Hour + 30*time.Minute}},
		{name: "ending at midnight", config: "[prime]\nday = \"08:00-00:00\"\n", want: config.Day{Start: 8 * time.Hour}},
		{name: "from midnight", config: "[prime]\nday = \"00:00-23:59\"\n", want: config.Day{End: 23*time.Hour + 59*time.Minute}},
		{name: "as an inline table", config: "prime = { day = \"06:00-21:00\" }\n", want: config.Day{Start: 6 * time.Hour, End: 21 * time.Hour}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.config+accountTOML("work")))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Prime.Day != tt.want {
				t.Errorf("Load() day = %+v, want %+v", cfg.Prime.Day, tt.want)
			}
			if on, want := cfg.Prime.On(), tt.want != (config.Day{}); on != want {
				t.Errorf("Load() primes: %v, want %v", on, want)
			}
		})
	}
}

func TestLoadTheDayTheWeeksStartOn(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   time.Weekday
	}{
		{name: "monday, without one given", want: time.Monday},
		{name: "monday, given", config: "week_starts = \"monday\"\n", want: time.Monday},
		{name: "sunday, its name capitalised", config: "week_starts = \"Sunday\"\n", want: time.Sunday},
		{name: "saturday, in capitals", config: "week_starts = \"SATURDAY\"\n", want: time.Saturday},
		{name: "wednesday, in mixed case", config: "week_starts = \"wEdNeSdAy\"\n", want: time.Wednesday},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.config+accountTOML("work")))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.WeekStarts != tt.want {
				t.Errorf("Load() starts the weeks on %v, want %v", cfg.WeekStarts, tt.want)
			}
		})
	}
}

func TestLoadEachPlanAnAccountCanBeOn(t *testing.T) {
	for _, plan := range []string{"pro", "max5x", "max20x"} {
		cfg, err := config.Load(writeConfig(t, accountTOML("work")+"plan = \""+plan+"\"\n"+accountTOML("side")+"\n[prices.plans]\n"+plan+" = 1\n"))
		if err != nil {
			t.Fatalf("Load() of an account on %s error = %v", plan, err)
		}
		if cfg.Accounts[0].Plan != plan || cfg.Accounts[1].Plan != "" || cfg.Prices.Plans[plan] != 1 {
			t.Errorf("Load() has work on %q and side on %q, the plans priced %v, want work on %s, side's plan unknown, and %s priced at $1",
				cfg.Accounts[0].Plan, cfg.Accounts[1].Plan, cfg.Prices.Plans, plan, plan)
		}
	}
	if !slices.Equal(config.Plans, []string{"pro", "max5x", "max20x"}) {
		t.Errorf("Plans = %q, want pro, max5x and max20x", config.Plans)
	}
}

func TestAModelsPricesAreCompleteWithEveryKindOfTokenGiven(t *testing.T) {
	every := config.ModelPrices{Input: new(1.0), Output: new(2.0), CacheRead: new(0.0), CacheWrite5m: new(3.0), CacheWrite1h: new(4.0)}
	if !every.Complete() {
		t.Errorf("%+v isn't complete, want it to be, a price of 0 among them", every)
	}
	for name, without := range map[string]func(*config.ModelPrices){
		"input":          func(m *config.ModelPrices) { m.Input = nil },
		"output":         func(m *config.ModelPrices) { m.Output = nil },
		"cache_read":     func(m *config.ModelPrices) { m.CacheRead = nil },
		"cache_write_5m": func(m *config.ModelPrices) { m.CacheWrite5m = nil },
		"cache_write_1h": func(m *config.ModelPrices) { m.CacheWrite1h = nil },
	} {
		some := every
		without(&some)
		if some.Complete() {
			t.Errorf("the prices without %s are complete, want them not to be", name)
		}
	}
}

func TestADayReadsAsPrimeGivesIt(t *testing.T) {
	for _, text := range []string{"08:00-23:00", "22:15-02:30", "08:00-00:00", "00:00-23:59", "07:05-19:45"} {
		if day, err := config.ParseDay(text); err != nil || day.String() != text {
			t.Errorf("ParseDay(%q) = %v, %v; want a day that reads as it", text, day, err)
		}
	}
}

func TestAccountsIDs(t *testing.T) {
	accounts := config.Accounts{{ID: "work", Label: "Work"}, {ID: "side", Label: "Side"}}
	if got, want := accounts.IDs(), []string{"work", "side"}; !slices.Equal(got, want) {
		t.Errorf("IDs() = %q, want %q", got, want)
	}
}

func TestAccountsPrimary(t *testing.T) {
	tests := []struct {
		name     string
		accounts config.Accounts
		want     string
	}{
		{name: "the one marked", accounts: config.Accounts{{ID: "work"}, {ID: "side", Primary: true}}, want: "side"},
		{name: "the first, without one marked", accounts: config.Accounts{{ID: "work"}, {ID: "side"}}, want: "work"},
		{name: "the only one", accounts: config.Accounts{{ID: "work"}}, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.accounts.Primary().ID; got != tt.want {
				t.Errorf("Primary() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestLoadAcceptsLoopbackListenAndHTTPUpstream(t *testing.T) {
	tests := []struct {
		name     string
		listen   string
		upstream string
	}{
		{name: "IPv4 loopback", listen: "127.0.0.1:4747", upstream: "https://api.anthropic.com"},
		{name: "IPv6 loopback", listen: "[::1]:4747", upstream: "https://api.anthropic.com"},
		{name: "elsewhere in 127.0.0.0/8", listen: "127.0.0.2:65535", upstream: "https://api.anthropic.com"},
		{name: "plain http upstream on IPv4 loopback", listen: "127.0.0.1:4747", upstream: "http://127.0.0.1:8080"},
		{name: "plain http upstream on IPv6 loopback", listen: "127.0.0.1:4747", upstream: "http://[::1]:8080"},
		{name: "https upstream with a path", listen: "127.0.0.1:4747", upstream: "https://example.com/anthropic"},
		{name: "https upstream on localhost", listen: "127.0.0.1:4747", upstream: "https://localhost:8443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := fmt.Sprintf("listen = %q\nupstream = %q\n", tt.listen, tt.upstream) + accountTOML("work")

			cfg, err := config.Load(writeConfig(t, content))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Listen != tt.listen || cfg.Upstream != tt.upstream {
				t.Errorf("Load() listen, upstream = %q, %q, want %q, %q", cfg.Listen, cfg.Upstream, tt.listen, tt.upstream)
			}
		})
	}
}

func TestLoadNotifications(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   config.Notifications
	}{
		{name: "limits, room and warnings at 90% without the table", want: config.Notifications{Limits: true, Room: true, Warning: 0.9}},
		{name: "the same with the table empty", config: "[notifications]\n", want: config.Notifications{Limits: true, Room: true, Warning: 0.9}},
		{name: "without limits", config: "[notifications]\nlimits = false\n", want: config.Notifications{Room: true, Warning: 0.9}},
		{name: "without room again", config: "[notifications]\nroom = false\n", want: config.Notifications{Limits: true, Warning: 0.9}},
		{name: "warning at another share", config: "[notifications]\nwarning = 0.75\n", want: config.Notifications{Limits: true, Room: true, Warning: 0.75}},
		{name: "without warnings", config: "[notifications]\nwarning = 0\n", want: config.Notifications{Limits: true, Room: true}},
		{name: "with moves", config: "[notifications]\nmoves = true\n", want: config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true}},
		{name: "as an inline table", config: "notifications = { moves = true }\n", want: config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true}},
		{
			name:   "none at all",
			config: "[notifications]\nlimits  = false\nroom    = false\nwarning = 0\nmoves   = false\n",
			want:   config.Notifications{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.config+accountTOML("work")))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Notifications != tt.want {
				t.Errorf("Load() notifications = %+v, want %+v", cfg.Notifications, tt.want)
			}
		})
	}
}

// keeps are the tables that say how long something is kept: each by its
// name, how long it keeps it where it doesn't say, and how long the config
// loaded from it has it kept.
var keeps = []struct {
	table     string
	byDefault time.Duration
	kept      func(*config.Config) time.Duration
}{
	{table: "history", byDefault: 400 * 24 * time.Hour, kept: func(cfg *config.Config) time.Duration { return cfg.History.Keep }},
	{table: "ledger", byDefault: 400 * 24 * time.Hour, kept: func(cfg *config.Config) time.Duration { return cfg.Ledger.Keep }},
}

func TestLoadHowLongTheHistoryAndTheLedgerAreKept(t *testing.T) {
	const day = 24 * time.Hour
	for _, kept := range keeps {
		keep := func(given string) string { return "[" + kept.table + "]\nkeep = \"" + given + "\"\n" }
		tests := []struct {
			name   string
			config string
			want   time.Duration
		}{
			{name: "its default without the table", want: kept.byDefault},
			{name: "its default with the table empty", config: "[" + kept.table + "]\n", want: kept.byDefault},
			{name: "a week and a day, the least", config: keep("8d"), want: 8 * day},
			{name: "a thousand days", config: keep("1000d"), want: 1000 * day},
			{name: "the most days a duration holds", config: keep("106751d"), want: 106751 * day},
			{name: "forever", config: keep("forever"), want: dayfile.Forever},
			{name: "forever, given more days than a duration holds", config: keep("106752d"), want: dayfile.Forever},
			{name: "forever, given more days than a count holds", config: keep("99999999999999999999d"), want: dayfile.Forever},
			{name: "as an inline table", config: kept.table + " = { keep = \"30d\" }\n", want: 30 * day},
		}
		t.Run(kept.table, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					cfg, err := config.Load(writeConfig(t, tt.config+accountTOML("work")))
					if err != nil {
						t.Fatalf("Load() error = %v", err)
					}
					if got := kept.kept(cfg); got != tt.want {
						t.Errorf("Load() keeps the %s %v, want %v", kept.table, got, tt.want)
					}
				})
			}
		})
	}
}

func TestLoadReportsAKeepThatIsntOne(t *testing.T) {
	tests := []struct {
		name  string
		given string
	}{
		{name: "short of a week and a day", given: "7d"},
		{name: "none", given: "0d"},
		{name: "without its unit", given: "14"},
		{name: "in weeks", given: "2w"},
		{name: "without a number", given: "d"},
		{name: "below none", given: "-1d"},
		{name: "with a sign", given: "+9d"},
		{name: "with its unit in capitals", given: "14D"},
		{name: "with a space", given: " 14d"},
		{name: "a fraction too great to be a count", given: "99999999999999999999.5d"},
		{name: "forever in capitals", given: "Forever"},
		{name: "given empty", given: ""},
	}
	for _, kept := range keeps {
		t.Run(kept.table, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					path := writeConfig(t, "["+kept.table+"]\nkeep = \""+tt.given+"\"\n"+accountTOML("work"))

					_, err := config.Load(path)
					want := []string{fmt.Sprintf("%s.keep %q: must be a whole number of days from 8d, a week and a day, or forever, such as %dd",
						kept.table, tt.given, kept.byDefault/(24*time.Hour))}
					if got := problems(t, path, err); !slices.Equal(got, want) {
						t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
					}
				})
			}
		})
	}
}

func TestParseDaysReadsAWholeNumberOfDays(t *testing.T) {
	tests := []struct {
		given string
		want  uint64
		ok    bool
	}{
		{given: "14d", want: 14, ok: true},
		{given: "0d", want: 0, ok: true},
		{given: "014d", want: 14, ok: true},
		{given: "401d", want: 401, ok: true},
		{given: "18446744073709551615d", want: math.MaxUint64, ok: true},
		{given: "18446744073709551616d", want: math.MaxUint64, ok: true},
		{given: "99999999999999999999d", want: math.MaxUint64, ok: true},
		{given: "+2d"}, {given: "-2d"}, {given: "14"}, {given: "d"}, {given: "14D"}, {given: " 14d"}, {given: "2w"}, {given: "1.5d"},
		{given: "14dd"}, {given: "99999999999999999999.5d"}, {given: "99999999999999999999 d"}, {given: ""},
	}
	for _, tt := range tests {
		if got, ok := config.ParseDays(tt.given); got != tt.want || ok != tt.ok {
			t.Errorf("ParseDays(%q) = %d, %v, want %d, %v", tt.given, got, ok, tt.want, tt.ok)
		}
	}
}

func TestMostDaysIsTheMostWholeDaysADurationHolds(t *testing.T) {
	const day = 24 * time.Hour
	if most := time.Duration(config.MostDays) * day; most <= 0 || math.MaxInt64-most >= day {
		t.Errorf("MostDays = %d, %v long; want the most whole days a time.Duration holds", config.MostDays, most)
	}
}

func TestExampleIsValid(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, config.Example))
	if err != nil {
		t.Fatalf("Load(Example) error = %v", err)
	}
	if primary := cfg.Accounts.Primary(); primary.ID != "work" {
		t.Errorf("Load(Example) primary = %+v, want work", primary)
	}
	if !strings.Contains(config.Example, "\n# reserve = 0.1 ") || slices.ContainsFunc(cfg.Accounts, func(a config.Account) bool { return a.Reserve != 0 }) {
		t.Errorf("Example reads\n%s\nwant a reserve shown, and left off", config.Example)
	}
	if !strings.Contains(config.Example, "# [prime]\n# day = \"08:00-23:00\"\n") || cfg.Prime.On() {
		t.Errorf("Example reads\n%s\nwant priming shown, and left off", config.Example)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load() error = %v, want one matching fs.ErrNotExist", err)
	}
}

func TestLoadUndecodableFile(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "syntax error", config: "listen = \"127.0.0.1:4747\n"},
		{name: "wrong type", config: "listen = 4747\n"},
		{name: "account as a single table", config: "[account]\nid = \"work\"\n"},
		{name: "a warning in words", config: "notifications = { warning = \"high\" }\n"},
		{name: "a reserve in words", config: "account = [{ id = \"work\", reserve = \"some\" }]\n"},
		{name: "the primary in words", config: "account = [{ id = \"work\", primary = \"yes\" }]\n"},
		{name: "a day as a number", config: "prime = { day = 8 }\n"},
		{name: "a keep as a number", config: "history = { keep = 14 }\n"},
		{name: "the ledger's keep as a number", config: "ledger = { keep = 90 }\n"},
		{name: "the week's start as a number", config: "week_starts = 1\n"},
		{name: "a plan as a number", config: "account = [{ id = \"work\", plan = 5 }]\n"},
		{name: "a plan's price in words", config: "prices = { plans = { max5x = \"ninety\" } }\n"},
		{name: "a model's price in words", config: "prices = { models = { claude-opus-5-5 = { input = \"cheap\" } } }\n"},
		{name: "a model's prices as a number", config: "prices = { models = { claude-opus-5-5 = 3 } }\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path)
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			if want := "parse config " + path + ": toml: line 1"; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Load() error = %q, want it to start with %q", err, want)
			}
		})
	}
}

func TestLoadReportsProblems(t *testing.T) {
	work := accountTOML("work")
	notLoopback := func(listen string) string {
		return fmt.Sprintf("listen %q: host must be a loopback IP address, 127.0.0.1 or ::1, so no other machine can use the proxy's tokens; a name, even localhost, can lead a client to another address", listen)
	}
	plaintext := func(upstream string) string {
		return fmt.Sprintf("upstream %q: must use https unless its host is a loopback IP address, such as 127.0.0.1, so account tokens never cross a network in plaintext", upstream)
	}
	reserve := func(account string, share string) string {
		return fmt.Sprintf("account %q: reserve %s: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1", account, share)
	}
	day := func(given string) string {
		return fmt.Sprintf("prime.day %q: must be two times of day, HH:MM, joined by -, such as 08:00-23:00", given)
	}
	keep := func(key, given, example string) string {
		return fmt.Sprintf("%s %q: must be a whole number of days from 8d, a week and a day, or forever, such as %s", key, given, example)
	}
	weekStarts := func(given string) string {
		return fmt.Sprintf("week_starts %q: must be a day of the week, by its name in English, such as monday", given)
	}
	plan := func(account, given string) string {
		return fmt.Sprintf("account %q: plan %q: must be pro, max5x or max20x, the plan the account's subscription is on", account, given)
	}
	planPrice := func(plan, given string) string {
		return fmt.Sprintf("prices.plans.%s %q: must be a number, 0 or more: the plan's price in US dollars a month", plan, given)
	}
	modelPrice := func(key, given string) string {
		return fmt.Sprintf("prices.models.%s %q: must be a number, 0 or more: the price in US dollars a million tokens", key, given)
	}
	const tokenEnv = `unknown key "account.token_env": tokens now live in files, at <state dir>/tokens/<id>, ` +
		"the state dir being $XDG_STATE_HOME/switchboard, else ~/.local/state/switchboard"
	tests := []struct {
		name   string
		config string
		want   []string
	}{
		{
			name:   "unknown top-level key",
			config: "listn = \"127.0.0.1:4747\"\n" + work,
			want:   []string{`unknown key "listn"`},
		},
		{
			name:   "unknown account key, once however many accounts repeat it",
			config: accountTOML("work") + "labl = \"Work\"\n" + accountTOML("personal") + "labl = \"Personal\"\n",
			want:   []string{`unknown key "account.labl"`},
		},
		{
			name:   "unknown table, without its keys",
			config: work + "\n[proxy]\nport = 4747\nhost.name = \"localhost\"\n",
			want:   []string{`unknown key "proxy"`},
		},
		{
			name:   "unknown notifications key",
			config: "[notifications]\nsound = true\n" + work,
			want:   []string{`unknown key "notifications.sound"`},
		},
		{
			name:   "unknown prime key",
			config: "[prime]\nstart = \"08:00\"\n" + work,
			want:   []string{`unknown key "prime.start"`},
		},
		{
			name:   "unknown history key",
			config: "[history]\nkeep = \"14d\"\ndays = 14\n" + work,
			want:   []string{`unknown key "history.days"`},
		},
		{
			name:   "unknown ledger key",
			config: "[ledger]\nkeep = \"90d\"\nsummaries = \"400d\"\n" + work,
			want:   []string{`unknown key "ledger.summaries"`},
		},
		{
			name:   "the token variable of a config from before token files, once however many accounts give one",
			config: work + "token_env = \"CLAUDE_TOKEN_WORK\"\n" + accountTOML("personal") + "token_env = \"CLAUDE_TOKEN_PERSONAL\"\n",
			want:   []string{tokenEnv},
		},
		{
			name:   "a warning at the whole of a window's limit",
			config: "[notifications]\nwarning = 1\n" + work,
			want:   []string{"notifications.warning 1: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none"},
		},
		{
			name:   "a warning past the whole of a window's limit",
			config: "[notifications]\nwarning = 1.5\n" + work,
			want:   []string{"notifications.warning 1.5: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none"},
		},
		{
			name:   "a warning below none",
			config: "[notifications]\nwarning = -0.1\n" + work,
			want:   []string{"notifications.warning -0.1: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none"},
		},
		{
			name:   "a warning that isn't a number",
			config: "[notifications]\nwarning = nan\n" + work,
			want:   []string{"notifications.warning NaN: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none"},
		},
		{
			name:   "an endless warning",
			config: "[notifications]\nwarning = inf\n" + work,
			want:   []string{"notifications.warning +Inf: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none"},
		},
		{
			name:   "a reserve of the whole of each window",
			config: work + "reserve = 1\n",
			want:   []string{reserve("work", "1")},
		},
		{
			name:   "a reserve past the whole of each window",
			config: work + "reserve = 1.5\n",
			want:   []string{reserve("work", "1.5")},
		},
		{
			name:   "a reserve below none",
			config: work + accountTOML("side") + "reserve = -0.1\n",
			want:   []string{reserve("side", "-0.1")},
		},
		{
			name:   "a reserve that isn't a number",
			config: work + "reserve = nan\n",
			want:   []string{reserve("work", "NaN")},
		},
		{
			name:   "an endless reserve",
			config: work + "reserve = inf\n",
			want:   []string{reserve("work", "+Inf")},
		},
		{
			name:   "two primaries",
			config: work + "primary = true\n" + accountTOML("side") + "primary = true\n",
			want:   []string{`primary is set on account "work" and account "side": only one account can be the primary, the one the browser and the Claude apps use`},
		},
		{
			name: "three primaries, once, naming each",
			config: work + "primary = true\n" + accountTOML("personal") + "primary = false\n" +
				"\n[[account]]\nprimary = true\n" + accountTOML("side") + "primary = true\n",
			want: []string{
				"account #3: id is required",
				`primary is set on account "work", account #3 and account "side": only one account can be the primary, the one the browser and the Claude apps use`,
			},
		},
		{name: "a day without its end", config: "[prime]\nday = \"08:00\"\n" + work, want: []string{day("08:00")}},
		{name: "a day with a time of day that isn't one", config: "[prime]\nday = \"08:00-24:00\"\n" + work, want: []string{day("08:00-24:00")}},
		{name: "a day with an hour of one digit", config: "[prime]\nday = \"8:00-23:00\"\n" + work, want: []string{day("8:00-23:00")}},
		{name: "a day with seconds", config: "[prime]\nday = \"08:00:00-23:00:00\"\n" + work, want: []string{day("08:00:00-23:00:00")}},
		{name: "a day with spaces", config: "[prime]\nday = \"08:00 - 23:00\"\n" + work, want: []string{day("08:00 - 23:00")}},
		{name: "a day of three times", config: "[prime]\nday = \"08:00-12:00-23:00\"\n" + work, want: []string{day("08:00-12:00-23:00")}},
		{name: "a day in words", config: "[prime]\nday = \"mornings\"\n" + work, want: []string{day("mornings")}},
		{
			name:   "a day that ends as it starts",
			config: "[prime]\nday = \"08:00-08:00\"\n" + work,
			want:   []string{`prime.day "08:00-08:00": must end at another time than it starts; an end before the start is past midnight`},
		},
		{name: "a week starting on a day cut short", config: "week_starts = \"mon\"\n" + work, want: []string{weekStarts("mon")}},
		{name: "a week starting on days", config: "week_starts = \"Sundays\"\n" + work, want: []string{weekStarts("Sundays")}},
		{name: "a week starting on a day in another language", config: "week_starts = \"lundi\"\n" + work, want: []string{weekStarts("lundi")}},
		{name: "a week starting on a day's number", config: "week_starts = \"1\"\n" + work, want: []string{weekStarts("1")}},
		{name: "a week starting on a day with a space", config: "week_starts = \" monday\"\n" + work, want: []string{weekStarts(" monday")}},
		{name: "a plan the table doesn't know", config: work + "plan = \"max10x\"\n", want: []string{plan("work", "max10x")}},
		{name: "a plan in another case", config: work + "plan = \"Max5x\"\n", want: []string{plan("work", "Max5x")}},
		{name: "a plan by the name it's shown by", config: work + accountTOML("side") + "plan = \"Max 20x\"\n", want: []string{plan("side", "Max 20x")}},
		{
			name:   "a plan's price below none",
			config: work + "\n[prices.plans]\nmax5x = -1\n",
			want:   []string{planPrice("max5x", "-1")},
		},
		{
			name:   "a plan's price that isn't a number",
			config: work + "\n[prices.plans]\npro = nan\n",
			want:   []string{planPrice("pro", "NaN")},
		},
		{
			name:   "an endless plan's price",
			config: work + "\n[prices.plans]\nmax20x = inf\n",
			want:   []string{planPrice("max20x", "+Inf")},
		},
		{
			name:   "the price of a plan the table doesn't know, its value never quoted",
			config: work + "\n[prices.plans]\nmax10x = 400\nteam = -1\n",
			want:   []string{`unknown key "prices.plans.max10x"`, `unknown key "prices.plans.team"`},
		},
		{
			name:   "a model's prices below none, and not a number, each in the config's order",
			config: work + "\n[prices.models.claude-opus-5-5]\ncache_write_1h = -6.4\ninput = -0.5\noutput = nan\ncache_read = 0.16\ncache_write_5m = -inf\n",
			want: []string{
				modelPrice("claude-opus-5-5.input", "-0.5"), modelPrice("claude-opus-5-5.output", "NaN"),
				modelPrice("claude-opus-5-5.cache_write_5m", "-Inf"), modelPrice("claude-opus-5-5.cache_write_1h", "-6.4"),
			},
		},
		{
			name:   "an endless price of a model the table doesn't know, by an id that needs quoting",
			config: work + "\n[prices.models.\"claude-test-1.5\"]\noutput = inf\n",
			want:   []string{modelPrice(`"claude-test-1.5".output`, "+Inf")},
		},
		{
			name:   "the models' prices, by their ids in order",
			config: work + "\n[prices.models.claude-sonnet-5]\ninput = -2\n\n[prices.models.claude-haiku-4-5]\ninput = -1\n",
			want:   []string{modelPrice("claude-haiku-4-5.input", "-1"), modelPrice("claude-sonnet-5.input", "-2")},
		},
		{
			name:   "a kind of token the table doesn't know, its value never quoted",
			config: work + "\n[prices.models.claude-opus-5-5]\ninput = 3\ncache_write_2h = -1\n",
			want:   []string{`unknown key "prices.models.claude-opus-5-5.cache_write_2h"`},
		},
		{
			name:   "an unknown table of prices",
			config: work + "\n[prices.tools]\nweb_search = 10\n",
			want:   []string{`unknown key "prices.tools"`},
		},
		{
			name:   "an unknown key of prices",
			config: "[prices]\ncurrency = \"GBP\"\n" + work,
			want:   []string{`unknown key "prices.currency"`},
		},
		{
			name:   "no accounts",
			config: "listen = \"127.0.0.1:4747\"\n",
			want:   []string{"no accounts: add an [[account]] table for each Claude subscription"},
		},
		{
			name:   "empty file",
			config: "",
			want:   []string{"no accounts: add an [[account]] table for each Claude subscription"},
		},
		{
			name:   "missing id",
			config: work + "\n[[account]]\nlabel = \"Personal\"\n",
			want:   []string{"account #2: id is required"},
		},
		{
			name:   "id starting with a dash",
			config: accountTOML("-work"),
			want:   []string{`account "-work": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "id with a space",
			config: accountTOML("side project"),
			want:   []string{`account "side project": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "id that's a path, which would name a token file elsewhere",
			config: accountTOML("../work"),
			want:   []string{`account "../work": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "id with a dot",
			config: accountTOML("work.old"),
			want:   []string{`account "work.old": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "the id pin takes to mean routing",
			config: accountTOML("auto"),
			want:   []string{`account "auto": id is reserved for switchboard pin auto`},
		},
		{
			name:   "the id pin takes to mean routing, in another case",
			config: accountTOML("Auto"),
			want:   []string{`account "Auto": id is reserved for switchboard pin auto`},
		},
		{
			name:   "duplicate id, once however often it repeats",
			config: work + accountTOML("work") + accountTOML("work"),
			want:   []string{`duplicate account id "work"`},
		},
		{
			name:   "ids that differ only in case, which would share a token file",
			config: work + accountTOML("Work"),
			want:   []string{`account ids "work" and "Work" differ only in case, so they'd share a token file, as macOS ignores case in file names`},
		},
		{
			name:   "ids that differ only in case, once for each however often it repeats",
			config: work + accountTOML("Work") + accountTOML("WORK") + accountTOML("Work"),
			want: []string{
				`account ids "work" and "Work" differ only in case, so they'd share a token file, as macOS ignores case in file names`,
				`account ids "work" and "WORK" differ only in case, so they'd share a token file, as macOS ignores case in file names`,
				`duplicate account id "Work"`,
			},
		},
		{
			name:   "an id that looks like a token, named by its place",
			config: work + accountTOML(tokenShaped),
			want:   []string{"account #2: id looks like a token, which an id mustn't, as it shows wherever the account does"},
		},
		{
			name:   "a label that looks like a token",
			config: work + "label = \"Work " + tokenShaped + "\"\n",
			want:   []string{`account "work": label looks like a token, which a label mustn't, as it shows wherever the account does`},
		},
		{
			name:   "empty listen",
			config: "listen = \"\"\n" + work,
			want:   []string{`listen "": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`},
		},
		{
			name:   "listen without a port",
			config: "listen = \"127.0.0.1\"\n" + work,
			want:   []string{`listen "127.0.0.1": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`},
		},
		{
			name:   "listen on every interface",
			config: "listen = \":4747\"\n" + work,
			want:   []string{notLoopback(":4747")},
		},
		{
			name:   "listen on the unspecified address",
			config: "listen = \"0.0.0.0:4747\"\n" + work,
			want:   []string{notLoopback("0.0.0.0:4747")},
		},
		{
			name:   "listen on a LAN address",
			config: "listen = \"192.168.1.20:4747\"\n" + work,
			want:   []string{notLoopback("192.168.1.20:4747")},
		},
		{
			name:   "listen on a host name",
			config: "listen = \"example.com:4747\"\n" + work,
			want:   []string{notLoopback("example.com:4747")},
		},
		{
			name:   "listen on localhost, a name",
			config: "listen = \"localhost:4747\"\n" + work,
			want:   []string{notLoopback("localhost:4747")},
		},
		{
			name:   "listen on loopback with a zone",
			config: "listen = \"[::1%lo0]:4747\"\n" + work,
			want:   []string{notLoopback("[::1%lo0]:4747")},
		},
		{
			name:   "listen on port 0",
			config: "listen = \"127.0.0.1:0\"\n" + work,
			want:   []string{`listen "127.0.0.1:0": port must be a number from 1 to 65535`},
		},
		{
			name:   "listen on a port out of range",
			config: "listen = \"127.0.0.1:65536\"\n" + work,
			want:   []string{`listen "127.0.0.1:65536": port must be a number from 1 to 65535`},
		},
		{
			name:   "listen on a named port",
			config: "listen = \"127.0.0.1:http\"\n" + work,
			want:   []string{`listen "127.0.0.1:http": port must be a number from 1 to 65535`},
		},
		{
			name:   "upstream without a scheme",
			config: "upstream = \"api.anthropic.com\"\n" + work,
			want:   []string{`upstream "api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "upstream with another scheme",
			config: "upstream = \"ftp://api.anthropic.com\"\n" + work,
			want:   []string{`upstream "ftp://api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "upstream without a host",
			config: "upstream = \"https://\"\n" + work,
			want:   []string{`upstream "https://": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "unparseable upstream",
			config: "upstream = \"https://[::1\"\n" + work,
			want:   []string{`upstream "https://[::1": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "plain http upstream to a remote host",
			config: "upstream = \"http://api.anthropic.com\"\n" + work,
			want:   []string{plaintext("http://api.anthropic.com")},
		},
		{
			name:   "plain http upstream to a LAN address",
			config: "upstream = \"http://192.168.1.20:8080\"\n" + work,
			want:   []string{plaintext("http://192.168.1.20:8080")},
		},
		{
			name:   "plain http upstream to localhost, a name",
			config: "upstream = \"http://localhost:8080\"\n" + work,
			want:   []string{plaintext("http://localhost:8080")},
		},
		{
			name: "several problems at once",
			config: "listen = \"0.0.0.0:4747\"\nupstream = \"api.anthropic.com\"\nweek_starts = \"someday\"\nverbose = true\n" +
				work + "primary = true\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n" +
				"\n[[account]]\nreserve = 2.0\nplan = \"max\"\n" +
				accountTOML("work") + "primary = true\n" +
				"\n[prime]\nday = \"23:00-23:00\"\n" +
				"\n[notifications]\nwarning = 1\n" +
				"\n[history]\nkeep = \"7d\"\n" +
				"\n[ledger]\nkeep = \"1y\"\n" +
				"\n[prices.plans]\nmax5x = -90\n" +
				"\n[prices.models.claude-opus-5-5]\ninput = -3.2\n",
			want: []string{
				`unknown key "verbose"`,
				tokenEnv,
				notLoopback("0.0.0.0:4747"),
				`upstream "api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`,
				weekStarts("someday"),
				"account #2: id is required",
				"account #2: reserve 2: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1",
				`account #2: plan "max": must be pro, max5x or max20x, the plan the account's subscription is on`,
				`duplicate account id "work"`,
				`primary is set on account "work" and account "work": only one account can be the primary, the one the browser and the Claude apps use`,
				`prime.day "23:00-23:00": must end at another time than it starts; an end before the start is past midnight`,
				"notifications.warning 1: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none",
				keep("history.keep", "7d", "400d"),
				keep("ledger.keep", "1y", "400d"),
				planPrice("max5x", "-90"),
				modelPrice("claude-opus-5-5.input", "-3.2"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path)
			if got := problems(t, path, err); !slices.Equal(got, tt.want) {
				t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestLoadNeverQuotesATokenGivenAsAnIDOrALabel(t *testing.T) {
	path := writeConfig(t, accountTOML(tokenShaped)+"primary = true\nreserve = 2.0\n"+
		accountTOML(tokenShaped)+"label = \""+tokenShaped+"\"\nprimary = true\n")

	_, err := config.Load(path)
	want := []string{
		"account #1: id looks like a token, which an id mustn't, as it shows wherever the account does",
		"account #1: reserve 2: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1",
		"account #2: id looks like a token, which an id mustn't, as it shows wherever the account does",
		"account #2: label looks like a token, which a label mustn't, as it shows wherever the account does",
		"primary is set on account #1 and account #2: only one account can be the primary, the one the browser and the Claude apps use",
	}
	if got := problems(t, path, err); !slices.Equal(got, want) {
		t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLoadNeverQuotesATokenGivenAsAValueOrAKey(t *testing.T) {
	path := writeConfig(t, "listen = \""+tokenShaped+"\"\nupstream = \""+tokenShaped+"\"\n"+tokenShaped+" = 1\n"+
		"week_starts = \""+tokenShaped+"\"\n"+
		accountTOML("work")+"plan = \""+tokenShaped+"\"\n"+
		"\n[prime]\nday = \""+tokenShaped+"\"\n"+
		"\n[history]\nkeep = \""+tokenShaped+"\"\n"+
		"\n[ledger]\nkeep = \""+tokenShaped+"\"\n"+
		"\n[prices.plans]\n"+tokenShaped+" = 1\n"+
		"\n[prices.models."+tokenShaped+"]\ninput = -1\n"+tokenShaped+" = 1\n")

	_, err := config.Load(path)
	want := []string{
		`unknown key "[redacted]"`,
		`unknown key "prices.models.[redacted].[redacted]"`,
		`listen "[redacted]": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`,
		`upstream "[redacted]": must be an absolute http or https URL, such as https://api.anthropic.com`,
		`week_starts "[redacted]": must be a day of the week, by its name in English, such as monday`,
		`account "work": plan "[redacted]": must be pro, max5x or max20x, the plan the account's subscription is on`,
		`prime.day "[redacted]": must be two times of day, HH:MM, joined by -, such as 08:00-23:00`,
		`history.keep "[redacted]": must be a whole number of days from 8d, a week and a day, or forever, such as 400d`,
		`ledger.keep "[redacted]": must be a whole number of days from 8d, a week and a day, or forever, such as 400d`,
		`unknown key "prices.plans.[redacted]"`,
		`prices.models.[redacted].input "-1": must be a number, 0 or more: the price in US dollars a million tokens`,
	}
	if got := problems(t, path, err); !slices.Equal(got, want) {
		t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLoadNeverQuotesATokenItCantParse(t *testing.T) {
	tests := []struct {
		name   string
		config string
		// want is the error, after the config's path.
		want string
	}{
		{
			name:   "a key without a value",
			config: tokenShaped + " =\n",
			want:   `toml: line 1 (last key "[redacted]"): expected value but found '\n' instead`,
		},
		{
			name:   "a key given twice",
			config: tokenShaped + " = 1\n" + tokenShaped + " = 2\n",
			want:   `toml: line 2 (last key "[redacted]"): Key '[redacted]' has already been defined.`,
		},
		{
			name:   "a key given twice in an account's table",
			config: accountTOML("work") + tokenShaped + " = 1\n" + tokenShaped + " = 2\n",
			want:   `toml: line 5 (last key "account.[redacted]"): Key 'account.[redacted]' has already been defined.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path)
			if want := "parse config " + path + ": " + tt.want; err == nil || err.Error() != want {
				t.Errorf("Load() error = %v, want %q", err, want)
			}
		})
	}
}

func TestLoadNeverQuotesAnOldConfigsTokenVariable(t *testing.T) {
	const pasted = "test-token-pasted-by-mistake"
	path := writeConfig(t, accountTOML("work")+"token_env = \""+pasted+"\"\n")

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load() succeeded, want an error")
	}
	if strings.Contains(err.Error(), pasted) {
		t.Error("Load() error quotes the token_env value")
	}
}

// problems returns the lines of a validation error from Load, one per problem.
func problems(t *testing.T, path string, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("Load() succeeded, want an error")
	}
	header := "invalid config " + path + ":\n"
	msg, ok := strings.CutPrefix(err.Error(), header)
	if !ok {
		t.Fatalf("Load() error = %q, want it to start with %q", err, header)
	}
	return strings.Split(msg, "\n")
}

// tokenShaped is shaped like a Claude token, though it's none, as a user
// might give one where an id or a label goes.
const tokenShaped = "sk-ant-oat01-fake_token-shaped"

// accountTOML returns an [[account]] table with the given id, which the
// test's own keys can follow.
func accountTOML(id string) string {
	return fmt.Sprintf("\n[[account]]\nid = %q\n", id)
}

// writeConfig writes a config file under the test's temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
