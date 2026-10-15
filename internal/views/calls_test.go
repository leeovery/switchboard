package views_test

import (
	"maps"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// assigned is a model of a session's on an account, as the router lists it,
// last seen at seen.
func assigned(model, account string, seen time.Time) status.Assignment {
	return status.Assignment{Model: model, Account: account, AssignedAt: local(9, 0, 0), LastSeen: seen}
}

// titled returns l, a side request, a title's.
func titled(l ledger.Line) ledger.Line {
	l.Class = "auxiliary"
	return l
}

// A call says where it came from while the move that brought its named model
// to its account is that model's latest: none once it's back where it was
// with no move since; the stream's move, its line yet to come, where it's
// newer than its lines' last; and of a row's two models, the move of the one
// naming its session, or, on a row that one isn't on, of its model seen
// last there.
func TestACallCameFromWhereItsNamedModelsLatestMoveLeft(t *testing.T) {
	tests := []struct {
		name    string
		lines   []ledger.Line
		running []status.Session
		moves   []views.Moved
		want    map[string]string
	}{
		{
			name: "back where it was, with no move since",
			lines: []ledger.Line{
				sessionAsked("b1", mover, "work", "new", local(9, 0, 0)),
				movedFrom("work", sessionAsked("b2", mover, "side", "moved: work hit its limit", local(10, 0, 0))),
				sessionAsked("b3", mover, "work", status.Rescored(2*time.Hour), local(12, 30, 0)),
			},
			running: []status.Session{{ID: mover, Assignments: []status.Assignment{assigned(opus, "work", local(12, 30, 0))}}},
			want:    map[string]string{"work": ""},
		},
		{
			name: "the stream's move, newer than its lines' last, its line to come",
			lines: []ledger.Line{
				sessionAsked("s1", mover, "work", "new", local(9, 0, 0)),
				movedFrom("work", sessionAsked("s2", mover, "personal", "moved: work hit its limit", local(11, 0, 0))),
			},
			running: []status.Session{{ID: mover, Assignments: []status.Assignment{assigned(opus, "personal", local(12, 0, 0))}}},
			moves:   []views.Moved{{Request: "s3", Session: mover, Model: opus, From: "personal", To: "side", Reason: "moved: personal hit its limit", At: local(13, 0, 0)}},
			want:    map[string]string{"side": "personal"},
		},
		{
			name: "of a row's two models, the one naming its session",
			lines: []ledger.Line{
				sessionAsked("n1", mover, "work", "new", local(9, 0, 0)),
				movedFrom("work", sessionAsked("n2", mover, "side", "moved: work is at its reserve", local(11, 0, 0))),
				titled(filedAs("n3", line(mover, haiku, "side", "new", local(13, 0, 0), smallUsage))),
			},
			running: []status.Session{{ID: mover, Assignments: []status.Assignment{assigned(opus, "side", local(12, 55, 0)), assigned(haiku, "side", local(13, 0, 0))}}},
			want:    map[string]string{"side": "work"},
		},
		{
			name: "on a row the model naming its session isn't on, its model seen last there",
			lines: []ledger.Line{
				sessionAsked("h1", mover, "work", "new", local(9, 0, 0)),
				titled(filedAs("h2", line(mover, haiku, "side", "new", local(9, 1, 0), smallUsage))),
				movedFrom("side", titled(filedAs("h3", line(mover, haiku, "personal", "moved: side hit its limit", local(12, 20, 0), smallUsage)))),
			},
			running: []status.Session{{ID: mover, Assignments: []status.Assignment{assigned(opus, "work", local(12, 50, 0)), assigned(haiku, "personal", local(12, 30, 0))}}},
			want:    map[string]string{"work": "", "personal": "side"},
		},
	}
	doc := status.Document{Source: status.SourceRouter, Accounts: []status.Account{{ID: "work"}, {ID: "personal"}, {ID: "side"}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := views.Stream{Live: true, Moves: tt.moves}
			held := views.HoldList(views.ReadList(sessionsLedger(tt.lines, now), tt.running), ledger.Pricing, now)
			got := make(map[string]string)
			for _, c := range views.CallsOf(doc.At(now, claude.Policy()), views.ListLive{Running: tt.running, Stream: stream}, held) {
				got[c.Account] = c.From
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("the calls came from %v, by their accounts, want %v", got, tt.want)
			}
		})
	}
}
