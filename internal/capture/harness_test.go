package capture

import (
	"slices"
	"testing"
	"time"
)

// Every fixture pins the traces' scale at 16 requests a minute, as the frames
// were drawn with it, where the live page scales as its rule says.
func TestAFixturePinsTheTracesScaleAsTheFramesDo(t *testing.T) {
	now := func() time.Time { return localMoment() }
	for _, f := range []Fixture{fixture(t, "overview-default-160"), {records: &records{}}} {
		if got := f.config(wide(37), now).TraceScale; got != 16 {
			t.Errorf("%q has the watch scale its traces at %v requests a minute, want 16", f.Name, got)
		}
	}
}

// A page gives History what the dashboard gives it, of its world: the world's
// accounts, as configured, the frames' weeks from Monday, Claude's week and
// Claude's families.
func TestAPageGivesHistoryItsWorldsAccountsTheWeeksAndTheFamilies(t *testing.T) {
	now := func() time.Time { return localMoment() }
	f := page{name: "a page", size: wide(37), world: accountWorlds.auto}.fixture(worldsNow())
	cfg := f.config(wide(37), now)

	var accounts []string
	for _, a := range cfg.Accounts {
		accounts = append(accounts, a.ID+":"+a.Plan)
	}
	if want := []string{"work:max20x", "personal:max20x", "side:max20x"}; !slices.Equal(accounts, want) {
		t.Errorf("the page's watch is given the accounts %q, want its world's, %q", accounts, want)
	}
	if cfg.WeekStarts != time.Monday || cfg.WeekWindow != "7d" {
		t.Errorf("the page's weeks start on %s, its accounts' the window %q; want Monday, as the frames' do, and Claude's week, 7d", cfg.WeekStarts, cfg.WeekWindow)
	}
	if cfg.Family == nil {
		t.Fatal("the page's watch reads no model's family, want Claude's")
	}
	if got := cfg.Family("claude-opus-5-5"); got != "opus" {
		t.Errorf("the page's watch reads claude-opus-5-5's family as %q, want opus", got)
	}
}

// A fixture names the Overview's windows as the dashboard does, so its
// windows are named before any account's are read.
func TestAFixtureNamesTheWindowsAsTheDashboardDoes(t *testing.T) {
	now := func() time.Time { return localMoment() }
	name := fixture(t, "overview-default-160").config(wide(37), now).WindowName
	if name == nil {
		t.Fatal("the fixture's watch names no window, want Claude's names")
	}
	for key, want := range map[string]string{"5h": "5-hour", "7d": "Week"} {
		if got := name(key); got != want {
			t.Errorf("the fixture's watch names window %s %q, want %q", key, got, want)
		}
	}
}
