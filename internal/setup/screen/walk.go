package screen

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/service"
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
// Summary is how setup stands as it's left, once every job of the screen's
// has ended, as Done says it a line a step, for the caller to print once the
// screen has gone. Unsaved is how each of the tokens' makings that saved
// nothing, nor was stopped, left setup, as Model.tokenLeft has it: above all
// every token made that isn't known saved, which lasts a year on the
// account, for the caller to show, as the screen can't. Interrupted says an interrupt was held as a token made was
// saved, for the caller to end as an interrupt has it.
type Left struct {
	Ending      Ending
	Err         error
	Summary     setup.Summary
	Unsaved     []setup.Tokened
	Interrupted bool
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
	// kept holds the outcome of the job the walk started last, as well as
	// its being given to the model, for Run to take in should the screen go
	// before the model does, as Model.gone has it.
	kept chan tea.Msg
}

// actions are what a step does as it's reached, of the steps that do
// something: any other is done on reaching it.
var actions = map[dashboard.SetupStep]func(Steps) job{
	dashboard.SetupPriming: readDay,
	dashboard.SetupService: serviceStep,
	dashboard.SetupLink:    linkClaude,
	dashboard.SetupSkill:   skill,
}

// openingOn is the walk opening on step, the steps before it passed
// through: a step before the first is the first, and one past Done is Done.
func openingOn(step dashboard.SetupStep) walk {
	step = min(max(step, dashboard.SetupAccounts), dashboard.SetupDone)
	return walk{step: step, passed: dashboard.SetupSteps()[:step]}.reached(step)
}

// reached is w at step, which is done on reaching it where it does nothing:
// Accounts does, as it reads the accounts the status line reads, as
// reviewed has it.
func (w walk) reached(step dashboard.SetupStep) walk {
	_, acts := actions[step]
	w.step, w.done, w.failed = step, !acts && step != dashboard.SetupAccounts, nil
	return w.keeping()
}

// keeping is w with a place of its own to keep the outcome of the job it
// starts next.
func (w walk) keeping() walk {
	w.kept = make(chan tea.Msg, 1)
	return w
}

// keep is j, its outcome kept on w's place as well as given, where it gives
// one. The place is j's alone, as keeping makes one for each job, so it's
// free; were it not, the outcome would go unkept rather than hold j, and
// with it Run, which waits for every job, for good.
func (w walk) keep(j job) job {
	kept := w.kept
	return func(ctx context.Context) tea.Msg {
		msg := j(ctx)
		if msg != nil {
			select {
			case kept <- msg:
			default:
			}
		}
		return msg
	}
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

// finishingSaid is what setup says, once its screen has gone, while the
// service step's swap runs on, waiting as long as the router it replaces
// takes to end its requests.
var finishingSaid = "Setup is finishing the " + dashboard.SetupService.Title() + " step: the router it replaces ends its requests first, which can take a minute or two."

// finishingService reports whether w stands at the service step while it's
// under way.
func (w walk) finishingService() bool {
	return w.step == dashboard.SetupService && !w.done
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
		return m.jobs.write(m.walk.keep(act(m.steps)))
	}
	return nil
}

// entered moves on, once the step showing is done, unless it failed: at
// Done, leaving to open the dashboard; at Accounts, as accountsEntered has
// it; at Priming, once its day is set, as dayEntered has it; at the claude
// link, as linkEntered has it; else as movedOn has it.
func (m Model) entered() (tea.Model, tea.Cmd) {
	switch w := m.walk; {
	case !w.done || w.failed != nil:
		return m, nil
	case w.step == dashboard.SetupDone:
		m.walk.toDashboard = true
		return m, tea.Quit
	case w.step == dashboard.SetupAccounts:
		return m.accountsEntered()
	case w.step == dashboard.SetupPriming:
		return m.dayEntered()
	case w.step == dashboard.SetupLink:
		return m.linkEntered()
	}
	return m.movedOn()
}

// movedOn is m at the next step, its page from the top, running its action,
// its spinner moving while it runs.
func (m Model) movedOn() (tea.Model, tea.Cmd) {
	m.walk, m.scroll, m.spins = m.walk.onward(), 0, m.spins+1
	return m, tea.Batch(m.action(), m.armSpin())
}

// servicedMsg is what the service step told, and the router's health, or why
// it failed.
type servicedMsg struct {
	serviced setup.Serviced
	err      error
}

// serviceStep is the service step's action: Service, stopping once its
// context is done, as servicedOf has it.
func serviceStep(steps Steps) job {
	return func(ctx context.Context) tea.Msg {
		serviced, err := steps.Service(ctx)
		return servicedOf(ctx, serviced, err)
	}
}

// servicedOf is what the service step told, as serviced and err say, once
// it has ended under ctx. Once ctx is done, as setup's leaving does, it
// counts only where the step's swap of the service began, as the swap runs
// to its end: one that failed fails the step; one done is said by what the
// step did, its router's answer unknown, not as a failure, as is one an
// interrupt held through the swap cut short; and anything before the swap
// is dropped, the step said of as not reached. A restart of the router
// before the swap, as the step may make, is outside this, as the account
// changes are.
func servicedOf(ctx context.Context, serviced setup.Serviced, err error) tea.Msg {
	cut := ctx.Err() != nil || errors.Is(err, accounts.ErrInterrupted)
	switch {
	case cut && serviced.Swap == service.Swapped:
		err = nil
	case ctx.Err() != nil && serviced.Swap == service.NotSwapped:
		return nil
	}
	return servicedMsg{serviced: serviced, err: err}
}

// serviced takes in what the service step told, each detail made safe to
// show, as done, or failed as its error says; and asks for the status line
// again, as the step may have started the router.
func (m Model) serviced(msg servicedMsg) (Model, tea.Cmd) {
	serviced := msg.serviced
	serviced.Said = slices.Clone(serviced.Said)
	for i := range serviced.Said {
		serviced.Said[i].Detail = prose.Clean(serviced.Said[i].Detail)
	}
	m.service = &serviced
	m.walk = m.walk.finished(msg.err)
	return m.askAgain()
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
