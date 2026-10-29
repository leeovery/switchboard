package setup

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// Summary is how setup stands, as the walk through its steps leaves it,
// which the screen's Done page says a line a step, and setup prints as it
// leaves: the step it stands at, why that step failed, where it did, and
// what each step told, nil for a step that told nothing.
type Summary struct {
	// At is the step setup stands at: StepDone once every other is passed
	// through.
	At Step
	// Failure is why the step at At failed, made safe to show: "" where it
	// didn't.
	Failure string
	// Review is the accounts, as the status line last read them.
	Review *Review
	// Day is the day priming is on over, as the Priming step last read or
	// set it: the zero Day where priming is off.
	Day     *config.Day
	Service *Serviced
	Link    *LinkFound
	Skill   *Said
}

// StepLine is a step's line in the summary: its row, labelled with the
// step's name, and its detail, the line walk's sentence left out; and under
// it, Add, what the claude link step leaves to put in place by hand, to
// copy: "" for every other. Directory is set where Add is the bin directory,
// to put first on PATH, rather than a line for a startup file.
type StepLine struct {
	Said
	Add       string
	Directory bool
}

// Lines are the summary's lines, a step a line, in the steps' order, up to
// the step it stands at. A step passed through says what it told, else ✓
// alone; the step it stands at says why it failed, where it did, else what
// it told, where it told anything; a step not reached has no line.
func (s Summary) Lines() []StepLine {
	var lines []StepLine
	for _, step := range Steps()[:StepDone] {
		l, told := s.told(step)
		switch {
		case step > s.At:
			return lines
		case step == s.At && s.Failure != "" && (!told || l.Mark != Failed):
			l, told = StepLine{Mark: Failed, Detail: s.Failure}, true
		case step < s.At && !told:
			l, told = StepLine{Mark: Done}, true
		}
		if told {
			l.Label = step.Title()
			lines = append(lines, l)
		}
	}
	return lines
}

// told is the step's line, as its outcome words it, but for its label, and
// whether it told anything to word.
func (s Summary) told(step Step) (StepLine, bool) {
	switch step {
	case StepAccounts:
		return lineOf(s.Review, Review.said)
	case StepPriming:
		return lineOf(s.Day, primingSaid)
	case StepService:
		return lineOf(s.Service, Serviced.said)
	case StepLink:
		if s.Link == nil {
			return StepLine{}, false
		}
		return s.Link.line(), true
	case StepSkill:
		return lineOf(s.Skill, Said.summed)
	}
	return StepLine{}, false
}

// lineOf is the line say words of outcome, where there's one.
func lineOf[O any](outcome *O, say func(O) Said) (StepLine, bool) {
	if outcome == nil {
		return StepLine{}, false
	}
	return StepLine{Said: say(*outcome)}, true
}

// said is the accounts' line: how many there are, and how many have
// working tokens, as the status line counts them; then the primary, and
// what's reserved on it for the Claude apps, where any is, or that none is
// the primary, of two or more. It's a warning where any token isn't
// working, none is the primary, or there are no accounts.
func (r Review) said() Said {
	if len(r.Accounts) == 0 {
		return Said{Mark: Warning, Detail: "none"}
	}
	said := Said{Mark: Done, Detail: prose.Grouped(len(r.Accounts)) + ", " + r.working()}
	if slices.ContainsFunc(r.Accounts, func(a Reviewed) bool { return !a.Token.Working() }) {
		said.Mark = Warning
	}
	if i := slices.IndexFunc(r.Accounts, func(a Reviewed) bool { return a.Primary }); i >= 0 {
		said.Detail += " · primary: " + r.Accounts[i].ID
		if reserve := r.Accounts[i].Reserve; reserve > 0 {
			said.Detail += ", with " + prose.PercentSet(reserve) + " reserved for the Claude apps"
		}
	}
	if r.Unmarked {
		said.Mark, said.Detail = Warning, said.Detail+" · none is the primary"
	}
	return said
}

// primingSaid is priming's line: on, over the day, or off.
func primingSaid(day config.Day) Said {
	if !(config.Prime{Day: day}).On() {
		return Said{Mark: Done, Detail: "off"}
	}
	return Said{Mark: Done, Detail: "on, " + prose.ClockSpan(day.Start, day.End)}
}

// said is the service's line: where the step failed, why, as its first
// failed row says it; else installed, and updated where it was installed
// afresh, then how its router answered, where it saw it, made safe to show,
// and what the step left for the user to do, a row's detail each. It's a
// warning where the router answered unhealthy, or anything's left to do.
func (s Serviced) said() Said {
	if i := slices.IndexFunc(s.Said, func(said Said) bool { return said.Mark == Failed && said.Label != "" }); i >= 0 {
		return Said{Mark: Failed, Detail: s.Said[i].summed().Detail}
	}
	said, did := Said{Mark: Done}, []string{"installed"}
	var left []string
	for _, row := range s.Said {
		switch {
		case row.Label == updatedLabel:
			did = append(did, "updated")
		case row.Mark == Warning && !news(row.Label):
			said.Mark, left = Warning, append(left, row.Detail)
		}
	}
	parts := []string{prose.List(did)}
	if h := s.Router; h != nil {
		parts = append(parts, "router "+service.Healthy(router.Health{OK: h.OK, Reason: prose.Clean(h.Reason)}))
		if !h.OK {
			said.Mark = Warning
		}
	}
	said.Detail = strings.Join(append(parts, left...), " · ")
	return said
}

// news reports whether a row of the service step's, by its label, tells
// what the step did, never anything left for the user to do.
func news(label string) bool {
	return label == updatedLabel || label == restartingLabel || label == restartedLabel || label == routerLabel
}

// line is the claude link's line, as the situation the step left says it,
// each in words of its own, after what the step did to the link: ✓ where a
// new terminal's claude goes through it; else ! and what keeps it from it,
// an alias in front of PATH's claude first, where one runs something else,
// and where there's something to put in place by hand, the line, or the bin
// directory, under it. "it" is the link, as the line's label names it; but
// where the alias leads, or where the link led before its repair, standing
// between the label and the link, the link is named.
func (f LinkFound) line() StepLine {
	sh := f.Shell
	if f.Made == LinkNotMade {
		l := StepLine{Mark: Warning, Detail: "not made"}
		if len(f.Said) > 0 {
			l.Detail = f.Said[0].Detail
		}
		return l
	}
	it := "it"
	if sh.Front != "" || f.Made == LinkRepaired {
		it = "the claude link"
	}
	first := it + " isn't first on your PATH"
	if sh.Unasked {
		first = "setup couldn't ask your shell whether " + it + " is first on your PATH"
	}
	fronted := func(detail string) string {
		if sh.Front != "" {
			return sh.Front + "; and " + detail
		}
		return detail
	}
	warning := func(detail, add string) StepLine {
		return StepLine{Mark: Warning, Detail: f.after(", but ", fronted(detail)), Add: add}
	}
	switch sh.Situation {
	case Through:
		return StepLine{Mark: Done, Detail: f.after(" · ", "your shell runs "+it)}
	case LineAdded:
		added := f.after(", and ", "switchboard's line is added to "+sh.Where)
		return StepLine{Mark: Warning, Detail: added + ", but " + fronted("setup hasn't asked your shell again whether a new terminal's claude goes through "+it)}
	case Unreached:
		return warning("there's no Claude Code for "+it+" to start: install Claude Code, then run setup again", "")
	case OfferLine:
		return warning(first+". Add this line to "+sh.Where+":", sh.Line)
	case CantAdd:
		return warning(first+", and setup couldn't add its line to "+sh.Where+". Add this line to it yourself:", sh.Line)
	case WhichFile:
		return warning(first+". Add this line to your shell's startup file, usually "+sh.Where+":", sh.Line)
	case RivalFile:
		return warning(first+". Add this line to the end of whichever of "+sh.Where+" and "+sh.Rival+" a new terminal reads:", sh.Line)
	case CantLook:
		return warning(first+", and setup can't look in "+sh.Where+" for its line. Add this line to the end of it, if it isn't there:", sh.Line)
	case OtherLine:
		return warning("switchboard's line in "+sh.Where+" reads another switchboard's file. Put this line in its place:", sh.Line)
	case NotTaking:
		return warning(f.notTaking(), "")
	case InFront:
		return StepLine{Mark: Warning, Detail: f.after(", but ", sh.Front)}
	}
	l := warning("setup can't check "+it+" for your shell. Put this directory first on your PATH:", sh.Dir)
	l.Directory = true
	return l
}

// notTaking says what keeps switchboard's line, there in its startup file,
// from taking effect, and how to put it right; or that setup couldn't ask
// the shell whether it does.
func (f LinkFound) notTaking() string {
	sh := f.Shell
	switch {
	case sh.Unasked:
		return "setup couldn't ask your shell whether switchboard's line in " + sh.Where + " takes effect"
	case sh.Last:
		return "something read after " + sh.Where + " changes PATH. Put switchboard's line after it, wherever that's read"
	}
	return "something after switchboard's line in " + sh.Where + " changes PATH. Move the line to the end of " + sh.Where + ", or what changes PATH after it to before it"
}

// did is what the step did to the claude link, as its line says it: created,
// or repaired, and where it led; "" for a link found in place.
func (f LinkFound) did() string {
	switch f.Made {
	case LinkCreated:
		return "created"
	case LinkRepaired:
		return "repaired: it led to " + f.Led
	}
	return ""
}

// after is detail after what the step did to the claude link, joined by
// sep: detail alone where it did nothing to it.
func (f LinkFound) after(sep, detail string) string {
	if did := f.did(); did != "" {
		return did + sep + detail
	}
	return detail
}

// summed is a row as a step's line has it, the skill's or the failed row of
// another step: its label, as the detail, lowered; then for one that failed,
// why.
func (s Said) summed() Said {
	detail := lowered(s.Label)
	if s.Mark == Failed {
		detail += ": " + s.Detail
	}
	return Said{Mark: s.Mark, Detail: detail}
}
