package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// routedAt is the moment the routing frames were drawn at: Thu 1 Oct 2026,
// 14:42:07.
var routedAt = time.Date(2026, 10, 1, 14, 42, 7, 0, time.UTC)

// routedClock is the time given on a day of routedAt's month.
func routedClock(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

// routedWindow is a window used so far, resetting at the time given.
func routedWindow(key string, used float64, resets time.Time) quota.Window {
	return quota.Window{Key: key, Label: key, Utilization: used, ResetsAt: resets}
}

// routedAccount is an account read a minute before routedAt, its 5-hour
// window and its week used as given.
func routedAccount(id string, session, week quota.Window) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: routedAt.Add(-time.Minute), Windows: []quota.Window{session, week}}
}

// routedWorld is the world the routing frames were drawn of: work at its cap
// till 17:10, its 5-hour at 95%; personal open, or, with limit set, at its
// limit till 15:54, its sessions moved to side; and side, where new sessions
// go; and the sessions running on each.
func routedWorld(limit bool) (status.Document, []status.Session) {
	work := routedAccount("work", routedWindow("5h", 0.95, routedClock(1, 17, 10)), routedWindow("7d", 0.34, routedClock(4, 21, 0)))
	work.Reserve = 0.1
	personal := routedAccount("personal", routedWindow("5h", 0.39, routedClock(1, 16, 30)), routedWindow("7d", 0.66, routedClock(3, 2, 0)))
	side := routedAccount("side", routedWindow("5h", 0.13, routedClock(1, 18, 50)), routedWindow("7d", 0.78, routedClock(5, 10, 0)))
	running := map[string]int{"personal": 2, "side": 4}
	if limit {
		personal.Windows[0] = routedWindow("5h", 1, routedClock(1, 15, 54))
		personal.Windows[0].Status = quota.StatusRejected
		personal.Limit = status.Limit{Windows: []string{"5h"}, Until: routedClock(1, 15, 54)}
		running = map[string]int{"side": 6}
	}
	doc := status.Document{
		GeneratedAt: routedAt, Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true},
		Sessions: 6, Accounts: []status.Account{work, personal, side},
	}.WorkedOut(claudeLike)
	return doc, sessionsRunning(running)
}

// sessionsRunning are sessions running on the accounts given, as many on
// each as given, each seen a minute ago.
func sessionsRunning(on map[string]int) []status.Session {
	var sessions []status.Session
	for account, n := range on {
		for i := range n {
			sessions = append(sessions, status.Session{ID: account + "-" + strconv.Itoa(i), Assignments: []status.Assignment{{
				Model: "claude-opus", Family: "opus", Account: account, AssignedAt: routedAt.Add(-time.Hour), LastSeen: routedAt.Add(-time.Minute),
			}}})
		}
	}
	return sessions
}

// dollars is an amount in dollars, to the cent.
func dollars(cents int64) *ledger.Picodollars {
	p := ledger.Picodollars(cents * 10_000_000_000)
	return &p
}

func TestTheRoutingCardIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	// The frames colour each figure along the ramp, where the design colours
	// it by its level, as usage does: its first stop below 70%, its second
	// from 70%, its last from 90%.
	figures := []spot{{14, 75, 78}, {14, 86, 89}, {15, 75, 78}, {15, 86, 89}, {16, 75, 78}, {16, 86, 89}}
	tests := []struct {
		stem  string
		limit bool
		card  RoutingCard
		// whole are the cells the frame draws otherwise than the world.
		whole []spot
	}{
		{stem: "routing-auto-160", card: RoutingCard{}},
		{stem: "routing-two-160", card: RoutingCard{Picked: []string{"personal", "side"}, Cursor: 2}},
		{stem: "routing-move-160", card: RoutingCard{Picked: []string{"personal"}, Cursor: 2, Move: true, Moving: 4, Cost: dollars(620)}},
		// The frame's card draws personal's week 64%, where its page, and
		// the world, have 66%.
		{stem: "routing-limit-160", limit: true, card: RoutingCard{Picked: []string{"side"}, Cursor: 3}, whole: []spot{{15, 87, 87}}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			doc, sessions := routedWorld(tt.limit)
			tt.card.Rows = RoutingRows(doc, sessions, routedAt, claudeLike)
			f := Frame{Width: 160, Height: 37, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions, Card: tt.card}
			drawn := screenOf(t, f.Draw(doc, routedAt))
			heldTo(t, tt.stem, reference(t, tt.stem), drawn, region{top: 9, bottom: 26, left: 39, right: 120}, shadeOf(nord.Canvas()), tt.whole, figures)
		})
	}
}

func TestTheRoutingCardsRowsAreAutoThenEachAccountNumbered(t *testing.T) {
	accounts := make([]status.Account, 10)
	for i := range accounts {
		accounts[i] = readAccount("a"+strconv.Itoa(i+1), sessionOf(0.1, time.Hour))
	}
	rows := RoutingRows(routerDoc("a1", 0, accounts...), nil, now, claudeLike)

	if len(rows) != 11 {
		t.Fatalf("%d rows, want 11: auto's, then the ten accounts'", len(rows))
	}
	if got, want := rows[0], (RoutingRow{Key: "0", Name: "auto", Open: true, Pickable: true}); got != want {
		t.Errorf("the first row is %+v, want auto's, %+v", got, want)
	}
	for i, row := range rows[1:] {
		want := strconv.Itoa(i + 1)
		if i == 9 {
			want = ""
		}
		if row.Account != accounts[i].ID || row.Key != want {
			t.Errorf("row %d is %s's, numbered %q, want %s's, numbered %q: the first nine alone have a number", i+2, row.Account, row.Key, accounts[i].ID, want)
		}
	}
}

func TestAnAccountIsOpenOrClosedAndWhy(t *testing.T) {
	refused := readAccount("refused", sessionOf(0.1, time.Hour))
	refused.Refused = status.Refusal{Until: now.Add(time.Hour).UTC(), Status: 401}
	familyRefused := readAccount("family", sessionOf(0.1, time.Hour))
	familyRefused.Refused = status.Refusal{Until: now.Add(time.Hour).UTC(), Status: 429, Family: "fable"}
	tokenless := readAccount("tokenless")
	tokenless.TokenSet = false
	capped := readAccount("capped", sessionOf(0.95, 2*time.Hour), weekOf(0.4, 3*day))
	capped.Reserve = 0.1
	unread := readAccount("unread")
	unread.FetchedAt = time.Time{}
	tests := []struct {
		name    string
		account status.Account
		pin     []string
		want    RoutingRow
	}{
		{name: "with room", account: readAccount("room", sessionOf(0.1, time.Hour)), want: RoutingRow{Open: true, Pickable: true}},
		{name: "a model family's requests refused alone", account: familyRefused, want: RoutingRow{Open: true, Pickable: true}},
		{name: "not read yet", account: unread, want: RoutingRow{Open: true, Pickable: true}},
		{name: "at its cap, closed in amber, but pickable", account: capped, want: RoutingRow{Reason: "at its cap till 15:12", Capped: true, Pickable: true}},
		{name: "at its cap, which a pin naming it spends", account: capped, pin: []string{"capped"}, want: RoutingRow{Open: true, Pickable: true}},
		{name: "held at its limit", account: limitedAccount("limited"), want: RoutingRow{Reason: "at its limit till 14:12"}},
		{name: "its token refused", account: refused, want: RoutingRow{Reason: "token refused (401)"}},
		{name: "without a token", account: tokenless, want: RoutingRow{Reason: "no token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDoc("", 0, tt.account)
			doc.Pin = status.Pin{Accounts: tt.pin}
			row := RoutingRows(doc, nil, now, claudeLike)[1]
			got := RoutingRow{Open: row.Open, Reason: row.Reason, Capped: row.Capped, Pickable: row.Pickable}
			if got != tt.want {
				t.Errorf("the row is %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAnAccountsFiguresAreItsWindowsAsUsageReadsThem(t *testing.T) {
	refused := readAccount("refused", sessionOf(0.3, time.Hour), weekOf(0.5, 3*day))
	refused.Refused = status.Refusal{Until: now.Add(time.Hour).UTC(), Status: 401}
	tokenless := readAccount("tokenless", sessionOf(0.3, time.Hour))
	tokenless.TokenSet = false
	tests := []struct {
		name          string
		account       status.Account
		session, week Figure
	}{
		{name: "read", account: readAccount("read", sessionOf(0.39, time.Hour), weekOf(0.66, 3*day)), session: Figure{Used: 0.39, Read: true}, week: Figure{Used: 0.66, Read: true}},
		{name: "its week not reported", account: readAccount("short", sessionOf(0.39, time.Hour)), session: Figure{Used: 0.39, Read: true}},
		{name: "its 5-hour reset since it was read", account: readAccount("reset", sessionOf(0.8, -time.Minute), weekOf(0.66, 3*day)), session: Figure{Read: true}, week: Figure{Used: 0.66, Read: true}},
		{name: "as last read, its token refused", account: refused, session: Figure{Used: 0.3, Read: true, Stale: true}, week: Figure{Used: 0.5, Read: true, Stale: true}},
		{name: "without a token, none", account: tokenless},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := RoutingRows(routerDoc("", 0, tt.account), nil, now, claudeLike)[1]
			if row.Session != tt.session || row.Week != tt.week {
				t.Errorf("the 5-hour reads %+v and the week %+v, want %+v and %+v", row.Session, row.Week, tt.session, tt.week)
			}
		})
	}
}

func TestAnAccountNotReadYetIsOpenItsFiguresFaint(t *testing.T) {
	unread := readAccount("unread")
	unread.FetchedAt = time.Time{}
	r := RoutingCard{Rows: RoutingRows(routerDoc("", 0, unread), nil, now, claudeLike)}
	row := r.choice(r.Rows[1], false, 76)[0]
	if got, want := strings.TrimRight(row.plain(), " "), "□ 1 unread      open      5-hour —     week —"; got != want {
		t.Errorf("its row reads %q, want %q", got, want)
	}
	for _, s := range row {
		if s.text == "—" && s.ink != faintInk {
			t.Errorf("its figure — is drawn in %+v, want text.faint", s.ink)
		}
	}
}

func TestAnAccountsSessionsRunningAreThoseSeenOnItWithinTheHour(t *testing.T) {
	seen := func(id, account string, ago time.Duration) status.Session {
		return status.Session{ID: id, Assignments: []status.Assignment{{Model: "claude-opus", Account: account, AssignedAt: now.Add(-2 * time.Hour), LastSeen: now.Add(-ago)}}}
	}
	sessions := []status.Session{
		seen("a1", "work", time.Minute), seen("a2", "work", 59*time.Minute), seen("a3", "work", 61*time.Minute), seen("a4", "side", time.Minute),
		{ID: "a5", Assignments: []status.Assignment{
			{Model: "claude-opus", Account: "work", AssignedAt: now.Add(-2 * time.Hour), LastSeen: now.Add(-time.Minute)},
			{Model: "claude-haiku", Account: "work", AssignedAt: now.Add(-2 * time.Hour), LastSeen: now.Add(-time.Minute)},
		}},
	}
	rows := RoutingRows(routerDoc("", 0, readAccount("work"), readAccount("side")), sessions, now, claudeLike)
	if rows[1].Running != 3 || rows[2].Running != 1 {
		t.Errorf("work runs %d and side %d, want 3 and 1: a session seen within the hour, once however many of its models are there", rows[1].Running, rows[2].Running)
	}
}

func TestTheOutcomeSaysWhatApplyingChangesInSentences(t *testing.T) {
	tests := []struct {
		name string
		card RoutingCard
		want []string
	}{
		{name: "auto", want: []string{"The router chooses an account for each new session."}},
		{name: "one account", card: RoutingCard{Picked: []string{"personal"}}, want: []string{"New sessions go to personal. When it's full, the router chooses."}},
		{name: "two", card: RoutingCard{Picked: []string{"personal", "side"}}, want: []string{"New sessions go to personal or side. When both are full, the router chooses."}},
		{name: "three, wrapped", card: RoutingCard{Picked: []string{"work", "personal", "side"}}, want: []string{
			"New sessions go to work, personal or side. When all are full, the router", "chooses.",
		}},
		{name: "m, the sessions moving and their cost", card: RoutingCard{Picked: []string{"personal"}, Move: true, Moving: 4, Cost: dollars(620)}, want: []string{
			"New sessions go to personal. When it's full, the router chooses.", "The 4 sessions running elsewhere move there too.", "Rebuilding their caches costs ≈ $6.20.",
		}},
		{name: "m, one session moving", card: RoutingCard{Picked: []string{"personal"}, Move: true, Moving: 1, Cost: dollars(156)}, want: []string{
			"New sessions go to personal. When it's full, the router chooses.", "The 1 session running elsewhere moves there too.", "Rebuilding its cache costs ≈ $1.56.",
		}},
		{name: "m, the cost not known", card: RoutingCard{Picked: []string{"personal"}, Move: true, Moving: 4}, want: []string{
			"New sessions go to personal. When it's full, the router chooses.", "The 4 sessions running elsewhere move there too.",
		}},
		{name: "m, costing nothing, every session idle past its cache", card: RoutingCard{Picked: []string{"personal"}, Move: true, Moving: 4, Cost: dollars(0)}, want: []string{
			"New sessions go to personal. When it's full, the router chooses.", "The 4 sessions running elsewhere move there too.",
		}},
		{name: "m, none moving", card: RoutingCard{Picked: []string{"personal"}, Move: true, Cost: dollars(0)}, want: []string{
			"New sessions go to personal. When it's full, the router chooses.",
		}},
		{name: "m, with auto picked, which moves nothing", card: RoutingCard{Move: true, Moving: 4, Cost: dollars(620)}, want: []string{
			"The router chooses an account for each new session.",
		}},
		{name: "refused, the router's reason in its place", card: RoutingCard{Picked: []string{"personal"}, Refused: "no usable token for personal"}, want: []string{
			"no usable token for personal",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.card.Rows = routedCard(nil).Rows
			outcome := tt.card.outcome(76)
			if len(outcome) != outcomeRows {
				t.Errorf("the outcome takes %d rows, want %d whatever it says", len(outcome), outcomeRows)
			}
			if got := plains(outcome); !slices.Equal(got, tt.want) {
				t.Errorf("the outcome reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheOutcomeNamesInBoldItsAccountsCountAndCostAndARefusalInRed(t *testing.T) {
	r := routedCard(func(r *RoutingCard) {
		r.Picked, r.Move, r.Moving, r.Cost = []string{"personal", "side"}, true, 4, dollars(620)
	})
	inked := map[string]ink{}
	for _, l := range r.outcome(76) {
		for _, s := range l {
			inked[s.text] = s.ink
		}
	}
	for text, want := range map[string]ink{"personal": titleInk, "side": titleInk, "4": titleInk, "≈": alertInk, "$6.20": alertInk, "New": secondaryInk, "or": secondaryInk} {
		if got := inked[text]; got != want {
			t.Errorf("%q is drawn in %+v, want %+v", text, got, want)
		}
	}
	r.Refused = "no usable token for personal"
	if got := r.outcome(76)[0][0]; got.ink != errorInk {
		t.Errorf("the router's reason is drawn in %+v, want state.destructive", got.ink)
	}
}

func TestTheTitleAndOpeningLineFollowM(t *testing.T) {
	tests := []struct {
		name           string
		card           RoutingCard
		title, opening string
	}{
		{name: "m clear", card: RoutingCard{Picked: []string{"personal"}}, title: "ROUTING · NEW SESSIONS", opening: "Pick the accounts new sessions go to."},
		{name: "m ticked", card: RoutingCard{Picked: []string{"personal"}, Move: true}, title: "ROUTING · NEW AND RUNNING SESSIONS", opening: "Pick the accounts new and running sessions go to."},
		{name: "m ticked, auto picked", card: RoutingCard{Move: true}, title: "ROUTING · NEW AND RUNNING SESSIONS", opening: "Pick the accounts new and running sessions go to."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := cardText(tt.card, Frame{Width: 160, Height: 37})
			if title != tt.title || body[0] != tt.opening {
				t.Errorf("titled %q, opening %q, want %q and %q", title, body[0], tt.title, tt.opening)
			}
		})
	}
}

func TestNewSessionsCardIs15RowsTallAndOneAnAccount(t *testing.T) {
	for _, accounts := range []int{1, 2, 3, 4} {
		doc := routerDoc("", 0, namedAccounts(accounts)...)
		r := RoutingCard{Rows: RoutingRows(doc, nil, now, claudeLike)}
		if got, want := len(r.card(Frame{Width: 160, Height: 40}).body)+chrome(true), 15+accounts; got != want {
			t.Errorf("with %d accounts, the card is %d rows tall, want %d", accounts, got, want)
		}
	}
}

func TestNoPickMTickOrRefusalMovesTheCard(t *testing.T) {
	refusal := strings.Repeat("the router refused the pin, as no account it names can take a request now; ", 8)
	// says are what the card can be told as it's picked: m clear, and
	// ticked, moving none, one, four and many sessions; or refused.
	says := []RoutingCard{
		{}, {Move: true}, {Move: true, Moving: 1, Cost: dollars(156)}, {Move: true, Moving: 4, Cost: dollars(620)},
		{Move: true, Moving: 999, Cost: dollars(99_999_999)}, {Refused: refusal},
	}
	for _, accounts := range []int{1, 2, 3, 4, 12} {
		doc := routerDoc("", 0, namedAccounts(accounts)...)
		rows := RoutingRows(doc, nil, now, claudeLike)
		for _, width := range []int{52, 160} {
			t.Run(fmt.Sprintf("%d accounts at %d columns", accounts, width), func(t *testing.T) {
				f := Frame{Width: width, Height: 37}
				opened := cardAt(f, RoutingCard{Rows: rows})
				for _, picked := range pickings(doc) {
					for _, said := range says {
						r := said
						r.Rows, r.Picked = rows, picked
						if got := cardAt(f, r); got != opened {
							t.Errorf("picking %q, m %v, moving %d, refused %v, the card sits at %+v, want it where it opened, %+v", picked, r.Move, r.Moving, r.Refused != "", got, opened)
						}
						if lines, room := len(wrappedAt(r.sentences(), f.cardWidth(routingWidth)-2*cardInset)), r.outcomeRoom(f.cardWidth(routingWidth)-2*cardInset); r.Refused == "" && lines > room {
							t.Errorf("picking %q, m %v, moving %d, the outcome takes %d rows, more than the %d kept for it", picked, r.Move, r.Moving, lines, room)
						}
					}
				}
				for cursor := range rows {
					if got := cardAt(f, RoutingCard{Rows: rows, Cursor: cursor}); got != opened {
						t.Errorf("the cursor on row %d, the card sits at %+v, want it where it opened, %+v", cursor+1, got, opened)
					}
				}
			})
		}
	}
}

// placing is where a card sits on the frame: its top row, and how many rows
// it takes.
type placing struct {
	y, height int
}

// cardAt is where the routing card sits on the frame.
func cardAt(f Frame, r RoutingCard) placing {
	k := r.card(f)
	height := len(k.body) + chrome(len(k.keys) > 0)
	return placing{y: (f.Height - height) / 2, height: height}
}

// namedAccounts are n accounts with room, named as accounts are, some
// briefly and some at length, the third held at its limit, so it can't be
// picked.
func namedAccounts(n int) []status.Account {
	names := []string{"work", "personal", "side", "client", "consultancy", "studio", "research-lab", "ops", "an-account-named-at-length", "lab", "team", "spare"}
	accounts := make([]status.Account, n)
	for i := range accounts {
		accounts[i] = readAccount(names[i], sessionOf(0.3, time.Hour), weekOf(0.4, 3*day))
		if i == 2 {
			accounts[i] = limitedAccount(names[i])
		}
	}
	return accounts
}

// pickings are what the card can have picked of doc's accounts, in the
// config's order: of four or fewer, every set of them; of more, none, each
// alone, each pair, all but each, and all.
func pickings(doc status.Document) [][]string {
	ids := make([]string, len(doc.Accounts))
	for i, a := range doc.Accounts {
		ids[i] = a.ID
	}
	var sets [][]string
	for set := range 1 << len(ids) {
		var picked []string
		for i, id := range ids {
			if set&(1<<i) != 0 {
				picked = append(picked, id)
			}
		}
		if n := len(picked); len(ids) <= 4 || n <= 2 || n >= len(ids)-1 {
			sets = append(sets, picked)
		}
	}
	return sets
}

// wrappedAt are the sentences wrapped to width cells, as the outcome wraps
// them.
func wrappedAt(sentences []line, width int) []line {
	var lines []line
	for _, s := range sentences {
		lines = append(lines, s.wrap(width)...)
	}
	return lines
}

func TestARefusalLongerThanItsRoomIsCutToIt(t *testing.T) {
	r := routedCard(func(r *RoutingCard) {
		r.Picked, r.Refused = []string{"personal"}, strings.Repeat("no account it names can take a request now ", 10)
	})
	outcome := plains(r.outcome(76))
	if len(outcome) != r.outcomeRoom(76) || !strings.HasSuffix(outcome[len(outcome)-1], "…") {
		t.Errorf("the refusal reads %q, want it in the %d rows kept for the outcome, the last cut short with …", outcome, r.outcomeRoom(76))
	}
	if !strings.HasPrefix(outcome[0], "no account it names can take a request now no account") {
		t.Errorf("the refusal reads %q, want its words from the first", outcome)
	}
}

func TestARowsMarkSaysWhetherItsPicked(t *testing.T) {
	r := RoutingCard{Picked: []string{"pinned-at-limit"}}
	tests := []struct {
		name string
		row  RoutingRow
		want span
	}{
		{name: "auto, an account picked", row: RoutingRow{Key: "0", Name: "auto", Open: true, Pickable: true}, want: span{"□", dimInk}},
		{name: "an account not picked", row: RoutingRow{Account: "side", Key: "3", Name: "side", Open: true, Pickable: true}, want: span{"□", dimInk}},
		{name: "one that can't be picked, no box", row: RoutingRow{Account: "limited", Key: "2", Name: "limited", Reason: "at its limit"}, want: spaces(1)},
		{name: "one that can't be picked, but is, as a pin set elsewhere picks it", row: RoutingRow{Account: "pinned-at-limit", Key: "1", Name: "pinned-at-limit", Reason: "at its limit"}, want: span{"■", keyInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.choice(tt.row, false, 76)[0][0]; got != tt.want {
				t.Errorf("its mark is %+v, want %+v", got, tt.want)
			}
		})
	}
	if got := (RoutingCard{}).choice(RoutingRow{Key: "0", Name: "auto", Open: true, Pickable: true}, false, 76)[0][0]; got != (span{"■", keyInk}) {
		t.Errorf("auto's mark, nothing picked, is %+v, want it picked", got)
	}
}

func TestTheKeysRowNamesTheNumbersAndKeepsEnterAndEscWhereItsShort(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		width    int
		want     string
	}{
		{name: "three accounts", accounts: 3, width: 160, want: "↑↓ move   space 0-3 tick   m move running   enter apply   esc cancel"},
		{name: "one account", accounts: 1, width: 160, want: "↑↓ move   space 0-1 tick   m move running   enter apply   esc cancel"},
		{name: "past nine accounts", accounts: 12, width: 160, want: "↑↓ move   space 0-9 tick   m move running   enter apply   esc cancel"},
		{name: "a phone with room for every key", accounts: 3, width: 76, want: "↑↓ move   space 0-3 tick   m move running   enter apply   esc cancel"},
		{name: "a phone a cell short, ↑↓ first to go", accounts: 3, width: 75, want: "space 0-3 tick   m move running   enter apply   esc cancel"},
		{name: "a phone with room for the numbers and m", accounts: 3, width: 66, want: "space 0-3 tick   m move running   enter apply   esc cancel"},
		{name: "a phone a cell short, m next, its row naming it", accounts: 3, width: 65, want: "space 0-3 tick   enter apply   esc cancel"},
		{name: "a phone of 52 columns", accounts: 3, width: 52, want: "space 0-3 tick   enter apply   esc cancel"},
		{name: "a phone with room for the numbers alone", accounts: 3, width: 49, want: "space 0-3 tick   enter apply   esc cancel"},
		{name: "a phone a cell short, the numbers last to go", accounts: 3, width: 48, want: "enter apply   esc cancel"},
		{name: "a narrow phone, enter and esc kept", accounts: 3, width: 30, want: "enter apply   esc cancel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RoutingCard{Rows: make([]RoutingRow, tt.accounts+1)}
			f := Frame{Width: tt.width, Height: 40}
			if got := keyRow(r.card(f).keys).plain(); got != tt.want {
				t.Errorf("the keys row reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRowsPastTheCardsRoomScrollTheCursorsKeptInView(t *testing.T) {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = readAccount("a"+strconv.Itoa(i+1), sessionOf(0.1, time.Hour))
	}
	rows := RoutingRows(routerDoc("a1", 0, accounts...), nil, now, claudeLike)
	// At 30 rows, the card's 26 at most leave its rows 12 lines.
	tests := []struct {
		name   string
		cursor int
		// first and end are the rows in view, from first to before end.
		first, end int
	}{
		{name: "the cursor at the top", cursor: 0, first: 0, end: 11},
		{name: "the cursor on the last row in view", cursor: 10, first: 0, end: 11},
		{name: "the cursor past them", cursor: 11, first: 2, end: 12},
		{name: "the cursor at the foot", cursor: 12, first: 2, end: 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 30}
			r := RoutingCard{Rows: rows, Cursor: tt.cursor}
			k := r.card(f)
			if height := len(k.body) + chrome(true); height != f.Height-routingSpare {
				t.Errorf("the card is %d rows tall, want %d, the page's height less 4", height, f.Height-routingSpare)
			}
			if want := (cardScroll{above: tt.first, below: len(rows) - tt.end}); k.scroll != want {
				t.Errorf("scrolled %+v, want %+v", k.scroll, want)
			}
			want := moreOf("▲", tt.first)
			for _, row := range rows[tt.first:tt.end] {
				want = append(want, row.Name)
			}
			want = append(want, moreOf("▼", len(rows)-tt.end)...)
			_, body := cardText(r, f)
			var got []string
			for _, l := range body[2:14] {
				got = append(got, choiceNamed(l))
			}
			if !slices.Equal(got, want) {
				t.Errorf("the rows in view read %q, want %q", got, want)
			}
		})
	}
}

func TestTheRowsInViewKeepTheCursorsAndRoomForWhatsOutOfView(t *testing.T) {
	// As a phone lays them out: auto's row a line, each account's two.
	phone := []int{1, 2, 2, 2, 2}
	tests := []struct {
		name         string
		heights      []int
		cursor, room int
		first, end   int
	}{
		{name: "all of them, where they fit", heights: phone, cursor: 4, room: 9, first: 0, end: 5},
		{name: "from the top, a line kept under them", heights: phone, cursor: 2, room: 6, first: 0, end: 3},
		{name: "up to the cursor, a line kept over them and one under", heights: phone, cursor: 3, room: 6, first: 2, end: 4},
		{name: "up to the cursor at the foot, a line kept over them", heights: phone, cursor: 4, room: 6, first: 3, end: 5},
		{name: "the cursor's alone, where nothing fits", heights: []int{2, 2, 2}, cursor: 2, room: 2, first: 2, end: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if first, end := scrolledTo(tt.heights, tt.cursor, tt.room); first != tt.first || end != tt.end {
				t.Errorf("rows %d to %d in view, want %d to %d", first, end, tt.first, tt.end)
			}
		})
	}
}

// moreOf is the line saying how many rows are out of view, where any are,
// as "▼ 2 more".
func moreOf(arrow string, n int) []string {
	if n == 0 {
		return nil
	}
	return []string{arrow + " " + strconv.Itoa(n) + " more"}
}

// choiceNamed is the name a line of the card's rows gives, as it lays it out
// from its name's column: or the line, where it says how many are out of
// view.
func choiceNamed(l string) string {
	if strings.HasPrefix(l, "▲") || strings.HasPrefix(l, "▼") {
		return l
	}
	return strings.Fields(string([]rune(l)[choiceName+2:]))[0]
}

func TestOnAPhoneAnAccountsWindowsTakeARowOfTheirOwn(t *testing.T) {
	r := routedCard(nil)
	f := Frame{Width: 52, Height: 40}
	if got := r.card(f).width; got != 50 {
		t.Errorf("the card is %d cells wide, want 50, the phone's width less a cell either side", got)
	}
	_, body := cardText(r, f)
	want := []string{
		"Pick the accounts new sessions go to.",
		"",
		"",
		"■ 0 auto        the router chooses",
		"□ 1 work        closed  at its cap till 17:…",
		"                5-hour 95%   week 34%",
		"□ 2 personal    open               2 running",
		"                5-hour 39%   week 66%",
		"□ 3 side        open               4 running",
		"                5-hour 13%   week 78%",
	}
	if got := body[:10]; !slices.Equal(got, want) {
		t.Errorf("the rows read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// routedCard is the routing card of the frames' world, nothing picked, and
// changed as given.
func routedCard(change func(*RoutingCard)) RoutingCard {
	doc, sessions := routedWorld(false)
	r := RoutingCard{Rows: RoutingRows(doc, sessions, routedAt, claudeLike)}
	if change != nil {
		change(&r)
	}
	return r
}

// cardText is the card's title and body as text alone, a line a row,
// right-trimmed, as it's laid out on the frame given.
func cardText(r RoutingCard, f Frame) (string, []string) {
	k := r.card(f)
	return k.title, plains(k.body)
}

// plains are the lines as text alone, each right-trimmed, but for the blank
// ones after the last that says anything.
func plains(lines []line) []string {
	var said []string
	for _, l := range lines {
		said = append(said, strings.TrimRight(l.plain(), " "))
	}
	for len(said) > 0 && said[len(said)-1] == "" {
		said = said[:len(said)-1]
	}
	return said
}
