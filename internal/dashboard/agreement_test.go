package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// At each hard moment, the views that say a fact of an account say the same
// of it: how it stands, whether each window is running, how much of it is
// used, whether its reading is stale, where it runs out and when, when it
// next resets, and where new sessions go. Each view's say is read off what
// it draws from, and a disagreement names both views. A view that says a
// fact otherwise by design is left out of it, as its say notes.
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
// ROUTING, the routing card, COMING UP, the status line, and Runway's lanes
// over the day and the week; and the moment, which says which accounts and
// windows there are, and which account and window each thing COMING UP says
// is of, in its order.
type drawn struct {
	now        time.Time
	moment     status.Moment
	usage      usageBlock
	routing    routingTable
	card       []RoutingRow
	coming     []happening
	statusLine statusLine
	day, week  Frame
}

// drawnAt is each view of doc at now, as a frame as wide as the frames draws
// it, and Runway's lanes as a frame of the policy alone draws them.
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
		day:        Frame{Policy: claudeLike, Span: Day},
		week:       Frame{Policy: claudeLike, Span: Week},
	}
	if len(d.coming) != len(m.ComingUp) {
		t.Fatalf("COMING UP says %d things, want the moment's %d", len(d.coming), len(m.ComingUp))
	}
	return d
}

// destination is where each view says new sessions go: the account ▲ marks
// in USAGE, on Runway over the day, and in the status line, and the one
// ROUTING says they come to; "none" where a view marks none. The status line
// of a probed document says nothing of it, nor ROUTING where it says they
// come to none.
func (d drawn) destination() []viewSays {
	usage, runway := "none", "none"
	var routing string
	for i, a := range d.moment.Accounts {
		if strings.Contains(d.usage.rows[i].name.plain(), "▲") {
			usage = a.ID
		}
		if len(d.day.badge(d.day.laneOf(a, d.now))) > 0 {
			runway = a.ID
		}
		if strings.Contains(standingWords(d.routing.accounts[i].standing, false, 200).plain(), "new sessions come here") {
			routing = a.ID
		}
	}
	says := []viewSays{{"USAGE", usage}, {"Runway", runway}}
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
// read of, and the card lets it be picked. Runway's lane over the day says
// it once anything's read of it, but of one whose last read failed, which it
// holds back all along, whatever else does (D7).
func (d drawn) stands(a status.AccountNow) []viewSays {
	i := d.place(a)
	says := []viewSays{
		{"ROUTING", routingStands(d.routing.accounts[i].standing.Kind)},
		{"the routing card", cardStands(d.card[i+1])},
	}
	if l := d.day.laneOf(a, d.now); l.known && a.Error == "" {
		says = append(says, viewSays{"Runway", laneStands(l, d.now)})
	}
	return says
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

// laneStands is how lane l says its account stands at now: taking new
// sessions where it has room, else held back as the first cause of the
// stretch it's in says.
func laneStands(l lane, now time.Time) string {
	for _, s := range l.stretches {
		if !s.holds(now) {
			continue
		}
		switch says := s.causes[0].says; {
		case says == "no token":
			return heldNoToken
		case strings.HasPrefix(says, "limit reached"):
			return heldLimit
		case strings.HasPrefix(says, "refused"):
			return heldRefused
		case strings.HasSuffix(says, "at its reserve"):
			return heldCap
		default:
			return "held: " + says
		}
	}
	return takes
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
// bar, the routing card's figure of the 5-hour window and the week every
// model shares, and Runway's badge of the week over the week; "none" where
// a view draws it empty.
func (d drawn) used(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if g, ok := d.gauge(a, w); ok {
		usage := "none"
		if !g.bar.empty {
			usage = status.Percent(g.bar.used)
		}
		says = append(says, viewSays{"USAGE", usage})
	}
	if f, ok := d.figure(a, w); ok {
		card := "none"
		if f.Read {
			card = status.Percent(f.Used)
		}
		says = append(says, viewSays{"the routing card", card})
	}
	l := d.week.laneOf(a, d.now)
	if weeks := d.week.weeks(l); len(weeks) > 0 && weeks[0].Key == w.Key {
		says = append(says, viewSays{"Runway", d.week.badge(l).plain()})
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
// by the window's projection, stale or not; and Runway's lanes, of a window
// they count, but of one held at its limit, after which a timeline draws
// where it runs out (#59), unless its token is refused, as a stale window
// runs out nowhere ahead either, and of an account whose last read failed
// (D7).
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
	if a.Error != "" || w.Held && !a.TokenRefused {
		return says
	}
	for _, f := range []Frame{d.day, d.week} {
		if f.counts(w) {
			says = append(says, viewSays{"Runway over the " + spanName(f.Span), laneOut(f.laneOf(a, d.now), f.naming(w))})
		}
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
	out, _, _ = strings.Cut(out, status.Separator)
	return "out ~" + out
}

// comingOut is where COMING UP says account a's window w runs out.
func (d drawn) comingOut(a status.AccountNow, w status.WindowNow) string {
	for i, u := range d.moment.ComingUp {
		if u.Account == a.ID && u.Window == w.Key && d.coming[i].runsOut {
			return "out ~" + status.Dated(d.now, d.coming[i].at)
		}
	}
	return "nowhere"
}

// laneOut is where lane l says a window it names as naming runs out.
func laneOut(l lane, naming string) string {
	for _, s := range l.stretches {
		for _, c := range s.causes {
			for _, verb := range []string{"runs out ~", "reaches its reserve ~"} {
				if at, ok := strings.CutPrefix(c.says, naming+verb); ok {
					return "out ~" + at
				}
			}
		}
	}
	return "nowhere"
}

// spanName names Runway's span.
func spanName(s Span) string {
	if s == Week {
		return "week"
	}
	return "day"
}

// resets is when each view says account a's window w next resets: GRAPHS,
// where its chart ends, of one running; Runway over the week, of a week it
// counts; and COMING UP, where it says the window resets.
func (d drawn) resets(a status.AccountNow, w status.WindowNow) []viewSays {
	var says []viewSays
	if ch := chartOf(a, shownWindow{key: w.Key, name: w.Name}, Trail{}, d.now); ch.spanned {
		says = append(says, viewSays{"GRAPHS", status.Dated(d.now, ch.start.Add(ch.length))})
	}
	for _, week := range d.week.weeks(d.week.laneOf(a, d.now)) {
		if week.Key == w.Key {
			at, _ := strings.CutPrefix(resetSays(week, d.now).plain(), "resets ")
			at, _, _ = strings.Cut(at, status.Separator)
			says = append(says, viewSays{"Runway", at})
		}
	}
	for i, u := range d.moment.ComingUp {
		if u.Account == a.ID && u.Window == w.Key && u.Kind == status.UpcomingReset {
			says = append(says, viewSays{"COMING UP", status.Dated(d.now, d.coming[i].at)})
		}
	}
	return says
}
