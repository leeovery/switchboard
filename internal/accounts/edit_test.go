package accounts_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// laidOut is a config laid out by hand, with comments: work, the primary,
// with a reserve, and personal, on a plan.
var laidOut = fmt.Sprintf(`# The accounts.
upstream = %q

[[account]]
id      = "work"     # for good
label   = "Work"
primary = true
reserve = 0.1        # the app's share

# The other one.
[[account]]
id   = "personal"
plan = "pro"         # for now
`, upstream)

// work and personal are laidOut's accounts, as it configures them.
var (
	work     = config.Account{ID: "work", Label: "Work", Primary: true, Reserve: 0.1}
	personal = config.Account{ID: "personal", Label: "personal", Plan: "pro"}
)

func TestEdit(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		change     accounts.Change
		want       string
		wantEdited accounts.Edited
	}{
		{
			name:       "a label, below the account's last key",
			id:         "personal",
			change:     accounts.Change{Label: new("Personal")},
			want:       strings.Replace(laidOut, "# for now\n", "# for now\nlabel = \"Personal\"\n", 1),
			wantEdited: accounts.Edited{Was: personal, Now: with(personal, func(a *config.Account) { a.Label = "Personal" })},
		},
		{
			name:       "no label, showing the account by its id",
			id:         "work",
			change:     accounts.Change{Label: new("")},
			want:       strings.Replace(laidOut, "label   = \"Work\"\n", "", 1),
			wantEdited: accounts.Edited{Was: work, Now: with(work, func(a *config.Account) { a.Label = "work" })},
		},
		{
			name:       "a plan, in place of the one it has, keeping the comment beside it",
			id:         "personal",
			change:     accounts.Change{Plan: new("max20x")},
			want:       strings.Replace(laidOut, `plan = "pro"`, `plan = "max20x"`, 1),
			wantEdited: accounts.Edited{Was: personal, Now: with(personal, func(a *config.Account) { a.Plan = "max20x" })},
		},
		{
			name:       "no plan",
			id:         "personal",
			change:     accounts.Change{Plan: new("")},
			want:       strings.Replace(laidOut, "plan = \"pro\"         # for now\n", "", 1),
			wantEdited: accounts.Edited{Was: personal, Now: with(personal, func(a *config.Account) { a.Plan = "" })},
		},
		{
			name:   "the primary, taking primary off the one that was",
			id:     "personal",
			change: accounts.Change{Primary: true},
			want: strings.Replace(strings.Replace(laidOut, "primary = true\n", "", 1),
				"# for now\n", "# for now\nprimary = true\n", 1),
			wantEdited: accounts.Edited{
				Was:           personal,
				Now:           with(personal, func(a *config.Account) { a.Primary = true }),
				FormerPrimary: "work",
			},
		},
		{
			name:       "a reserve, in place of the one it has, keeping the comment beside it",
			id:         "work",
			change:     accounts.Change{Reserve: new(0.05)},
			want:       strings.Replace(laidOut, "reserve = 0.1        #", "reserve = 0.05        #", 1),
			wantEdited: accounts.Edited{Was: work, Now: with(work, func(a *config.Account) { a.Reserve = 0.05 })},
		},
		{
			name:       "no reserve",
			id:         "work",
			change:     accounts.Change{Reserve: new(0.0)},
			want:       strings.Replace(laidOut, "reserve = 0.1        # the app's share\n", "", 1),
			wantEdited: accounts.Edited{Was: work, Now: with(work, func(a *config.Account) { a.Reserve = 0 })},
		},
		{
			name:   "everything together",
			id:     "personal",
			change: accounts.Change{Label: new("Personal"), Plan: new("max5x"), Primary: true, Reserve: new(0.025)},
			want: strings.Replace(strings.Replace(laidOut, "primary = true\n", "", 1),
				"plan = \"pro\"         # for now\n",
				"plan = \"max5x\"         # for now\nlabel = \"Personal\"\nprimary = true\nreserve = 0.025\n", 1),
			wantEdited: accounts.Edited{
				Was:           personal,
				Now:           config.Account{ID: "personal", Label: "Personal", Plan: "max5x", Primary: true, Reserve: 0.025},
				FormerPrimary: "work",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, laidOut)

			edited, err := w.registry(&user{}, &fakeAPI{}).Edit(tt.id, tt.change)
			if err != nil || edited != tt.wantEdited {
				t.Fatalf("Edit(%q) = %+v, %v; want %+v", tt.id, edited, err, tt.wantEdited)
			}
			w.checkConfig(t, tt.want)
			if got := account(t, w.load(t), tt.id); got != tt.wantEdited.Now {
				t.Errorf("%s loads as %+v, want %+v", tt.id, got, tt.wantEdited.Now)
			}
		})
	}
}

func TestEditWritesNothingWhenEveryValueGivenIsTheAccounts(t *testing.T) {
	const unmarked = "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"personal\"\n"
	tests := []struct {
		name   string
		config string
		id     string
		change accounts.Change
	}{
		{name: "its label", config: laidOut, id: "work", change: accounts.Change{Label: new("Work")}},
		{name: "its id as its label, which shows it by its id already", config: laidOut, id: "personal", change: accounts.Change{Label: new("personal")}},
		{name: "its plan", config: laidOut, id: "personal", change: accounts.Change{Plan: new("pro")}},
		{name: "no plan, having none", config: laidOut, id: "work", change: accounts.Change{Plan: new("")}},
		{name: "its reserve", config: laidOut, id: "work", change: accounts.Change{Reserve: new(0.1)}},
		{name: "the primary, being it", config: laidOut, id: "work", change: accounts.Change{Primary: true}},
		{name: "the primary, being the first, with none marked", config: unmarked, id: "work", change: accounts.Change{Primary: true}},
		{
			name:   "everything together",
			config: laidOut,
			id:     "work",
			change: accounts.Change{Label: new("Work"), Plan: new(""), Primary: true, Reserve: new(0.1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			then := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
			if err := os.Chtimes(w.config, then, then); err != nil {
				t.Fatal(err)
			}

			edited, err := w.registry(&user{}, &fakeAPI{}).Edit(tt.id, tt.change)
			if err != nil || edited.Changed() || edited.FormerPrimary != "" {
				t.Fatalf("Edit(%q) = %+v, %v; want it unchanged", tt.id, edited, err)
			}
			w.checkConfig(t, tt.config)
			if info, err := os.Stat(w.config); err != nil || !info.ModTime().Equal(then) {
				t.Errorf("the config was written: %v, %v", info.ModTime(), err)
			}
		})
	}
}

func TestEditRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		id      string
		change  accounts.Change
		wantErr string
		wantIs  error
	}{
		{
			name:    "an account not configured",
			config:  laidOut,
			id:      "side",
			change:  accounts.Change{Label: new("Side")},
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "an account not configured, with no config",
			id:      "side",
			change:  accounts.Change{Primary: true},
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a plan that isn't one",
			config:  laidOut,
			id:      "work",
			change:  accounts.Change{Plan: new("free")},
			wantErr: `account "work": plan "free": must be pro, max5x or max20x, the plan the account's subscription is on`,
		},
		{
			name:    "a reserve of the whole window, after a label it then doesn't write",
			config:  laidOut,
			id:      "work",
			change:  accounts.Change{Label: new("Day job"), Reserve: new(1.0)},
			wantErr: `account "work": reserve 1: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)

			_, err := w.registry(&user{}, &fakeAPI{}).Edit(tt.id, tt.change)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Edit(%q) error = %v, want %q", tt.id, err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Edit(%q) error = %v, want one matching %v", tt.id, err, tt.wantIs)
			}
			if tt.config == "" {
				if _, err := os.Stat(w.config); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("the config: %v, want none made", err)
				}
				return
			}
			w.checkConfig(t, tt.config)
		})
	}
}

func TestChangeAsks(t *testing.T) {
	tests := []struct {
		name   string
		change accounts.Change
		want   bool
	}{
		{name: "nothing", change: accounts.Change{}},
		{name: "a label, even none", change: accounts.Change{Label: new("")}, want: true},
		{name: "a plan, even none", change: accounts.Change{Plan: new("")}, want: true},
		{name: "the primary", change: accounts.Change{Primary: true}, want: true},
		{name: "a reserve, even none", change: accounts.Change{Reserve: new(0.0)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.change.Asks(); got != tt.want {
				t.Errorf("%+v.Asks() = %v, want %v", tt.change, got, tt.want)
			}
		})
	}
}

func TestEditIsLogged(t *testing.T) {
	w := newWorld(t, laidOut)
	log := logstest.Capture(t)

	if _, err := w.registry(&user{}, &fakeAPI{}).Edit("personal", accounts.Change{Primary: true, Reserve: new(0.05)}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"level=INFO", `msg="edited an account"`, "account=personal", "primary=true", "plan=pro", "reserve=0.05"}; !log.Has(want...) {
		t.Errorf("the log reads\n%s\nwant a line with %q", log, want)
	}
}

// with returns a, changed as change changes it.
func with(a config.Account, change func(*config.Account)) config.Account {
	change(&a)
	return a
}

// account returns the account with the given id that cfg configures.
func account(t *testing.T, cfg *config.Config, id string) config.Account {
	t.Helper()
	for _, a := range cfg.Accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no account %q is configured", id)
	return config.Account{}
}
