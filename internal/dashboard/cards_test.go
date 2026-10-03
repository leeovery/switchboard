package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// pressedWork is work as the frames have it, the primary keeping a tenth of
// every window back: under pressure, its session reaching its reserve
// before it resets, with two sessions, one busy; and side, open, where new
// sessions go.
func pressedWork() status.Document {
	work := pressedAccount("work")
	work.Primary, work.Reserve, work.Sessions = true, 0.1, 2
	work.Windows = append(work.Windows, fableOf(0.12, 4*day))
	doc := routerDoc("side", 2, work, readAccount("side", sessionOf(0.12, 4*time.Hour), weekOf(0.33, 5*day), fableOf(0.04, 5*day)))
	doc.Primary = "work"
	return doc
}

// cardOf draws the card of doc's account with the given id at now, width
// cells wide at the density d, and returns its rows.
func cardOf(t *testing.T, f Frame, doc status.Document, id string, width int, d density) []string {
	t.Helper()
	faces, shown := f.faces(doc, now)
	i := slices.IndexFunc(faces, func(fc face) bool { return fc.account.ID == id })
	if i < 0 {
		t.Fatalf("no account %s", id)
	}
	bars := barRowsFor(len(shown), width-2*(padding+1))
	c := newCanvas(width, d.rows(bars))
	f.card(c, faces[i], now, 0, 0, width, d, bars, labelWidth(doc, shown))
	return c.rows(Look{})
}

func TestEachDensityTakesItsRows(t *testing.T) {
	tests := []struct {
		density density
		want    int
	}{
		{density: density{fullCard, 4}, want: 12 + 4 + 2},
		{density: density{midCard, 6}, want: 6 + 6 + 2},
		{density: density{midCard, 3}, want: 6 + 3 + 2},
		{density: density{compactCard, 3}, want: 4 + 3 + 2},
		{density: density{compactCard, 2}, want: 4 + 2 + 2},
	}
	for _, tt := range tests {
		if got := tt.density.rows(2); got != tt.want {
			t.Errorf("a %+v card with two rows of bars takes %d rows, want %d", tt.density, got, tt.want)
		}
	}
}

func TestACardAtEachDensity(t *testing.T) {
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike, Sessions: []status.Session{
		{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: now.Add(-20 * time.Second).UTC()}}},
		{ID: "7f3a0c94", Assignments: []status.Assignment{{Account: "work", LastSeen: now.Add(-9 * time.Minute).UTC()}}},
	}}
	tests := []struct {
		name    string
		density density
		want    []string
	}{
		{name: "full: its readout, a 4-row chart and its axis, blanks between", density: densities[0], want: []string{
			"╭─ 1 work ─────────────────────────── ◆ primary ─╮",
			"│                                                │",
			"│  ● under pressure · new sessions go elsewhere  │",
			"│                                                │",
			"│  █▀▀ █▀█ ▀ █     SESSION  5-hour window        │",
			"│  ▀▀█ █▀█ ▄▀      → reaches its reserve ~14:20  │",
			"│  ▀▀▀ ▀▀▀ ▀ ▀     resets 16:12 · in 3h          │",
			"│                                                │",
			"│                    │                           │",
			"│                    │                           │",
			"│  ▅▅▅▅▅▅▅▅▅▅▅▅▅▅ no history yet                 │",
			"│  ██████████████████│ ⠠ ⠠ ⠡⠁⠢✕⠠ ⠠ ⠠ ⠠ ⠠ ⠠ ⠠ ⠠   │",
			"│  11:12           now                    16:12  │",
			"│                                                │",
			"│  Week     ██████▏┃██████▎░╎░  34% → 79%        │",
			"│  Fable wk ██▏██░░┃░░░░░░░░╎░  12% → 28%        │",
			"│                                                │",
			"╰─ ● ○ ───────────────────────────── 2 sessions ─╯",
		}},
		{name: "mid: a one-line header, a 6-row chart and its axis", density: densities[1], want: []string{
			"╭─ 1 work ─────────────────────────── ◆ primary ─╮",
			"│  ● under pressure · new sessions go elsewhere  │",
			"│                                                │",
			"│  Session  58%  → out ~14:20      resets 16:12  │",
			"│                    │                           │",
			"│                    │                           │",
			"│                    │                           │",
			"│  ▄▄▄▄▄▄▄▄▄▄▄▄▄▄ no history yet                 │",
			"│  ██████████████████│  ⠁⠂⠂⠄⡀                    │",
			"│  ██████████████████│ ⠐ ⠐ ⠐ ⠑✕⠐ ⠐ ⠐ ⠐ ⠐ ⠐ ⠐ ⠐   │",
			"│  11:12           now                    16:12  │",
			"│  Week     ██████▏┃██████▎░╎░  34% → 79%        │",
			"│  Fable wk ██▏██░░┃░░░░░░░░╎░  12% → 28%        │",
			"╰─ ● ○ ───────────────────────────── 2 sessions ─╯",
		}},
		{name: "compact: the header and a 2-row chart", density: densities[len(densities)-1], want: []string{
			"╭─ 1 work ─────────────────────────── ◆ primary ─╮",
			"│  ● under pressure · new sessions go elsewhere  │",
			"│  Session  58%  → out ~14:20      resets 16:12  │",
			"│                    │                           │",
			"│  ▇▇▇▇▇▇▇▇▇▇▇▇▇▇ no history yet ⠠ ⠠ ⠠ ⠠ ⠠ ⠠ ⠠   │",
			"│  Week     ██████▏┃██████▎░╎░  34% → 79%        │",
			"│  Fable wk ██▏██░░┃░░░░░░░░╎░  12% → 28%        │",
			"╰─ ● ○ ───────────────────────────── 2 sessions ─╯",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cardOf(t, f, pressedWork(), "work", 50, tt.density)
			if !slices.Equal(got, tt.want) {
				t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestACardsStateLeadsWithItsMarkInItsColour(t *testing.T) {
	tests := []struct {
		condition status.Condition
		mark      string
		tone      theme.Token
		says      ink
	}{
		{condition: status.Tokenless, mark: "✕", tone: theme.StateDestructive, says: ink{token: theme.StateDestructive, bold: true}},
		{condition: status.Unreadable, mark: "!", tone: theme.StateDestructive, says: ink{token: theme.StateDestructive, bold: true}},
		{condition: status.Unread, mark: "…", tone: theme.TextSubtle, says: dimInk},
		{condition: status.Limited, mark: "■", tone: theme.StateDestructive, says: ink{token: theme.StateDestructive, bold: true}},
		{condition: status.PartlyLimited, mark: "■", tone: theme.AccentAttention, says: ink{token: theme.AccentAttention, bold: true}},
		{condition: status.Reserved, mark: "●", tone: theme.AccentAttention, says: ink{token: theme.AccentAttention, bold: true}},
		{condition: status.Pressed, mark: "●", tone: theme.AccentAttention, says: ink{token: theme.AccentAttention, bold: true}},
		{condition: status.Idle, mark: "○", tone: theme.TextSubtle, says: ink{token: theme.TextTertiary, bold: true}},
		{condition: status.Open, mark: "●", tone: theme.StatePositive, says: ink{token: theme.StatePositive, bold: true}},
	}
	for _, tt := range tests {
		fc := face{state: status.State{Condition: tt.condition, Says: "says", Then: "then"}}
		want := line{{tt.mark + " ", ink{token: tt.tone}}, {"says", tt.says}, {" · then", mutedInk}}
		if got := fc.stateLine(); !slices.Equal(got, want) {
			t.Errorf("condition %d reads %+v, want %+v", tt.condition, got, want)
		}
		if got := fc.tone(); got != tt.tone {
			t.Errorf("condition %d is in %v, want %v", tt.condition, got, tt.tone)
		}
	}
}

func TestACardsTopEdge(t *testing.T) {
	a := readAccount("personal", sessionOf(0.2, time.Hour))
	primary := with(a, func(a *status.Account) { a.Primary = true })
	tests := []struct {
		name  string
		fc    face
		width int
		want  string
	}{
		{name: "its place and name", fc: face{account: a, place: 2}, width: 50, want: "╭─ 2 personal ───────────────────────────────────╮"},
		{name: "the primary", fc: face{account: primary, place: 2}, width: 50, want: "╭─ 2 personal ─────────────────────── ◆ primary ─╮"},
		{
			name: "every badge, the name kept whole", fc: face{account: primary, place: 2, pinned: true, next: true}, width: 50,
			want: "╭─ 2 personal ─── ◆ primary · ● pinned · ▲ next ─╮",
		},
		{
			name: "a long name, cut to leave room for every badge", fc: face{account: with(a, func(a *status.Account) { a.Label = "personal-laptop" }), place: 2}, width: 50,
			want: "╭─ 2 personal-… ─────────────────────────────────╮",
		},
		{name: "on a narrow card, the name given the room the badges shown leave", fc: face{account: a, place: 2}, width: 30, want: "╭─ 2 personal ───────────────╮"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(tt.width, 1)
			topEdge(c, tt.fc, 0, 0, tt.width, borderInk)
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("drew\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestTheCardNewSessionsGoToIsEdgedInAccentMode(t *testing.T) {
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike}
	doc := pressedWork()
	faces, shown := f.faces(doc, now)
	for i, want := range []ink{borderInk, bestBorderInk} {
		c := newCanvas(50, 20)
		f.card(c, faces[i], now, 0, 0, 50, densities[0], barRowsFor(len(shown), 44), labelColumn)
		if got := c.at(0, 0).ink; got != want {
			t.Errorf("%s's card is edged in %+v, want %+v", faces[i].account.ID, got, want)
		}
		if got := c.at(49, 5).ink; got != want {
			t.Errorf("%s's card's side is in %+v, want %+v", faces[i].account.ID, got, want)
		}
	}
}

func TestACardsBottomEdge(t *testing.T) {
	tests := []struct {
		name string
		fc   face
		want string
	}{
		{name: "a dot each, lit while busy, and how many", fc: face{busy: []bool{true, false, true}, sessions: 3}, want: "╰─ ● ○ ● ─────────────────────────── 3 sessions ─╯"},
		{name: "none", fc: face{}, want: "╰────────────────────────────────── no sessions ─╯"},
		{name: "the document's count, without the router's list", fc: face{sessions: 2}, want: "╰─────────────────────────────────── 2 sessions ─╯"},
		{
			name: "more than fit, as many as do", fc: face{busy: slices.Repeat([]bool{true}, 20), sessions: 20},
			want: "╰─ ● ● ● ● ● ● ● ● ● ● ● ● ● ● … ── 20 sessions ─╯",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(50, 1)
			bottomEdge(c, tt.fc, 0, 0, 50, borderInk)
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("drew\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
	c := newCanvas(50, 1)
	bottomEdge(c, face{busy: []bool{true, false}, sessions: 2}, 0, 0, 50, borderInk)
	if lit, idle := c.at(3, 0).ink, c.at(5, 0).ink; lit != positiveInk || idle != dimInk {
		t.Errorf("the dots are in %+v and %+v, want the busy one lit positive, the idle one dim", lit, idle)
	}
}

func TestEveryCardShowsTheSameWindowsInTheSameOrder(t *testing.T) {
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike, Featured: "7d"}
	doc := routerDoc("", 0,
		readAccount("work", sessionOf(0.2, 3*time.Hour), weekOf(0.3, 4*day), fableOf(0.1, 4*day)),
		readAccount("side", sessionOf(0.4, 3*time.Hour), weekOf(0.5, 4*day)),
	)
	tests := []struct {
		id string
		// want is how each of its bars' rows starts: its label, or blank for
		// a window it hasn't.
		want []string
	}{
		{id: "work", want: []string{"│  Session  ", "│  Fable wk "}},
		{id: "side", want: []string{"│  Session  ", "│                                                │"}},
	}
	for _, tt := range tests {
		rows := cardOf(t, f, doc, tt.id, 50, densities[0])
		if !strings.Contains(rows[4], "WEEK") {
			t.Errorf("%s's readout is\n%s\nwant its week, as the setting says", tt.id, rows[4])
		}
		for i, want := range tt.want {
			if got := rows[14+i]; !strings.HasPrefix(got, want) {
				t.Errorf("%s's bar %d reads\n%s\nwant it to start %q", tt.id, i+1, got, want)
			}
		}
	}
}

func TestAReadoutsDigitsTakeTheColourOfItsAccountsState(t *testing.T) {
	doc := routerDoc("work", 0, readAccount("work", sessionOf(0.33, 3*time.Hour), weekOf(0.2, 4*day)), limitedAccount("personal"))
	doc.Events = []status.Event{{ID: 1, At: now.Add(-30 * time.Minute).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}}}
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike}
	tests := []struct {
		id   string
		want []string
		ink  ink
	}{
		{
			id: "work", ink: ink{token: theme.StatePositive},
			want: []string{"▀▀█ ▀▀█ ▀ █     SESSION  5-hour window", "▀▀█ ▀▀█ ▄▀      → 82% by its reset", "▀▀▀ ▀▀▀ ▀ ▀     resets 16:12 · in 3h"},
		},
		{
			id: "personal", ink: errorInk,
			want: []string{"▄█  ▄ █▀█ █▀█    SESSION  5-hour window", " █  ▄ █ █ █ █    limit reached at 12:42", "▀▀▀   ▀▀▀ ▀▀▀    its window resets 14:12"},
		},
	}
	for _, tt := range tests {
		faces, _ := f.faces(doc, now)
		i := slices.IndexFunc(faces, func(fc face) bool { return fc.account.ID == tt.id })
		c := newCanvas(44, bigRows)
		f.readout(c, faces[i], now, 0, 0, 44)
		if got := c.rows(Look{}); !slices.Equal(got, tt.want) {
			t.Errorf("%s's readout reads\n%s\nwant\n%s", tt.id, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
		}
		if got := c.at(0, 2).ink; got != tt.ink {
			t.Errorf("%s's digits are in %+v, want %+v", tt.id, got, tt.ink)
		}
	}
}

func TestAReadoutKeepsWhenItRunsOutWholeOverTheRateItGoesBy(t *testing.T) {
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike}
	for _, tt := range []struct {
		reserve float64
		want    string
	}{
		{reserve: 0.1, want: "→ reaches its reserve ~14:20"},
		{reserve: 0, want: "→ runs out ~14:42 at its la…"},
	} {
		doc := pressedWork()
		doc.Accounts[0].Reserve = tt.reserve
		faces, _ := f.faces(doc, now)
		c := newCanvas(44, bigRows)
		f.readout(c, faces[0], now, 0, 0, 44)
		if got := strings.TrimSpace(strings.TrimPrefix(c.rows(Look{})[1], "▀▀█ █▀█ ▄▀")); got != tt.want {
			t.Errorf("with a reserve of %v, it reads %q, want %q", tt.reserve, got, tt.want)
		}
	}
}

func TestAHeaderSaysAFeaturedWindowInALine(t *testing.T) {
	lapsed := readAccount("spare", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.05, 6*day))
	lapsed.Lapsed = []string{"5h"}
	doc := routerDoc("", 0, pressedWork().Accounts[0], limitedAccount("personal"), lapsed)
	doc.Prime = status.Prime{Window: "5h", Slots: []status.Slot{{Account: "spare", At: "16:20", Next: now.Add(3 * time.Hour).UTC()}}}
	tests := []struct {
		id      string
		feature Feature
		want    string
	}{
		{id: "work", want: "Session  58%  → out ~14:20      resets 16:12"},
		{id: "personal", want: "Session  100%  back 14:12       resets 14:12"},
		{id: "spare", want: "Week     5%  → 35%          resets Sun 13:12"},
		{id: "spare", feature: "5h", want: "Session  0%  not started    next prime 16:12"},
	}
	for _, tt := range tests {
		f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike, Featured: tt.feature}
		faces, _ := f.faces(doc, now)
		i := slices.IndexFunc(faces, func(fc face) bool { return fc.account.ID == tt.id })
		c := newCanvas(44, 1)
		f.header(c, faces[i], now, 0, 0, 44, labelColumn)
		if got := c.rows(Look{})[0]; got != tt.want {
			t.Errorf("%s featuring %q reads\n%q\nwant\n%q", tt.id, tt.feature, got, tt.want)
		}
	}
}

func TestFitHeadKeepsALinesFirstPartWhole(t *testing.T) {
	l := line{{"→ runs out ~16:05", alertInk}, {" at its last-30-min rate", mutedInk}}
	tests := []struct {
		width int
		want  string
	}{
		{width: 50, want: "→ runs out ~16:05 at its last-30-min rate"},
		{width: 28, want: "→ runs out ~16:05 at its la…"},
		{width: 21, want: "→ runs out ~16:05"},
		{width: 17, want: "→ runs out ~16:05"},
		{width: 12, want: "→ runs out…"},
	}
	for _, tt := range tests {
		if got := text(l.fitHead(tt.width)); got != tt.want {
			t.Errorf("fitHead(%d) = %q, want %q", tt.width, got, tt.want)
		}
	}
}
