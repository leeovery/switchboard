package views_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// wordsSaid runs words together, each account named in brackets, so a test
// sees both what they say and what they name.
func wordsSaid(w status.Words) string {
	var b strings.Builder
	for _, p := range w {
		if p.Kind == status.PartAccount {
			b.WriteString("[" + p.Text + "]")
			continue
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

func TestWhereASessionRunsIsItsNamedModelsAccountAndHowItCameThere(t *testing.T) {
	now := time.Date(2026, 10, 7, 16, 30, 0, 0, time.UTC)
	const opus, haiku = "claude-opus-5-5", "claude-haiku-4-5"
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 7, hour, minute, 0, 0, time.UTC) }
	moved := views.SessionMove{At: at(14, 31), Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit"}
	running, ended := views.Activity{Doing: views.Idle}, views.Activity{Doing: views.Ended}
	tests := []struct {
		name string
		page views.SessionPage
		want string
	}{
		{name: "since the move that brought it there, off an account that couldn't take it",
			page: views.SessionPage{Account: "side", Model: opus, Moves: []views.SessionMove{moved}, Activity: running},
			want: "on [side] since 14:31, moved from [work]: [work] reached its limit"},
		{name: "since a move of its own words",
			page: views.SessionPage{Account: "side", Model: opus, Activity: running,
				Moves: []views.SessionMove{{At: at(9, 12), Model: opus, From: "work", To: "side", Reason: "rescored after 15h 7m idle"}}},
			want: "on [side] since 09:12, rescored after 15h 7m idle: [side] had the most room"},
		{name: "another model's move onto it telling nothing of how it came there",
			page: views.SessionPage{Account: "side", Model: opus, Activity: running,
				Moves:  []views.SessionMove{{At: at(14, 40), Model: haiku, From: "work", To: "side", Reason: "moved: work hit its limit"}},
				Models: []views.SessionModel{{Model: opus, Account: "side", Reason: status.ReasonNew}}},
			want: "on [side] since it started"},
		{name: "its model's last move taking it elsewhere, why the router has it where it is now",
			page: views.SessionPage{Account: "personal", Model: opus, Moves: []views.SessionMove{moved}, Activity: running,
				Models: []views.SessionModel{{Model: opus, Account: "personal", Reason: "rescored after 1h idle"}}},
			want: "on [personal]: rescored after 1h idle: [personal] had the most room"},
		{name: "why the router has it there",
			page: views.SessionPage{Account: "work", Model: opus, Activity: running, Models: []views.SessionModel{{Model: opus, Account: "work", Reason: status.ReasonSticky}}},
			want: "on [work]: sticky"},
		{name: "since it started where no router gives why",
			page: views.SessionPage{Account: "work", Model: opus, Activity: running},
			want: "on [work] since it started"},
		{name: "once ended, where it ran, its named model's moves alone",
			page: views.SessionPage{Account: "side", Model: opus, Activity: ended,
				Moves: []views.SessionMove{moved, {At: at(14, 40), Model: haiku, From: "work", To: "personal", Reason: "moved: work hit its limit"}}},
			want: "ran on [work], then [side] from 14:31: [work] reached its limit"},
		{name: "once ended, on the one account it never left",
			page: views.SessionPage{Account: "work", Model: opus, Activity: ended},
			want: "ran on [work]"},
		{name: "none, named on no account", page: views.SessionPage{Model: opus, Activity: running}},
		{name: "none, ended on no account", page: views.SessionPage{Model: opus, Activity: ended}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordsSaid(tt.page.Where(now)); got != tt.want {
				t.Errorf("Where = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestASessionsMovesAreItsNamedModelsAlone(t *testing.T) {
	const opus, haiku = "claude-opus-5-5", "claude-haiku-4-5"
	onto := views.SessionMove{Model: opus, From: "work", To: "side"}
	back := views.SessionMove{Model: opus, From: "side", To: "work"}
	other := views.SessionMove{Model: haiku, From: "work", To: "personal"}
	p := views.SessionPage{Account: "work", Model: opus, Moves: []views.SessionMove{onto, other, back}}
	if m, ok := p.MovedOnto(); !ok || m != back {
		t.Errorf("MovedOnto = %+v, %v; want its model's last move, which brought it back onto work", m, ok)
	}
	if first, moves := p.RanOn(); first != "work" || !reflect.DeepEqual(moves, []views.SessionMove{onto, back}) {
		t.Errorf("RanOn = %q, %+v; want work first, then its model's two moves", first, moves)
	}
	p.Account = "personal"
	if m, ok := p.MovedOnto(); ok {
		t.Errorf("MovedOnto on personal = %+v, want none, its model's last move taking it to work, another's to personal", m)
	}
}

func TestATurnsToolsAreTheThreeItCalledMost(t *testing.T) {
	tests := []struct {
		tools []views.ToolCalls
		want  string
	}{
		{tools: []views.ToolCalls{{Tool: "Bash", Times: 14}, {Tool: "Edit", Times: 9}, {Tool: "Read", Times: 8}, {Tool: "Grep", Times: 2}}, want: "Bash ×14, Edit ×9, Read ×8"},
		{tools: []views.ToolCalls{{Tool: "Read\x1b[2J", Times: 1}}, want: "Read [2J ×1"},
		{},
	}
	for _, tt := range tests {
		if got := (views.Turn{Tools: tt.tools}).ToolsSaid(); got != tt.want {
			t.Errorf("ToolsSaid of %+v = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

func TestTheWindowsASessionTookPointsOfAreNamedShortestFirst(t *testing.T) {
	tests := []struct {
		points map[string]int
		want   string
	}{
		{points: map[string]int{"7d": 6, "5h": 41}, want: "≈ 41 points of its 5-hour · 6 of its week"},
		{points: map[string]int{"5h": 1, "7d": 1, "7d_oi": 2}, want: "≈ 1 point of its 5-hour · 1 of its week · 2 of its Fable week"},
		{points: map[string]int{"5h": 0, "7d": 3}, want: "≈ 3 points of its week"},
		{points: map[string]int{"5h": 0}},
		{},
	}
	for _, tt := range tests {
		if got := (views.SessionAccount{Points: tt.points}).PointsSaid(); got != tt.want {
			t.Errorf("PointsSaid of %v = %q, want %q", tt.points, got, tt.want)
		}
	}
}
