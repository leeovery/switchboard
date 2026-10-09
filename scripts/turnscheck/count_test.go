package main

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

const (
	opus  = "claude-opus-4-1"
	haiku = "claude-haiku-4-5"
)

var day = time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local)

// at is the time on day at hour, minute and second.
func at(hour, minute, second int) time.Time {
	return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second)
}

// asked is a message line of session's, arrived when, of a thread messages
// long, asking model, answered with stop half a minute later.
func asked(session string, when time.Time, messages int, stop, model string) ledger.Line {
	return ledger.Line{
		At:      when,
		Kind:    ledger.KindMessage,
		Session: session,
		Model:   model,
		Status:  200,
		TotalMS: 30_000,
		Shape:   ledger.Shape{Messages: messages},
		Answer:  ledger.Answer{Stop: stop},
	}
}

func TestTheTallyCountsTheTurnsEachSessionsLinesTell(t *testing.T) {
	check := asked("work", at(9, 2, 0), 9, "end_turn", haiku)
	check.Kind = ledger.KindCheck
	counted := asked("personal", at(9, 30, 0), 9, "", opus)
	counted.Kind = ledger.KindCount
	canceled := asked("work", at(9, 12, 0), 4, "", opus)
	canceled.Status, canceled.Canceled = 0, true
	// Its turn ends at 10:00:59, an hour from the minute it started in.
	exactlyAnHour := asked("personal", at(10, 0, 0), 2, "end_turn", opus)
	exactlyAnHour.TotalMS = 59_000
	lines := []ledger.Line{
		asked("work", at(9, 0, 0), 3, "tool_use", opus),
		asked("personal", at(9, 0, 59), 1, "tool_use", opus),
		asked("work", at(9, 1, 0), 1, "end_turn", haiku),
		check,
		asked("work", at(9, 3, 0), 5, "end_turn", opus),
		asked("work", at(9, 10, 0), 2, "tool_use", opus),
		asked("work", at(9, 11, 0), 0, "", opus),
		canceled,
		counted,
		asked("", at(9, 40, 0), 1, "end_turn", opus),
		exactlyAnHour,
		asked("work", at(10, 0, 0), 6, "max_tokens", opus),
		asked("work", at(11, 0, 0), 7, "tool_use", opus),
		asked("work", at(11, 30, 0), 1, "end_turn", ""),
		asked("work", at(12, 0, 30), 8, "end_turn", opus),
		asked("work", at(13, 0, 0), 9, "tool_use", opus),
		asked("personal", at(14, 0, 0), 1, "tool_use", opus),
	}

	got := count(slices.Values(lines))
	slices.Sort(got.requests)
	want := tally{
		messages: 15, sessionless: 1, unmeasured: 1,
		sessions:   2,
		requests:   []int{1, 1, 1, 2, 3, 3, 3},
		stops:      map[string]int{"end_turn": 3, "max_tokens": 1},
		canceled:   1,
		going:      2,
		passedOver: map[string]int{haiku: 1, unnamed: 1},
		shorter:    2,
		overAnHour: 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("count() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestATurnRunsFromTheMinuteItStartedToTheOneItEndedIn(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		over  int
	}{
		{"an hour to the minute is not over one", []ledger.Line{asked("work", at(9, 0, 59), 1, "tool_use", opus), asked("work", at(9, 59, 30), 2, "end_turn", opus)}, 0},
		{"a minute more is", []ledger.Line{asked("work", at(9, 0, 59), 1, "tool_use", opus), asked("work", at(10, 0, 30), 2, "end_turn", opus)}, 1},
		{"a turn going is never counted", []ledger.Line{asked("work", at(9, 0, 0), 1, "tool_use", opus), asked("work", at(12, 0, 0), 2, "tool_use", opus)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := count(slices.Values(tt.lines)).overAnHour; got != tt.over {
				t.Errorf("turns over an hour = %d, want %d", got, tt.over)
			}
		})
	}
}

func TestTheSpreadIsTheLeastTheMedianAndTheMost(t *testing.T) {
	tests := []struct {
		name        string
		counts      []int
		least, most int
		median      string
	}{
		{"none", nil, 0, 0, "0"},
		{"an odd number's middle", []int{9, 1, 4}, 1, 9, "4"},
		{"an even number's middle two's mean", []int{8, 1, 4, 3}, 1, 8, "3.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			least, median, most := spread(tt.counts)
			if least != tt.least || median != tt.median || most != tt.most {
				t.Errorf("spread(%v) = %d, %s, %d, want %d, %s, %d", tt.counts, least, median, most, tt.least, tt.median, tt.most)
			}
		})
	}
}
