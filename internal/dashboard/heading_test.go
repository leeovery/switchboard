package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestROUTERSaysHowTheRouterIs(t *testing.T) {
	healthy := threeRouted()
	unhealthy := threeRouted()
	unhealthy.Router = status.Health{Reason: "failed 6 of 8 requests in the last 5 minutes"}
	restart := threeRouted()
	restart.Restart = status.Restart{Reason: "config changed", Since: now.UTC()}
	pinned := threeRouted()
	pinned.Pin = status.Pin{Accounts: []string{"work"}, Since: now.UTC()}
	pinnedToTwo := threeRouted()
	pinnedToTwo.Pin = status.Pin{Accounts: []string{"work", "side"}, Since: now.UTC()}
	stuck := probed(readAccount("work", sessionOf(0.2, time.Hour)))
	stuck.Fallback = status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}
	asked := probed(readAccount("work", sessionOf(0.2, time.Hour)))
	asked.Fallback = status.Fallback{}
	one := routerDoc("work", 3, readAccount("work", sessionOf(0.2, time.Hour)))
	priming := one
	priming.Prime = status.Prime{Day: "08:00-22:00", Window: "5h"}
	tests := []struct {
		name string
		doc  status.Document
		lost time.Time
		want []string
	}{
		{name: "healthy", doc: healthy, want: []string{"● healthy", "5 sessions · auto"}},
		{name: "unhealthy, and why", doc: unhealthy, want: []string{"● unhealthy", "5 sessions · auto", "failed 6 of 8 requests in the last 5 minutes"}},
		{name: "with a restart due", doc: restart, want: []string{"● healthy", "5 sessions · auto", "restart due (config changed)"}},
		{name: "pinned to an account", doc: pinned, want: []string{"● healthy", "5 sessions · pinned to work"}},
		{name: "pinned to two", doc: pinnedToTwo, want: []string{"● healthy", "5 sessions · pinned to work and side"}},
		{name: "not answering, its last document on screen", doc: restart, lost: now.Add(-2 * time.Minute), want: []string{"○ no router since 13:10", "5 sessions · auto", "restart due (config changed)"}},
		{name: "probing, the router not running", doc: probed(readAccount("work", sessionOf(0.2, time.Hour))), want: []string{"○ probing", "router not running"}},
		{name: "probing, the router not answering as it should", doc: stuck, want: []string{"○ probing", "router unhealthy: no answer within 500ms"}},
		{name: "probing as asked", doc: asked, want: []string{"○ probing, as asked"}},
		{name: "one account, priming", doc: priming, want: []string{"● healthy", "3 sessions · priming 08:00–22:00"}},
		{name: "one account, not priming", doc: one, want: []string{"● healthy", "3 sessions"}},
		{name: "nothing read", doc: status.Document{}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range flow(routerSays(tt.doc, tt.lost, now), []int{80, 80, 80}, " · ") {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ROUTER says %q, want %q", got, tt.want)
			}
		})
	}
}

func TestROUTERsLastLineAlwaysShowsARestartDue(t *testing.T) {
	doc := func(healthy bool, reason string, restart bool, pin ...string) status.Document {
		d := threeRouted()
		d.Router = status.Health{Healthy: healthy, Reason: reason}
		if restart {
			d.Restart = status.Restart{Reason: "config changed", Since: now.UTC()}
		}
		d.Pin = status.Pin{Accounts: pin}
		return d
	}
	reason := "6 of the 8 requests in the last 5 minutes failed"
	inRow := []int{24, 24, 55}
	tests := []struct {
		name   string
		doc    status.Document
		lost   time.Time
		widths []int
		want   []string
	}{
		{
			name: "a restart due, its routing folded onto its sessions' line", doc: doc(true, "", true, "work", "side"), widths: inRow,
			want: []string{"● healthy", "5 sessions · pinned to…", "restart due (config changed)"},
		},
		{
			name: "unhealthy, its reason last", doc: doc(false, reason, false, "work", "side"), widths: inRow,
			want: []string{"● unhealthy", "5 sessions · pinned to…", reason},
		},
		{
			name: "unhealthy with a restart due: its reason wrapping under how it is", doc: doc(false, reason, true), widths: inRow,
			want: []string{"● unhealthy · 6 of the 8", "requests in the last 5…", "restart due (config changed)"},
		},
		{
			name: "unhealthy with a restart due, in two rows: its sessions after its reason", doc: doc(false, reason, true), widths: []int{47, 47, 118},
			want: []string{"● unhealthy · 6 of the 8 requests in the last 5", "minutes failed · 5 sessions · auto", "restart due (config changed)"},
		},
		{
			name: "its last document on screen, unhealthy: its reason kept", doc: doc(false, reason, false), lost: now.Add(-2 * time.Minute), widths: inRow,
			want: []string{"○ no router since 13:10", "5 sessions · auto", reason},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range routerLines(tt.doc, tt.lost, now, tt.widths) {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ROUTER says %q, want %q", got, tt.want)
			}
		})
	}
	rows := frameOf(160, 40).Draw(doc(true, "", true, "work", "side"), now)
	if got, want := rows[5], " restart due (config changed)"; !strings.HasPrefix(got, want) {
		t.Errorf("ROUTER's last line reads %q, want it to start %q", got, want)
	}
}

func TestCOMINGUPStartsClearOfTheRoomsSummed(t *testing.T) {
	var accounts []status.Account
	for i := range 12 {
		accounts = append(accounts, readAccount(fmt.Sprintf("a%d", i), sessionOf(0.1, 3*time.Hour), weekOf(0.1, 4*day)))
	}
	doc := routerDoc("a0", 0, accounts...)
	for _, width := range []int{160, 120} {
		f := frameOf(width, 40)
		c := newCanvas(f.Width, f.Height)
		f.heading(c, doc, now, 2)
		for _, row := range c.rows(Look{})[2 : 2+f.headingRows(len(doc.Accounts))] {
			if !strings.Contains(row, roomGlyph) {
				continue
			}
			if _, after, _ := strings.CutLast(row, "of 12"); !strings.HasPrefix(after, "  ") {
				t.Errorf("at %d columns, ROOM LEFT and COMING UP read %q, want two blanks after the rooms summed", width, row)
			}
		}
	}
}

func TestROUTERSaysItInItsInks(t *testing.T) {
	unhealthy := threeRouted()
	unhealthy.Router = status.Health{Reason: "failing"}
	unhealthy.Restart = status.Restart{Reason: "config changed"}
	tests := []struct {
		name string
		doc  status.Document
		lost time.Time
		// want are inks the parts said in, by their text.
		want map[string]theme.Token
	}{
		{name: "healthy", doc: threeRouted(), want: map[string]theme.Token{"● ": theme.StatePositive, "healthy": theme.TextPrimary, "5 sessions": theme.TextMuted}},
		{name: "unhealthy", doc: unhealthy, want: map[string]theme.Token{"● ": theme.StateDestructive, "unhealthy": theme.StateDestructive, "failing": theme.StateDestructive, "restart due (config changed)": theme.AccentAttention}},
		{name: "lost", doc: threeRouted(), lost: now, want: map[string]theme.Token{"○ no router since 13:12": theme.TextSubtle}},
		{name: "probing", doc: probed(), want: map[string]theme.Token{"○ probing": theme.TextSubtle, "router not running": theme.TextSubtle}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			said := together(routerSays(tt.doc, tt.lost, now), " · ")
			for text, token := range tt.want {
				i := slices.IndexFunc(said, func(s span) bool { return s.text == text })
				if i < 0 || said[i].ink.token != token {
					t.Errorf("ROUTER says %+v, want %q in %v", said, text, token)
				}
			}
		})
	}
}

func TestFlowLaysPartsOnLines(t *testing.T) {
	text := func(s string) line { return line{{s, mutedInk}} }
	tests := []struct {
		name   string
		chunks []chunk
		widths []int
		want   []string
	}{
		{name: "following on a line", chunks: []chunk{fresh(text("5 sessions")), {text: text("auto")}}, widths: []int{24, 24}, want: []string{"5 sessions · auto"}},
		{name: "each on a line of its own", chunks: []chunk{fresh(text("● healthy")), fresh(text("5 sessions"))}, widths: []int{24, 24}, want: []string{"● healthy", "5 sessions"}},
		{name: "on the next line, where it doesn't fit", chunks: []chunk{fresh(text("5 sessions")), {text: text("pinned to work and side")}}, widths: []int{24, 24}, want: []string{"5 sessions", "pinned to work and side"}},
		{name: "broken between words onto the next lines", chunks: []chunk{fresh(text("router unhealthy: no answer within 500ms"))}, widths: []int{24, 24}, want: []string{"router unhealthy: no", "answer within 500ms"}},
		{name: "its last line cut short", chunks: []chunk{fresh(text("one two three four five six seven"))}, widths: []int{9, 9}, want: []string{"one two", "three fo…"}},
		{name: "after the last line, following on it, cut short", chunks: []chunk{fresh(text("● healthy")), fresh(text("restart due (config changed)"))}, widths: []int{20}, want: []string{"● healthy · restart…"}},
		{name: "nothing", chunks: nil, widths: []int{24}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range flow(tt.chunks, tt.widths, " · ") {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("flow() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNEWSESSIONSGOTOSaysWhere(t *testing.T) {
	noRoom := threeRouted()
	noRoom.Best = ""
	nothingRead := routerDoc("", 0, status.Account{ID: "work", Label: "work", TokenSet: true}, status.Account{ID: "side", Label: "side", TokenSet: true})
	tests := []struct {
		name     string
		doc      status.Document
		numbered bool
		want     string
		wantInk  theme.Token
	}{
		{name: "the account, by its place", doc: threeRouted(), numbered: true, want: "▲ 3 side", wantInk: theme.AccentMode},
		{name: "the account alone, on a phone", doc: threeRouted(), want: "▲ side", wantInk: theme.AccentMode},
		{name: "none with room", doc: noRoom, numbered: true, want: "no account has room right now", wantInk: theme.StateDestructive},
		{name: "nothing read of any", doc: nothingRead, numbered: true, want: "nothing read yet", wantInk: theme.TextSubtle},
		{name: "no accounts", doc: status.Document{}, numbered: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newSessionsGo(tt.doc, tt.numbered)
			if got := l.plain(); got != tt.want {
				t.Errorf("newSessionsGo() = %q, want %q", got, tt.want)
			}
			if len(l) > 0 && l[len(l)-1].ink.token != tt.wantInk {
				t.Errorf("newSessionsGo() says %q in %v, want %v", tt.want, l[len(l)-1].ink.token, tt.wantInk)
			}
		})
	}
}

func TestOpenCountsTheAccountsTheRouterChoosesANewSessionsAmong(t *testing.T) {
	open := func(id string) status.Account { return readAccount(id, sessionOf(0.2, time.Hour), weekOf(0.3, 2*day)) }
	atReserve := readAccount("held", sessionOf(0.2, time.Hour), weekOf(0.95, 2*day))
	atReserve.Reserve = 0.1
	lapsed := readAccount("idle", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.3, 2*day))
	lapsed.Lapsed = []string{"5h"}
	tokenless := status.Account{ID: "gone", Label: "gone"}
	refused := open("refused")
	refused.Refused = status.Refusal{Status: 401, Until: now.Add(time.Hour)}
	refusedOpus := open("opus")
	refusedOpus.Refused = status.Refusal{Status: 403, Family: "opus", Until: now.Add(time.Hour)}
	fableLimited := open("fable")
	fableLimited.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: now.Add(time.Hour)}
	unnamed := open("unnamed")
	unnamed.Limit = status.Limit{Until: now.Add(time.Hour)}
	pinned := func(d status.Document, ids ...string) status.Document {
		d.Pin = status.Pin{Accounts: ids}
		return d
	}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "the frames': one under pressure, one at its limit, one open", doc: threeRouted(), want: "1 of 3 open"},
		{name: "every one open, idle or held back for some models alone", doc: routerDoc("a", 0, open("a"), lapsed, refusedOpus, fableLimited), want: "4 of 4 open"},
		{name: "every one under pressure", doc: routerDoc("a", 0, pressedAccount("a"), pressedAccount("b")), want: "2 of 2 open"},
		{name: "none at its reserve, without its token or refused its every request", doc: routerDoc("a", 0, open("a"), atReserve, tokenless, refused), want: "1 of 4 open"},
		{name: "none held back by a limit naming no window, its windows with room", doc: routerDoc("a", 0, open("a"), unnamed), want: "1 of 2 open"},
		{name: "the pin's alone, its reserve spent", doc: pinned(routerDoc("held", 0, open("a"), atReserve), "held"), want: "1 of 2 open"},
		{
			name: "the pin's alone, under pressure, though others aren't",
			doc:  pinned(routerDoc("a", 0, pressedAccount("a"), open("b"), open("c")), "a"),
			want: "1 of 3 open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(30, 3)
			(Frame{Policy: claudeLike}).sessionsSlot(c, tt.doc, now, 0, 0, 30)
			if got := strings.TrimSpace(c.rows(Look{})[2]); got != tt.want {
				t.Errorf("NEW SESSIONS GO TO says %q, want %q", got, tt.want)
			}
		})
	}
}

func TestROOMLEFTGivesEachAccountsRoom(t *testing.T) {
	f := Frame{Policy: claudeLike}
	lapsed := readAccount("idle", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.3, 2*day))
	lapsed.Lapsed = []string{"5h"}
	refused := readAccount("refused", sessionOf(0.2, time.Hour), weekOf(0.3, 2*day))
	refused.Refused = status.Refusal{Status: 401, Until: now.Add(time.Hour)}
	weekReset := readAccount("reset", sessionOf(0.2, time.Hour), weekOf(0.7, -time.Hour))
	tests := []struct {
		name          string
		account       status.Account
		session, week float64
	}{
		{name: "the share of each it has left", account: readAccount("a", sessionOf(0.25, time.Hour), weekOf(0.75, 2*day)), session: 0.75, week: 0.25},
		{name: "at its limit", account: limitedAccount("a"), session: 0, week: 0.11},
		{name: "its token refused, which takes no request", account: refused, session: 0, week: 0.7},
		{name: "its session lapsed, all of it", account: lapsed, session: 1, week: 0.7},
		{name: "its week reset since it was read, all of it", account: weekReset, session: 0.8, week: 1},
		{name: "used past its limit, none", account: readAccount("a", sessionOf(1.04, time.Hour), weekOf(1.2, 2*day)), session: 0, week: 0},
		{name: "not read, none", account: status.Account{ID: "a", TokenSet: true}, session: 0, week: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := f.sessionRoom(tt.account, now); !near(got, tt.session) {
				t.Errorf("sessionRoom() = %v, want %v", got, tt.session)
			}
			if got := f.weekRoom(tt.account, now); !near(got, tt.week) {
				t.Errorf("weekRoom() = %v, want %v", got, tt.week)
			}
		})
	}
}

func TestROOMLEFTsBarsNarrowAsAccountsAreAdded(t *testing.T) {
	for n, want := range map[int]int{1: 8, 3: 8, 4: 8, 6: 5, 8: 4, 10: 3, 20: 3} {
		if got := barCells(n); got != want {
			t.Errorf("barCells(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestROOMLEFTsBarsFillToTheShareLeft(t *testing.T) {
	tests := []struct {
		name string
		look Look
		left float64
		want string
	}{
		{name: "in colour, the empty cells faint", look: Screen(builtin(t, "nord")), left: 0.42, want: "▆▆▆▆▆▆▆▆"},
		{name: "without colour, the empty cells a track", look: NoColour(), left: 0.42, want: "▆▆▆░░░░░"},
		{name: "full", look: NoColour(), left: 1, want: "▆▆▆▆▆▆▆▆"},
		{name: "empty", look: NoColour(), left: 0, want: "░░░░░░░░"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := (Frame{Look: tt.look}).roomBar(tt.left, 8)
			if got := bar.plain(); got != tt.want {
				t.Errorf("roomBar(%v) = %q, want %q", tt.left, got, tt.want)
			}
			if bar[0].ink != accentInk || bar[1].ink != emptyRoomInk {
				t.Errorf("roomBar(%v) is in %+v, want the room in accent.mode and the rest the border, faint", tt.left, bar)
			}
		})
	}
	nord := Screen(builtin(t, "nord"))
	if got, want := nord.colour(emptyRoomInk), rgb(0x434C5D); !sameColor(got, want) {
		t.Errorf("an empty cell is %v, want %v: the border blended 30%% into the canvas", got, want)
	}
}

func TestTheHeadingIsArrangedByWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		doc   status.Document
		// want are the heading's rows, from the third, as text, trimmed.
		want []string
	}{
		{name: "in a row from 150 columns", width: 160, doc: threeRouted(), want: []string{
			" ROUTER                   NEW SESSIONS GO TO              ROOM LEFT, IN ACCOUNTS                          COMING UP",
			" ● healthy                ▲ 3 side                        5h    ▆▆▆░░░░░ ░░░░░░░░ ▆▆▆▆▆▆▆░  1.3 of 3      14:12  personal back from its limit             in 1h",
			" 5 sessions · auto        1 of 3 open                     week  ▆▆▆▆▆░░░ ▆░░░░░░░ ▆▆▆▆▆░░░  1.4 of 3      14:42  work runs out at its pace            in 1h 30m",
			"                                                                1        2        3                       16:12  work's session resets                    in 3h",
		}},
		{name: "in two rows of two from 100", width: 120, doc: threeRouted(), want: []string{
			" ROUTER                                          NEW SESSIONS GO TO",
			" ● healthy                                       ▲ 3 side",
			" 5 sessions · auto                               1 of 3 open",
			"",
			" ROOM LEFT, IN ACCOUNTS                          COMING UP",
			" 5h    ▆▆▆░░░░░ ░░░░░░░░ ▆▆▆▆▆▆▆░  1.3 of 3      14:12  personal back from its limit                              in 1h",
			" week  ▆▆▆▆▆░░░ ▆░░░░░░░ ▆▆▆▆▆░░░  1.4 of 3      14:42  work runs out at its pace                             in 1h 30m",
			"       1        2        3                       16:12  work's session resets                                     in 3h",
		}},
		{name: "in two lines under 100, for a phone", width: 52, doc: threeRouted(), want: []string{
			" ● healthy · 5 sessions · auto",
			" new → ▲ side                   room 5h 1.3  wk 1.4",
		}},
		{name: "a line of its own with one account", width: 160, doc: routerDoc("work", 3, readAccount("work", sessionOf(0.58, 3*time.Hour), weekOf(0.34, 4*day))), want: []string{
			" ROUTER   ● healthy  ·  3 sessions                                                                                                ROOM LEFT   5h 42%   week 66%",
		}},
		{name: "a line of its own with one account, on a phone", width: 52, doc: routerDoc("work", 3, readAccount("work", sessionOf(0.58, 3*time.Hour), weekOf(0.34, 4*day))), want: []string{
			" ● healthy · 3 sessions         room 5h 42%  wk 66%",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 40)
			c := newCanvas(f.Width, f.Height)
			top := 2
			if f.phone() {
				top = 4
			}
			f.heading(c, tt.doc, now, top)
			rows := c.rows(Look{})[top : top+f.headingRows(len(tt.doc.Accounts))]
			if !slices.Equal(rows, tt.want) {
				t.Errorf("the heading is\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestTheHeadingIsLabelsAloneBeforeAnythingIsRead(t *testing.T) {
	f := frameOf(160, 40)
	c := newCanvas(f.Width, f.Height)
	f.heading(c, status.Document{}, now, 2)
	if rows := f.headingRows(0); rows != 4 {
		t.Errorf("headingRows() = %d, want 4: its rows kept", rows)
	}
	rows := c.rows(Look{})
	if want := " ROUTER                   NEW SESSIONS GO TO              ROOM LEFT, IN ACCOUNTS                          COMING UP"; rows[2] != want {
		t.Errorf("row 3 = %q, want %q", rows[2], want)
	}
	for i, row := range rows[3:6] {
		if row != "" {
			t.Errorf("row %d = %q, want nothing under the labels", i+4, row)
		}
	}
}

func TestROOMLEFTSumsTheRoomsAsDrawn(t *testing.T) {
	f := Frame{Policy: claudeLike}
	doc := routerDoc("a", 0,
		readAccount("a", sessionOf(0.58, time.Hour), weekOf(0.34, 2*day)),
		limitedAccount("b"),
		readAccount("c", sessionOf(0.12, time.Hour), weekOf(0.33, 2*day)),
	)
	if got := f.roomSum(doc, now, f.sessionRoom); got != "1.3" {
		t.Errorf("the 5h row sums to %s, want 1.3", got)
	}
	if got := f.roomSum(doc, now, f.weekRoom); got != "1.4" {
		t.Errorf("the week row sums to %s, want 1.4", got)
	}
}

// near reports whether a and b are equal but for rounding.
func near(a, b float64) bool {
	return a-b < 1e-9 && b-a < 1e-9
}
