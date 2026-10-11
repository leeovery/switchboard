package capture

import (
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A request of a session's own conversation either calls a tool or ends its
// turn, so a turn still going with no tool call to make has none of them
// answered yet: its request out is its conversation's, and its lines before
// it are side requests for its prompt.
func TestATurnStillGoingWithNoToolCallToMakeHasOnlySideRequests(t *testing.T) {
	now := localMoment()
	p := sessionPlan{
		id: "ffff0000-0000-4000-8000-000000000006", dir: "~/Code/switchboard", start: clockAt(now, 0, 14, 0, 0),
		turns: []turnPlan{{start: 0, minutes: 10, requests: 8, tools: []toolCount{{"Read", 3}}}, {start: 30, minutes: 10, requests: 4}},
		stays: []stay{{"work", 0}},
		asked: clockAt(now, 0, 14, 38, 0),
	}
	lines := p.lines()

	if len(lines) != 12 {
		t.Fatalf("the session has %d lines, want its turns' 12", len(lines))
	}
	if first := lines[7]; first.Class != classMain || first.Answer.Stop != "end_turn" {
		t.Errorf("the first turn ends with a %s request stopping at %q, want its conversation's, ending it", first.Class, first.Answer.Stop)
	}
	for _, l := range lines[8:] {
		if l.Prompt != lines[8].Prompt || l.Class != classAuxiliary || l.Answer.Stop != "end_turn" || len(l.Answer.Tools) > 0 {
			t.Errorf("the turn going has a %s request of %s stopping at %q, calling %v, want each a side request for its prompt",
				l.Class, l.Prompt, l.Answer.Stop, l.Answer.Tools)
		}
		if !endOf(l).Before(p.asked) {
			t.Errorf("a request of the turn going ends at %s, want before its request out, at %s", endOf(l), p.asked)
		}
	}
	if lines[8].Prompt == lines[7].Prompt || lines[8].Kind != ledger.KindMessage {
		t.Errorf("the turn going's lines are of prompt %s, a %s, want a message of its own prompt", lines[8].Prompt, lines[8].Kind)
	}
}
