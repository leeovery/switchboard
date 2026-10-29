package screen

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// Ending is how setup's screen was left.
type Ending int

const (
	// Paused is setup left before Done: run again, it carries on as each
	// step finds things.
	Paused Ending = iota
	// Finished is setup left at Done, without opening the dashboard.
	Finished
	// ToDashboard is setup left at Done by enter, to open the dashboard.
	ToDashboard
	// Failed is setup left at a step that failed, which stopped it there.
	Failed
)

// Left is how setup's screen was left, and for Failed, why the step failed.
type Left struct {
	Ending Ending
	Err    error
}

// walk is where setup stands in its steps: the step showing, the steps
// passed through, and how the step showing stands.
type walk struct {
	step   dashboard.SetupStep
	passed []dashboard.SetupStep
	// done is set once the step showing has done what it does, and failed is
	// why it failed, where it did, which stops setup there.
	done   bool
	failed error
	// toDashboard is set where enter left Done, for the dashboard to open.
	toDashboard bool
}

// actions are what a step does as it's reached, of the steps that do
// something: any other is done on reaching it.
var actions = map[dashboard.SetupStep]func(Steps) job{
	dashboard.SetupPriming: readDay,
	dashboard.SetupSkill:   skill,
}

// openingOn is the walk opening on step, the steps before it passed
// through: a step before the first is the first, and one past Done is Done.
func openingOn(step dashboard.SetupStep) walk {
	step = min(max(step, dashboard.SetupAccounts), dashboard.SetupDone)
	return walk{step: step, passed: dashboard.SetupSteps()[:step]}.reached(step)
}

// reached is w at step, which is done on reaching it where it does nothing.
func (w walk) reached(step dashboard.SetupStep) walk {
	_, acts := actions[step]
	w.step, w.done, w.failed = step, !acts, nil
	return w
}

// onward is w at the step after the one showing, which is passed through.
func (w walk) onward() walk {
	w.passed = append(slices.Clip(w.passed), w.step)
	return w.reached(w.step + 1)
}

// finished is w with the step showing done, failed as err says.
func (w walk) finished(err error) walk {
	w.done, w.failed = true, err
	return w
}

// left is how setup is left from where w stands: failing, at a step that
// failed; at Done, to the dashboard where enter left it, else without it;
// else paused.
func (w walk) left() Left {
	switch {
	case w.failed != nil:
		return Left{Ending: Failed, Err: w.failed}
	case w.step != dashboard.SetupDone:
		return Left{Ending: Paused}
	case w.toDashboard:
		return Left{Ending: ToDashboard}
	}
	return Left{Ending: Finished}
}

// action is the command that does what the step showing does, as it's
// reached: setup's writing actions, as every step's is, run one at a time.
func (m Model) action() tea.Cmd {
	if act, ok := actions[m.walk.step]; ok {
		return m.jobs.write(act(m.steps))
	}
	return nil
}

// entered moves on, once the step showing is done, unless it failed: at
// Done, leaving to open the dashboard; at Priming, once its day is set, as
// dayEntered has it; else as movedOn has it.
func (m Model) entered() (tea.Model, tea.Cmd) {
	switch w := m.walk; {
	case !w.done || w.failed != nil:
		return m, nil
	case w.step == dashboard.SetupDone:
		m.walk.toDashboard = true
		return m, tea.Quit
	case w.step == dashboard.SetupPriming:
		return m.dayEntered()
	}
	return m.movedOn()
}

// movedOn is m at the next step, its page from the top, running its action.
func (m Model) movedOn() (tea.Model, tea.Cmd) {
	m.walk, m.scroll = m.walk.onward(), 0
	return m, m.action()
}

// skilledMsg is what the skill step did, or why it failed.
type skilledMsg struct {
	said setup.Said
	err  error
}

// skill is the skill step's action: Skill.
func skill(steps Steps) job {
	return func(context.Context) tea.Msg {
		said, err := steps.Skill()
		return skilledMsg{said: said, err: err}
	}
}

// skilled takes in what the skill step did, its detail made safe to show, as
// done, or failed as its error says.
func (m Model) skilled(msg skilledMsg) Model {
	said := msg.said
	said.Detail = prose.Clean(said.Detail)
	m.skill = &said
	m.walk = m.walk.finished(msg.err)
	return m
}
