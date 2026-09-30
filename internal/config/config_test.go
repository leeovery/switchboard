package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

func TestLoad(t *testing.T) {
	path := writeConfig(t, `
listen   = "[::1]:9000"
upstream = "http://127.0.0.1:8080"

[[account]]
id      = "work"
label   = "Work"
reserve = 0.05

[[account]]
id      = "personal"
label   = "Personal"
primary = true
reserve = 0.2

[prime]
day = "07:30-22:45"

[notifications]
limits  = false
room    = true
warning = 0.75
moves   = true
`)
	want := &config.Config{
		Listen:   "[::1]:9000",
		Upstream: "http://127.0.0.1:8080",
		Accounts: []config.Account{
			{ID: "work", Label: "Work", Reserve: 0.05},
			{ID: "personal", Label: "Personal", Primary: true, Reserve: 0.2},
		},
		Prime:         config.Prime{Day: config.Day{Start: 7*time.Hour + 30*time.Minute, End: 22*time.Hour + 45*time.Minute}},
		Notifications: config.Notifications{Room: true, Warning: 0.75, Moves: true},
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
		Listen:   "127.0.0.1:4747",
		Upstream: "https://api.anthropic.com",
		Accounts: []config.Account{
			{ID: "work", Label: "work", Primary: true, Reserve: 0.1},
			{ID: "personal", Label: "Personal"},
		},
		Notifications: config.Notifications{Limits: true, Room: true, Warning: 0.9},
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

func TestLoadThePrimaryAndTheReserves(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   []config.Account
	}{
		{
			name:   "the first, without one marked",
			config: accountTOML("work") + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.1}, {ID: "side", Label: "side"}},
		},
		{
			name:   "the one marked",
			config: accountTOML("work") + accountTOML("side") + "primary = true\n",
			want:   []config.Account{{ID: "work", Label: "work"}, {ID: "side", Label: "side", Primary: true, Reserve: 0.1}},
		},
		{
			name:   "the first, with the others marked not to be",
			config: accountTOML("work") + "primary = false\n" + accountTOML("side") + "primary = false\n",
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.1}, {ID: "side", Label: "side"}},
		},
		{
			name:   "with the primary's reserve given",
			config: accountTOML("work") + "reserve = 0.25\n" + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.25}, {ID: "side", Label: "side"}},
		},
		{
			name:   "with none on the primary",
			config: accountTOML("work") + "reserve = 0\n" + accountTOML("side"),
			want:   []config.Account{{ID: "work", Label: "work", Primary: true}, {ID: "side", Label: "side"}},
		},
		{
			name:   "with a reserve on another account",
			config: accountTOML("work") + accountTOML("side") + "reserve = 0.05\n",
			want:   []config.Account{{ID: "work", Label: "work", Primary: true, Reserve: 0.1}, {ID: "side", Label: "side", Reserve: 0.05}},
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

func TestExampleIsValid(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, config.Example))
	if err != nil {
		t.Fatalf("Load(Example) error = %v", err)
	}
	if primary := cfg.Accounts.Primary(); primary.ID != "work" || primary.Reserve != 0.1 {
		t.Errorf("Load(Example) primary = %+v, want work, with a reserve of 0.1", primary)
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
			config: "listen = \"0.0.0.0:4747\"\nupstream = \"api.anthropic.com\"\nverbose = true\n" +
				work + "primary = true\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n" +
				"\n[[account]]\nreserve = 2.0\n" +
				accountTOML("work") + "primary = true\n" +
				"\n[prime]\nday = \"23:00-23:00\"\n" +
				"\n[notifications]\nwarning = 1\n",
			want: []string{
				`unknown key "verbose"`,
				tokenEnv,
				notLoopback("0.0.0.0:4747"),
				`upstream "api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`,
				"account #2: id is required",
				"account #2: reserve 2: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1",
				`duplicate account id "work"`,
				`primary is set on account "work" and account "work": only one account can be the primary, the one the browser and the Claude apps use`,
				`prime.day "23:00-23:00": must end at another time than it starts; an end before the start is past midnight`,
				"notifications.warning 1: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none",
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
		accountTOML("work")+"\n[prime]\nday = \""+tokenShaped+"\"\n")

	_, err := config.Load(path)
	want := []string{
		`unknown key "[redacted]"`,
		`listen "[redacted]": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`,
		`upstream "[redacted]": must be an absolute http or https URL, such as https://api.anthropic.com`,
		`prime.day "[redacted]": must be two times of day, HH:MM, joined by -, such as 08:00-23:00`,
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
