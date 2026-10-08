package router

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

var (
	extraOn     = quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	extraWarned = quota.ExtraUsage{Status: quota.StatusAllowedWarning, Utilization: new(0.85), ResetsAt: extraOn.ResetsAt}
	extraOff    = quota.ExtraUsage{Status: quota.StatusRejected}
)

func TestExtraUsageStandsAsTheRequestSentLastLeftIt(t *testing.T) {
	s := newTestState(&testClock{now: start})
	earlier, later := s.mark(), s.mark()
	probing := quota.Probe{Windows: []quota.Window{session}, Extra: extraOff}
	steps := []struct {
		name   string
		record func()
		want   quota.ExtraUsage
	}{
		{name: "an answer's", record: func() { s.recordExtra("work", extraWarned, later) }, want: extraWarned},
		{name: "a late answer to a request sent before, which is outweighed", record: func() { s.recordExtra("work", extraOn, earlier) }, want: extraWarned},
		{name: "none given, which leaves it", record: func() { s.recordExtra("work", quota.ExtraUsage{}, s.mark()) }, want: extraWarned},
		{name: "a probe that failed, which leaves it", record: func() {
			s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), s.mark(), readings.FromProbe)
		}, want: extraWarned},
		{name: "a probe sent before, which is outweighed", record: func() {
			s.recordProbe("work", probing, nil, earlier, readings.FromProbe)
		}, want: extraWarned},
		{name: "a probe sent since", record: func() {
			s.recordProbe("work", probing, nil, s.mark(), readings.FromProbe)
		}, want: extraOff},
	}
	for _, step := range steps {
		step.record()
		if work, _ := s.document().Account("work"); !reflect.DeepEqual(work.Extra, step.want) {
			t.Errorf("after %s, work reads extra usage %s, want %s", step.name, extraText(work.Extra), extraText(step.want))
		}
	}
	if side, _ := s.document().Account("side"); side.Extra.Given() {
		t.Errorf("side reads extra usage %s, want none, as none was given of it", extraText(side.Extra))
	}
}

func TestTheStateTellsOfExtraUsageTaken(t *testing.T) {
	lastSession := session
	lastSession.ResetsAt = session.ResetsAt.Add(-5 * time.Hour)
	stale := func(extra quota.ExtraUsage) quota.Probe {
		return quota.Probe{Windows: []quota.Window{lastSession}, Extra: extra}
	}
	tests := []struct {
		name string
		// change changes s, whose reading of work's session, and extra usage
		// of side, were taken in off the answers to requests sent after
		// earlier.
		change func(s *state, earlier moment)
		// want counts the changes told of, and wantReadOff the readings off
		// the answers to requests.
		want, wantReadOff changeCount
	}{
		{name: "off an answer", change: func(s *state, _ moment) { s.recordExtra("work", extraWarned, s.mark()) }, wantReadOff: 1},
		{name: "none given", change: func(s *state, _ moment) { s.recordExtra("work", quota.ExtraUsage{}, s.mark()) }},
		{name: "off a late answer, outweighed", change: func(s *state, earlier moment) { s.recordExtra("side", extraWarned, earlier) }},
		{name: "off a probe whose windows are stale", change: func(s *state, earlier moment) {
			s.recordProbe("work", stale(extraWarned), nil, earlier, readings.FromProbe)
		}, want: 1},
		{name: "none off a probe whose windows are stale", change: func(s *state, earlier moment) {
			s.recordProbe("work", stale(quota.ExtraUsage{}), nil, earlier, readings.FromProbe)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes, readOff changeCount
			s := newState(testAccounts(), testPolicy, claude.Provider{}.Family, at(start), changes.hear, readOff.hear)
			earlier := s.mark()
			s.record("work", []quota.Window{session}, s.mark())
			s.recordExtra("side", extraOn, s.mark())
			changes, readOff = 0, 0

			tt.change(s, earlier)
			if changes != tt.want || readOff != tt.wantReadOff {
				t.Errorf("told of %d changes and %d readings, want %d and %d", changes, readOff, tt.want, tt.wantReadOff)
			}
		})
	}
}

func TestTheStateFileKeepsEachAccountsExtraUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saved := newTestFile(at(start), testAccounts())
	saved.load(path)
	sent := saved.state.mark()
	saved.state.record("work", []quota.Window{session}, sent)
	saved.state.recordExtra("work", extraOn, sent)
	saved.state.record("side", []quota.Window{session}, saved.state.mark())
	saved.save()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), `"extra_usage"`); got != 1 {
		t.Errorf("the state file holds extra usage %d times, want once, for work alone:\n%s", got, data)
	}
	loaded := newTestFile(at(start), testAccounts())
	loaded.load(path)
	if work, _ := loaded.state.document().Account("work"); !reflect.DeepEqual(work.Extra, extraOn) {
		t.Errorf("once loaded, work reads extra usage %s, want what was saved: %s", extraText(work.Extra), extraText(extraOn))
	}
	if side, _ := loaded.state.document().Account("side"); side.Extra.Given() {
		t.Errorf("once loaded, side reads extra usage %s, want none, as none was saved", extraText(side.Extra))
	}
	loaded.state.recordExtra("work", extraWarned, loaded.state.mark())
	if work, _ := loaded.state.document().Account("work"); !reflect.DeepEqual(work.Extra, extraWarned) {
		t.Errorf("after an answer, work reads extra usage %s, want the answer's %s: what was saved counts as sent before it", extraText(work.Extra), extraText(extraWarned))
	}
}

// extraText gives extra usage as JSON, its utilization's value rather than
// its address.
func extraText(e quota.ExtraUsage) string {
	data, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
