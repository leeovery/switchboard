package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// cardLane is work's lane, numbered 1, its use of the window keyed "5h" read
// at used, with room through every column from now's where no stretch is
// given, else held back but for the stretches given.
func cardLane(used float64, stretches ...views.LaneStretch) laidLane {
	ln := laneWith(stretches...)
	ln.Use, ln.Room = views.Use{Key: "5h", Used: used, Read: true}, len(stretches) == 0
	ln.name = accountName{number: "1", label: "work"}
	return ln
}

// unusedLane is work's lane, numbered 1, its use not read as given, held
// back but for the stretches given.
func unusedLane(use views.Use, stretches ...views.LaneStretch) laidLane {
	ln := cardLane(0, stretches...)
	ln.Use, ln.Room = use, false
	return ln
}

// spanInks are the inks of the parts' spans, in order, as no number's.
func spanInks(parts []line) []ink {
	var inks []ink
	for _, s := range slices.Concat(parts...) {
		inks = append(inks, s.ink.unnumbered())
	}
	return inks
}

// A card's words are its room, 66% left, the figure in its use's level's
// colour, as usage colours a use, faded as a stale reading's; then the words
// Runway says under its lane in that window's view, after prose.Separator,
// each phrase after the one before alike, inked as Runway inks them. Where
// its use isn't read, the lane's words alone, or what Runway's label says of
// it. Out of the pool, every word is faint, the figure bold.
func TestACardsWordsAreItsRoomThenWhatRunwaySaysUnderItsLane(t *testing.T) {
	five, week := runwayLive(views.RunwayFiveHour), runwayLive(views.RunwayWeek)
	tl := five.Timeline
	capped := stretchOf(tl, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20)
	capped.AsItResets = true
	reaches := stretchOf(tl, views.CellCapped, routedClock(1, 15, 45), routedClock(1, 17, 10), 12, 20)
	reaches.Projected, reaches.AsItResets = true, true
	out := stretchOf(tl, views.CellLimited, routedClock(1, 16, 5), routedClock(1, 16, 55), 14, 18)
	out.Projected, out.AsItResets = true, true
	limited := stretchOf(tl, views.CellLimited, routedClock(1, 13, 58), routedClock(1, 15, 30), 2, 10)
	refused := cardLane(0.34, stretchOf(tl, views.CellRefused, routedClock(1, 14, 12), routedClock(1, 15, 0), 3, 7))
	refused.Use.Stale, refused.account.TokenRefused = true, true
	refused.account.Refusal = status.Refusal{Until: routedClock(1, 15, 0), Status: 401}
	stale := cardLane(0.34)
	stale.Use.Stale = true
	out2 := cardLane(0.34)
	out2.account.OutOfPool = true
	unreadable := unusedLane(views.Use{Key: "5h", Unread: true})
	unreadable.account.Routing = status.Routing{Kind: status.RoutingUnreadable, Says: "can't read it", Clauses: []string{"HTTP 529", "Overloaded"}}
	resets := cardLane(0.34)
	resets.Resets = []views.Reset{{Column: 100, At: routedClock(4, 21, 0)}}
	weekTL := week.Timeline
	monday := stretchOf(weekTL, views.CellLimited, routedClock(5, 6, 0), routedClock(5, 12, 0), weekTL.Column(routedClock(5, 6, 0)), 120)
	monday.Projected = true
	resetFirst := cardLane(0.34, monday)
	resetFirst.Resets = []views.Reset{{Column: weekTL.Column(routedClock(4, 21, 0)), At: routedClock(4, 21, 0)}}
	sunday := stretchOf(weekTL, views.CellLimited, routedClock(4, 21, 6), routedClock(4, 23, 6), 0, 120)
	sunday.Projected = true
	sameColumn := cardLane(0.34, sunday)
	sameColumn.Resets = []views.Reset{{Column: sunday.Column, At: routedClock(4, 19, 58)}}
	ramp1, ramp4 := ink{token: theme.VizRamp1, bold: true}, ink{token: theme.VizRamp4, bold: true}
	tests := []struct {
		name string
		view RunwayShown
		lane laidLane
		want string
		inks []ink
	}{
		{name: "lasting", view: five, lane: cardLane(0.39), want: "61% left  ·  room all day", inks: []ink{ramp1, mutedInk, mutedInk, dimInk}},
		{
			name: "running out", view: five, lane: cardLane(0.66, out),
			want: "34% left  ·  runs out ~16:05 at its pace  ·  back 16:55, as its 5-hour resets", inks: []ink{ramp1, mutedInk, mutedInk, alertInk, mutedInk, mutedInk},
		},
		{
			name: "capped", view: five, lane: cardLane(0.95, capped),
			want: "5% left  ·  at its cap 14:31  ·  back 17:10, as its 5-hour resets", inks: []ink{ramp4, mutedInk, mutedInk, alertInk, mutedInk, mutedInk},
		},
		{
			name: "reaching its reserve at its pace", view: five, lane: cardLane(0.6, reaches),
			want: "40% left  ·  reaches its cap ~15:45 at its pace  ·  back 17:10, as its 5-hour resets", inks: []ink{ramp1, mutedInk, mutedInk, alertInk, mutedInk, mutedInk},
		},
		{
			name: "limited", view: five, lane: cardLane(1, limited),
			want: "0% left  ·  at its limit 13:58  ·  back 15:30", inks: []ink{ramp4, mutedInk, mutedInk, exhaustedInk, mutedInk},
		},
		{
			name: "one hold running into the next", view: five,
			lane: cardLane(1, heldOn(limited, routedClock(1, 17, 10)), stretchOf(tl, views.CellCapped, routedClock(1, 15, 30), routedClock(1, 17, 10), 11, 20)),
			want: "0% left  ·  at its limit 13:58  ·  lifts 15:30  ·  at its cap 15:30  ·  back 17:10",
			inks: []ink{ramp4, mutedInk, mutedInk, exhaustedInk, mutedInk, mutedInk, alertInk, mutedInk},
		},
		{
			name: "its token refused", view: five, lane: refused,
			want: "66% left  ·  token refused (401) 14:12  ·  back 15:00", inks: []ink{dimInk, mutedInk, mutedInk, exhaustedInk, mutedInk},
		},
		{name: "stale", view: five, lane: stale, want: "66% left  ·  room all day", inks: []ink{dimInk, mutedInk, mutedInk, dimInk}},
		{
			name: "out of the pool", view: five, lane: out2,
			want: "66% left  ·  room all day", inks: []ink{{token: theme.TextFaint, bold: true}, faintInk, faintInk, faintInk},
		},
		{
			name: "its week resetting before a stretch, in Runway's order", view: week, lane: resetFirst,
			want: "66% left  ·  resets Sun 21:00  ·  runs out ~Mon 06:00 at its pace  ·  back Mon 12:00",
			inks: []ink{ramp1, mutedInk, mutedInk, mutedInk, mutedInk, alertInk, mutedInk},
		},
		{
			name: "its week resetting in the column a later stretch starts in", view: week, lane: sameColumn,
			want: "66% left  ·  resets Sun 19:58  ·  runs out ~Sun 21:06 at its pace  ·  back Sun 23:06",
			inks: []ink{ramp1, mutedInk, mutedInk, mutedInk, mutedInk, alertInk, mutedInk},
		},
		{name: "the week resetting", view: week, lane: resets, want: "66% left  ·  resets Sun 21:00", inks: []ink{ramp1, mutedInk, mutedInk, mutedInk}},
		{name: "not read", view: five, lane: unusedLane(views.Use{Key: "5h", Unread: true}), want: "not read", inks: []ink{dimInk}},
		{name: "its first read failed", view: five, lane: unreadable, want: "can't read it: HTTP 529 · Overloaded", inks: []ink{dimInk}},
		{
			name: "without a token", view: five, lane: unusedLane(views.Use{Tokenless: true}, stretchOf(tl, views.CellTokenless, time.Time{}, time.Time{}, 0, 135)),
			want: "no token", inks: []ink{exhaustedInk},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := tt.view.view().cardWords(tt.lane, routedAt)
			if got := slices.Concat(words...).plain(); got != tt.want {
				t.Fatalf("the card's words say %q, want %q", got, tt.want)
			}
			if got := spanInks(words); !slices.Equal(got, tt.inks) {
				t.Errorf("the card's words are in %v, want %v", got, tt.inks)
			}
		})
	}
}

// A card says what Runway says under its account's lane in that window's
// live view: of each live Runway moment's lanes, as Runway's frame holds
// them, taken as a card's, every phrase Runway places under each lane, in
// its order.
func TestACardSaysWhatRunwaySaysUnderItsLane(t *testing.T) {
	for _, rm := range viewstest.RunwayMoments() {
		if !rm.Day.IsZero() {
			continue
		}
		t.Run(rm.Name, func(t *testing.T) {
			f, doc, now := runwayFramed(t, rm.Name)
			m := doc.At(now, claude.Policy())
			lanes := WindowLanes{Window: f.Runway.Window, Timeline: f.Runway.Timeline, Lanes: f.Runway.Lanes}
			for _, ln := range f.laidLanes(m, runwayFit{fits: true, shown: len(m.Accounts)}) {
				card := slices.Concat(lanes.view().cardWords(ln, now)...).plain()
				rest := card
				for _, w := range f.Runway.view().wordsUnder(ln, now) {
					said := w.said.plain()
					i := strings.Index(rest, said)
					if i < 0 {
						t.Errorf("%s's card says %q, want it saying %q, as Runway does, after what it said before", ln.ID, card, said)
						break
					}
					rest = rest[i+len(said):]
				}
			}
		})
	}
}

// A card's room is what Runway's use of the window, as its label says it,
// leaves of 100%: the two sum to it at a half-percent as anywhere, and past
// its limit the room is none, never below.
func TestACardsRoomIsWhatRunwaysUseLeaves(t *testing.T) {
	for _, used := range []float64{0.125, 0.345, 0.685, 0.995, 1, 1.02} {
		ln := cardLane(used)
		room, use := ln.roomLeft()[0].text, ln.useFigure().text
		left, _ := strconv.Atoi(strings.TrimSuffix(room, "%"))
		of, _ := strconv.Atoi(strings.TrimSuffix(use, "%"))
		if want := max(100-of, 0); left != want {
			t.Errorf("used %v, Runway says %s and the card %s left, want %d%% left", used, use, room, want)
		}
	}
}

// A card's room is numbered as the room left in its window, to brighten as
// it changes.
func TestACardsRoomIsNumberedAsItsWindowsRoom(t *testing.T) {
	words := runwayLive(views.RunwayFiveHour).view().cardWords(cardLane(0.34), routedAt)
	want := ink{}.numbered(Number{Kind: NumberRoom, Account: "work", Of: "5h"}).mark
	if got := words[0][0].ink.mark; got != want {
		t.Errorf("the room is marked %v, want as work's 5h room, %v", got, want)
	}
}

// A card's head has its label on its first row, as Runway's lanes label an
// account, and its words, whole parts left off their end where they don't
// fit: on a row of their own under it, from its first column; or beside it,
// on the label's row, ending at the card's last column, two cells clear of
// the label at the least, the label cut toward its least only once the
// words are down to their first part, and the words gone where even then
// they don't fit, the label then alone on its row, as under them. A label
// cut on a row of its own keeps its number and its marks whole within the
// card.
func TestACardsHeadLaysItsWordsUnderItsLabelOrBesideIt(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	capped := stretchOf(five.Timeline, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 5, 20)
	capped.AsItResets = true
	work := cardLane(0.95, capped)
	research := work
	research.name = accountName{number: "2", label: "client-research", primary: true}
	marked := work
	marked.name = accountName{number: "3", label: "someone", primary: true, best: true}
	unreadable := unusedLane(views.Use{Key: "5h", Unread: true})
	unreadable.name = accountName{number: "3", label: "someone@example.com"}
	unreadable.account.Routing = status.Routing{Kind: status.RoutingUnreadable, Says: "can't read it", Clauses: []string{"HTTP 529", "Overloaded"}}
	tests := []struct {
		name   string
		lane   laidLane
		width  int
		beside bool
		label  string
		words  string
	}{
		{name: "under, every part", lane: work, width: 65, label: "1 work", words: "5% left  ·  at its cap 14:31  ·  back 17:10, as its 5-hour resets"},
		{name: "under, the last part left off", lane: work, width: 64, label: "1 work", words: "5% left  ·  at its cap 14:31  ·  back 17:10"},
		{name: "under, two left off", lane: work, width: 42, label: "1 work", words: "5% left  ·  at its cap 14:31"},
		{name: "under, the room alone", lane: work, width: 27, label: "1 work", words: "5% left"},
		{name: "under, the label whole in its own row", lane: work, width: 9, label: "1 work", words: "5% left"},
		{name: "under, the label cut", lane: work, width: 5, label: "1 wo…", words: "5% l…"},
		{name: "under, the room cut", lane: work, width: 6, label: "1 work", words: "5% le…"},
		{name: "under, a marked label cut", lane: research, width: 12, label: "2 client-… ◆", words: "5% left"},
		{name: "under, a label with both marks cut", lane: marked, width: 10, label: "3 som… ◆ ▲", words: "5% left"},
		{name: "beside, every part", lane: work, width: 75, beside: true, label: "1 work", words: "5% left  ·  at its cap 14:31  ·  back 17:10, as its 5-hour resets"},
		{name: "beside, the last part left off", lane: work, width: 74, beside: true, label: "1 work", words: "5% left  ·  at its cap 14:31  ·  back 17:10"},
		{name: "beside, two left off", lane: work, width: 52, beside: true, label: "1 work", words: "5% left  ·  at its cap 14:31"},
		{name: "beside, the room alone", lane: work, width: 37, beside: true, label: "1 work", words: "5% left"},
		{name: "beside, the room alone at its least", lane: work, width: 17, beside: true, label: "1 work", words: "5% left"},
		{name: "beside, a long label whole", lane: research, width: 30, beside: true, label: "2 client-research ◆", words: "5% left"},
		{name: "beside, a long label cut", lane: research, width: 28, beside: true, label: "2 client-resea… ◆", words: "5% left"},
		{name: "beside, a long label at its least", lane: research, width: 23, beside: true, label: "2 client-… ◆", words: "5% left"},
		{name: "beside, the words gone, the label whole on its row", lane: research, width: 22, beside: true, label: "2 client-research ◆"},
		{name: "beside, the words gone, the label whole again", lane: unreadable, width: 37, beside: true, label: "3 someone@example.com"},
		{name: "beside, the words gone, the label cut on its row", lane: research, width: 12, beside: true, label: "2 client-… ◆"},
		{name: "beside, the words gone, the label just whole on its row", lane: research, width: 19, beside: true, label: "2 client-research ◆"},
		{name: "beside, a long label beside long words", lane: unreadable, width: 62, beside: true, label: "3 someone@example.com", words: "can't read it: HTTP 529 · Overloaded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Frame{}.cardHeadOf(five.view(), tt.lane, tt.width, tt.beside, routedAt)
			if got := h.label.plain(); got != tt.label {
				t.Errorf("its label reads %q, want %q", got, tt.label)
			}
			said := h.words.said.plain()
			if said != tt.words {
				t.Errorf("its words read %q, want %q", said, tt.words)
			}
			wantRow, from := 1, 0
			if tt.beside {
				wantRow, from = 0, tt.width-h.words.said.width()
			}
			if h.row != wantRow || h.words.x != from {
				t.Errorf("its words are on row %d from column %d, want row %d from column %d", h.row, h.words.x, wantRow, from)
			}
			if tt.beside && said != "" && h.words.x < h.label.width()+2 {
				t.Errorf("its words start at column %d, its label ending at %d, want them two clear of it", h.words.x, h.label.width())
			}
		})
	}
}

// A card's head is drawn from where it's put: its label on its first row,
// its words on theirs, picked out by the cue while its account newly stands
// as its lane has it at now, as Runway cues the words under its lane.
func TestACardsHeadIsDrawnItsWordsCuedAsRunwayCuesThem(t *testing.T) {
	five := runwayLive(views.RunwayFiveHour)
	capped := cardLane(0.95, stretchOf(five.Timeline, views.CellCapped, routedClock(1, 14, 31), routedClock(1, 17, 10), 0, 20))
	capped.Cells = slices.Repeat([]views.Cell{views.CellCapped}, 136)
	tests := []struct {
		name    string
		changed map[string]float64
		beside  bool
		cued    bool
	}{
		{name: "under, newly at its cap", changed: map[string]float64{"work": 0.25}, cued: true},
		{name: "beside, newly at its cap", changed: map[string]float64{"work": 0.25}, beside: true, cued: true},
		{name: "under, at its cap a while"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Frame{Changed: tt.changed}.cardHeadOf(five.view(), capped, 50, tt.beside, routedAt)
			c := newCanvas(54, 3)
			h.draw(c, Look{coloured: true}, 2, 1)
			label, words := rowOf(c, 1), rowOf(c, 1+h.row)
			if label[:8] != "  1 work" {
				t.Errorf("the label's row reads %q, want the label from column 2", label)
			}
			wantWords := "  5% left  ·  at its cap 14:31  ·  back 17:10"
			if tt.beside {
				wantWords = "  1 work                5% left  ·  at its cap 14:31"
			}
			if words != wantWords {
				t.Errorf("the words' row reads %q, want %q", words, wantWords)
			}
			if got := c.at(2+h.words.x, 1+h.row).ink.on.token == theme.BgAttention; got != tt.cued || h.cued != tt.cued {
				t.Errorf("its words are cued %t, picked out %t, want %t", h.cued, got, tt.cued)
			}
		})
	}
}
