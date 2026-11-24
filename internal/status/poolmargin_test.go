package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
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

// An account whose token alone is refused takes requests in the forecast
// from when the refusal lifts; one barred by anything else, its token
// refused among it, takes none all the play.
func TestTheForecastPoolHasAnAccountTakeRequestsFromWhenItsTokenRefusalLifts(t *testing.T) {
	lifts := built.Add(9 * time.Minute)
	refused := status.Refusal{Until: lifts, Status: 401}
	tests := []struct {
		name      string
		change    func(*status.Account)
		wantTakes bool
		wantFrom  time.Time
	}{
		{name: "nothing barring it", change: func(*status.Account) {}, wantTakes: true},
		{name: "its token refused", change: func(a *status.Account) { a.Refused = refused }, wantTakes: true, wantFrom: lifts},
		{name: "a model family refused", change: func(a *status.Account) { a.Refused = status.Refusal{Until: lifts, Status: 403, Family: "opus"} }, wantTakes: true},
		{name: "its read refused", change: func(a *status.Account) { a.RefusedRead = quota.TokenRefused }},
		{name: "its token and its read refused", change: func(a *status.Account) { a.Refused, a.RefusedRead = refused, quota.TokenRefused }},
		{name: "another subscription's token, refused", change: func(a *status.Account) { a.TokenForeign, a.Refused = true, refused }},
		{name: "no usable token, refused", change: func(a *status.Account) { a.TokenSet, a.Refused = false, refused }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := forecasting(routerDocOf(on(readAt("work", windowIn("7d", 0.3, 6*day)), tt.change), readAt("personal", windowIn("7d", 0.5, 6*day))), 0.01)

			pool, ok := doc.ForecastPool(policy)

			if !ok {
				t.Fatal("ForecastPool() reported nothing to play")
			}
			if got := pool.Accounts[0]; got.Takes != tt.wantTakes || !got.TakesFrom.Equal(tt.wantFrom) {
				t.Errorf("ForecastPool()'s work takes %t from %v, want %t from %v", got.Takes, got.TakesFrom, tt.wantTakes, tt.wantFrom)
			}
		})
	}
}

// work's token refused for 9 minutes leaves its half week to the pool, which
// runs out 100 hours on, as without the refusal; its read refused keeps it
// out all week, and personal's half runs out 50 hours on.
func TestAPoolMarginCountsTheRoomOfAnAccountWhoseTokenRefusalLifts(t *testing.T) {
	tests := []struct {
		name   string
		change func(*status.Account)
		want   time.Duration
	}{
		{name: "its token refused", change: func(a *status.Account) { a.Refused = status.Refusal{Until: built.Add(9 * time.Minute), Status: 401} }, want: 100 * time.Hour},
		{name: "its read refused", change: func(a *status.Account) { a.RefusedRead = quota.TokenRefused }, want: 50 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sharing()
			tt.change(&doc.Accounts[0])

			when, ok := doc.WorkedOut(policy).At(built, policy).PoolMargin().RunsOut()

			if want := built.Add(tt.want); !ok || when.Sub(want).Abs() > 10*time.Minute {
				t.Errorf("PoolMargin().RunsOut() = %v, %t, want about %v", when, ok, want)
			}
		})
	}
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
			wantFull: "runs out ~Sat · lasts if you use 20% less", wantShort: "out ~Sat",
		},
		{
			name: "running out days on, not lasting at 50% less", margin: forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day)},
			wantFull: "runs out ~Sat", wantShort: "out ~Sat",
		},
		{
			name: "running out within a day", margin: forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(3 * time.Hour), Less: 5},
			wantFull: "runs out ~16:12 · lasts if you use 5% less", wantShort: "out ~16:12",
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

	want := status.Words{status.Plain("runs out ~"), status.Time("Sat"), status.Plain(" · lasts if you use "), status.Figure("20%"), status.Plain(" less")}
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

func TestAPoolMarginWhoseRunOutTheClockHasReachedSaysNothing(t *testing.T) {
	status.MarginIs(t, forecast.Margin{Kind: forecast.MarginRunsOut, RunsOut: built.Add(5 * day), Less: 20})
	m := sharing().At(built, policy)

	for _, at := range []time.Time{built.Add(5 * day), built.Add(5*day + time.Hour)} {
		got := m.At(at).PoolMargin()

		if words, short := got.Words(), got.ShortWords(); words != nil || short != nil {
			t.Errorf("at %v, PoolMargin() says %q, short %q, want nothing", at, words, short)
		}
		if when, ok := got.RunsOut(); ok {
			t.Errorf("at %v, PoolMargin().RunsOut() = %v, true, want false", at, when)
		}
	}
}

// The pool runs out 100 hours on, and lasts if you use 35% less: its room then
// runs to 154 hours, past the weeks' reset at 144, which brings a week's
// room each back for the day that's left.
func TestAPoolMarginIsTheDocumentsForecastPoolPlayed(t *testing.T) {
	got := sharing().WorkedOut(policy).At(built, policy).PoolMargin()

	when, ok := got.RunsOut()
	if !ok || when.Sub(built.Add(100*time.Hour)).Abs() > 10*time.Minute {
		t.Errorf("PoolMargin().RunsOut() = %v, %t, want about %v", when, ok, built.Add(100*time.Hour))
	}
	if want := "runs out ~Fri · lasts if you use 35% less"; got.Words().String() != want {
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

func TestAMomentUnderAnotherPolicyPlaysItsMarginAgain(t *testing.T) {
	played := status.CountMargins(t)
	doc := sharing().WorkedOut(policy)
	other := policy
	other.Tiebreak = "7d"

	_ = doc.At(built, policy).PoolMargin()
	_ = doc.At(built, other).PoolMargin()

	if got := played(); got != 2 {
		t.Errorf("the margin was played %d times, want twice, once for each policy", got)
	}
}
