package dashboard

import (
	"cmp"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// At each hard moment, the views that say a fact of an account say the same
// of it: how it stands, whether each window is running, how much of it is
// used, whether its reading is stale, where it runs out and when, when it
// next resets, and where new sessions go, as the document's best has them go.
// Each view's say is read off what it draws from, and a disagreement names
// both views. A view that says a fact otherwise by design is left out of it,
// as its say notes.
func TestTheViewsAgreeAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			d := drawnAt(t, hm.Doc, hm.Now)
			agree(t, "new sessions go to", d.destination())
			for _, a := range d.moment.Accounts {
				agree(t, a.ID+" stands", d.stands(a))
				for _, w := range a.Windows {
					of := a.ID + "'s " + w.Key
					agree(t, of+" is", d.running(a, w))
					agree(t, of+" is used", d.used(a, w))
					agree(t, of+" is read", d.stale(a, w))
					agree(t, of+" runs out", d.runsOut(a, w))
					agree(t, of+" resets", d.resets(a, w))
				}
			}
		})
	}
}

// viewSays is what a view says of a fact.
type viewSays struct {
	view, says string
}

// agree fails where any view says otherwise than the first that says it.
func agree(t *testing.T, fact string, says []viewSays) {
	t.Helper()
	for _, s := range says[min(len(says), 1):] {
		if s.says != says[0].says {
			t.Errorf("%s: %s says %q, but %s says %q", fact, says[0].view, says[0].says, s.view, s.says)
		}
	}
}

// drawn are the views of a document at a moment, each as it's drawn: USAGE,
// ROUTING, the routing card, COMING UP and the status line; and the moment,
// which says which accounts and windows there are, and which account and
// window each thing COMING UP says is of, in its order.
type drawn struct {
	now        time.Time
	moment     status.Moment
	usage      usageBlock
	routing    routingTable
	card       []RoutingRow
	coming     []happening
	statusLine statusLine
}

// drawnAt is each view of doc at now, as a frame as wide as the frames draws
// it.
func drawnAt(t *testing.T, doc status.Document, now time.Time) drawn {
	t.Helper()
	f := frameOf(160, 40).At(doc, now)
	m := f.Moment(doc, now)
	d := drawn{
		now: now, moment: m,
		usage:      newUsageBlock(m, 160, Look{}, ByWindow, usageWindows(m, nil, labelsOf(m))),
		routing:    f.routingTableOf(doc, now),
		card:       RoutingRows(m, nil, views.Stream{}),
		coming:     f.upcoming(doc, now),
		statusLine: f.pageStatus(doc, now),
	}
	if len(d.coming) != len(m.ComingUp) {
		t.Fatalf("COMING UP says %d things, want the moment's %d", len(d.coming), len(m.ComingUp))
	}
	return d
}

// destination is where each view says new sessions go, after where the
// document's best, the router's choice for a new session, has them go, as the
// moment reads it: the account ▲ marks in USAGE and in the status line, and
// the one ROUTING says they come to; "none" where a view marks none. The
// status line of a probed document says nothing of it, nor ROUTING where it
// says they come to none.
func (d drawn) destination() []viewSays {
	usage, next := "none", cmp.Or(d.moment.Next, "none")
	var routing string
	for i, a := range d.moment.Accounts {
		if strings.Contains(d.usage.rows[i].name.plain(), "▲") {
			usage = a.ID
		}
		if strings.Contains(standingWords(d.routing.accounts[i].standing, false, 200).plain(), "new sessions come here") {
			routing = a.ID
		}
	}
	says := []viewSays{{"the document's best, as the moment reads it", next}, {"USAGE", usage}}
	if routing != "" {
		says = append(says, viewSays{"ROUTING", routing})
	}
	if d.moment.Probed {
		return says
	}
	marked := "none"
	if to, ok := strings.CutPrefix(d.statusLine.destination.plain(), "▲ "); ok {
		marked = d.named(to)
	}
	return append(says, viewSays{"the status line", marked})
}

// named is the id of the account the views name as given: the name itself
// where none is.
func (d drawn) named(n string) string {
	for _, a := range d.moment.Accounts {
		if name(a) == n {
			return a.ID
		}
	}
	return n
}

// stands is how each view says account a stands: taking new sessions, or
// what holds it back from them. ROUTING's "can't read it" is a state it
// says, not a hold: the router keeps a pin to an account nothing has been
// read of, and the card lets it be picked.
func (d drawn) stands(a status.AccountNow) []viewSays {
	i := d.place(a)
	return []viewSays{
		{"ROUTING", routingStands(d.routing.accounts[i].standing.Kind)},
		{"the routing card", cardStands(d.card[i+1])},
	}
}

// place is account a's place among the moment's, from 0.
func (d drawn) place(a status.AccountNow) int {
	return a.Place - 1
}

// What holds an account back from new sessions, as the views agree on it.
const (
	takes       = "takes new sessions"
	heldNoToken = "held: no token"
	heldLimit   = "held: limit"
	heldRefused = "held: token refused"
	heldCap     = "held: cap"
)

// routingStands is how ROUTING's kind k says an account stands.
func routingStands(k status.RoutingKind) string {
	switch k {
	case status.RoutingTokenless:
		return heldNoToken
	case status.RoutingLimited:
		return heldLimit
	case status.RoutingRefused:
		return heldRefused
	case status.RoutingCapped:
		return heldCap
	default:
		return takes
	}
}

// cardStands is how the routing card's row r says its account stands.
func cardStands(r RoutingRow) string {
	switch {
	case r.Open:
		return takes
	case r.Capped:
		return heldCap
	case r.Reason == "no token":
		return heldNoToken
	case strings.HasPrefix(r.Reason, "at its limit"):
		return heldLimit
	case strings.HasPrefix(r.Reason, "token refused"):
		return heldRefused
	default:
		return r.Reason
	}
}

// running is whether each view says account a's window w is running, or
// hasn't started: USAGE's words, GRAPHS' chart, spanned or not, and of the
// window a request starts, ROUTING where it says the account's idle.
func (d drawn) running(a status.AccountNow, w status.WindowNow) []viewSays {
	chart := "running"
	if !chartOf(a, shownWindow{key: w.Key, name: w.Name}, Trail{}, d.now).spanned {
		chart = "not started"
	}
	says := []viewSays{{"GRAPHS", chart}}
	if g, ok := d.gauge(a, w); ok {
		usage := "running"
		if strings.HasPrefix(g.words.full.plain(), "not started") {
			usage = "not started"
		}
		says = append(says, viewSays{"USAGE", usage})
	}
	if w.Started && d.routing.accounts[d.place(a)].standing.Kind == status.RoutingIdle {
		says = append(says, viewSays{"ROUTING", "not started"})
	}
	return says
}

// gauge is USAGE's gauge of account a's window w, reporting false where
// USAGE doesn't show the window.
func (d drawn) gauge(a status.AccountNow, w status.WindowNow) (gauge, bool) {
	for j, shown := range d.usage.windows {
		if shown.key == w.Key {
			return d.usage.rows[d.place(a)].gauges[j], true
		}
	}
	return gauge{}, false
}

// used is how much of account a's window w each view says is used: USAGE's
// bar, and the routing card's figure of the 5-hour window and the week every
// model shares; "none" where a view draws it empty.
func (d drawn) used(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if g, ok := d.gauge(a, w); ok {
		usage := "none"
		if !g.bar.empty {
			usage = prose.Percent(g.bar.used)
		}
		says = append(says, viewSays{"USAGE", usage})
	}
	if f, ok := d.figure(a, w); ok {
		card := "none"
		if f.Read {
			card = prose.Percent(f.Used)
		}
		says = append(says, viewSays{"the routing card", card})
	}
	return says
}

// figure is the routing card's figure of account a's window w, reporting
// false where the card has none of it.
func (d drawn) figure(a status.AccountNow, w status.WindowNow) (Figure, bool) {
	row := d.card[d.place(a)+1]
	switch {
	case w.Started:
		return row.Session, true
	case w.Shared && quota.WeekLong(w.Key):
		return row.Week, true
	default:
		return Figure{}, false
	}
}

// stale is whether each view says account a's window w reads as last read,
// since which its reading has gone stale: USAGE's words, and the routing
// card's figure.
func (d drawn) stale(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if g, ok := d.gauge(a, w); ok {
		says = append(says, viewSays{"USAGE", staleness(strings.HasPrefix(g.words.full.plain(), "not read since "))})
	}
	if f, ok := d.figure(a, w); ok {
		says = append(says, viewSays{"the routing card", staleness(f.Stale)})
	}
	return says
}

// staleness says whether a reading's stale.
func staleness(stale bool) string {
	if stale {
		return "stale"
	}
	return "fresh"
}

// runsOut is where each view says account a's window w runs out, as "out ~"
// and when, or "nowhere": USAGE's words; COMING UP, of an account that could
// take a request as the document was built, as it gates those that couldn't
// (D3); ROUTING's pressure, of the pressure window while it says the
// account's under pressure, but of one stale, as the router's pressure goes
// by the window's projection, stale or not.
func (d drawn) runsOut(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if g, ok := d.gauge(a, w); ok {
		usage := "nowhere"
		if g.runsOut {
			usage = outSaid(g.words.full.plain())
		}
		says = append(says, viewSays{"USAGE", usage})
	}
	if built, _ := d.moment.At(d.moment.ReadAt).Account(a.ID); built.CanRise {
		says = append(says, viewSays{"COMING UP", d.comingOut(a, w)})
	}
	routing := d.routing.accounts[d.place(a)].standing
	if routing.Kind == status.RoutingPressed && w.Pressure && w.Kind != status.WindowStale {
		says = append(says, viewSays{"ROUTING", outSaid(standingWords(routing, false, 200).plain())})
	}
	return says
}

// outSaid is the run-out words say, from "out ~" to the next separator,
// "nowhere" where they say none.
func outSaid(words string) string {
	_, out, ok := strings.Cut(words, "out ~")
	if !ok {
		return "nowhere"
	}
	out, _, _ = strings.Cut(out, prose.Separator)
	return "out ~" + out
}

// comingOut is where COMING UP says account a's window w runs out.
func (d drawn) comingOut(a status.AccountNow, w status.WindowNow) string {
	for i, u := range d.moment.ComingUp {
		if u.Account == a.ID && u.Window == w.Key && d.coming[i].runsOut {
			return "out ~" + prose.Dated(d.now, d.coming[i].at)
		}
	}
	return "nowhere"
}

// resets is when each view says account a's window w next resets: GRAPHS,
// where its chart ends, of one running; and COMING UP, where it says the
// window resets.
func (d drawn) resets(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if ch := chartOf(a, shownWindow{key: w.Key, name: w.Name}, Trail{}, d.now); ch.spanned {
		says = append(says, viewSays{"GRAPHS", prose.Dated(d.now, ch.start.Add(ch.length))})
	}
	for i, u := range d.moment.ComingUp {
		if u.Account == a.ID && u.Window == w.Key && u.Kind == status.UpcomingReset {
			says = append(says, viewSays{"COMING UP", prose.Dated(d.now, d.coming[i].at)})
		}
	}
	return says
}
