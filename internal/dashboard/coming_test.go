package dashboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestCOMINGUPSaysWhatsNextSoonestFirst(t *testing.T) {
	lapsed := readAccount("spare", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.05, 6*day))
	lapsed.Lapsed = []string{"5h"}
	pastLimit := readAccount("over", sessionOf(0.3, time.Hour), weekOf(0.2, 3*day))
	pastLimit.Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(-time.Minute)}
	spent := readAccount("spent", sessionOf(0.2, 3*time.Hour), weekOf(0.3, 3*day), windowOf("7d_oi", "Fable week", 1, 2*day))
	running := readAccount("client", sessionOf(0.1, 3*time.Hour), weekOf(0.7, 3*day))
	fable := readAccount("fable", sessionOf(0.1, 2*time.Hour), weekOf(0.2, 3*day), windowOf("7d_oi", "Fable week", 1, 4*day))
	fable.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: now.Add(4 * day)}
	reserved := pressedAccount("work")
	reserved.Reserve = 0.1
	pinned := routerDoc("work", 0, reserved)
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	capped := readAccount("capped", sessionOf(0.2, 2*time.Hour), weekOf(0.95, 2*day))
	capped.Reserve = 0.1
	modelCapped := readAccount("fable", sessionOf(0.2, 2*time.Hour), weekOf(0.3, 3*day), windowOf("7d_oi", "Fable week", 0.95, 2*day))
	modelCapped.Reserve = 0.1
	tests := []struct {
		name  string
		doc   status.Document
		slots []status.Slot
		want  []string
	}{
		{
			name: "the frames': a limit lifting, a session running out, and resetting, a week running out, and every week resetting",
			doc:  threeRouted(),
			want: []string{
				"14:12 personal back from its limit",
				"14:42 work runs out at its pace",
				"16:12 work's session resets",
				"17:12 side's session resets",
				"Thu 13:12 personal's week resets",
				"Fri 13:12 work's week resets",
				"Fri 14:39 side's week runs out at its pace",
				"Sat 13:12 side's week resets",
			},
		},
		{
			name:  "primes from the router, an idle account's among them",
			doc:   routerDoc("a", 0, readAccount("a", sessionOf(0.1, 2*time.Hour)), lapsed),
			slots: []status.Slot{{Account: "spare", At: "13:50", Next: now.Add(38 * time.Minute)}, {Account: "a", At: "03:50", Next: now.Add(14 * time.Hour)}},
			want: []string{
				"13:50 spare is primed",
				"15:12 a's session resets",
				"03:12 a is primed",
				"Sun 13:12 spare's week resets",
			},
		},
		{
			name:  "nothing that's passed, nor a prime the document doesn't time",
			doc:   routerDoc("a", 0, readAccount("a", sessionOf(0.1, -time.Minute)), pastLimit),
			slots: []status.Slot{{Account: "a", At: "03:50"}},
			want:  []string{"14:12 over's session resets", "Thu 13:12 over's week resets"},
		},
		{
			name: "a limit on a model's own window, which leaves the session running",
			doc:  routerDoc("fable", 0, fable),
			want: []string{"15:12 fable's session resets", "Thu 13:12 fable's week resets", "Fri 13:12 fable back from its Fable week limit"},
		},
		{
			name: "an account its reserve holds back, reaching it before its limit",
			doc:  routerDoc("work", 0, reserved),
			want: []string{"14:20 work reaches its reserve", "16:12 work's session resets", "Fri 13:12 work's week resets"},
		},
		{
			name: "an account a pin names, its reserve not holding it back",
			doc:  pinned,
			want: []string{"14:42 work runs out at its pace", "16:12 work's session resets", "Fri 13:12 work's week resets"},
		},
		{
			name: "a window other than the session running out, named",
			doc:  routerDoc("client", 0, running),
			want: []string{"16:12 client's session resets", "Wed 06:20 client's week runs out at its pace", "Thu 13:12 client's week resets"},
		},
		{
			name: "a window read spent, without the router, back as it resets",
			doc:  probed(spent, limitedAccount("held")),
			want: []string{
				"14:12 held back from its limit",
				"16:12 spent's session resets",
				"Wed 13:12 spent back from its Fable week limit",
				"Thu 13:12 spent's week resets",
				"Thu 13:12 held's week resets",
			},
		},
		{
			name: "an account back from its cap as its week resets",
			doc:  routerDoc("capped", 0, capped),
			want: []string{"15:12 capped's session resets", "Wed 13:12 capped back from its reserve"},
		},
		{
			name: "an account back from the cap on a model's own week",
			doc:  routerDoc("fable", 0, modelCapped),
			want: []string{"15:12 fable's session resets", "Wed 13:12 fable back from its Fable week reserve", "Thu 13:12 fable's week resets"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			doc.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: tt.slots}
			var got []string
			for _, h := range upcoming(doc.WorkedOut(claudeLike), now, claudeLike) {
				got = append(got, status.When(now, h.at)+" "+h.name+h.what)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("upcoming() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCOMINGUPDrawsWhatTheDocumentGivesAfterNow(t *testing.T) {
	doc := routerDoc("work", 0, readAccount("work", sessionOf(0.1, 2*time.Hour)))
	doc.ComingUp = []status.Upcoming{
		{At: now.Add(-time.Minute).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "7d"},
		{At: now.UTC(), Account: "work", Kind: status.UpcomingPrime},
		{At: now.Add(time.Hour).UTC(), Account: "work", Kind: "later", Window: "5h"},
		{At: now.Add(2 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "5h"},
	}
	var got []string
	for _, h := range upcoming(doc, now, claudeLike) {
		got = append(got, status.When(now, h.at)+" "+h.name+h.what)
	}
	if want := []string{"15:12 work's session resets"}; !slices.Equal(got, want) {
		t.Errorf("upcoming() = %q, want %q: what the document gives, after now, of the kinds it knows", got, want)
	}
}

func TestCOMINGUPsInks(t *testing.T) {
	doc := threeRouted()
	doc.Prime = status.Prime{Slots: []status.Slot{{Account: "side", Next: now.Add(time.Minute)}}}
	reserved := pressedAccount("client")
	reserved.Reserve = 0.1
	capped := readAccount("capped", sessionOf(0.2, 2*time.Hour), weekOf(0.95, 2*day))
	capped.Reserve = 0.1
	doc.Accounts = append(doc.Accounts, reserved, readAccount("lab", sessionOf(0.1, 3*time.Hour), weekOf(0.7, 3*day)), capped)
	want := map[string]theme.Token{
		" is primed":                   theme.AccentPrimary,
		" back from its limit":         theme.TextMuted,
		" back from its reserve":       theme.TextMuted,
		" runs out at its pace":        theme.AccentAttention,
		" reaches its reserve":         theme.AccentAttention,
		"'s week runs out at its pace": theme.AccentAttention,
		"'s session resets":            theme.TextMuted,
		"'s week resets":               theme.TextMuted,
	}
	seen := map[string]bool{}
	for _, h := range upcoming(doc.WorkedOut(claudeLike), now, claudeLike) {
		seen[h.what] = true
		if h.ink.token != want[h.what] {
			t.Errorf("%q is in %v, want %v", h.what, h.ink.token, want[h.what])
		}
	}
	for what := range want {
		if !seen[what] {
			t.Errorf("COMING UP says nothing that%s", what)
		}
	}
}

func TestCOMINGUPListsThreeWithTheirCountdownsAtTheRight(t *testing.T) {
	f := frameOf(160, 10)
	c := newCanvas(f.Width, f.Height)
	f.comingSlot(c, threeRouted(), now, 115, 0)

	rows := c.rows(Look{})
	want := []string{
		"COMING UP",
		"14:12  personal back from its l… | in 1h",
		"14:42  work runs out at its pace | in 1h 30m",
		"16:12  work's session resets | in 3h",
	}
	for i, w := range want {
		if got := apart.ReplaceAllString(strings.TrimSpace(rows[i]), " | "); got != w {
			t.Errorf("row %d = %q, want %q", i+1, got, w)
		}
	}
	if !strings.HasSuffix(rows[2], "in 1h 30m") || len([]rune(rows[2])) != f.edge() {
		t.Errorf("row 3 = %q, want its countdown ending a column short of the frame's edge", rows[2])
	}
	if rows[4] != "" {
		t.Errorf("row 5 = %q, want three things at most", rows[4])
	}
}

func TestCOMINGUPsTimesAreAsFarFromTheirWordsAsRECENTsOnAPhone(t *testing.T) {
	doc := routerDoc("work", 0, pressedAccount("work"))
	doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"}}
	for _, width := range []int{52, 160} {
		f := frameOf(width, 10)
		c := newCanvas(f.Width, f.Height)
		f.comingLines(c, upcoming(doc, now, f.Policy), now, 0, 0, 1)
		coming := c.rows(Look{})[0]
		recent := f.tellings(doc, now, 1)[0].lead.plain()
		if gap, want := coming[len("14:42"):strings.Index(coming, "work")], recent[len("13:11"):strings.Index(recent, "●")]; gap != want {
			t.Errorf("at %d columns, COMING UP's time is %q from its words, want %q, as RECENT's", width, gap, want)
		}
	}
}

// apart are the blanks that set a line's right apart from its left.
var apart = regexp.MustCompile(` {3,}`)
