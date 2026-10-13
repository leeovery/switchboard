package dashboard

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// At each hard moment, ROUTING, the routing card, COMING UP and LATELY's
// quiet line each say what the moment has of the accounts: each its own
// facts, worked out of the one moment, agreeing with it and with each other.
func TestROUTINGTheCardAndCOMINGUPAgreeWithTheMomentAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			f := frameOf(160, 40)
			m := f.Moment(hm.Doc, hm.Now)
			checkROUTINGAgrees(t, m, f.routingTableOf(hm.Doc, hm.Now))
			checkCardAgrees(t, m, RoutingRows(m, nil, views.Stream{}))
			checkCOMINGUPAgrees(t, m, f.upcoming(hm.Doc, hm.Now))
			if said := f.quiet(m) != nil; said != (m.Routed || m.Probed) {
				t.Errorf("LATELY says why it's quiet: %v; want %v, as the document was read from the router or by probing", said, !said)
			}
		})
	}
}

// heldFromEvery are the kinds ROUTING says in red of an account held back
// from every request by Anthropic, or without a token, which the card closes
// in red, and won't pick.
var heldFromEvery = []status.RoutingKind{status.RoutingTokenless, status.RoutingLimited, status.RoutingRefused}

// checkROUTINGAgrees checks ROUTING's table t against the moment m: each
// account's state is the moment's, in red where it can take no request for
// want of a token, a limit, a refused token or a first read; new sessions
// said to come to the account ▲ marks alone; and its pin, its fade and its
// place in the pool as the moment has them.
func checkROUTINGAgrees(t *testing.T, m status.Moment, table routingTable) {
	t.Helper()
	if len(table.accounts) != len(m.Accounts) {
		t.Fatalf("ROUTING has %d accounts, want %d", len(table.accounts), len(m.Accounts))
	}
	for i, a := range m.Accounts {
		r := table.accounts[i]
		if !reflect.DeepEqual(r.standing, a.Routing) {
			t.Errorf("%s: ROUTING says %q, want the moment's %q", a.ID, r.standing, a.Routing)
		}
		words := standingWords(r.standing, false, 200)
		red, wantRed := words[0].ink == errorInk, !a.TokenSet || limited(a) || a.TokenRefused || !a.Read && a.Error != ""
		if red != wantRed {
			t.Errorf("%s: ROUTING says %q in red: %v; want %v", a.ID, words.plain(), red, wantRed)
		}
		comesHere := strings.Contains(words.plain(), "new sessions come here")
		open := r.standing.Kind == status.RoutingOpen || r.standing.Kind == status.RoutingNext
		if comesHere && !a.Next || a.Next && open && len(m.Accounts) > 1 && !comesHere {
			t.Errorf("%s: ROUTING says new sessions come here: %v; ▲ marks it: %v", a.ID, comesHere, a.Next)
		}
		pooled := slices.ContainsFunc(table.pool, func(p accountRouting) bool { return p.id == a.ID })
		if r.pinned != a.Pinned || r.faded != a.OutOfPool || pooled != a.InPool {
			t.Errorf("%s: ROUTING has it pinned %v, faded %v, pooled %v; want %v, %v, %v", a.ID, r.pinned, r.faded, pooled, a.Pinned, a.OutOfPool, a.InPool)
		}
	}
}

// checkCardAgrees checks the routing card's rows against the moment m, and
// against what ROUTING says of each account: closed in red, and not to be
// picked, where ROUTING says it's held back from every request; closed by
// its cap where ROUTING says its cap holds it; and its windows' figures
// their use at the moment, faded where they're stale.
func checkCardAgrees(t *testing.T, m status.Moment, rows []RoutingRow) {
	t.Helper()
	if len(rows) != len(m.Accounts)+1 {
		t.Fatalf("the card has %d rows, want auto's and %d accounts'", len(rows), len(m.Accounts))
	}
	for i, a := range m.Accounts {
		row, kind := rows[i+1], a.Routing.Kind
		held := slices.Contains(heldFromEvery, kind)
		if closedRed := !row.Open && !row.Capped; closedRed != held || row.Pickable == held {
			t.Errorf("%s: the card closes it in red: %v, picks it: %v; ROUTING says it's %q", a.ID, closedRed, row.Pickable, a.Routing)
		}
		if row.Capped != (kind == status.RoutingCapped) {
			t.Errorf("%s: the card closes it by its cap: %v; ROUTING says it's %q", a.ID, row.Capped, a.Routing)
		}
		session, _ := a.Window(claudeLike.Started)
		want := Figure{}
		if !drawsEmpty(session.Kind) {
			want = Figure{Used: session.Used, Read: true, Stale: session.Kind == status.WindowStale}
		}
		if row.Session != want {
			t.Errorf("%s: the card's 5-hour figure is %+v, want %+v", a.ID, row.Session, want)
		}
	}
}

// checkCOMINGUPAgrees checks what COMING UP says against the moment m: what
// comes after it, as it has it, in its order, a window running out where the
// moment says that's what comes.
func checkCOMINGUPAgrees(t *testing.T, m status.Moment, things []happening) {
	t.Helper()
	if len(things) != len(m.ComingUp) {
		t.Fatalf("COMING UP says %d things, want the moment's %d", len(things), len(m.ComingUp))
	}
	for i, u := range m.ComingUp {
		if h := things[i]; !h.at.Equal(u.At) || !h.at.After(m.Now) || h.runsOut != (u.Kind == status.UpcomingRunsOut) {
			t.Errorf("COMING UP's %d is at %v, running out %v; want %v, after %v, running out %v", i, h.at, h.runsOut, u.At, m.Now, u.Kind == status.UpcomingRunsOut)
		}
	}
}
