package dashboard

import "testing"

func TestWhatSettlesOnScreenAloneMovesTheFrame(t *testing.T) {
	ten := tenRouting()
	// USAGE alone, too short for every account: studio's scrolled out of view.
	scrolled := frameOf(160, 30)
	scrolled.Sections = Sections{}.Toggled(SectionRouting).Toggled(SectionComing).Toggled(SectionLately)
	panels := frameOf(160, 60)
	panels.View = Sessions
	workBar, studioBar := Ref{Account: "work", Window: "5h"}, Ref{Account: "studio", Window: "5h"}
	tests := []struct {
		name       string
		f          Frame
		brightened map[Number]float64
		eased      map[Ref]float64
		want       bool
	}{
		{name: "nothing settling", f: scrolled},
		{name: "a number on screen brightening", f: scrolled, brightened: map[Number]float64{workSession: 0.5}, want: true},
		{name: "a number scrolled out of view brightening", f: scrolled, brightened: map[Number]float64{{Kind: NumberUsed, Account: "studio", Of: "5h"}: 0.5}},
		{name: "a bar on screen easing", f: scrolled, eased: map[Ref]float64{workBar: 0.3}, want: true},
		{name: "a bar scrolled out of view easing", f: scrolled, eased: map[Ref]float64{studioBar: 0.3}},
		{name: "on Sessions, nothing settling", f: panels},
		{name: "on Sessions, a panel's bar easing", f: panels, eased: map[Ref]float64{workBar: 0.3}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.f
			f.Brightened, f.Eased = tt.brightened, tt.eased
			if got := f.Motion(ten, routedAt); got.Smooth != tt.want {
				t.Errorf("Motion is %+v, want Smooth %t", got, tt.want)
			}
		})
	}
}
