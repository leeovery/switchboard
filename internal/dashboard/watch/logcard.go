package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// openLogCard opens the details of the Log's line chosen, as enter does:
// nothing yet, the card being built after the page.
func (m Model) openLogCard() (Model, tea.Cmd) {
	return m, nil
}

// logCardKey acts on a key while the Log's details card is open.
func (m Model) logCardKey(string) (tea.Model, tea.Cmd) {
	return m, nil
}

// logCard is the Log's details card, as the model stands at now.
func (m Model) logCard(time.Time) dashboard.Opened {
	return dashboard.LogCard{}
}

// frozenPage is the page behind a card as it stood when the card opened,
// f, with what of it the card moves taken from the model as it stands: as
// yet, nothing.
func (m Model) frozenPage(f dashboard.Frame) dashboard.Frame {
	return f
}
