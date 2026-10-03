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
	pastLimit := limitedAccount("over")
	pastLimit.Limit.Until = now.Add(-time.Minute)
	fable := readAccount("fable", sessionOf(0.1, 2*time.Hour), weekOf(0.2, 3*day), windowOf("7d_oi", "Fable week", 1, 4*day))
	fable.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: now.Add(4 * day)}
	reserved := pressedAccount("work")
	reserved.Reserve = 0.1
	pinned := routerDoc("work", 0, reserved)
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	tests := []struct {
		name  string
		doc   status.Document
		slots []status.Slot
		want  []string
	}{
		{
			name: "the frames': a limit lifting, a session running out, and resetting",
			doc:  threeRouted(),
			want: []string{
				"14:12 personal back from its limit",
				"14:42 work runs out at its pace",
				"16:12 work's session resets",
				"17:12 side's session resets",
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
			},
		},
		{
			name:  "nothing that's passed, nor a prime the document doesn't time",
			doc:   routerDoc("a", 0, readAccount("a", sessionOf(0.1, -time.Minute)), pastLimit),
			slots: []status.Slot{{Account: "a", At: "03:50"}},
			want:  []string{"14:12 over's session resets"},
		},
		{
			name: "a limit on a model's own window, which leaves the session running",
			doc:  routerDoc("fable", 0, fable),
			want: []string{"15:12 fable's session resets", "Fri 13:12 fable back from its Fable week limit"},
		},
		{
			name: "an account its reserve holds back, reaching it before its limit",
			doc:  routerDoc("work", 0, reserved),
			want: []string{"14:20 work reaches its reserve", "16:12 work's session resets"},
		},
		{
			name: "an account a pin names, its reserve not holding it back",
			doc:  pinned,
			want: []string{"14:42 work runs out at its pace", "16:12 work's session resets"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.doc.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: tt.slots}
			var got []string
			for _, h := range upcoming(tt.doc, now, claudeLike) {
				got = append(got, status.When(now, h.at)+" "+h.name+h.what)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("upcoming() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCOMINGUPsInks(t *testing.T) {
	doc := threeRouted()
	doc.Prime = status.Prime{Slots: []status.Slot{{Account: "side", Next: now.Add(time.Minute)}}}
	reserved := pressedAccount("client")
	reserved.Reserve = 0.1
	doc.Accounts = append(doc.Accounts, reserved)
	want := map[string]theme.Token{
		" is primed":            theme.AccentPrimary,
		" back from its limit":  theme.TextMuted,
		" runs out at its pace": theme.AccentAttention,
		" reaches its reserve":  theme.AccentAttention,
		"'s session resets":     theme.TextMuted,
	}
	seen := map[string]bool{}
	for _, h := range upcoming(doc, now, claudeLike) {
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

// apart are the blanks that set a line's right apart from its left.
var apart = regexp.MustCompile(` {3,}`)
