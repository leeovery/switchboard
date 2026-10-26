package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// cardsPage is Graphs width by height showing WEEK and 5-HOUR, of the
// accounts configured with the ids given, as text alone, with Runway's lanes
// over its live view of each window built of doc at at, as the watch builds
// them.
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
// and where it lays each burn-down section's cards, by the section.
func pageCards(f Frame, doc status.Document, at time.Time) (*canvas, map[GraphsSection][]cardLaid) {
	laid := f.graphsLaid()
	c := newCanvas(f.Width, laid.rows)
	f.graphsDrawn(c, f.Moment(doc, at), laid)
	cards := make(map[GraphsSection][]cardLaid)
	for _, s := range laid.sections {
		if burnsDown(s.section) {
			cards[s.section] = laid.cards(s)
		}
	}
	return c, cards
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

// burnKeys are the keys of the window each burn-down section charts, in
// printPolicy.
var burnKeys = map[GraphsSection]string{GraphsWeek: "7d", GraphsFiveHour: "5h"}

// Each configured account has a card in WEEK and in 5-HOUR, in the config's
// order, where the page lays it out: its label, its number and name, from its
// first column, and its chart and axis as the card of its window draws them,
// its ✕ where Runway's lane says its hold began; across as many as the width
// takes.
func TestGraphsDrawsEveryAccountsCardWhereThePageLaysItOut(t *testing.T) {
	four := trio(func(d *status.Document) { d.Accounts = append(d.Accounts, printAccount("client", 0.2, 0.47, 0)) })
	tests := []struct {
		name          string
		doc           status.Document
		ids           []string
		width, height int
		across        int
	}{
		{name: "160 by 37", doc: trio(), ids: trioIDs, width: 160, height: 37, across: 3},
		{name: "108 by 44", doc: trio(), ids: trioIDs, width: 108, height: 44, across: 2},
		{name: "73 by 61", doc: trio(), ids: trioIDs, width: 73, height: 61, across: 1},
		{name: "52 by 63", doc: trio(), ids: trioIDs, width: 52, height: 63, across: 1},
		{name: "four accounts at 160", doc: four, ids: append(slices.Clone(trioIDs), "client"), width: 160, height: 37, across: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, tt.width, tt.height, tt.ids...)
			c, sections := pageCards(f, tt.doc, printNow)
			m := tt.doc.At(printNow, printPolicy)
			for section, cards := range sections {
				lanes, key := f.Graphs.Lanes[burnDowns[section]], burnKeys[section]
				if len(cards) != len(tt.ids) {
					t.Fatalf("%s has %d cards, want one an account, %d", section.heading(), len(cards), len(tt.ids))
				}
				var columns []int
				for i, card := range cards {
					if label, want := labelOf(c, card), fmt.Sprintf("%d %s", i+1, tt.ids[i]); !strings.HasPrefix(label, want) {
						t.Errorf("%s's card %d is labelled %q, want it to start %q", section.heading(), i+1, label, want)
					}
					a, _ := m.Account(tt.ids[i])
					l, _ := lanes.lane(a.ID)
					cd := burnCardOf(a, key, Trail{}, printNow)
					cd.heldSince = l.HeldSince(a, key, lanes.Timeline)
					h := card.chart.rows - 1
					if got, want := regionOf(c, card.chart), cardRows(cardIn(cd, Look{}, card.chart.width, h), h); !slices.Equal(got, want) {
						t.Errorf("%s's %s card's chart is\n%s\nwant\n%s", section.heading(), a.ID, strings.Join(got, "\n"), strings.Join(want, "\n"))
					}
					if !slices.Contains(columns, card.chart.x) {
						columns = append(columns, card.chart.x)
					}
				}
				if len(columns) != tt.across {
					t.Errorf("%s's cards are in %d columns, want %d across", section.heading(), len(columns), tt.across)
				}
			}
			if len(sections) != 2 {
				t.Errorf("the page has %d burn-down sections, want WEEK and 5-HOUR", len(sections))
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
		{name: "words beside", width: 108, height: 44},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(trio(), printNow, tt.width, tt.height, trioIDs...)
			c, sections := pageCards(f, trio(), printNow)
			work := sections[GraphsWeek][0]
			label := labelOf(c, work)
			words := regionOf(c, bounds{x: work.chart.x, y: work.words, width: work.chart.width, rows: 1})[0]
			if tt.apart {
				if label != "1 work" || work.words != work.label+1 || !strings.HasPrefix(words, "66% left  ·  ") {
					t.Errorf("work's head is %q over %q, want its label alone over its words", label, words)
				}
				return
			}
			if work.words != work.label || !strings.HasPrefix(label, "1 work  ") || !strings.Contains(label, "  66% left") || len([]rune(label)) != work.chart.width {
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
			c, sections := pageCards(f, doc, printNow)
			for section, cards := range sections {
				side := cards[2]
				if rows := chartOnly(c, side); !emptyButNow(rows) {
					t.Errorf("side's %s chart is\n%s\nwant it empty but for now's line", section.heading(), strings.Join(rows, "\n"))
				}
				words := regionOf(c, bounds{x: side.chart.x, y: side.words, width: side.chart.width, rows: 1})[0]
				if words != tt.words {
					t.Errorf("side's %s words read %q, want %q", section.heading(), words, tt.words)
				}
			}
		})
	}
}

// A card is numbered as Runway's lane numbers its account, by the moment,
// whatever the config's order, as while the router keeps a config the
// dashboard's no longer has: so with personal removed, side is still 3. A
// configured account the moment doesn't hold yet, as just after it's added
// at the config's end, has its card in the config's order, numbered after
// the moment's last place, its label alone over its chart, empty but for
// now's line.
// numbering is a config over a router's document, as Graphs numbers its
// accounts: the document; the ids configured, in the config's order; and
// the label each account the document lacks takes, by its id.
type numbering struct {
	name   string
	doc    status.Document
	ids    []string
	lacked map[string]string
}

// numberings are configs over documents that tell numbering by the moment's
// places from numbering by the config's: an account added, last and among
// the rest; one removed and one added; two added; one removed, still the
// router's; and ten the router holds, configured in another order.
func numberings() []numbering {
	return []numbering{
		{name: "an account added, not yet the router's", doc: trio(), ids: []string{"work", "personal", "side", "client"}, lacked: map[string]string{"client": "4 client"}},
		{name: "an account added among the rest", doc: trio(), ids: []string{"work", "client", "personal", "side"}, lacked: map[string]string{"client": "4 client"}},
		{name: "one removed and one added", doc: trio(), ids: []string{"work", "side", "client"}, lacked: map[string]string{"client": "4 client"}},
		{name: "two added", doc: trio(), ids: []string{"work", "client", "personal", "spare", "side"}, lacked: map[string]string{"client": "4 client", "spare": "5 spare"}},
		{name: "an account removed, still the router's", doc: trio(), ids: []string{"work", "side"}},
		{name: "ten the router's, in another order", doc: tenHeld(), ids: slices.Concat(tenIDs[5:], tenIDs[:5])},
	}
}

// config is the numbering's config, as the document marks its primary.
func (n numbering) config() config.Accounts {
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

// want is the label the account with the given id takes, as n numbers it:
// as Runway's lanes label it, byte for byte, where the document holds it;
// else its number after the document's last.
func (n numbering) want(id string) (label string, lacked bool) {
	if label, ok := n.lacked[id]; ok {
		return label, true
	}
	m := n.doc.At(printNow, printPolicy)
	i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.ID == id })
	named := namesOf(m)[i]
	return named.number + " " + named.label, false
}

// labelledAs reports whether a label drawn is the one wanted: whole, or
// followed by what comes after it on its row.
func labelledAs(drawn, want string) bool {
	rest, ok := strings.CutPrefix(drawn, want)
	return ok && (rest == "" || strings.HasPrefix(rest, " "))
}

func TestGraphsCardsAreNumberedAsRunwayNumbersTheirAccounts(t *testing.T) {
	for _, tt := range numberings() {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, 160, 37, tt.ids...)
			c, sections := pageCards(f, tt.doc, printNow)
			for section, cards := range sections {
				for i, card := range cards {
					id, label := tt.ids[i], labelOf(c, card)
					want, lacked := tt.want(id)
					if !labelledAs(label, want) || lacked && label != want {
						t.Errorf("%s's %s card is labelled %q, want %q", section.heading(), id, label, want)
					}
					if rows := chartOnly(c, card); lacked && !emptyButNow(rows) {
						t.Errorf("%s's %s chart is\n%s\nwant it empty but for now's line", id, section.heading(), strings.Join(rows, "\n"))
					}
				}
			}
		})
	}
}

// The router holding ten accounts, one of them removed from the config,
// Graphs numbers the nine the config keeps as Runway numbers them, aligned
// to the router's ten.
func TestGraphsNumbersAreAlignedAsRunwaysOfAllTheRoutersAccounts(t *testing.T) {
	m := tenHeld().At(printNow, printPolicy)
	_, names := numberedAsRunway(m, configured(tenIDs[:9]...))
	runway := namesOf(m)
	for i, n := range names {
		if n.number != runway[i].number || n.label != runway[i].label {
			t.Errorf("%s is numbered %q, want %q, as Runway numbers it", tenIDs[i], n.number, runway[i].number)
		}
	}
}

// Numbers are right-aligned to the widest of the places given and the
// moment's count, as Runway aligns them: a card numbered after the moment's
// ninth adds a digit, and so does the moment's tenth, though no card is
// numbered past 9.
func TestCardNumbersAreAlignedToTheWidestPlaceOrTheMomentsCount(t *testing.T) {
	accounts := []status.AccountNow{{ID: "work"}, {ID: "client"}}
	tests := []struct {
		places []int
		of     int
		want   []string
	}{
		{places: []int{1, 10}, of: 9, want: []string{" 1", "10"}},
		{places: []int{1, 2}, of: 10, want: []string{" 1", " 2"}},
		{places: []int{1, 2}, of: 3, want: []string{"1", "2"}},
	}
	for _, tt := range tests {
		var numbers []string
		for _, n := range namesAt(accounts, tt.places, tt.of) {
			numbers = append(numbers, n.number)
		}
		if !slices.Equal(numbers, tt.want) {
			t.Errorf("places %v of %d are numbered %q, want %q", tt.places, tt.of, numbers, tt.want)
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
			c, sections := pageCards(f, tt.doc, printNow)
			side := sections[GraphsWeek][2]
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

// A window its cap holds has its ✕ in the column its hold began, as the
// router's event tells it; none where nothing tells when.
func TestGraphsHeldCardCrossesWhereItsHoldBeganAndNowhereElse(t *testing.T) {
	capped := trio(func(d *status.Document) {
		d.Events = append(d.Events, status.Event{At: on(1, 14, 31).UTC(), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05})
	})
	tests := []struct {
		name    string
		doc     status.Document
		crossed bool
	}{
		{name: "told by the router's event", doc: capped, crossed: true},
		{name: "nothing telling when", doc: trio()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cardsPage(tt.doc, printNow, 160, 37, trioIDs...)
			c, sections := pageCards(f, tt.doc, printNow)
			// The 5-hour runs 12:10 to 17:10, 50 columns of 6 minutes: 14:31
			// falls in column 23.
			col, crossed := crossColumn(c, sections[GraphsFiveHour][0])
			if crossed != tt.crossed || crossed && col != 23 {
				t.Errorf("work's 5-hour has ✕ %v, in column %d; want %v, in column 23", crossed, col, tt.crossed)
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
	// 5-hour cards of 50 columns, 6 minutes each.
	for _, at := range []time.Time{printNow, printNow.Add(6 * time.Minute)} {
		f := cardsPage(trio(), at, 160, 37, trioIDs...)
		c, sections := pageCards(f, trio(), at)
		columns = append(columns, nowColumnOf(c, sections[GraphsFiveHour][1]))
	}
	if columns[0] < 0 || columns[1] != columns[0]+1 {
		t.Errorf("personal's 5-hour has now in columns %v, want a column on, 6 minutes later", columns)
	}
}

// Before the watch builds the lanes, a card has its label alone over its
// chart, and no words.
func TestGraphsCardBeforeItsLanesAreBuiltIsLabelledAlone(t *testing.T) {
	f := cardsPage(trio(), printNow, 160, 37, trioIDs...)
	f.Graphs.Lanes = nil
	c, sections := pageCards(f, trio(), printNow)
	work := sections[GraphsWeek][0]
	words := regionOf(c, bounds{x: work.chart.x, y: work.words, width: work.chart.width, rows: 1})[0]
	if label := labelOf(c, work); label != "1 work" || words != "" {
		t.Errorf("work's head is %q over %q, want its label alone", label, words)
	}
	if rows := chartOnly(c, work); !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, nowLine) }) {
		t.Errorf("work's chart is\n%s\nwant it drawn", strings.Join(rows, "\n"))
	}
}
