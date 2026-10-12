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
			doc := routerDoc("", 0, tt.accounts...)
			if got := shownWindows(doc, now, claudeLike); !slices.Equal(got, tt.wantShown) {
				t.Errorf("shownWindows() = %q, want %q", got, tt.wantShown)
			}
			if got := hiddenWindows(doc, now, claudeLike); !slices.Equal(got, tt.wantHidden) {
				t.Errorf("hiddenWindows() = %q, want %q", got, tt.wantHidden)
			}
		})
	}
}

func TestTheLineOverTheKeyLineSaysWhichWindowsHide(t *testing.T) {
	doc := routerDoc("", 0, readAccount("work", sessionOf(0.2, time.Hour), fableOf(0, 4*day), windowOf("7d_opus", "Opus week", 0, 4*day)))
	tests := []struct {
		keys []string
		want string
	}{
		{keys: []string{"7d_oi"}, want: "Fable wk hidden: unused on every account"},
		{keys: []string{"7d_oi", "7d_opus"}, want: "Fable wk and Opus wk hidden: unused on every account"},
		{keys: nil, want: ""},
	}
	for _, tt := range tests {
		if got := text(hiddenNote(doc, tt.keys)); got != tt.want {
			t.Errorf("hiddenNote(%q) = %q, want %q", tt.keys, got, tt.want)
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
