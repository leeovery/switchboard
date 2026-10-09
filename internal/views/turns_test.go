package views_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

var start = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

// asked is a message line of a thread messages long, answered with stop.
func asked(messages int, stop string) ledger.Line {
	return ledger.Line{
		Kind:   ledger.KindMessage,
		Status: 200,
		Shape:  ledger.Shape{Messages: messages},
		Answer: ledger.Answer{Stop: stop},
	}
}

// canceled is a message line of a thread messages long its client went away
// from before its answer stopped.
func canceled(messages int) ledger.Line {
	line := asked(messages, "")
	line.Status, line.Canceled = 0, true
	return line
}

// errored is a message line of a thread messages long answered with an error.
func errored(messages int) ledger.Line {
	line := asked(messages, "")
	line.Status, line.Answer.Error = 529, ledger.Error{Type: "overloaded_error"}
	return line
}

// of is a line of the given kind, a thread messages long, answered with stop.
func of(kind string, messages int, stop string) ledger.Line {
	line := asked(messages, stop)
	line.Kind = kind
	return line
}

// session returns lines as a session's, in the order given: the nth arrived
// n minutes after start, its request named by n, and each took 30 seconds.
func session(lines ...ledger.Line) []ledger.Line {
	for i := range lines {
		lines[i].Request = fmt.Sprint(i)
		lines[i].At = start.Add(time.Duration(i) * time.Minute)
		lines[i].TotalMS = 30_000
	}
	return lines
}

// told describes turns, each as a mark a request, m on the main thread and s
// off it, and how it ended: its stop reason, canceled, or going.
func told(turns []views.Turn) []string {
	var described []string
	for _, turn := range turns {
		var marks strings.Builder
		for _, r := range turn.Requests {
			marks.WriteString(map[bool]string{true: "m", false: "s"}[r.Main])
		}
		ending := turn.Stop
		switch {
		case turn.Canceled:
			ending = "canceled"
		case turn.Going():
			ending = "going"
		}
		described = append(described, marks.String()+" "+ending)
	}
	return described
}

func TestTurnsAreToldFromTheShapeOfASessionsLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		want  []string
	}{
		{"no lines tell no turn", nil, nil},
		{
			"a turn ends at an answer on its main thread that stops at end_turn",
			session(asked(1, "tool_use"), asked(3, "tool_use"), asked(5, "end_turn")),
			[]string{"mmm end_turn"},
		},
		{"or at max_tokens", session(asked(1, "max_tokens")), []string{"m max_tokens"}},
		{"or at stop_sequence", session(asked(1, "stop_sequence")), []string{"m stop_sequence"}},
		{"or at refusal", session(asked(1, "refusal")), []string{"m refusal"}},
		{
			"or at any stop other than tool_use",
			session(asked(1, "model_context_window_exceeded")),
			[]string{"m model_context_window_exceeded"},
		},
		{"an answer that stops at tool_use leaves its turn going", session(asked(1, "tool_use")), []string{"m going"}},
		{
			"a request as long as the main thread's last is on it",
			session(asked(5, "tool_use"), asked(5, "end_turn")),
			[]string{"mm end_turn"},
		},
		{
			"a side request starts shorter than the main thread, and its end_turn ends nothing",
			session(asked(5, "tool_use"), asked(1, "end_turn"), asked(4, "end_turn"), asked(7, "end_turn")),
			[]string{"mssm end_turn"},
		},
		{
			"a main-thread request its client canceled ends its turn",
			session(asked(3, "tool_use"), canceled(5)),
			[]string{"mm canceled"},
		},
		{
			"a side request its client canceled ends nothing",
			session(asked(5, "tool_use"), canceled(2), asked(7, "end_turn")),
			[]string{"msm end_turn"},
		},
		{
			"an answer's stop tells how it ended, its client gone or not",
			session(func() ledger.Line { l := asked(1, "end_turn"); l.Canceled = true; return l }()),
			[]string{"m end_turn"},
		},
		{
			"a request answered with an error ends nothing",
			session(asked(3, "tool_use"), errored(5), asked(5, "end_turn")),
			[]string{"mmm end_turn"},
		},
		{
			"the request after a turn's end starts the next, its main thread afresh, so a compacted conversation starts one short",
			session(asked(9, "end_turn"), asked(1, "tool_use"), asked(3, "end_turn"), asked(2, "end_turn")),
			[]string{"m end_turn", "mm end_turn", "m end_turn"},
		},
		{
			"checks and counts are left out",
			session(of(ledger.KindCheck, 9, "end_turn"), asked(1, "tool_use"), of(ledger.KindCount, 1, ""), of(ledger.KindCheck, 1, "end_turn"), asked(3, "end_turn")),
			[]string{"mm end_turn"},
		},
		{
			"a request whose thread length is unknown is on no thread, and ends nothing",
			session(asked(3, "tool_use"), asked(0, "end_turn"), canceled(0), asked(5, "end_turn")),
			[]string{"mssm end_turn"},
		},
		{
			"a turn that starts with a request whose length is unknown has its main thread start after it",
			session(asked(0, "end_turn"), asked(1, "end_turn")),
			[]string{"sm end_turn"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := told(views.Turns(tt.lines)); !slices.Equal(got, tt.want) {
				t.Errorf("Turns() told %q, want %q", got, tt.want)
			}
		})
	}
}

func TestATurnHoldsItsRequestsAndWhenItStartedAndEnded(t *testing.T) {
	lines := session(asked(1, "tool_use"), asked(3, "end_turn"), asked(1, "tool_use"))
	turns := views.Turns(lines)
	if len(turns) != 2 {
		t.Fatalf("Turns() told %d turns, want 2", len(turns))
	}
	ended, going := turns[0], turns[1]
	var requests []string
	for _, r := range ended.Requests {
		requests = append(requests, r.Request)
	}
	if want := []string{"0", "1"}; !slices.Equal(requests, want) {
		t.Errorf("the first turn holds requests %q, want %q, in the order they arrived", requests, want)
	}
	if !ended.Started.Equal(lines[0].At) {
		t.Errorf("the first turn started %v, want %v, when its first request arrived", ended.Started, lines[0].At)
	}
	if want := lines[1].At.Add(30 * time.Second); !ended.Ended.Equal(want) {
		t.Errorf("the first turn ended %v, want %v, when the request that ended it did", ended.Ended, want)
	}
	if !going.Started.Equal(lines[2].At) || !going.Ended.IsZero() || !going.Going() {
		t.Errorf("the second turn started %v and ended %v, want started %v and going", going.Started, going.Ended, lines[2].At)
	}
}

func TestATurnsThreadIsItsMainThreadsLengthAsItLastStood(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		want  int
	}{
		{"the last main-thread request's", session(asked(2, "tool_use"), asked(6, "tool_use"), asked(1, "end_turn"), asked(0, "")), 6},
		{"none where no request was on it", session(asked(0, "end_turn")), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.Turns(tt.lines)[0].Thread(); got != tt.want {
				t.Errorf("Thread() = %d, want %d", got, tt.want)
			}
		})
	}
}
