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

func TestTheWindowsEveryCardShows(t *testing.T) {
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

func TestTheLineOverTheFooterSaysWhichWindowsHide(t *testing.T) {
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

func TestAutoFeaturesWhatWillStopTheAccountFirst(t *testing.T) {
	// A week 80% used three days in runs out a day on, at its pace, before it
	// resets in four: sooner than a session that runs out in 90 minutes is
	// only where the session doesn't.
	runningWeek := weekOf(0.8, 4*day)
	atReserve := readAccount("work", sessionOf(0.3, 3*time.Hour), weekOf(0.92, 4*day))
	atReserve.Reserve, atReserve.AtReserve = 0.1, []string{"7d"}
	lapsed := readAccount("work", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.05, 4*day))
	lapsed.Lapsed = []string{"5h"}
	tests := []struct {
		name    string
		account status.Account
		setting Feature
		want    string
	}{
		{name: "the window at its limit, whatever runs out", account: with(limitedAccount("work"), func(a *status.Account) { a.Windows[1] = runningWeek }), want: "5h"},
		{name: "the window at its reserve", account: atReserve, want: "7d"},
		{name: "the window running out soonest: the session", account: pressedAccount("work"), want: "5h"},
		{name: "the window running out soonest: the week", account: readAccount("work", sessionOf(0.3, 3*time.Hour), runningWeek), want: "7d"},
		{name: "else the most used: the week", account: readAccount("work", sessionOf(0.25, 3*time.Hour), weekOf(0.5, 4*day)), want: "7d"},
		{name: "else the most used: the session", account: readAccount("work", sessionOf(0.6, 3*time.Hour), weekOf(0.5, 4*day)), want: "5h"},
		{name: "a lapsed session reading empty", account: lapsed, want: "7d"},
		{name: "nothing used: the first", account: readAccount("work", sessionOf(0, 3*time.Hour), weekOf(0, 4*day)), want: "5h"},
		{name: "the week, as w sets it", account: limitedAccount("work"), setting: "7d", want: "7d"},
		{name: "a window it hasn't, as auto", account: readAccount("work", sessionOf(0.6, 3*time.Hour), weekOf(0.5, 4*day)), setting: "7d_oi", want: "5h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDoc("", 0, tt.account)
			got, ok := featured(doc, tt.account, now, claudeLike, tt.setting, shownWindows(doc, now, claudeLike))
			if !ok || got.Key != tt.want {
				t.Errorf("featured() = %q, %v, want %q", got.Key, ok, tt.want)
			}
		})
	}
	if _, ok := featured(routerDoc("", 0), status.Account{ID: "work", TokenSet: true}, now, claudeLike, Auto, nil); ok {
		t.Error("featured() of an account nothing's been read of reports a window, want none")
	}
}

func TestWCyclesTheWindowsInUse(t *testing.T) {
	fable := routerDoc("", 0, readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0.1, 4*day)))
	noFable := routerDoc("", 0, readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0, 4*day)))
	tests := []struct {
		name string
		doc  status.Document
		from Feature
		want Feature
	}{
		{name: "auto, to the 5-hour window", doc: fable, from: Auto, want: "5h"},
		{name: "the 5-hour window, to the week", doc: fable, from: "5h", want: "7d"},
		{name: "the week, to another in use", doc: fable, from: "7d", want: "7d_oi"},
		{name: "the last, round to auto", doc: fable, from: "7d_oi", want: Auto},
		{name: "the week, round to auto, past one unused", doc: noFable, from: "7d", want: Auto},
		{name: "one no longer in use, as auto", doc: noFable, from: "7d_oi", want: "5h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.Next(tt.doc, now, claudeLike); got != tt.want {
				t.Errorf("Next() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheFeaturedWindowsNames(t *testing.T) {
	doc := routerDoc("", 0, readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 4*day), fableOf(0.1, 4*day)))
	for setting, want := range map[Feature]string{Auto: "auto", "5h": "5h", "7d": "week", "7d_oi": "Fable wk", "7d_x": "7d_x"} {
		if got := setting.Name(doc, claudeLike); got != want {
			t.Errorf("Name() of %q = %q, want %q", setting, got, want)
		}
	}
}

// with is the account a as change leaves it.
func with(a status.Account, change func(*status.Account)) status.Account {
	change(&a)
	return a
}

// text is the line's text, without its inks.
func text(l line) string {
	var s string
	for _, sp := range l {
		s += sp.text
	}
	return s
}
