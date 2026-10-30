package screen

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// linkedMsg is what the claude link step found, as Link or AskAgain found
// it, or why it failed.
type linkedMsg struct {
	found setup.LinkFound
	err   error
}

// linkClaude is the claude link step's action: Link, stopping once its
// context is done.
func linkClaude(steps Steps) job {
	return func(ctx context.Context) tea.Msg {
		found, err := steps.Link(ctx)
		return linkedMsg{found: found, err: err}
	}
}

// asking reports whether the claude link step asks the user's login shell
// now which claude it runs: under way, before it has ended.
func (m Model) asking() bool {
	w := m.walk
	return w.step == dashboard.SetupLink && !w.done && !m.linkEnded
}

// linked takes in what the claude link step found, as found and made safe
// to show, where it found it: where that's final, the shell running
// switchboard's claude or setup unable to ask it, the step ends, as ending
// has it; anywhere else, the step done, its page showing how to put it
// right by hand. Where it failed, the step failed, as its error says, what
// it found with the error never said, as it isn't what the step left; but
// an interrupt from outside the terminal as it asked leaves setup, as q
// does.
func (m Model) linked(msg linkedMsg) (Model, tea.Cmd) {
	switch {
	case m.walk.step != dashboard.SetupLink:
		return m, nil
	case errors.Is(msg.err, accounts.ErrInterrupted):
		return m, tea.Quit
	case msg.err != nil:
		m.walk = m.walk.finished(msg.err)
		return m, nil
	}
	found := msg.found
	m.found, m.link = &found, cleanLink(&found)
	if s := found.Shell.Situation; s == setup.Through || s == setup.Unreached {
		return m.ending()
	}
	m.walk = m.walk.finished(nil)
	return m, nil
}

// checkable reports whether r asks the shell again: at the claude link step,
// done, before it has ended, where what it found leaves something to put
// right by hand that asking again can tell is.
func (m Model) checkable() bool {
	w := m.walk
	return w.step == dashboard.SetupLink && w.done && w.failed == nil && !m.linkEnded && m.found != nil && m.found.Shell.Remedy().Checkable
}

// checkingAgain asks the shell again which claude it runs, as AskAgain does,
// from what the step found, the step under way till it answers, its page
// saying so, its spinner moving.
func (m Model) checkingAgain() (Model, tea.Cmd) {
	steps, found := m.steps, *m.found
	m.walk.done, m.spins = false, m.spins+1
	again := m.jobs.read(func(ctx context.Context) tea.Msg {
		again, err := steps.AskAgain(ctx, found)
		return linkedMsg{found: again, err: err}
	})
	return m, tea.Batch(again, m.armSpin())
}

// linkEntered moves on to the Skill once the claude link step has ended;
// before then, enter leaves what's left by hand for now, ending the step.
func (m Model) linkEntered() (tea.Model, tea.Cmd) {
	if m.linkEnded {
		return m.movedOn()
	}
	return m.ending()
}

// hookedMsg is what the session hook's install said.
type hookedMsg struct {
	said setup.Said
}

// ending ends the claude link step, installing the session hook, a writing
// action, once whatever the step found: the step is under way till it's
// installed, as hooked has it, so the walk moves on, and the Skill writes,
// only once it has.
func (m Model) ending() (Model, tea.Cmd) {
	m.linkEnded, m.walk.done = true, false
	steps := m.steps
	return m, m.jobs.write(func(context.Context) tea.Msg {
		return hookedMsg{said: steps.SessionHook()}
	})
}

// hooked takes in what the session hook's install said, its detail made
// safe to show, the claude link step, which waits on it, done.
func (m Model) hooked(msg hookedMsg) Model {
	said := msg.said
	said.Detail = prose.Clean(said.Detail)
	m.hook = &said
	m.walk = m.walk.finished(nil)
	return m
}

// cleanLink is found, what the claude link step found, what it shows made
// safe to show, what it offers to copy keeping its spacing, as copied: nil
// where it's nil. File is left as it is, as setup writes to it.
func cleanLink(found *setup.LinkFound) *setup.LinkFound {
	if found == nil {
		return nil
	}
	cleaned := *found
	cleaned.Led = prose.Clean(found.Led)
	cleaned.Said = slices.Clone(found.Said)
	for i := range cleaned.Said {
		cleaned.Said[i].Detail = prose.Clean(cleaned.Said[i].Detail)
	}
	sh := &cleaned.Shell
	sh.Said.Detail, sh.Where, sh.Rival, sh.Front = prose.Clean(sh.Said.Detail), prose.Clean(sh.Where), prose.Clean(sh.Rival), prose.Clean(sh.Front)
	sh.Line, sh.Dir = prose.Visible(sh.Line), prose.Visible(sh.Dir)
	return &cleaned
}
