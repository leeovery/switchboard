package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// cardsPage is Graphs width by height showing WEEK, of the accounts
// configured with the ids given, as text alone, with Runway's lanes over its
// live view of the week built of doc at at, as the watch builds them.
func cardsPage(doc status.Document, at time.Time, width, height int, ids ...string) Frame {
	f := Frame{Width: width, Height: height, View: Graphs, Policy: printPolicy}
	f.Graphs.Accounts = configured(ids...)
	m := doc.At(at, printPolicy)
	f.Graphs.Lanes = make(map[views.RunwayWindow]WindowLanes)
	for _, w := range GraphsWindows(f.Graphs.Sections) {
		tl := views.LiveTimeline(w, at, 136)
		l := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: tl, Window: w})
		f.Graphs.Lanes[w] = WindowLanes{Window: w, Timeline: tl, Lanes: &l}
	}
	return f
}

// trioIDs are the ids of trio's accounts, in the config's order.
var trioIDs = []string{"work", "personal", "side"}

// pageCards is f's Graphs of doc at at drawn whole, as the page lays it out,
// and where it lays WEEK's cards.
func pageCards(f Frame, doc status.Document, at time.Time) (*canvas, []cardLaid) {
	laid := f.graphsLaid()
	c := newCanvas(f.Width, laid.rows)
	f.graphsDrawn(c, f.Moment(doc, at), laid)
	for _, s := range laid.sections {
		if s.section == GraphsWeek {
			return c, laid.cards(s)
		}
	}
	return c, nil
}

// regionOf is the text of c in the bounds given, a row each, trimmed.
func regionOf(c *canvas, b bounds) []string {
	rows := make([]string, b.rows)
	for y := range rows {
		var row strings.Builder
		for x := b.x; x < b.x+b.width; x++ {
			row.WriteString(c.at(x, b.y+y).glyph)
		}
		rows[y] = strings.TrimRight(row.String(), " ")
	}
	return rows
}

// labelOf is the text of the card's label row on c, from its first column
// to its last.
func labelOf(c *canvas, card cardLaid) string {
	return regionOf(c, bounds{x: card.chart.x, y: card.label, width: card.chart.width, rows: 1})[0]
}

// Each configured account has a card in WEEK, in the config's order, where
// the page lays it out: its label, its name and marks, from its first
// column, and its chart and axis as the card of its week draws them, its ✕
// where Runway's lane says its hold began; two across where two fit, one
// where they don't.
func TestGraphsDrawsEveryAccountsCardWhereThePageLaysItOut(t *testing.T) {
	four := trio(func(d *status.Document) { d.Accounts = append(d.Accounts, printAccount("client", 0.2, 0.47, 0)) })
	tests := []struct {
		name          string
		doc           status.Document
		ids           []string
		width, height int
		across        int
	}{
		{name: "160 by 37", doc: trio(), ids: trioIDs, width: 160, height: 37, across: 2},
		{name: "108 by 44", doc: trio(), ids: trioIDs, width: 108, height: 44, across: 2},
		{name: "73 by 61", doc: trio(), ids: trioIDs, width: 73, height: 61, across: 1},
		{name: "52 by 63", doc: trio(), ids: trioIDs, width: 52, height: 63, across: 1},
		{name: "four accounts at 160", doc: four, ids: append(slices.Clone(trioIDs), "client"), width: 160, height: 37, across: 2},
		{name: "one account at 160", doc: trio(), ids: trioIDs[:1], width: 160, height: 37, across: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, tt.width, tt.height, tt.ids...)
			c, cards := pageCards(f, tt.doc, printNow)
			m := tt.doc.At(printNow, printPolicy)
			lanes, key := f.Graphs.Lanes[GraphsWindow], "7d"
			if len(cards) != len(tt.ids) {
				t.Fatalf("WEEK has %d cards, want one an account, %d", len(cards), len(tt.ids))
			}
			var columns []int
			for i, card := range cards {
				if label, want := labelOf(c, card), tt.ids[i]; !labelledAs(label, want) {
					t.Errorf("WEEK's card %d is labelled %q, want it to start %q", i+1, label, want)
				}
				a, _ := m.Account(tt.ids[i])
				l, _ := lanes.lane(a.ID)
				cd := burnCardOf(a, key, Trail{}, printNow)
				cd.heldSince = l.HeldSince(a, key, lanes.Timeline)
				h := card.chart.rows - 1
				if got, want := regionOf(c, card.chart), cardRows(cardIn(cd, Look{}, card.chart.width, h), h); !slices.Equal(got, want) {
					t.Errorf("%s's card's chart is\n%s\nwant\n%s", a.ID, strings.Join(got, "\n"), strings.Join(want, "\n"))
				}
				if card.chart.width != cards[0].chart.width {
					t.Errorf("%s's card is %d wide, want %d, as wide as the first", a.ID, card.chart.width, cards[0].chart.width)
				}
				if !slices.Contains(columns, card.chart.x) {
					columns = append(columns, card.chart.x)
				}
			}
			if len(columns) != tt.across {
				t.Errorf("WEEK's cards are in %d columns, want %d across", len(columns), tt.across)
			}
		})
	}
}

// A card's words are on a row of their own under its label where the page
// has rows to spare for them, as at 160 by 37; else on the label's row,
// ending at the card's last column, as at 108 by 44.
func TestGraphsCardsHeadsTakeTheFormTheStepGives(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		apart         bool
	}{
		{name: "words apart", width: 160, height: 37, apart: true},
		{name: "words beside", width: 108, height: 28},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(trio(), printNow, tt.width, tt.height, trioIDs...)
			c, cards := pageCards(f, trio(), printNow)
			work := cards[0]
			label := labelOf(c, work)
			words := regionOf(c, bounds{x: work.chart.x, y: work.words, width: work.chart.width, rows: 1})[0]
			if tt.apart {
				if label != "work" || work.words != work.label+1 || !strings.HasPrefix(words, "66% left  ·  ") {
					t.Errorf("work's head is %q over %q, want its label alone over its words", label, words)
				}
				return
			}
			if work.words != work.label || !strings.HasPrefix(label, "work  ") || !strings.Contains(label, "  66% left") || len([]rune(label)) != work.chart.width {
				t.Errorf("work's head is %q, want its label then its words, ending at its last column, %d", label, work.chart.width)
			}
		})
	}
}

// chartOnly are the rows of the card's chart on c, its axis left off.
func chartOnly(c *canvas, card cardLaid) []string {
	rows := regionOf(c, card.chart)
	return rows[:len(rows)-1]
}

// emptyButNow reports whether the rows given hold now's line alone, the
// same column down every one.
func emptyButNow(rows []string) bool {
	for _, row := range rows {
		if strings.TrimLeft(row, " ") != nowLine || len(row) != len(rows[0]) {
			return false
		}
	}
	return true
}

// A card whose account has nothing to draw, not read yet, whose first read
// failed, or with no usable token, has its chart empty but for now's line;
// its words as Runway's lane has them, not a stale one's.
func TestGraphsCardWithNothingToDrawIsEmptyButForNowsLine(t *testing.T) {
	tests := []struct {
		name   string
		change func(*status.Account)
		words  string
	}{
		{name: "not read yet", change: func(a *status.Account) { a.FetchedAt = time.Time{} }, words: "not read"},
		{
			name: "its first read failed", words: "can't read it: usage endpoint answered 500",
			change: func(a *status.Account) {
				a.FetchedAt, a.Windows, a.Error = time.Time{}, nil, "usage endpoint answered 500"
			},
		},
		{name: "without a usable token", change: func(a *status.Account) { a.TokenSet = false }, words: "no token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := trio(account("side", tt.change))
			f := cardsPage(doc, printNow, 160, 37, trioIDs...)
			c, cards := pageCards(f, doc, printNow)
			side := cards[2]
			if rows := chartOnly(c, side); !emptyButNow(rows) {
				t.Errorf("side's chart is\n%s\nwant it empty but for now's line", strings.Join(rows, "\n"))
			}
			words := regionOf(c, bounds{x: side.chart.x, y: side.words, width: side.chart.width, rows: 1})[0]
			if words != tt.words {
				t.Errorf("side's words read %q, want %q", words, tt.words)
			}
		})
	}
}

// disagreement is a config over a router's document, as Graphs names its
// accounts: the document; the ids configured, in the config's order; and
// the ids of the accounts the document lacks.
type disagreement struct {
	name   string
	doc    status.Document
	ids    []string
	lacked []string
}

// disagreements are configs over documents that disagree with them: an
// account added, last and among the rest; one removed and one added; two
// added; one removed, still the router's; and ten the router holds,
// configured in another order.
func disagreements() []disagreement {
	return []disagreement{
		{name: "an account added, not yet the router's", doc: trio(), ids: []string{"work", "personal", "side", "client"}, lacked: []string{"client"}},
		{name: "an account added among the rest", doc: trio(), ids: []string{"work", "client", "personal", "side"}, lacked: []string{"client"}},
		{name: "one removed and one added", doc: trio(), ids: []string{"work", "side", "client"}, lacked: []string{"client"}},
		{name: "two added", doc: trio(), ids: []string{"work", "client", "personal", "spare", "side"}, lacked: []string{"client", "spare"}},
		{name: "an account removed, still the router's", doc: trio(), ids: []string{"work", "side"}},
		{name: "ten the router's, in another order", doc: tenHeld(), ids: slices.Concat(tenIDs[5:], tenIDs[:5])},
	}
}

// config is the disagreement's config, as the document marks its primary.
func (n disagreement) config() config.Accounts {
	accounts := configured(n.ids...)
	for i := range accounts {
		accounts[i].Primary = slices.ContainsFunc(n.doc.Accounts, func(a status.Account) bool { return a.ID == accounts[i].ID && a.Primary })
	}
	return accounts
}

// tenIDs are the ids of tenHeld's accounts, in its order.
var tenIDs = []string{"work", "personal", "side", "client", "lab", "team", "ops", "studio", "spare", "research"}

// tenHeld is the router's document of ten accounts, read as trio's are.
func tenHeld() status.Document {
	doc := trio()
	doc.Accounts = nil
	for _, id := range tenIDs {
		doc.Accounts = append(doc.Accounts, printAccount(id, 0.3, 0.4, 0))
	}
	return doc
}

// want is the label the account with the given id takes: as Runway's lanes
// label it, byte for byte, where the document holds it; else its id, as the
// config labels it.
func (n disagreement) want(id string) (label string, lacked bool) {
	if slices.Contains(n.lacked, id) {
		return id, true
	}
	m := n.doc.At(printNow, printPolicy)
	i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.ID == id })
	return namesOf(m)[i].label, false
}

// labelledAs reports whether a label drawn is the one wanted: whole, or
// followed by what comes after it on its row.
func labelledAs(drawn, want string) bool {
	rest, ok := strings.CutPrefix(drawn, want)
	return ok && (rest == "" || strings.HasPrefix(rest, " "))
}

// A card is labelled as Runway's lane labels its account, unnumbered,
// whatever the config's order, as while the router keeps a config the
// dashboard's no longer has. A configured account the moment doesn't hold
// yet, as just after it's added at the config's end, has its card in the
// config's order, its label alone over its chart, empty but for now's line.
func TestGraphsCardsAreLabelledAsRunwayLabelsTheirAccounts(t *testing.T) {
	for _, tt := range disagreements() {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, 160, 37, tt.ids...)
			c, cards := pageCards(f, tt.doc, printNow)
			for i, card := range cards {
				id, label := tt.ids[i], labelOf(c, card)
				want, lacked := tt.want(id)
				if !labelledAs(label, want) || lacked && label != want {
					t.Errorf("%s's card is labelled %q, want %q", id, label, want)
				}
				if rows := chartOnly(c, card); lacked && !emptyButNow(rows) {
					t.Errorf("%s's chart is\n%s\nwant it empty but for now's line", id, strings.Join(rows, "\n"))
				}
			}
		})
	}
}

// The router holding ten accounts, one of them removed from the config,
// Graphs names the nine the config keeps as Runway names them, marks and
// all.
func TestGraphsNamesTheAccountsAsRunwayDoes(t *testing.T) {
	m := tenHeld().At(printNow, printPolicy)
	_, names := graphsNamed(m, configured(tenIDs[:9]...))
	runway := namesOf(m)
	for i, n := range names {
		if n != runway[i] {
			t.Errorf("%s is named %+v, want %+v, as Runway names it", tenIDs[i], n, runway[i])
		}
	}
}

// A stale account's card is faded as a stale reading is everywhere, and one
// the pool leaves out faint, as the Overview draws it, its label among it:
// so side's week, read at half its room since Thursday began, fills the
// column before now's in its ink, faded as either fades it.
func TestGraphsStaleCardIsFadedAndOneOutOfThePoolFaint(t *testing.T) {
	tests := []struct {
		name      string
		doc       status.Document
		faintName bool
	}{
		{name: "stale", doc: trio(account("side", func(a *status.Account) { a.Error = "usage endpoint answered 500" }))},
		{name: "out of the pool", doc: trio(pinnedTo("work", "personal")), faintName: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, 160, 37, trioIDs...)
			f.Trails = Trails{{Account: "side", Window: "7d"}: {Readings: []score.Reading{{At: on(1, 0, 0).UTC(), Utilization: 0.5}}}}
			c, cards := pageCards(f, tt.doc, printNow)
			side := cards[2]
			foot := c.at(side.chart.x+nowColumnOf(c, side)-1, side.chart.y+side.chart.rows-2).ink
			if want := 1 - (1-roomFade)*(1-staleFade); !sameFade(ink{fade: foot.fade}, ink{fade: want}) || !foot.dim {
				t.Errorf("side's room before now is in %+v, want faded %v, dim", foot, want)
			}
			if name := c.at(side.chart.x+2, side.label).ink; (name.token == theme.TextFaint) != tt.faintName {
				t.Errorf("side's label is in %v, want text.faint %v", name.token, tt.faintName)
			}
		})
	}
}

// crossColumn is the column of ✕ in the card's chart on c, from its first,
// reporting false where it has none.
func crossColumn(c *canvas, card cardLaid) (int, bool) {
	for _, row := range chartOnly(c, card) {
		if i := slices.Index([]rune(row), []rune(outMark)[0]); i >= 0 {
			return i, true
		}
	}
	return 0, false
}

// A week its cap holds has its ✕ in the column its hold began, as the
// router's event tells it; none where nothing tells when.
func TestGraphsHeldCardCrossesWhereItsHoldBeganAndNowhereElse(t *testing.T) {
	atCap := account("work", func(a *status.Account) { a.Windows[0].Utilization, a.Windows[1].Utilization = 0.5, 0.95 })
	told := on(1, 14, 31)
	capped := trio(atCap, func(d *status.Document) {
		d.Events = append(d.Events, status.Event{At: told.UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"7d"}, Reserve: 0.05})
	})
	tests := []struct {
		name    string
		doc     status.Document
		crossed bool
	}{
		{name: "told by the router's event", doc: capped, crossed: true},
		{name: "nothing telling when", doc: trio(atCap)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if w, _ := tt.doc.At(printNow, printPolicy).Accounts[0].Window("7d"); w.Kind != status.WindowCapped {
				t.Fatalf("work's week is of kind %d, want held at its cap", w.Kind)
			}
			f := cardsPage(tt.doc, printNow, 160, 37, trioIDs...)
			c, cards := pageCards(f, tt.doc, printNow)
			// The week runs a week to Sun 4 Oct 21:00, across the card's
			// columns: the event's time falls in the column it's told in.
			reset := on(4, 21, 0)
			want := views.Timeline{Start: reset.Add(-7 * 24 * time.Hour), End: reset, Columns: cards[0].chart.width}.Column(told)
			col, crossed := crossColumn(c, cards[0])
			if crossed != tt.crossed || crossed && col != want {
				t.Errorf("work's week has ✕ %v, in column %d; want %v, in column %d", crossed, col, tt.crossed, want)
			}
		})
	}
}

// nowColumnOf is the column of now's line in the card's chart on c, the first
// it's drawn in from its top, -1 where it's in none.
func nowColumnOf(c *canvas, card cardLaid) int {
	for _, row := range chartOnly(c, card) {
		if i := slices.Index([]rune(row), []rune(nowLine)[0]); i >= 0 {
			return i
		}
	}
	return -1
}

// A card's chart moves on with the clock: a column's time later, its now is
// a column on.
func TestGraphsCardsMoveOnWithTheClock(t *testing.T) {
	var columns []int
	// Week cards of 78 columns, a week's 78th each.
	step := 7 * 24 * time.Hour / 78
	for _, at := range []time.Time{printNow, printNow.Add(step)} {
		f := cardsPage(trio(), at, 160, 37, trioIDs...)
		c, cards := pageCards(f, trio(), at)
		if cards[1].chart.width != 78 {
			t.Fatalf("personal's card is %d columns, want 78", cards[1].chart.width)
		}
		columns = append(columns, nowColumnOf(c, cards[1]))
	}
	if columns[0] < 0 || columns[1] != columns[0]+1 {
		t.Errorf("personal's week has now in columns %v, want a column on, a column's time later", columns)
	}
}

// Before the watch builds the lanes, a card has its label alone over its
// chart, and no words.
func TestGraphsCardBeforeItsLanesAreBuiltIsLabelledAlone(t *testing.T) {
	f := cardsPage(trio(), printNow, 160, 37, trioIDs...)
	f.Graphs.Lanes = nil
	c, cards := pageCards(f, trio(), printNow)
	work := cards[0]
	words := regionOf(c, bounds{x: work.chart.x, y: work.words, width: work.chart.width, rows: 1})[0]
	if label := labelOf(c, work); label != "work" || words != "" {
		t.Errorf("work's head is %q over %q, want its label alone", label, words)
	}
	if rows := chartOnly(c, work); !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, nowLine) }) {
		t.Errorf("work's chart is\n%s\nwant it drawn", strings.Join(rows, "\n"))
	}
}

// sideWeek is trio, side's week used as given, resetting 148 hours after
// printNow, past the end of Runway's live week view, side keeping the
// reserve given.
func sideWeek(used, reserve float64) status.Document {
	return trio(account("side", func(a *status.Account) {
		a.Reserve = reserve
		for i := range a.Windows {
			if a.Windows[i].Key == "7d" {
				a.Windows[i].Utilization, a.Windows[i].ResetsAt = used, printNow.Add(148*time.Hour).UTC()
			}
		}
	}))
}

// A card's words say where its window runs out, or reaches its cap, at its
// pace, at the time usage and GLANCE give, wherever that falls before the
// window resets: past the end of Runway's live view of it, where Runway's
// lane says nothing of it, as inside it; never that it has room all week,
// over a chart that runs out. Runway's lane over its view says what it said
// of the same moment: room all week, where the run-out is past its end.
func TestGraphsCardsSayWhereTheirWindowRunsOutPastRunwaysView(t *testing.T) {
	moments := []struct {
		name   string
		doc    status.Document
		says   string
		runway string
	}{
		{name: "running out past the view's end", doc: sideWeek(0.12, 0), says: "out", runway: "room all week"},
		{name: "reaching its cap past the view's end", doc: sideWeek(0.12, 0.01), says: "caps", runway: "room all week"},
		{name: "running out inside the view", doc: sideWeek(0.3, 0), says: "out", runway: "out"},
	}
	sizes := []struct{ width, height int }{{160, 37}, {108, 44}, {73, 61}, {52, 63}}
	for _, mo := range moments {
		m := mo.doc.At(printNow, printPolicy)
		side, _ := m.Account("side")
		w, _ := side.Window("7d")
		if !w.RunsOut {
			t.Fatalf("%s: side's week doesn't run out", mo.name)
		}
		want := mo.says + " ~" + prose.RunOut(printNow, w.Out.At)
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s at %d by %d", mo.name, size.width, size.height), func(t *testing.T) {
				f := cardsPage(mo.doc, printNow, size.width, size.height, trioIDs...)
				c, cards := pageCards(f, mo.doc, printNow)
				card := cards[2]
				words := regionOf(c, bounds{x: card.chart.x, y: card.words, width: card.chart.width, rows: 1})[0]
				if !strings.Contains(words, want) || strings.Contains(words, "room all") {
					t.Errorf("side's week card says %q, want it saying %q", words, want)
				}
				lanes := f.Graphs.Lanes[views.RunwayWeek]
				l, _ := lanes.lane("side")
				runway := wordsRead(lanes.view().wordsUnder(laneLaid(l, side, accountName{}), printNow, 0))
				if !strings.HasPrefix(strings.TrimSpace(runway), mo.runway) {
					t.Errorf("Runway says %q under side's lane, want it saying %q", runway, mo.runway)
				}
			})
		}
	}
}

// A card whose window newly runs out cues its words, as the frame's
// TurnedOut has it, where the run-out falls past the end of Runway's live
// view as where it falls inside it. Runway cues its own words of the same
// lane as it did: where its lane draws the run-out, and not where it says
// room all week.
func TestGraphsCardsCueARunOutPastRunwaysView(t *testing.T) {
	tests := []struct {
		name   string
		used   float64
		runway bool
	}{
		{name: "running out past the view's end", used: 0.12},
		{name: "running out inside the view", used: 0.3, runway: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sideWeek(tt.used, 0)
			f := cardsPage(doc, printNow, 160, 37, trioIDs...)
			f.TurnedOut = map[Ref]float64{{Account: "side", Window: "7d"}: 0.25}
			side, _ := doc.At(printNow, printPolicy).Account("side")
			lanes := f.Graphs.Lanes[views.RunwayWeek]
			l, _ := lanes.lane("side")
			ln := laneLaid(l, side, accountName{label: "side"})
			h := f.cardHeadOf(lanes.view(), ln, 50, false, printNow)
			c := newCanvas(50, 2)
			h.draw(c, Look{coloured: true}, 0, 0)
			if picked := c.at(h.words.x, h.row).ink.on.token == theme.BgAttention; !h.cued || !picked {
				t.Errorf("side's card's words are cued %t, picked out %t, want both", h.cued, picked)
			}
			if _, cued := f.wordsCue(ln, lanes.Timeline.NowColumn()); cued != tt.runway {
				t.Errorf("Runway cues side's words %t, want %t", cued, tt.runway)
			}
		})
	}
}
