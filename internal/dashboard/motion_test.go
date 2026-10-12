package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/status"
)

func TestARenderedFrameIsDrawnAsDrawDrawsIt(t *testing.T) {
	f := frameOf(160, 60)
	f.Look = Screen(builtin(t, "nord"))
	f.Brightened = map[Number]float64{workSession: 0.5}
	f.Changed = map[string]float64{"work": 0.25}
	for _, card := range []Opened{nil, Help{}} {
		f.Card = card
		if got, _ := f.Rendered(threeRouted(), now); !slices.Equal(got, f.Draw(threeRouted(), now)) {
			t.Errorf("under %T, Rendered's rows are\n%s\nwant them as Draw draws them\n%s", card, strings.Join(got, "\n"), strings.Join(f.Draw(threeRouted(), now), "\n"))
		}
	}
}

func TestACueMarksWhatItPicksOutCuedAsWellAsWhatItWas(t *testing.T) {
	workBar := Ref{Account: "work", Window: "5h"}
	c := newCanvas(12, 1)
	c.line(0, 0, line{span{"58%", mutedInk}.numbered(workSession)})
	c.barred(4, 0, 4, workBar)
	cue(c, Look{}, 0, 0, 10, 0.5)

	if got := c.numbers()[workSession]; got != "58%" {
		t.Errorf("the cued number reads %q, want it still 58%%", got)
	}
	for x, here := range c.cells[0] {
		var m mark
		if here.mark != (unique.Handle[mark]{}) {
			m = here.mark.Value()
		}
		if cued := x < 10; m.cued != cued {
			t.Errorf("cell %d is cued %t, want %t", x, m.cued, cued)
		}
		if barred := 4 <= x && x < 8; (m.bar == workBar) != barred {
			t.Errorf("cell %d is marked %+v, want it work's bar %t", x, m, barred)
		}
	}
}

func TestWhatSettlesOnScreenAloneMovesTheFrame(t *testing.T) {
	ten, three := tenRouting(), threeRouted()
	news := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
		status.Event{At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "side"},
	)
	// usageAlone is USAGE alone, too short for every account: studio's
	// scrolled out of view at 30 rows, and lab's, whose words say out ~, at 24.
	usageAlone := func(height int) Frame {
		f := frameOf(160, height)
		f.Sections = Sections{}.Toggled(SectionRouting).Toggled(SectionComing).Toggled(SectionLately)
		return f
	}
	sessions := frameOf(160, 60)
	sessions.View = Sessions
	workBar, studioBar := Ref{Account: "work", Window: "5h"}, Ref{Account: "studio", Window: "5h"}
	tests := []struct {
		name   string
		f      Frame
		doc    status.Document
		at     time.Time
		settle func(f *Frame)
		want   bool
	}{
		{name: "nothing settling", f: usageAlone(30), doc: ten, at: routedAt, settle: func(*Frame) {}},
		{
			name: "a number on screen brightening", f: usageAlone(30), doc: ten, at: routedAt, want: true,
			settle: func(f *Frame) { f.Brightened = map[Number]float64{workSession: 0.5} },
		},
		{
			name: "a number scrolled out of view brightening", f: usageAlone(30), doc: ten, at: routedAt,
			settle: func(f *Frame) {
				f.Brightened = map[Number]float64{{Kind: NumberUsed, Account: "studio", Of: "5h"}: 0.5}
			},
		},
		{
			name: "a bar on screen easing", f: usageAlone(30), doc: ten, at: routedAt, want: true,
			settle: func(f *Frame) { f.Eased = map[Ref]float64{workBar: 0.3} },
		},
		{
			name: "a bar scrolled out of view easing", f: usageAlone(30), doc: ten, at: routedAt,
			settle: func(f *Frame) { f.Eased = map[Ref]float64{studioBar: 0.3} },
		},
		{name: "on Sessions, nothing settling", f: sessions, doc: ten, at: routedAt, settle: func(*Frame) {}},
		{
			name: "on Sessions, a panel's bar easing", f: sessions, doc: ten, at: routedAt, want: true,
			settle: func(f *Frame) { f.Eased = map[Ref]float64{workBar: 0.3} },
		},
		{
			name: "a window's words on screen cued, out ~", f: usageAlone(24), doc: ten, at: routedAt, want: true,
			settle: func(f *Frame) { f.TurnedOut = map[Ref]float64{{Account: "client", Window: "5h"}: 0.5} },
		},
		{
			name: "a window's words scrolled out of view cued, out ~", f: usageAlone(24), doc: ten, at: routedAt,
			settle: func(f *Frame) { f.TurnedOut = map[Ref]float64{{Account: "lab", Window: "5h"}: 0.5} },
		},
		{
			name: "an account's state on screen cued", f: frameOf(160, 30), doc: three, at: now, want: true,
			settle: func(f *Frame) { f.Changed = map[string]float64{"work": 0.5} },
		},
		{
			name: "an account's state cued, ROUTING without room", f: frameOf(160, 24), doc: three, at: now,
			settle: func(f *Frame) { f.Changed = map[string]float64{"work": 0.5} },
		},
		{
			name: "an event fresh, its row on screen in LATELY", f: frameOf(160, 60), doc: news, at: now, want: true,
			settle: func(f *Frame) { f.Fresh = map[int]float64{1: 0.5} },
		},
		{
			name: "an event fresh, LATELY folded", f: frameOf(160, 30), doc: news, at: now,
			settle: func(f *Frame) { f.Fresh = map[int]float64{2: 0.5} },
		},
		{
			name: "an event fresh, its row on screen in LOG", f: sessions, doc: news, at: now, want: true,
			settle: func(f *Frame) { f.Fresh = map[int]float64{1: 0.5} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.f
			tt.settle(&f)
			if got := f.Motion(tt.doc, tt.at); got.Smooth != tt.want {
				t.Errorf("Motion is %+v, want Smooth %t", got, tt.want)
			}
			if _, got := f.Rendered(tt.doc, tt.at); got != f.Motion(tt.doc, tt.at) {
				t.Errorf("Rendered's motion is %+v, want it as Motion has it", got)
			}
		})
	}
}
