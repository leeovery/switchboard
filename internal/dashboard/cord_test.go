package dashboard

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

// drawnCord is the cord k drawn on a canvas of its own in the glyphs g, its
// jack in column jack, every cell in the ink given; and its path.
func drawnCord(k cord, jack int, g cordGlyphs, base ink) (*canvas, []point) {
	path := k.path(jack)
	c := newCanvas(jack+1, k.to+1)
	for i, p := range path {
		c.text(p.x, p.y, k.glyph(p, i, len(path)-1, g), base)
	}
	return c, path
}

func TestACordRunsAlongItsRowThenDownItsBendToItsJack(t *testing.T) {
	tests := []struct {
		name   string
		k      cord
		glyphs cordGlyphs
		want   []string
	}{
		{name: "on its jack's row, straight", k: cord{plug: 2, from: 0, to: 0}, glyphs: heavyCord, want: []string{"  ●━━━━◉"}},
		{
			name: "to a jack below, bent", k: cord{plug: 2, from: 0, to: 2, bend: 4}, glyphs: heavyCord,
			want: []string{"  ●━┓", "    ┃", "    ┗━━◉"},
		},
		{
			name: "light, where the look can't dim it", k: cord{plug: 2, from: 0, to: 2, bend: 4}, glyphs: lightCord,
			want: []string{"  ●─┐", "    │", "    └──◉"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := drawnCord(tt.k, 7, tt.glyphs, ink{})
			var got []string
			for y := range c.height() {
				got = append(got, rowOf(c, y))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the cord is drawn\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestTheCordsGlyphsAreTheDesigns(t *testing.T) {
	for drawn, want := range map[string]string{
		plugGlyph: "●", pluggedJack: "◉", freeJack: "○", refusedJack: "✕", hangingGlyph: "╌",
	} {
		if drawn != want {
			t.Errorf("a cord's glyph is %q, want %q", drawn, want)
		}
	}
}

func TestAPulseTravelsACord(t *testing.T) {
	tests := []struct {
		name  string
		call  Call
		token theme.Token
		// head is how far along the cord's path its head is, and behind
		// which way the rest of it lies.
		head, behind int
	}{
		{name: "out, as a request goes", call: Call{Doing: Asking, Pulsing: true, Pulse: Pulse{Along: 0.5}}, token: theme.AccentKey, head: 10, behind: -1},
		{name: "back, as its answer ends", call: Call{Doing: Answered, Pulsing: true, Pulse: Pulse{Along: 0.25, Back: true}}, token: theme.AccentKey, head: 15, behind: 1},
		{name: "back, red, as it's refused", call: Call{Doing: Refused, Pulsing: true, Pulse: Pulse{Along: 0.5, Back: true, Red: true}}, token: theme.StateDestructive, head: 10, behind: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, path := drawnCord(cord{plug: 0, from: 0, to: 0}, 20, heavyCord, ink{token: theme.AccentKey})
			travel(c, path, theme.AccentKey, tt.call)
			for k := range pulseLength {
				p := path[tt.head+k*tt.behind]
				want := ink{token: tt.token, glow: trailGlow - glowStep*float64(k)}
				if k == 0 {
					want = titleInk
				}
				if got := c.cells[p.y][p.x].ink; got != want {
					t.Errorf("the pulse's cell %d, at %d, is %+v, want %+v", k, p.x, got, want)
				}
			}
			past := path[tt.head+pulseLength*tt.behind]
			if got := c.cells[past.y][past.x].ink; got != (ink{token: theme.AccentKey}) {
				t.Errorf("the cell past the pulse, at %d, is %+v, want the cord's own", past.x, got)
			}
		})
	}
}

func TestWhatTravelsACordIsWhatItsCallIsDoing(t *testing.T) {
	cordInk := ink{token: theme.AccentKey}
	tests := []struct {
		name string
		call Call
		// jack is the glyph its jack is drawn as, and its ink; and shimmers
		// whether every shimmerEvery-th cell is lit, its shimmer moved a
		// step.
		jack     string
		jackInk  ink
		shimmers bool
	}{
		{name: "asking", call: Call{Doing: Asking}, jack: pluggedJack, jackInk: cordInk},
		{name: "its answer streaming, every fourth cell lit and its jack", call: Call{Doing: Streaming, Shimmer: 1}, jack: pluggedJack, jackInk: titleInk, shimmers: true},
		{name: "its answer ended", call: Call{Doing: Answered}, jack: pluggedJack, jackInk: cordInk},
		{name: "refused", call: Call{Doing: Refused, Status: 429}, jack: refusedJack, jackInk: exhaustedInk},
		{name: "throttled", call: Call{Doing: Throttled, Status: 429}, jack: pluggedJack, jackInk: cordInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, path := drawnCord(cord{plug: 0, from: 0, to: 2, bend: 9}, 20, heavyCord, cordInk)
			travel(c, path, theme.AccentKey, tt.call)
			last := path[len(path)-1]
			if got := c.cells[last.y][last.x]; got.glyph != tt.jack || got.ink != tt.jackInk {
				t.Errorf("its jack is %+v, want %q in %+v", got, tt.jack, tt.jackInk)
			}
			for i := 1; i < len(path)-1; i++ {
				want := cordInk
				if tt.shimmers && (i+1)%shimmerEvery == 0 {
					want = ink{token: theme.AccentKey, glow: shimmerGlow, bold: true}
				}
				if got := c.cells[path[i].y][path[i].x].ink; got != want {
					t.Errorf("its cord's cell %d is %+v, want %+v", i, got, want)
				}
			}
		})
	}
}
