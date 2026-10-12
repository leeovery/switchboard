package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func fableOf(used float64, resetsIn time.Duration) quota.Window {
	return windowOf("7d_oi", "Fable week", used, resetsIn)
}

// using is the account a whose window with the given key it's seen used
// lately, at rate a share of it an hour.
func using(a status.Account, key string, rate float64) status.Account {
	a.Rates = append(a.Rates, status.Rate{Window: key, Rate: rate, Since: now.Add(-30 * time.Minute).UTC()})
	return a
}

func TestTheWindowsShown(t *testing.T) {
	tests := []struct {
		name       string
		accounts   []status.Account
		wantShown  []string
		wantHidden []string
	}{
		{
			name: "those every model shares, always, and a model's own no account uses, hidden",
			accounts: []status.Account{
				readAccount("work", fableOf(0, 4*day), sessionOf(0, time.Hour), weekOf(0, 4*day)),
				readAccount("side", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0, 4*day)),
			},
			wantShown:  []string{"5h", "7d"},
			wantHidden: []string{"7d_oi"},
		},
		{
			name: "a model's own one account has used",
			accounts: []status.Account{
				readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0, 4*day)),
				readAccount("side", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0.04, 4*day)),
			},
			wantShown: []string{"5h", "7d", "7d_oi"},
		},
		{
			name: "a model's own one account used, reset since it was read, so unused at now",
			accounts: []status.Account{
				readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0, 4*day)),
				readAccount("side", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0.12, -time.Hour)),
			},
			wantShown:  []string{"5h", "7d"},
			wantHidden: []string{"7d_oi"},
		},
		{
			name: "a model's own one account is heading to use",
			accounts: []status.Account{
				using(readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0, 4*day)), "7d_oi", 0.01),
			},
			wantShown: []string{"5h", "7d", "7d_oi"},
		},
		{name: "nothing read", accounts: []status.Account{{ID: "work", TokenSet: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := routerDoc("", 0, tt.accounts...).At(now, claudeLike)
			if got := windowKeys(m.Shown); !slices.Equal(got, tt.wantShown) {
				t.Errorf("the windows shown are %q, want %q", got, tt.wantShown)
			}
			if got := windowKeys(m.Hidden); !slices.Equal(got, tt.wantHidden) {
				t.Errorf("the windows hidden are %q, want %q", got, tt.wantHidden)
			}
		})
	}
}

func TestTheLineOverTheKeyLineSaysWhichWindowsHide(t *testing.T) {
	m := routerDoc("", 0, readAccount("work", sessionOf(0.2, time.Hour), fableOf(0, 4*day), windowOf("7d_opus", "Opus week", 0, 4*day))).At(now, claudeLike)
	tests := []struct {
		keys []string
		want string
	}{
		{keys: []string{"7d_oi"}, want: "Fable wk hidden: unused on every account"},
		{keys: []string{"7d_oi", "7d_opus"}, want: "Fable wk and Opus wk hidden: unused on every account"},
		{keys: nil, want: ""},
	}
	for _, tt := range tests {
		var hidden []status.WindowName
		for _, key := range tt.keys {
			hidden = append(hidden, windowNamed(m, key))
		}
		if got := text(hiddenNote(hidden)); got != tt.want {
			t.Errorf("hiddenNote(%q) = %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestAWindowIsNamedAsTheMomentNamesIt(t *testing.T) {
	m := routerDoc("", 0,
		readAccount("work", windowOf("5h", "", 0.2, time.Hour), fableOf(0, 4*day)),
		readAccount("side", windowOf("7d", "Week\x1b[2J", 0.3, 4*day)),
	).At(now, claudeLike)
	tests := []struct {
		key  string
		want status.WindowName
	}{
		{key: "5h", want: status.WindowName{Key: "5h", Label: "5h", Name: "5-hour"}},
		{key: "7d", want: status.WindowName{Key: "7d", Label: "Week\x1b[2J", Name: "Week [2J"}},
		{key: "7d_oi", want: status.WindowName{Key: "7d_oi", Label: "Fable week", Name: "Fable week"}},
		{key: "1h", want: status.WindowName{Key: "1h", Label: "1h", Name: "1-hour"}},
		{key: "7d_new", want: status.WindowName{Key: "7d_new", Label: "7d_new", Name: "7d_new"}},
	}
	for _, tt := range tests {
		if got := windowNamed(m, tt.key); got != tt.want {
			t.Errorf("windowNamed(%q) = %+v, want %+v: as the moment names it, or one it lacks by its key", tt.key, got, tt.want)
		}
		if got, want := labelsOf(m)(tt.key), tt.want.Label; got != want {
			t.Errorf("labelsOf()(%q) = %q, want %q", tt.key, got, want)
		}
	}
}

// text is the line's text, without its inks.
func text(l line) string {
	var s string
	for _, sp := range l {
		s += sp.text
	}
	return s
}
