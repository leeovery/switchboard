package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// burnCards draws the cards of the burn-down section placed, of the moment
// m, as laid out: each configured account's, in the config's order,
// numbered as graphsNamed has it, where laid's cards places it, as
// burnCardDrawn draws it, so one the moment doesn't hold yet, having no
// lane, draws its label alone, no words, as a card does before its lanes
// are built, and its chart empty but for now's line; none where the
// moment's policy shares no window of the section's.
func (f Frame) burnCards(c *canvas, m status.Moment, laid graphsLaid, placed graphsPlaced) {
	window := burnDowns[placed.section]
	key := window.KeyIn(m.Policy())
	if key == "" {
		return
	}
	s := burnSection{key: key, lanes: f.Graphs.Lanes[window], beside: !laid.apart, now: m.Now}
	accounts, names := graphsNamed(m, f.Graphs.Accounts)
	for i, at := range laid.cards(placed) {
		l, built := s.lanes.lane(accounts[i].ID)
		f.burnCardDrawn(c, s, laneLaid(l, accounts[i], names[i]), built, at)
	}
}

// burnSection is what a burn-down section's cards are drawn by: the key of
// its window in the moment's policy; Runway's lanes over its live view of
// that window, as the watch built them; whether each card's words are
// beside its label, as the step of room has them; and now.
type burnSection struct {
	key    string
	lanes  WindowLanes
	beside bool
	now    time.Time
}

// burnCardDrawn draws the card of the lane ln's account's window in the
// section s where at places it: its head, as cardHeadOf has it, its words
// beside its label where s says so, else on the row under; or, where the
// lane isn't built yet, its label alone, as within cuts it. Then its chart
// and its axis, as burnCardOf has them from its trail, its ✕ at the start of
// a hold as the lane's HeldSince has it, none where nothing tells.
func (f Frame) burnCardDrawn(c *canvas, s burnSection, ln laidLane, built bool, at cardLaid) {
	width := at.chart.width
	if built {
		f.cardHeadOf(s.lanes.view(), ln, width, s.beside, s.now).draw(c, f.Look, at.chart.x, at.label)
	} else {
		c.line(at.chart.x, at.label, ln.name.within(width))
	}
	card := burnCardOf(ln.account, s.key, f.Trails[Ref{Account: ln.ID, Window: s.key}], s.now)
	card.heldSince = ln.HeldSince(ln.account, s.key, s.lanes.Timeline)
	card.draw(c, f.Look, at.chart)
}

// lane is the lane of the account with the given id among the lanes,
// reporting whether they hold one: else a lane of that account alone, with
// nothing judged in it, as before the watch builds them.
func (w WindowLanes) lane(id string) (views.Lane, bool) {
	if w.Lanes != nil {
		if i := slices.IndexFunc(w.Lanes.Accounts, func(l views.Lane) bool { return l.ID == id }); i >= 0 {
			return w.Lanes.Accounts[i], true
		}
	}
	return views.Lane{ID: id}, false
}
