package screen

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// primingForm is the Priming page as the screen holds it: the step as it was
// read, its day as it was last read or set; the day as it was read, as its
// field first shows it; the field Your day, and whether it's typed in; and
// why setup refused the day typed, where it did. Why the step failed, where
// it did, is the walk's, as the summary says it.
type primingForm struct {
	set     setup.Primed
	read    string
	day     entry
	typing  bool
	refused string
}

// page is the Priming page as dashboard.Setup draws it.
func (f primingForm) page() *dashboard.SetupPrimingPage {
	return &dashboard.SetupPrimingPage{Day: f.day.field(f.typing), Refused: f.refused}
}

// typed is what's typed in the field, without the blanks round it, as a
// field of blanks is left empty.
func (f primingForm) typed() string {
	return strings.TrimSpace(f.day.value())
}

// primedMsg is the Priming step as it read it on reaching it, or why it
// couldn't.
type primedMsg struct {
	primed setup.Primed
	err    error
}

// readDay is the Priming step's action: Priming.
func readDay(steps Steps) job {
	return func(context.Context) tea.Msg {
		primed, err := steps.Priming()
		return primedMsg{primed: primed, err: err}
	}
}

// primed takes in the step as it read it, as done: skipped, moving on, as
// movedOn has it; else its field holding the day, typed in, none where
// priming is off. Where it couldn't be read, the step failed.
func (m Model) primed(msg primedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.walk = m.walk.finished(msg.err)
		return m, nil
	}
	var read string
	if day := msg.primed.Day; day != (config.Day{}) {
		read = day.String()
	}
	m.priming = &primingForm{set: msg.primed, read: read, day: entryOf(read), typing: true}
	m.walk = m.walk.finished(nil)
	if msg.primed.Skipped {
		return m.movedOn()
	}
	return m, nil
}

// dayEntered acts on enter at the Priming page, once its day is read: a day
// as it was read moves on, as nothing's to write; one config.ParseDay
// refuses is said under the field, as refusal has it, setup staying on the
// page; any other, the field cleared among them, is set with SetDay, the
// step under way till it's set.
func (m Model) dayEntered() (tea.Model, tea.Cmd) {
	typed := m.priming.typed()
	if typed == m.priming.read {
		return m.movedOn()
	}
	if _, err := config.ParseDay(typed); err != nil {
		f := *m.priming
		f.refused = refusal(err)
		m.priming = &f
		return m, nil
	}
	m.walk = m.walk.keeping()
	m.walk.done = false
	return m, m.jobs.write(m.walk.keep(setDay(m.steps, typed)))
}

// refusal is why config refused a day, made safe to show: why the value is
// wrong alone, where config says, as the field above it shows what's typed.
func refusal(err error) string {
	if wrong, ok := errors.AsType[*config.WrongValue](err); ok {
		return prose.Clean(wrong.Why)
	}
	return prose.Clean(err.Error())
}

// daySetMsg is the day SetDay set, or why it failed.
type daySetMsg struct {
	day config.Day
	err error
}

// setDay is the day set with SetDay, as dayEntered has it.
func setDay(steps Steps, day string) job {
	return func(context.Context) tea.Msg {
		set, err := steps.SetDay(day)
		return daySetMsg{day: set, err: err}
	}
}

// daySet moves on once the day is set, holding it; where SetDay failed, the
// step failed.
func (m Model) daySet(msg daySetMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.walk = m.walk.finished(msg.err)
		return m, nil
	}
	f := *m.priming
	f.set.Day = msg.day
	m.priming = &f
	return m.movedOn()
}

// typing reports whether the Priming page's field is typed in, taking the
// keys that edit it: from its day's being read, till esc gives it up, but
// while the day typed is being set, when the keys act as on any page under
// way, q quitting; or till the step fails.
func (m Model) typing() bool {
	return m.walk.step == dashboard.SetupPriming && m.priming != nil && m.priming.typing && m.walk.done && m.walk.failed == nil
}

// typedIn acts on a key while the field is typed in: ctrl+c leaves; enter
// moves on, as entered has it; esc gives the field up, for q to leave and j
// and k to scroll; and the field takes the keys that edit it, as textfield
// has them, j, k and q typing, so PgUp and PgDn alone scroll the page.
func (m Model) typedIn(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); k {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.entered()
	case "esc":
		return m.focused(false), nil
	}
	if day, ok := m.priming.day.pressed(key); ok {
		return m.edited(day), nil
	}
	if rows, pages, ok := dashboard.ScrollKey(key.String()); ok {
		return m.scrolled(rows, pages), nil
	}
	return m, nil
}

// pasted is m with text pasted into the field typed in: the add card's, the
// edit card's, the paste card's or the reserve card's, where one's open, as
// each card's pasted has it; or the Priming page's.
func (m Model) pasted(text string) Model {
	switch {
	case m.newPool != nil:
		if !m.newPool.checking {
			card := m.newPool.pasted(text)
			m.newPool = &card
		}
	case m.add != nil && !m.add.checking:
		card := m.add.pasted(text)
		m.add = &card
	case m.edit != nil && !m.edit.saving:
		card := m.edit.pasted(text)
		m.edit = &card
	case m.paste != nil && !m.paste.checking:
		card := m.paste.pasted(text)
		m.paste = &card
	case m.reserve != nil && !m.reserve.saving:
		card := m.reserve.pasted(text)
		m.reserve = &card
	case m.typing():
		return m.edited(m.priming.day.pasted(text))
	}
	return m
}

// edited is m with the field holding day, the day it refused unsaid once
// it's changed.
func (m Model) edited(day entry) Model {
	f := *m.priming
	if day.value() != f.day.value() {
		f.refused = ""
	}
	f.day = day
	m.priming = &f
	return m
}

// focused is m with the field typed in, or given up, as typing says: on the
// Priming page, once its day is read and while the step hasn't failed.
func (m Model) focused(typing bool) Model {
	if m.walk.step != dashboard.SetupPriming || m.priming == nil || m.walk.failed != nil {
		return m
	}
	f := *m.priming
	f.typing = typing
	m.priming = &f
	return m
}
