package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// logCarding is what the watch holds of the Log's details card while it's
// open: the details of the line it's of, a request's or an event's, built
// afresh of what the Log holds at each change, so its figures follow each
// read while the page behind it stands still; and how far its body is
// scrolled, where it scrolls within.
type logCarding struct {
	request *views.RequestDetails
	event   *views.EventDetails
	scroll  int
}

// detailed is the details of the Log's line chosen, as its card holds them:
// a request's, of its row and its line; an event's, of every event the
// Events read and the ledger's lines the Log read. It's false where no line
// is chosen.
func (t logTab) detailed() (logCarding, bool) {
	if line, ok := t.page.ChosenRequest(); ok {
		d, ok := line.Details()
		return logCarding{request: &d}, ok
	}
	_, event, ok := t.page.Chosen()
	if !ok || event == nil {
		return logCarding{}, false
	}
	d, ok := t.page.Events.Details(event.Key, t.read)
	return logCarding{event: &d}, ok
}

// openLogCard opens the details of the Log's line chosen, as enter does,
// where a line is chosen; nothing where none is.
func (m Model) openLogCard() (Model, tea.Cmd) {
	card, ok := m.log.detailed()
	if !ok {
		return m, nil
	}
	m.log.card = card
	return m.placeLogCard(), nil
}

// placeLogCard has the card open over the Log, placed by its line: the
// table scrolled, where the card fits by its line on neither side, so it
// fits under it, as LogFitted has it; the page standing still behind the
// card as it stands now, till a key steps the card on.
func (m Model) placeLogCard() Model {
	f := dashboard.Frame{Width: m.size.Width, Height: m.size.Height, View: dashboard.Log, Log: m.log.page}
	m.log.page = f.LogFitted(m.logCard())
	return m.cover(logCardOpen)
}

// logCardKey acts on a key while the Log's details card is open, which takes
// every key but ctrl+c: ↑ and ↓ step to the line before or after, its
// details in the card, the lit line following and the card placed afresh;
// g, on an event's card, shows Requests paused at the event's moment; esc
// closes the card, the table left paused at the line chosen; j and k scroll
// the card a row, and PgDn and PgUp a page, where it scrolls within; and the
// rest, s among them till a session's page is built, do nothing.
func (m Model) logCardKey(key string) (tea.Model, tea.Cmd) {
	switch {
	case key == "esc":
		m.log.card = logCarding{}
		return m.uncover()
	case key == "up":
		return m.stepLogCard(-1), nil
	case key == "down":
		return m.stepLogCard(1), nil
	case key == "g" && m.log.card.event != nil:
		m.log.page = m.log.page.Landed(m.log.card.event.Row.At, m.logRows())
		m.log.card = logCarding{}
		return m.uncover()
	}
	if by, ok := scrollKey(key); ok {
		f, _, _ := m.screen(m.now())
		m.log.card.scroll = by.from(m.log.card.scroll, f.CardScrolling())
	}
	return m, nil
}

// stepLogCard steps the card by places, ↑ -1 and ↓ 1, to the line before or
// after the one it's of, as the table's choice moves, its details in the
// card, unscrolled, and the card placed by its line afresh.
func (m Model) stepLogCard(by int) Model {
	m.log.page = m.log.page.Chose(by, m.logRows())
	if card, ok := m.log.detailed(); ok {
		m.log.card = card
	}
	return m.placeLogCard()
}

// logCard is the Log's details card as the watch holds it.
func (m Model) logCard() dashboard.LogCard {
	c := m.log.card
	return dashboard.LogCard{Request: c.request, Event: c.event, Scroll: c.scroll}
}

// followedByLogCard is m with the details of the Log's card built afresh of
// what the Log holds, while the card is open, scrolled as it was, so its
// figures follow each read.
func (m Model) followedByLogCard() Model {
	if m.open != logCardOpen {
		return m
	}
	if card, ok := m.log.detailed(); ok {
		card.scroll = m.log.card.scroll
		m.log.card = card
	}
	return m
}
