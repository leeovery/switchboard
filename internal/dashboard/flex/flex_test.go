package flex_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
)

func TestLay(t *testing.T) {
	tests := []struct {
		name  string
		room  int
		parts []flex.Part
		want  []flex.Span
		fits  bool
	}{
		{
			name: "parts lie in turn, a gap before each but the first",
			room: 20,
			parts: []flex.Part{
				{Least: 4, Natural: 4, Gap: 3},
				{Least: 6, Natural: 6, Gap: 1},
				{Least: 3, Natural: 3, Gap: 2},
			},
			want: []flex.Span{{At: 0, Size: 4}, {At: 5, Size: 6}, {At: 13, Size: 3}},
			fits: true,
		},
		{
			name:  "a natural past the most is held at the most",
			room:  40,
			parts: []flex.Part{{Least: 2, Natural: 30, Most: 10}},
			want:  []flex.Span{{At: 0, Size: 10}},
			fits:  true,
		},
		{
			name:  "a natural under the least is held at the least",
			room:  10,
			parts: []flex.Part{{Least: 8, Natural: 3}},
			want:  []flex.Span{{At: 0, Size: 8}},
			fits:  true,
		},
		{
			name:  "a most under the least is held at the least",
			room:  20,
			parts: []flex.Part{{Least: 6, Natural: 8, Most: 3, Grows: true}},
			want:  []flex.Span{{At: 0, Size: 6}},
			fits:  true,
		},
		{
			name: "a least under 0 counts as 0",
			room: 4,
			parts: []flex.Part{
				{Least: -3, Natural: 4},
				{Least: 5, Natural: 5, Gap: 1},
			},
			want: []flex.Span{{At: 0}, {At: 1, Size: 5}},
		},
		{
			name:  "a most under 0 counts as none",
			room:  5,
			parts: []flex.Part{{Most: -3, Grows: true}},
			want:  []flex.Span{{At: 0, Size: 5}},
			fits:  true,
		},
		{
			name: "a turn to narrow in under 0 counts as narrowing freely",
			room: 5,
			parts: []flex.Part{
				{Least: 2, Natural: 6, Narrows: -1},
				{Least: 1, Natural: 1, Grows: true, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 3}, {At: 4, Size: 1}},
			fits: true,
		},
		{
			name: "a Kept under 0 counts as keeping nothing",
			room: 3,
			parts: []flex.Part{
				{Least: 4, Natural: 4, Leaves: 1, Kept: -2},
				{Least: 3, Natural: 3, Gap: 1},
			},
			want: []flex.Span{{At: 0, Left: true}, {At: 0, Size: 3}},
			fits: true,
		},
		{
			name: "a gap under 0 counts as none",
			room: 10,
			parts: []flex.Part{
				{Least: 2, Natural: 2},
				{Least: 3, Natural: 3, Gap: -2},
			},
			want: []flex.Span{{At: 0, Size: 2}, {At: 2, Size: 3}},
			fits: true,
		},
		{
			name: "names are raised toward their naturals before bars",
			room: 25,
			parts: []flex.Part{
				{Least: 8, Natural: 20},
				{Least: 10, Natural: 30, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 14}, {At: 15, Size: 10}},
			fits: true,
		},
		{
			name: "a part that grows reaches its most before one with no most passes its natural",
			room: 50,
			parts: []flex.Part{
				{Least: 8, Natural: 12, Most: 24, Grows: true},
				{Least: 10, Natural: 10, Grows: true, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 24}, {At: 25, Size: 25}},
			fits: true,
		},
		{
			name:  "parts that grow share evenly, the odd cell to the first",
			room:  5,
			parts: []flex.Part{{Grows: true}, {Grows: true}},
			want:  []flex.Span{{At: 0, Size: 3}, {At: 3, Size: 2}},
			fits:  true,
		},
		{
			name:  "the odd cell to the part marked Odd",
			room:  5,
			parts: []flex.Part{{Grows: true}, {Grows: true, Odd: true}},
			want:  []flex.Span{{At: 0, Size: 2}, {At: 2, Size: 3}},
			fits:  true,
		},
		{
			name:  "parts that grow to a most share evenly up to it, before those with none",
			room:  15,
			parts: []flex.Part{{Most: 2, Grows: true}, {Most: 10, Grows: true}, {Grows: true}},
			want:  []flex.Span{{At: 0, Size: 2}, {At: 2, Size: 10}, {At: 12, Size: 3}},
			fits:  true,
		},
		{
			name: "a part gives way in its turn, to its Kept, where the room lacks",
			room: 11,
			parts: []flex.Part{
				{Least: 5, Natural: 5, Leaves: 2, Kept: 1},
				{Least: 4, Natural: 4, Leaves: 1, Kept: 1, Gap: 1},
				{Least: 3, Natural: 3, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 5}, {At: 6, Size: 1, Left: true}, {At: 8, Size: 3}},
			fits: true,
		},
		{
			name: "the next turn is taken only where the last left the room lacking",
			room: 10,
			parts: []flex.Part{
				{Least: 5, Natural: 5, Leaves: 2, Kept: 1},
				{Least: 4, Natural: 4, Leaves: 1, Kept: 1, Gap: 1},
				{Least: 3, Natural: 3, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 1, Left: true}, {At: 2, Size: 1, Left: true}, {At: 4, Size: 3}},
			fits: true,
		},
		{
			name: "parts that share a turn give way together, and the first part still shown takes no gap",
			room: 12,
			parts: []flex.Part{
				{Least: 5, Natural: 5, Leaves: 1},
				{Least: 5, Natural: 5, Leaves: 1, Gap: 1},
				{Least: 5, Natural: 5, Gap: 1},
			},
			want: []flex.Span{{At: 0, Left: true}, {At: 0, Left: true}, {At: 0, Size: 5}},
			fits: true,
		},
		{
			name: "a part that gives way keeping nothing takes its gap with it",
			room: 10,
			parts: []flex.Part{
				{Least: 4, Natural: 4},
				{Least: 6, Natural: 6, Leaves: 1, Gap: 2},
				{Least: 3, Natural: 3, Gap: 2},
			},
			want: []flex.Span{{At: 0, Size: 4}, {At: 4, Left: true}, {At: 6, Size: 3}},
			fits: true,
		},
		{
			name: "a part that narrows in a turn holds its natural till then",
			room: 25,
			parts: []flex.Part{
				{Least: 6, Natural: 12, Narrows: 1},
				{Least: 10, Natural: 10, Grows: true, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 12}, {At: 13, Size: 12}},
			fits: true,
		},
		{
			name: "a part narrows in its turn as far as the room lacks",
			room: 20,
			parts: []flex.Part{
				{Least: 6, Natural: 12, Narrows: 1},
				{Least: 10, Natural: 10, Grows: true, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 9}, {At: 10, Size: 10}},
			fits: true,
		},
		{
			name: "parts that share a turn to narrow in narrow the last first",
			room: 15,
			parts: []flex.Part{
				{Least: 6, Natural: 12, Narrows: 1},
				{Least: 4, Natural: 8, Narrows: 1, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 10}, {At: 11, Size: 4}},
			fits: true,
		},
		{
			name: "a Kept past the least is held at the least",
			room: 6,
			parts: []flex.Part{
				{Least: 3, Natural: 3},
				{Least: 4, Natural: 4, Leaves: 1, Kept: 9, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 3}, {At: 4, Size: 4, Left: true}},
		},
		{
			name: "a part that narrows in a turn takes none of what's spare",
			room: 30,
			parts: []flex.Part{
				{Least: 8, Natural: 16, Most: 32, Grows: true, Narrows: 1},
				{Least: 4, Natural: 4, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 16}, {At: 17, Size: 4}},
			fits: true,
		},
		{
			name: "what a part frees at once goes to the free parts, not back to one narrowed before it",
			room: 23,
			parts: []flex.Part{
				{Least: 6, Natural: 12, Narrows: 1},
				{Least: 7, Natural: 7, Leaves: 2, Kept: 2, Gap: 1},
				{Least: 9, Natural: 9, Grows: true, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 6}, {At: 7, Size: 2, Left: true}, {At: 10, Size: 13}},
			fits: true,
		},
		{
			name: "what a part frees at once raises a free part toward its natural first",
			room: 43,
			parts: []flex.Part{
				{Least: 10, Natural: 10},
				{Least: 6, Natural: 6, Leaves: 1, Gap: 2},
				{Least: 24, Natural: 30, Grows: true, Gap: 2},
			},
			want: []flex.Span{{At: 0, Size: 10}, {At: 10, Left: true}, {At: 12, Size: 31}},
			fits: true,
		},
		{
			name: "where even every turn taken doesn't fit, the spans lie as the turns left them",
			room: 8,
			parts: []flex.Part{
				{Least: 5, Natural: 9},
				{Least: 4, Natural: 4, Leaves: 1, Kept: 1, Gap: 1},
				{Least: 3, Natural: 6, Narrows: 2, Gap: 1},
			},
			want: []flex.Span{{At: 0, Size: 5}, {At: 6, Size: 1, Left: true}, {At: 8, Size: 3}},
		},
		{
			name: "nothing laid out fits",
			want: []flex.Span{},
			fits: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, fits := flex.Lay(tt.room, tt.parts...)
			if !slices.Equal(got, tt.want) || fits != tt.fits {
				t.Errorf("Lay(%d) = %+v, %v; want %+v, %v", tt.room, got, fits, tt.want, tt.fits)
			}
		})
	}
}

// An example row's parts, in switchboard's order.
const (
	callIDPart = iota
	callDirPart
	callDoingPart
	callFromPart
	callAccountPart
	bendsPart
	toLinesPart
	barsPastPart
	barsPart
	wordsPart
)

// switchboard is an example row of parts, shaped as the Switchboard's once
// were: a call's id, its directory, what it's doing, ▸'s from and the
// account; the bends' room, after the plugs' gap; the jacks, the gap to the
// lines and the lines' label and percentage; the lines' bars past 16 cells,
// and their first 16; and the lines' words, or usage's short form. Its
// turns interleave narrowing, leaving and a part that grows.
var switchboard = []flex.Part{
	callIDPart:      {Least: 4, Natural: 4},
	callDirPart:     {Least: 6, Natural: 12, Narrows: 2, Gap: 1},
	callDoingPart:   {Least: 13, Natural: 13, Gap: 1},
	callFromPart:    {Least: 7, Natural: 7, Leaves: 3, Kept: 2},
	callAccountPart: {Least: 6, Natural: 8, Narrows: 6},
	bendsPart:       {Least: 9, Natural: 9, Grows: true, Gap: 2},
	toLinesPart:     {Least: 34, Natural: 34},
	barsPastPart:    {Natural: 16, Narrows: 1},
	barsPart:        {Least: 8, Natural: 16, Narrows: 5},
	wordsPart:       {Least: 22, Natural: 22, Leaves: 4, Kept: 14},
}

// Lay takes an example row's turns in order, each only while the parts pass
// the room, and gives what a leaving frees past what's lacking to the part
// that grows.
func TestLayTakesAnExampleRowsTurnsInOrder(t *testing.T) {
	type columns struct {
		room, bars, dir, account int
		shortFrom, shortWords    bool
	}
	tests := []struct {
		width int
		want  columns
		fits  bool
	}{
		{width: 160, want: columns{room: 24, bars: 32, dir: 12, account: 8}, fits: true},
		{width: 159, want: columns{room: 23, bars: 32, dir: 12, account: 8}, fits: true},
		{width: 145, want: columns{room: 9, bars: 32, dir: 12, account: 8}, fits: true},
		{width: 144, want: columns{room: 9, bars: 31, dir: 12, account: 8}, fits: true},
		{width: 129, want: columns{room: 9, bars: 16, dir: 12, account: 8}, fits: true},
		{width: 128, want: columns{room: 9, bars: 16, dir: 11, account: 8}, fits: true},
		{width: 123, want: columns{room: 9, bars: 16, dir: 6, account: 8}, fits: true},
		{width: 122, want: columns{room: 13, bars: 16, dir: 6, account: 8, shortFrom: true}, fits: true},
		{width: 121, want: columns{room: 12, bars: 16, dir: 6, account: 8, shortFrom: true}, fits: true},
		{width: 118, want: columns{room: 9, bars: 16, dir: 6, account: 8, shortFrom: true}, fits: true},
		{width: 117, want: columns{room: 16, bars: 16, dir: 6, account: 8, shortFrom: true, shortWords: true}, fits: true},
		{width: 116, want: columns{room: 15, bars: 16, dir: 6, account: 8, shortFrom: true, shortWords: true}, fits: true},
		{width: 110, want: columns{room: 9, bars: 16, dir: 6, account: 8, shortFrom: true, shortWords: true}, fits: true},
		{width: 109, want: columns{room: 9, bars: 15, dir: 6, account: 8, shortFrom: true, shortWords: true}, fits: true},
		{width: 102, want: columns{room: 9, bars: 8, dir: 6, account: 8, shortFrom: true, shortWords: true}, fits: true},
		{width: 101, want: columns{room: 9, bars: 8, dir: 6, account: 7, shortFrom: true, shortWords: true}, fits: true},
		{width: 100, want: columns{room: 9, bars: 8, dir: 6, account: 6, shortFrom: true, shortWords: true}, fits: true},
		{width: 99, want: columns{room: 9, bars: 8, dir: 6, account: 6, shortFrom: true, shortWords: true}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			spans, fits := flex.Lay(tt.width, switchboard...)
			got := columns{
				room:       spans[bendsPart].Size,
				bars:       spans[barsPastPart].Size + spans[barsPart].Size,
				dir:        spans[callDirPart].Size,
				account:    spans[callAccountPart].Size,
				shortFrom:  spans[callFromPart].Left,
				shortWords: spans[wordsPart].Left,
			}
			if got != tt.want || fits != tt.fits {
				t.Errorf("at %d columns: %+v, %v; want %+v, %v", tt.width, got, fits, tt.want, tt.fits)
			}
		})
	}
}

// The Log's tail's parts, in logTail's order.
const (
	beforePart = iota
	extraPart
	stoppedPart
	routingPart
	shortPart
)

// logTail are the Log's Requests' last columns as its spread has them: the
// columns before, from its left; an extra, which leaves first; stopped;
// routing, cut to 24 cells before it folds into stopped, and taking the odd
// cell where the two share; and the cells short of the last column it ends,
// once whole, which fold with it.
var logTail = []flex.Part{
	beforePart:  {Least: 100, Natural: 100},
	extraPart:   {Least: 6, Natural: 6, Leaves: 1, Gap: 2},
	stoppedPart: {Least: 27, Natural: 27, Grows: true, Gap: 2},
	routingPart: {Least: 24, Natural: 30, Grows: true, Leaves: 2, Odd: true, Gap: 2},
	shortPart:   {Most: 2, Grows: true, Leaves: 2},
}

func TestLaySharesTheLogsTailAsItsSpreadDoes(t *testing.T) {
	// Each row's spans are logTail's parts', in its order: the columns before,
	// the extra, stopped, routing and the cells short.
	tests := []struct {
		width int
		want  []flex.Span
		fits  bool
	}{
		{width: 200, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 41}, {At: 153, Size: 45}, {At: 198, Size: 2}}, fits: true},
		{width: 199, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 41}, {At: 153, Size: 44}, {At: 197, Size: 2}}, fits: true},
		{width: 171, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 27}, {At: 139, Size: 30}, {At: 169, Size: 2}}, fits: true},
		{width: 170, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 27}, {At: 139, Size: 30}, {At: 169, Size: 1}}, fits: true},
		{width: 168, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 27}, {At: 139, Size: 29}, {At: 168}}, fits: true},
		{width: 163, want: []flex.Span{{At: 0, Size: 100}, {At: 102, Size: 6}, {At: 110, Size: 27}, {At: 139, Size: 24}, {At: 163}}, fits: true},
		{width: 162, want: []flex.Span{{At: 0, Size: 100}, {At: 100, Left: true}, {At: 102, Size: 27}, {At: 131, Size: 30}, {At: 161, Size: 1}}, fits: true},
		{width: 154, want: []flex.Span{{At: 0, Size: 100}, {At: 100, Left: true}, {At: 102, Size: 52}, {At: 154, Left: true}, {At: 154, Left: true}}, fits: true},
		{width: 128, want: []flex.Span{{At: 0, Size: 100}, {At: 100, Left: true}, {At: 102, Size: 27}, {At: 129, Left: true}, {At: 129, Left: true}}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			got, fits := flex.Lay(tt.width, logTail...)
			if !slices.Equal(got, tt.want) || fits != tt.fits {
				t.Errorf("at %d columns:\n got %+v, %v\nwant %+v, %v", tt.width, got, fits, tt.want, tt.fits)
			}
		})
	}
}

// laid is what Lay made of the parts given in room: the parts as it holds
// them, their spans, and whether they fit.
type laid struct {
	given, parts []flex.Part
	room         int
	spans        []flex.Span
	fits         bool
}

// layRules are what Lay holds to, whatever it's given, each saying how a
// layout breaks it, or nothing.
var layRules = []struct {
	name  string
	broke func(laid) string
}{
	{"each span within its least and its most, or at its Kept where it gave way", outOfBounds},
	{"gaps only before the parts shown, and none before the first", misplaced},
	{"false only where, with every turn taken, the parts don't fit", fitsWrongly},
	{"the spans and gaps fill the room wherever a free part grows with no most", underfilled},
	{"the free parts reach their naturals, in their order, before any grows", raisedOutOfOrder},
	{"a growing part reaches its most before one with no most passes its natural", grewOutOfOrder},
	{"parts that grow share evenly, the odd cell to one marked Odd, else the first", sharedUnevenly},
	{"parts narrow and give way in their turns, where the room lacks and no sooner", gaveWayWrongly},
	{"more room never takes more away", tookMoreWithMore},
}

// A part a first pass decided on is laid again by a second: gone where it
// gave way keeping nothing; fixed at what it kept, its gap kept, where it
// gave way; and any other as it was, giving way and narrowing no more.
func TestAPartDecidedOnIsLaidAgain(t *testing.T) {
	part := flex.Part{Least: 4, Natural: 12, Most: 20, Grows: true, Narrows: 2, Leaves: 3, Kept: 4, Gap: 2, Odd: true}
	tests := []struct {
		name  string
		span  flex.Span
		want  flex.Part
		shown bool
	}{
		{name: "gone", span: flex.Span{At: 9, Left: true}, want: part},
		{name: "kept", span: flex.Span{At: 9, Size: 4, Left: true}, want: flex.Part{Least: 4, Natural: 4, Gap: 2}, shown: true},
		{
			name: "shown, narrowed", span: flex.Span{At: 9, Size: 7},
			want: flex.Part{Least: 4, Natural: 12, Most: 20, Grows: true, Kept: 4, Gap: 2, Odd: true}, shown: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, shown := part.Decided(tt.span)
			if shown != tt.shown || shown && got != tt.want {
				t.Errorf("Decided = %+v, %t, want %+v, %t", got, shown, tt.want, tt.shown)
			}
		})
	}
}

func TestLayHoldsItsRulesWhateverItsGiven(t *testing.T) {
	for seed := range uint64(5000) {
		r := rand.New(rand.NewPCG(seed, 0xf1e8))
		given := drawnParts(r)
		room := r.IntN(120)
		spans, fits := flex.Lay(room, given...)
		l := laid{given: given, parts: heldParts(given), room: room, spans: spans, fits: fits}
		for _, rule := range layRules {
			if broke := rule.broke(l); broke != "" {
				t.Errorf("seed %d: %s: %s\nroom %d, parts %+v\nspans %+v, %v", seed, rule.name, broke, room, given, spans, fits)
			}
		}
	}
}

// drawnParts are as many as seven parts r draws, of every kind Lay is given:
// fixed and narrowing, growing to a most and with none, narrowing in a turn
// or freely, giving way to nothing or keeping some, with gaps and odd cells;
// and now and then one with a number past what Lay holds it to, as unheld
// draws it.
func drawnParts(r *rand.Rand) []flex.Part {
	parts := make([]flex.Part, r.IntN(8))
	for i := range parts {
		least := r.IntN(10)
		p := flex.Part{Least: least, Natural: least + r.IntN(2)*r.IntN(10), Grows: r.IntN(3) == 0, Gap: r.IntN(3), Odd: r.IntN(4) == 0}
		if r.IntN(2) == 0 {
			p.Most = p.Natural + r.IntN(10)
		}
		if r.IntN(3) == 0 {
			p.Narrows = 1 + r.IntN(4)
		}
		if r.IntN(3) == 0 {
			p.Leaves, p.Kept = 1+r.IntN(4), r.IntN(2)*r.IntN(least+1)
		}
		if r.IntN(4) == 0 {
			unheld[r.IntN(len(unheld))](r, &p)
		}
		parts[i] = p
	}
	return parts
}

// unheld are the ways a part's numbers pass what Lay holds them to: a natural
// past its most or under its least; a most under its least; what it keeps
// past its least; and a least, most, turn to narrow in, Kept or gap under 0.
var unheld = []func(r *rand.Rand, p *flex.Part){
	func(r *rand.Rand, p *flex.Part) { p.Natural = p.Most + 1 + r.IntN(10) },
	func(r *rand.Rand, p *flex.Part) { p.Natural = p.Least - 1 - r.IntN(10) },
	func(r *rand.Rand, p *flex.Part) { p.Most = 1 + r.IntN(max(p.Least, 1)) },
	func(r *rand.Rand, p *flex.Part) { p.Kept = p.Least + 1 + r.IntN(5) },
	func(r *rand.Rand, p *flex.Part) { p.Least = -1 - r.IntN(5) },
	func(r *rand.Rand, p *flex.Part) { p.Most = -1 - r.IntN(5) },
	func(r *rand.Rand, p *flex.Part) { p.Narrows = -1 - r.IntN(3) },
	func(r *rand.Rand, p *flex.Part) { p.Kept = -1 - r.IntN(5) },
	func(r *rand.Rand, p *flex.Part) { p.Gap = -1 - r.IntN(3) },
}

// heldParts are the parts as Lay holds them: no number under 0, a most no
// less than the least, a natural within the two, and what a part keeps no
// more than its least.
func heldParts(parts []flex.Part) []flex.Part {
	held := make([]flex.Part, len(parts))
	for i, p := range parts {
		p.Least, p.Most, p.Narrows, p.Kept, p.Gap = max(p.Least, 0), max(p.Most, 0), max(p.Narrows, 0), max(p.Kept, 0), max(p.Gap, 0)
		p.Natural, p.Kept = max(p.Natural, p.Least), min(p.Kept, p.Least)
		if p.Most > 0 {
			p.Most = max(p.Most, p.Least)
			p.Natural = min(p.Natural, p.Most)
		}
		held[i] = p
	}
	return held
}

// free reports whether the part narrows freely and hasn't given way.
func free(p flex.Part, s flex.Span) bool {
	return p.Narrows == 0 && !s.Left
}

// end is the cell after the spans' last.
func end(spans []flex.Span) int {
	if len(spans) == 0 {
		return 0
	}
	return spans[len(spans)-1].At + spans[len(spans)-1].Size
}

// floorAt is how many cells the parts take, their gaps among them, with the
// turns before turn taken, and turn's narrowing too where narrowed says: each
// part that gave way at its Kept, shown only where it keeps some; each free
// part, and each that narrowed, at its least; and the rest at their naturals.
func floorAt(parts []flex.Part, turn int, narrowed bool) int {
	taken, first := 0, true
	for _, p := range parts {
		gone := p.Leaves > 0 && p.Leaves < turn
		if gone && p.Kept == 0 {
			continue
		}
		if !first {
			taken += p.Gap
		}
		first = false
		switch {
		case gone:
			taken += p.Kept
		case p.Narrows == 0, p.Narrows < turn, p.Narrows == turn && narrowed:
			taken += p.Least
		default:
			taken += p.Natural
		}
	}
	return taken
}

func outOfBounds(l laid) string {
	for i, p := range l.parts {
		s := l.spans[i]
		most := math.MaxInt
		switch {
		case !p.Grows, p.Narrows > 0:
			most = p.Natural
		case p.Most > 0:
			most = p.Most
		}
		switch {
		case s.Left && s.Size != p.Kept:
			return fmt.Sprintf("part %d gave way at %d, not its Kept", i, s.Size)
		case !s.Left && (s.Size < p.Least || s.Size > most):
			return fmt.Sprintf("part %d is %d, outside %d to %d", i, s.Size, p.Least, most)
		}
	}
	return ""
}

func misplaced(l laid) string {
	if len(l.spans) != len(l.parts) {
		return fmt.Sprintf("%d spans for %d parts", len(l.spans), len(l.parts))
	}
	at, first := 0, true
	for i, p := range l.parts {
		s := l.spans[i]
		want := at
		if s.Left && p.Kept == 0 {
			if s.At != at || s.Size != 0 {
				return fmt.Sprintf("part %d, gone, lies at %d, %d long, not at %d", i, s.At, s.Size, at)
			}
			continue
		}
		if !first {
			want += p.Gap
		}
		if s.At != want {
			return fmt.Sprintf("part %d lies at %d, not %d", i, s.At, want)
		}
		at, first = s.At+s.Size, false
	}
	return ""
}

func fitsWrongly(l laid) string {
	floor := floorAt(l.parts, math.MaxInt, false)
	switch {
	case l.fits != (floor <= l.room):
		return fmt.Sprintf("fits %v with every turn taken at %d", l.fits, floor)
	case l.fits && end(l.spans) > l.room:
		return fmt.Sprintf("fits, but runs to %d", end(l.spans))
	}
	return ""
}

func underfilled(l laid) string {
	for i, p := range l.parts {
		if l.fits && free(p, l.spans[i]) && p.Grows && p.Most == 0 && end(l.spans) != l.room {
			return fmt.Sprintf("part %d grows with no most, but the spans end at %d", i, end(l.spans))
		}
	}
	return ""
}

func raisedOutOfOrder(l laid) string {
	short := -1
	for i, p := range l.parts {
		s := l.spans[i]
		switch {
		case !free(p, s):
		case short >= 0 && s.Size > p.Least:
			return fmt.Sprintf("part %d is past its least while part %d is short of its natural", i, short)
		case short < 0 && s.Size < p.Natural:
			short = i
		}
	}
	for i, p := range l.parts {
		if short >= 0 && free(p, l.spans[i]) && l.spans[i].Size > p.Natural {
			return fmt.Sprintf("part %d grew while part %d is short of its natural", i, short)
		}
	}
	return ""
}

func grewOutOfOrder(l laid) string {
	for i, p := range l.parts {
		if !free(p, l.spans[i]) || !p.Grows || p.Most == 0 || l.spans[i].Size == p.Most {
			continue
		}
		for j, q := range l.parts {
			if free(q, l.spans[j]) && q.Grows && q.Most == 0 && l.spans[j].Size > q.Natural {
				return fmt.Sprintf("part %d grew past its natural while part %d is short of its most", j, i)
			}
		}
	}
	return ""
}

func sharedUnevenly(l laid) string {
	for i, p := range l.parts {
		if free(p, l.spans[i]) && l.spans[i].Size < p.Natural {
			return ""
		}
	}
	for _, mosts := range []bool{true, false} {
		sharing := sharers(l, mosts)
		for ai, a := range sharing {
			if l.parts[a].Most > 0 && l.spans[a].Size == l.parts[a].Most {
				continue
			}
			for bi, b := range sharing {
				over := grown(l, b) - grown(l, a)
				if over > 1 || over == 1 && bi > ai {
					return fmt.Sprintf("part %d grew by %d, part %d by %d", b, grown(l, b), a, grown(l, a))
				}
			}
		}
	}
	return ""
}

// sharers are the free parts that grow, to a most or with none as mosts says,
// those marked Odd first.
func sharers(l laid, mosts bool) []int {
	var odd, rest []int
	for i, p := range l.parts {
		switch {
		case !free(p, l.spans[i]) || !p.Grows || (p.Most > 0) != mosts:
		case p.Odd:
			odd = append(odd, i)
		default:
			rest = append(rest, i)
		}
	}
	return append(odd, rest...)
}

// grown is how far part i grew past its natural.
func grown(l laid, i int) int {
	return l.spans[i].Size - l.parts[i].Natural
}

func gaveWayWrongly(l laid) string {
	for i, p := range l.parts {
		s := l.spans[i]
		if p.Leaves > 0 {
			if must := floorAt(l.parts, p.Leaves, true) > l.room; s.Left != must {
				return fmt.Sprintf("part %d gave way %v, where its turn had to: %v", i, s.Left, must)
			}
		}
		if p.Narrows == 0 || s.Left {
			continue
		}
		before, after := floorAt(l.parts, p.Narrows, false), floorAt(l.parts, p.Narrows, true)
		switch {
		case before <= l.room && s.Size != p.Natural:
			return fmt.Sprintf("part %d narrowed to %d, though the room didn't lack before its turn", i, s.Size)
		case after > l.room && s.Size != p.Least:
			return fmt.Sprintf("part %d stopped at %d, though its turn's whole narrowing didn't fit", i, s.Size)
		case before > l.room && after <= l.room && end(l.spans) != l.room:
			return fmt.Sprintf("part %d's turn narrowed to end at %d, not as far as the room lacked", i, end(l.spans))
		}
	}
	return ""
}

func tookMoreWithMore(l laid) string {
	wider, fits := flex.Lay(l.room+1, l.given...)
	if l.fits && !fits {
		return "it fits in room, but not in a cell more"
	}
	for i, p := range l.parts {
		switch {
		case wider[i].Left && !l.spans[i].Left:
			return fmt.Sprintf("part %d gave way with a cell more", i)
		case p.Narrows > 0 && !wider[i].Left && wider[i].Size < l.spans[i].Size:
			return fmt.Sprintf("part %d narrowed with a cell more", i)
		}
	}
	return ""
}
