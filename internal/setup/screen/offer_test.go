package screen

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// offerTop is the offer card's top edge at 160×37, over linkLeft: its body
// seven rows, the line wrapped onto two, so the card is twelve rows tall,
// centred down the screen by the dashboard Card's one rule.
const offerTop = (37 - 12) / 2

// offerTitle is the offer card's top edge as it starts, naming the file.
const offerTitle = "╭─ ADD A LINE TO ~/.zshrc ─"

// lineAdded is what AddLine leaves where it adds the line: as Link found it,
// but the line added, the shell to ask again.
var lineAdded = func() setup.LinkFound {
	found := situated(setup.LineAdded, "switchboard's line added to the end of ~/.zshrc")
	found.Shell.Said.Mark = setup.Done
	return found
}()

func TestWhereLinkOffersTheLineTheOfferCardOpensOverThePage(t *testing.T) {
	h := linkHarness(t, linkLeft)
	if h.model.offer == nil {
		t.Fatal("no offer card is open where Link offers the line")
	}
	rows := h.rows()
	for y, want := range map[int]string{6: linkPurpose, 8: fact(linkRow), 10: fact(linkLeft.Shell.Said)} {
		if rows[y] != want {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], want)
		}
	}
	if !strings.Contains(rows[offerTop], offerTitle) {
		t.Errorf("row %d reads %q, want the offer card's top edge, %q", offerTop, rows[offerTop], offerTitle)
	}
	if rows[36] != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", rows[36])
	}
	for _, row := range rows {
		if strings.Contains(row, strings.TrimSpace(addTheLine)) {
			t.Errorf("the page reads %q under the card, want nothing to put right by hand while it offers the line", row)
		}
	}
	for _, key := range []string{"q", "c", "d", "r", "j"} {
		if msgs := h.press(key); len(msgs) != 0 {
			t.Errorf("%s at the offer card sends %v, want nothing", key, msgs)
		}
	}
	if h.model.offer == nil || len(h.steps.addedOf) != 0 || len(h.steps.askedOf) != 0 {
		t.Errorf("the keys left the card open: %t, AddLine run %d times, the shell asked again %d; want open, neither", h.model.offer != nil, len(h.steps.addedOf), len(h.steps.askedOf))
	}
}

func TestEnterAtTheOfferCardAddsTheLine(t *testing.T) {
	notTaking := situated(setup.NotTaking, "switchboard's line is in ~/.zshrc, but something after it changes PATH")
	otherLine := situated(setup.OtherLine, "switchboard's line in ~/.zshrc reads another switchboard's file")
	cantAdd := situated(setup.CantAdd, "setup can't add its line to ~/.zshrc: permission denied")
	tests := []struct {
		name  string
		added setup.LinkFound
		again []setup.LinkFound
		// asked is whether the shell is asked again, at once, without r.
		asked bool
		want  map[int]string
	}{
		{name: "the line added, then switchboard's claude: done, the hook installed", added: lineAdded, again: []setup.LinkFound{linkThrough}, asked: true, want: map[int]string{
			10: fact(shellThrough), 12: fact(hookInstalled), 36: keyLine("", skillRight),
		}},
		{name: "the line added, the shell running another claude still: by hand, as it now says", added: lineAdded, again: []setup.LinkFound{notTaking}, asked: true, want: map[int]string{
			10: fact(notTaking.Shell.Said),
			13: "  Move switchboard's line to the end of ~/.zshrc, or what changes PATH after it to before it.",
			15: orTheDir, 17: "      " + binDir, 19: pressR,
			36: keyLine("d copy the directory   r check again", byHandRight),
		}},
		{name: "another switchboard's line: by hand, in its place", added: otherLine, want: map[int]string{
			10: fact(otherLine.Shell.Said),
			13: "  Put this line in its place:", 15: "      " + shellLine, 17: orTheDir, 19: "      " + binDir, 21: pressR,
			36: keyLine("c copy the line   d copy the directory   r check again", byHandRight),
		}},
		{name: "a line it couldn't add: by hand, added yourself", added: cantAdd, want: map[int]string{
			10: fact(cantAdd.Shell.Said),
			13: addTheLine, 15: "      " + shellLine, 17: orTheDir, 19: "      " + binDir, 21: pressR,
			36: keyLine("c copy the line   d copy the directory   r check again", byHandRight),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := linkHarness(t, linkLeft)
			h.steps.lineAdded, h.steps.again = tt.added, tt.again
			h.pressAndDeliver("enter")
			if h.model.offer != nil {
				t.Error("the offer card is open once AddLine has answered, want it closed")
			}
			tt.want[6], tt.want[8] = linkPurpose, fact(linkRow)
			wantRows(t, h.rows(), tt.want)
			if added := h.steps.addedOf; len(added) != 1 || added[0].Shell.Situation != setup.OfferLine {
				t.Errorf("AddLine was given %+v, want what Link found, once", added)
			}
			asked := h.steps.askedOf
			if got := len(asked) == 1; got != tt.asked {
				t.Fatalf("the shell was asked again %d times, want it asked at once: %t", len(asked), tt.asked)
			}
			if tt.asked && asked[0].Shell.Situation != setup.LineAdded {
				t.Errorf("AskAgain was given %+v, want what AddLine left", asked[0])
			}
		})
	}
}

func TestTheLineAddedTheStepEndsWithTheHookOnceBeforeTheSkill(t *testing.T) {
	h := linkHarness(t, linkLeft)
	h.steps.lineAdded, h.steps.again = lineAdded, []setup.LinkFound{linkThrough}
	h.pressAndDeliver("enter")
	if h.steps.hooks != 1 || h.steps.skills != 0 {
		t.Fatalf("once the shell runs switchboard's claude, the hook was installed %d times, the skill written %d; want once, and not yet", h.steps.hooks, h.steps.skills)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupSkill {
		t.Fatalf("enter reaches %s, want the Skill", got.Title())
	}
	if h.steps.hooks != 1 || h.steps.skills != 1 {
		t.Errorf("the hook was installed %d times, the skill written %d; want each once", h.steps.hooks, h.steps.skills)
	}
}

func TestAddingTheLineGivesAddLineWhatLinkFoundAsSetupGaveIt(t *testing.T) {
	given := linkLeft
	given.Shell.Where, given.Shell.Line = "~/zsh\x1b[2J/.zshrc", "if [ -r \"$HOME/x\x1b[2J\" ]; then . \"$HOME/x\x1b[2J\"; fi  # switchboard"
	h := linkHarness(t, given)
	for y, row := range h.rows() {
		if strings.ContainsAny(row, "\x1b\x07") {
			t.Errorf("row %d draws a control: %q", y, row)
		}
	}
	h.steps.lineAdded = situated(setup.CantAdd, "setup can't add its line")
	h.pressAndDeliver("enter")
	if added := h.steps.addedOf; len(added) != 1 || added[0].Shell.Line != given.Shell.Line || added[0].Shell.Where != given.Shell.Where {
		t.Errorf("AddLine was given %+v, want what Link found, as setup gave it", added)
	}
}

func TestAddLineFailingFailsTheStepWordingNothingItReturned(t *testing.T) {
	h := linkHarness(t, linkLeft)
	h.steps.lineAdded, h.steps.lineErr = situated(setup.CantAdd, "setup can't add its line, which isn't to be said"), errNoLink
	h.pressAndDeliver("enter")
	wantRows(t, h.rows(), map[int]string{
		6:  linkPurpose,
		8:  fact(linkRow),
		10: fact(setup.Said{Mark: setup.Failed, Label: "Not set up", Detail: errNoLink.Error()}),
		36: keyLine("", "q quit"),
	})
	if h.model.link.Shell.Situation != setup.OfferLine {
		t.Errorf("the screen keeps %v as what the step found, want what Link found, not what AddLine returned with its error", h.model.link.Shell.Situation)
	}
	if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNoLink}) {
		t.Errorf("setup is left %+v, want failing at the claude link", got)
	}
	h.pressAndDeliver("enter")
	if h.steps.hooks != 0 || len(h.steps.askedOf) != 0 {
		t.Errorf("the hook was installed %d times, the shell asked again %d; want neither at a failed step", h.steps.hooks, len(h.steps.askedOf))
	}
}

func TestNAndEscCloseTheOfferCardOntoTheByHandPage(t *testing.T) {
	for _, key := range []string{"n", "esc"} {
		t.Run(key, func(t *testing.T) {
			h := linkHarness(t, linkLeft)
			h.pressAndDeliver(key)
			wantRows(t, h.rows(), map[int]string{
				6: linkPurpose, 8: fact(linkRow), 10: fact(linkLeft.Shell.Said),
				13: addTheLine, 15: "      " + shellLine, 17: orTheDir, 19: "      " + binDir, 21: pressR,
				36: keyLine("c copy the line   d copy the directory   r check again", byHandRight),
			})
			if len(h.steps.addedOf) != 0 || h.steps.hooks != 0 {
				t.Errorf("%s ran AddLine %d times, installed the hook %d; want neither", key, len(h.steps.addedOf), h.steps.hooks)
			}
			h.pressAndDeliver("enter")
			if h.steps.hooks != 1 || h.steps.skills != 0 {
				t.Errorf("enter by hand installed the hook %d times, wrote the skill %d; want once, and not yet", h.steps.hooks, h.steps.skills)
			}
			h.pressAndDeliver("enter")
			if got := h.model.page().At; got != dashboard.SetupSkill || h.steps.hooks != 1 {
				t.Errorf("the second enter reaches %s, the hook installed %d times; want the Skill, once", got.Title(), h.steps.hooks)
			}
		})
	}
}

func TestWhileTheLineIsAddedOrTheShellAskedOnlyQAndCtrlCAct(t *testing.T) {
	h := linkHarness(t, linkLeft)
	h.steps.lineAdded, h.steps.again = lineAdded, []setup.LinkFound{linkThrough}
	adding := h.update(named["enter"])
	if h.model.offer == nil || !h.model.offer.adding {
		t.Fatal("the offer card isn't adding the line once enter's pressed")
	}
	if !strings.Contains(h.rows()[offerTop+2], "●∙∙  Adding the line to the end of ~/.zshrc") {
		t.Errorf("the card's first row reads %q, want it saying it's adding the line", h.rows()[offerTop+2])
	}
	if _, spins := h.model.page().SpinDelay(); !spins {
		t.Error("the card's spinner doesn't move while the line's added")
	}
	dead := []string{"enter", "n", "esc", "c", "d", "r", "j"}
	pressDead := func(while string) {
		t.Helper()
		for _, key := range dead {
			if msgs := h.press(key); len(msgs) != 0 {
				t.Errorf("%s while %s sends %v, want nothing", key, while, msgs)
			}
		}
		for _, key := range []string{"q", "ctrl+c"} {
			if !quits(h.press(key)) {
				t.Errorf("%s while %s doesn't leave", key, while)
			}
		}
	}
	pressDead("the line's added")
	var asking []tea.Msg
	for _, msg := range adding {
		asking = append(asking, h.update(msg)...)
	}
	if !h.model.asking() {
		t.Fatal("the shell isn't asked once the line's added")
	}
	pressDead("the shell's asked")
	if len(h.steps.addedOf) != 1 || len(h.steps.askedOf) != 1 {
		t.Errorf("AddLine ran %d times, the shell asked again %d; want each once", len(h.steps.addedOf), len(h.steps.askedOf))
	}
	h.deliver(asking...)
	if h.steps.hooks != 1 {
		t.Errorf("the hook was installed %d times once the shell answered, want once", h.steps.hooks)
	}
}

// addTillReleased is setup's steps whose AddLine says it's started, and ends
// only once it's let go, saying it has.
type addTillReleased struct {
	*fakeSteps
	started, release, ended chan struct{}
}

func (s *addTillReleased) AddLine(found setup.LinkFound) (setup.LinkFound, error) {
	close(s.started)
	<-s.release
	defer close(s.ended)
	return s.fakeSteps.AddLine(found)
}

func TestLeavingAsTheLineIsAddedWaitsForTheWrite(t *testing.T) {
	for _, quit := range []string{"q", "\x03"} {
		t.Run(strings.ReplaceAll(quit, "\x03", "ctrl+c"), func(t *testing.T) {
			steps := &addTillReleased{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
			steps.found, steps.lineAdded = linkLeft, lineAdded
			linked := make(chan struct{})
			var once sync.Once
			term := terminal{
				keys: "\r", ready: linked,
				then: quit, thenReady: steps.started,
				saw: func(msg tea.Msg) {
					if _, ok := msg.(linkedMsg); ok {
						once.Do(func() { close(linked) })
					}
				},
			}
			returned := make(chan Left, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, term)
				if err != nil {
					t.Errorf("run() error = %v, want none", err)
				}
				returned <- left
			}()
			select {
			case <-steps.started:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, AddLine hadn't begun")
			}
			select {
			case <-returned:
				t.Fatal("run() returned while AddLine wrote, want it to wait for the write to end")
			case <-time.After(overlap):
			}
			close(steps.release)
			left := <-returned
			select {
			case <-steps.ended:
			default:
				t.Error("run() returned before AddLine ended")
			}
			if left != (Left{Ending: Paused}) {
				t.Errorf("run() left setup %+v, want it paused", left)
			}
			if steps.hooks != 0 || len(steps.askedOf) != 0 {
				t.Errorf("the hook was installed %d times, the shell asked again %d; want neither once setup's left", steps.hooks, len(steps.askedOf))
			}
		})
	}
}

func TestTheLineIsAddedAsAWritingActionNeverRunOnceSetupsLeftWhileAnotherWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h := &harness{t: t, clock: &fakeClock{now: start}, steps: newFakeSteps()}
	h.steps.found, h.steps.lineAdded = linkLeft, lineAdded
	h.model = New(ctx, h.config(dashboard.SetupLink))
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	// Another writing action holds the writing actions' one place.
	h.model.jobs.writing <- struct{}{}
	next, cmd := h.model.Update(named["enter"])
	h.model = next.(Model)
	adding := make(chan []tea.Msg, 1)
	go func() { adding <- h.run(cmd) }()
	// Time for the write to reach its turn, so setup's left as it waits there.
	time.Sleep(overlap)
	if !quits(h.press("q")) {
		t.Fatal("q while the line's added doesn't leave")
	}
	cancel()
	<-adding
	h.model.jobs.wait()
	<-h.model.jobs.writing
	if added := len(h.steps.addedOf); added != 0 {
		t.Errorf("AddLine ran %d times once setup had left while another write held its turn, want never: it's a writing action", added)
	}
}
