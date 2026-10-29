package screen

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/textfield"
	"github.com/leeovery/switchboard/internal/prose"
)

// primingForm is the Priming page as the screen holds it: the day as it was
// read, as its field shows it; the field Your day, and whether it's typed
// in; why setup refused the day typed, where it did; and why the step
// failed, where it did.
type primingForm struct {
	read    string
	day     textfield.Text
	typing  bool
	refused string
	failure string
}

// page is the Priming page as dashboard.Setup draws it.
func (f primingForm) page() *dashboard.SetupPrimingPage {
	day := dashboard.Field{Value: f.day.Value(), Cursor: f.day.Cursor(), Typing: f.typing}
	return &dashboard.SetupPrimingPage{Day: day, Refused: f.refused, Failure: f.failure}
}

// typed is what's typed in the field, without the blanks round it, as a
// field of blanks is left empty.
func (f primingForm) typed() string {
	return strings.TrimSpace(f.day.Value())
}

// primedMsg is the day priming spreads the resets over, as the step read it
// on reaching it, or why it couldn't.
type primedMsg struct {
	day config.Day
	err error
}

// readDay is the Priming step's action: Priming.
func readDay(steps Steps) job {
	return func(context.Context) tea.Msg {
		day, err := steps.Priming()
		return primedMsg{day: day, err: err}
	}
}

// primed takes in the day the step read, its field holding it, typed in, as
// done; none where priming is off. Where it couldn't be read, the step
// failed.
func (m Model) primed(msg primedMsg) Model {
	if msg.err != nil {
		return m.primingFailed(msg.err)
	}
	var read string
	if msg.day != (config.Day{}) {
		read = msg.day.String()
	}
	m.priming = &primingForm{read: read, day: textfield.Of(read), typing: true}
	m.walk = m.walk.finished(nil)
	return m
}

// primingFailed is m with the Priming step failed, as err says, its field
// given up.
func (m Model) primingFailed(err error) Model {
	var f primingForm
	if m.priming != nil {
		f = *m.priming
	}
	f.typing, f.failure = false, prose.Clean(err.Error())
	m.priming = &f
	m.walk = m.walk.finished(err)
	return m
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
	m.walk.done = false
	return m, m.jobs.write(setDay(m.steps, typed))
}

// refusal is why config refused a day, made safe to show: why the value is
// wrong alone, where config says, as the field above it shows what's typed.
func refusal(err error) string {
	if wrong, ok := errors.AsType[*config.WrongValue](err); ok {
		return prose.Clean(wrong.Why)
	}
	return prose.Clean(err.Error())
}

// daySetMsg is why SetDay failed: nil where it set the day.
type daySetMsg struct {
	err error
}

// setDay is the day set with SetDay, as dayEntered has it.
func setDay(steps Steps, day string) job {
	return func(context.Context) tea.Msg {
		_, err := steps.SetDay(day)
		return daySetMsg{err: err}
	}
}

// daySet moves on once the day is set; where SetDay failed, the step failed.
func (m Model) daySet(msg daySetMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.primingFailed(msg.err), nil
	}
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
	if day, ok := m.priming.day.Pressed(key); ok {
		return m.edited(day), nil
	}
	if rows, pages, ok := dashboard.ScrollKey(key.String()); ok {
		return m.scrolled(rows, pages), nil
	}
	return m, nil
}

// pasted is m with text pasted into the field where it's typed in.
func (m Model) pasted(text string) Model {
	if !m.typing() {
		return m
	}
	return m.edited(m.priming.day.Pasted(text))
}

// edited is m with the field holding day, the day it refused unsaid once
// it's changed.
func (m Model) edited(day textfield.Text) Model {
	f := *m.priming
	if day.Value() != f.day.Value() {
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
