package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// momentsLedger is a moment's lines as the ledger reads a session's page's:
// each session read whole, every line of it, by its id, oldest first, and
// each line handed to take with its day's date. A page reads nothing else of
// it.
type momentsLedger struct {
	views.Ledger
	lines []ledger.Line
}

// SessionsOf reads as Sessions does: a moment's sessions are all there are.
func (l momentsLedger) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(date string, l ledger.Line)) map[string]ledger.SessionFrom {
	return l.Sessions(ids, rest, take)
}

func (l momentsLedger) Sessions(_ []string, _ func(string) ledger.Rest, take func(date string, l ledger.Line)) map[string]ledger.SessionFrom {
	sessions := make(map[string][]ledger.Line)
	for _, line := range l.lines {
		if take != nil {
			take(line.At.Local().Format(time.DateOnly), line)
		}
		sessions[line.Session] = append(sessions[line.Session], line)
	}
	return ledger.SessionsWhole(sessions)
}

// momentsShown is the page of the session with the given id at the moment
// hm, as the views build it, held and live, and its story, the newest turn
// chosen.
func momentsShown(hm viewstest.SessionMoment, id string) (*SessionShown, bool) {
	running := hm.Sessions
	if hm.Unlisted {
		running = nil
	}
	held := views.HoldPage(id, views.ReadSessions(momentsLedger{lines: hm.Lines}, running, hm.Now, id), ledger.Pricing, 0, hm.Now)
	live := views.PageLive{Routed: views.Routed(running, id), Unlisted: hm.Unlisted, Stream: hm.Stream, Now: hm.Now}
	page, ok := held.Page(live)
	if !ok {
		return nil, false
	}
	shown := &SessionShown{Page: page, Story: held.Story(page, live)}
	if len(page.Turns) > 0 {
		shown.Turn = page.Turns[0].Turn
	}
	return shown, true
}

// At each moment the views have to tell what a session is doing at, its
// page draws what the views say of it, as SESSIONS draws what they say of
// each session running: on row 6, what it's doing and where it runs, and
// when it started and how it has run; on row 7, where its models go, or
// once it's over, its models and how long its lines are kept; a row each of
// the accounts it ran on, its time there and its requests; a row each of its
// turns, its number and how long it took; and ↑ asking on its newest turn
// while it waits, and only then. A phone's two rows of now say the same,
// short, at each moment too.
func TestASessionsPageSaysWhatItsViewsSayAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			for _, id := range slices.Sorted(maps.Keys(hm.Page)) {
				shown, ok := momentsShown(hm, id)
				if !ok {
					t.Fatalf("%s has no page", status.ShownID(id))
				}
				f := frameOf(160, 40)
				f.Sessions, f.Stream, f.Session = hm.Sessions, hm.Stream, shown
				at := f.At(hm.Doc, hm.Now)
				c, d := at.page(hm.Doc, hm.Now), at.sessionDrawn(hm.Doc, hm.Now)
				checkNowRows(t, c, d.l, shown.Page, hm.Now)
				checkAccountRows(t, c, d, shown.Page, hm.Now)
				checkTurnRows(t, c, d, shown.Page, hm.Now)

				phone := frameOf(52, 32)
				phone.Sessions, phone.Stream, phone.Session = hm.Sessions, hm.Stream, shown
				checkPhoneNowRows(t, phone.At(hm.Doc, hm.Now).page(hm.Doc, hm.Now), phone.sessionLayout(len(shown.Page.Accounts)), shown.Page, hm.Now)
			}
		})
	}
}

// unmarked takes the marks after an account's name out of a row, as the
// views' words don't say them.
var unmarked = strings.NewReplacer(" ▲", "", " ◆", "")

// checkNowRows checks that the page's rows 6 and 7, drawn on c as l lays
// them out, say what the views say of page p at now.
func checkNowRows(t *testing.T, c *canvas, l sessionLayout, p views.SessionPage, now time.Time) {
	t.Helper()
	shown := status.ShownID(p.Session)
	said, goes := unmarked.Replace(rowOf(c, l.now)), rowOf(c, l.now+1)
	if want := p.Activity.Said(now) + "   " + p.Where(now).Words().String(); !strings.HasPrefix(said, strings.TrimRight(want, " ")) {
		t.Errorf("%s's row 6 reads\n%q\nwant it to say\n%q", shown, said, want)
	}
	if !p.Started.IsZero() && !strings.HasSuffix(said, " "+p.StartedSaid(now)+" · "+p.RanSaid(now)) {
		t.Errorf("%s's row 6 reads %q, want it to end with when it started, %q, and how it has run, %q", shown, said, p.StartedSaid(now), p.RanSaid(now))
	}
	var models []string
	for _, g := range p.Goes {
		if p.Runs() {
			models = append(models, ledger.ShortModelID(g.Model)+" → "+g.Account)
		} else {
			models = append(models, ledger.ShortModelID(g.Model))
		}
	}
	want := strings.Join(models, "   ")
	if !p.Runs() {
		want = strings.Join(models, " · ") + "   " + views.KeptUntil(p.KeptUntil, now)
	}
	if !strings.HasPrefix(goes, want) {
		t.Errorf("%s's row 7 reads\n%q\nwant it to say\n%q", shown, goes, want)
	}
}

// checkPhoneNowRows checks that a phone's two rows of now, drawn on c as l
// lays them out, say what the views say of page p at now, each cut with …
// where it doesn't fit: what it's doing, then where it runs, since when; and
// why, short, then at the right how it has run.
func checkPhoneNowRows(t *testing.T, c *canvas, l sessionLayout, p views.SessionPage, now time.Time) {
	t.Helper()
	shown := status.ShownID(p.Session)
	said, why := unmarked.Replace(rowOf(c, l.now)), rowOf(c, l.now+1)
	want := p.Activity.Said(now)
	if where := p.Where(now).OnSince.String(); where != "" {
		want += "  " + where
	}
	if !saysCut(said, want) {
		t.Errorf("%s's phone row 7 reads\n%q\nwant it to say\n%q", shown, said, want)
	}
	ran := p.RanSaid(now)
	short := strings.TrimRight(strings.TrimSuffix(why, ran), " ")
	if !strings.HasSuffix(why, ran) || !saysCut(short, p.Where(now).Short.String()) {
		t.Errorf("%s's phone row 8 reads\n%q\nwant it to say\n%q\nthen at the right %q", shown, why, p.Where(now).Short.String(), ran)
	}
}

// saysCut reports whether a row drawn says want, whole, or cut with … where
// it doesn't fit.
func saysCut(drawn, want string) bool {
	return drawn == want || strings.HasSuffix(drawn, prose.Ellipsis) && strings.HasPrefix(want, strings.TrimSuffix(drawn, prose.Ellipsis))
}

// checkAccountRows checks that the page's rows of the accounts it ran on,
// drawn on c as d lays them out, say what the views say of each of page p's
// stays at now.
func checkAccountRows(t *testing.T, c *canvas, d sessionDrawn, p views.SessionPage, now time.Time) {
	t.Helper()
	for i, s := range p.Stays(now) {
		row := []rune(rowOf(c, d.l.accountsAt+i) + strings.Repeat(" ", c.width))
		if !strings.Contains(stayText(row, d, stayName), " "+s.Account) || stayText(row, d, stayTime) != s.Said ||
			stayText(row, d, stayRequests) != prose.Counted(s.Requests, "request") {
			t.Errorf("%s's row of %s reads\n%q\nwant its time there, %q, and its %d requests", status.ShownID(p.Session), s.Account, strings.TrimRight(string(row), " "), s.Said, s.Requests)
		}
	}
}

// checkTurnRows checks that the page's rows of its turns, drawn on c as d
// lays them out, say what the views say of page p's turns at now: each its
// number and how long it took, "…" after it while it goes; and ↑ asking on
// the newest, while p waits, and only then.
func checkTurnRows(t *testing.T, c *canvas, d sessionDrawn, p views.SessionPage, now time.Time) {
	t.Helper()
	shown := status.ShownID(p.Session)
	for i, turn := range p.Turns[:min(len(p.Turns), d.l.turns)] {
		row := []rune(rowOf(c, d.l.turnsAt+i) + strings.Repeat(" ", c.width))
		took, going := turn.Took(now)
		if going {
			took += prose.Ellipsis
		}
		if number := turnText(row, d, turnNumber); number != prose.Grouped(turn.Turn) || turnText(row, d, turnTook) != took {
			t.Errorf("%s's row of turn %d reads\n%q\nwant its number and its took, %q", shown, turn.Turn, strings.TrimRight(string(row), " "), took)
		}
		asking := turnText(row, d, turnAsking)
		if want := i == 0 && p.Activity.Waiting(); (asking == "↑ asking") != want {
			t.Errorf("%s's row of turn %d ends %q, want ↑ asking: %t, as it waits: %t", shown, turn.Turn, asking, want, p.Activity.Waiting())
		}
	}
}
