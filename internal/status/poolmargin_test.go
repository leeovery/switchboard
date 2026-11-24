package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/status"
)

// sharing is the router's document of work and personal, each half its week
// used, resetting six days after it was built, the pool's use 1% of a Pro
// week an hour, as fast as its day's, none of it held on an account: the room
// they have, an account's worth, runs out 100 hours on, before they reset.
func sharing() status.Document {
	return forecasting(routerDocOf(
		on(readAt("work", windowIn("7d", 0.5, 6*day)), fastAt("7d", 0.005)),
		on(readAt("personal", windowIn("7d", 0.5, 6*day)), fastAt("7d", 0.005)),
	), 0.01, holdingNone...)
}

func TestAPoolMarginSaysHowNearThePoolIsToRunningOut(t *testing.T) {
	tests := []struct {
		name      string
		margin    forecast.Margin
		wantFull  string
		wantShort string
	}{
		{
			name: "lasting, but not a step busier", margin: forecast.Margin{Kind: forecast.MarginLasts},
			wantFull: "lasts the week, under 5% to spare", wantShort: "under 5% to spare",
		},
		{
			name: "lasting 15% busier", margin: forecast.Margin{Kind: forecast.MarginLasts, Spare: 15},
			wantFull: "lasts the week, 15% to spare", wantShort: "15% to spare",
		},
		{
			name: "lasting 50% busier", margin: forecast.Margin{Kind: forecast.MarginLasts, Spare: 50},
			wantFull: "lasts the week, 50% to spare", wantShort: "50% to spare",
		},
		{
			name: "lasting past as far as it plays", margin: forecast.Margin{Kind: forecast.MarginLasts, Spare: 55},
			wantFull: "lasts the week, over 50% to spare", wantShort: "over 50% to spare",
		},
		{
			name: "running out days on, lasting at 20% less", margin: forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day), Less: 20},
			wantFull: "runs out ~Sat · lasts at 20% less", wantShort: "out ~Sat",
		},
		{
			name: "running out days on, not lasting at 50% less", margin: forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day)},
			wantFull: "runs out ~Sat", wantShort: "out ~Sat",
		},
		{
			name: "running out within a day", margin: forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(3 * time.Hour), Less: 5},
			wantFull: "runs out ~16:12 · lasts at 5% less", wantShort: "out ~16:12",
		},
		{name: "saying nothing", margin: forecast.Margin{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status.MarginIs(t, tt.margin)

			got := sharing().At(built, policy).PoolMargin()

			if full, short := got.Words().String(), got.ShortWords().String(); full != tt.wantFull || short != tt.wantShort {
				t.Errorf("PoolMargin() says %q, short %q, want %q, short %q", full, short, tt.wantFull, tt.wantShort)
			}
		})
	}
}

func TestAPoolMarginsFiguresAndTimeAreWordsOfTheirOwn(t *testing.T) {
	status.MarginIs(t, forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day), Less: 20})

	got := sharing().At(built, policy).PoolMargin().Words()

	want := status.Words{status.Plain("runs out ~"), status.Time("Sat"), status.Plain(" · lasts at "), status.Figure("20%"), status.Plain(" less")}
	if !slices.Equal(got, want) {
		t.Errorf("PoolMargin().Words() = %+v, want %+v", got, want)
	}
}

func TestAPoolMarginsRunOutIsSaidFromTheMomentsNow(t *testing.T) {
	status.MarginIs(t, forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day)})

	got := sharing().At(built, policy).At(built.Add(4*day + 20*time.Hour)).PoolMargin()

	if when, ok := got.RunsOut(); !ok || !when.Equal(built.Add(5*day)) {
		t.Errorf("PoolMargin().RunsOut() = %v, %t, want %v", when, ok, built.Add(5*day))
	}
	if want := "runs out ~13:12"; got.Words().String() != want {
		t.Errorf("PoolMargin() says %q, want %q", got.Words(), want)
	}
}

// The pool runs out 100 hours on, and lasts at 35% less use: its room then
// runs to 154 hours, past the weeks' reset at 144, which brings a week's
// room each back for the day that's left.
func TestAPoolMarginIsTheDocumentsForecastPoolPlayed(t *testing.T) {
	got := sharing().WorkedOut(policy).At(built, policy).PoolMargin()

	when, ok := got.RunsOut()
	if !ok || when.Sub(built.Add(100*time.Hour)).Abs() > 10*time.Minute {
		t.Errorf("PoolMargin().RunsOut() = %v, %t, want about %v", when, ok, built.Add(100*time.Hour))
	}
	if want := "runs out ~Fri · lasts at 35% less"; got.Words().String() != want {
		t.Errorf("PoolMargin() says %q, want %q", got.Words(), want)
	}
}

func TestAPoolMarginSaysNothingWithoutAPoolToForecast(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
	}{
		{name: "no demand, as a probed document has none", doc: func() status.Document {
			doc := sharing()
			doc.Demand = nil
			return doc
		}()},
		{name: "one account", doc: forecasting(routerDocOf(on(readAt("work", windowIn("7d", 0.5, 6*day)), fastAt("7d", 0.01))), 0.01)},
		{name: "no room as the document was built", doc: forecasting(routerDocOf(
			readAt("work", windowIn("7d", 1, 6*day)),
			readAt("personal", windowIn("7d", 1, 6*day)),
		), 0.01)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.doc.WorkedOut(policy).At(built, policy).PoolMargin()

			if words, short := got.Words(), got.ShortWords(); words != nil || short != nil {
				t.Errorf("PoolMargin() says %q, short %q, want nothing", words, short)
			}
			if when, ok := got.RunsOut(); ok {
				t.Errorf("PoolMargin().RunsOut() = %v, true, want false", when)
			}
		})
	}
}

func TestADocumentsMarginIsPlayedOnceAndOnlyWhenAskedFor(t *testing.T) {
	played := status.CountMargins(t)
	doc := sharing().WorkedOut(policy)
	m := doc.At(built, policy)
	if got := played(); got != 0 {
		t.Errorf("the margin was played %d times before it was asked for, want none", got)
	}

	_ = m.PoolMargin()
	_ = m.At(built.Add(time.Hour)).PoolMargin()
	_ = doc.At(built.Add(2*time.Hour), policy).PoolMargin()
	_ = doc.HoldAt(built.Add(3*time.Hour), policy).Moment().PoolMargin()

	if got := played(); got != 1 {
		t.Errorf("the margin was played %d times, want once, as the document was worked out", got)
	}
}
