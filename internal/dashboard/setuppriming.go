package dashboard

import "github.com/leeovery/switchboard/internal/setup"

// SetupPrimingPage is the Priming step's page as it stands: the field Your day,
// and why the day typed in it isn't one, where setup refused it; or why the
// step failed, reading the day or setting it, where it did.
type SetupPrimingPage struct {
	Day              Field
	Refused, Failure string
}

// What the Priming page says: what priming is for, in the lines the page
// breaks it into; its field's name and hint; and how to leave it off.
var primingPurpose = []line{
	{{"Priming starts each account's 5-hour window on a schedule, so their resets are spread through your day", secondaryInk}},
	{{"instead of all arriving at once. It's optional.", secondaryInk}},
}

const (
	dayName    = "Your day"
	dayHint    = "start and end, as HH:MM-HH:MM"
	primingOff = "Leave it empty to keep priming off."
	// primingNotSet labels the fact of a step that failed, reading the day
	// or setting it.
	primingNotSet = "Not set"
)

// primingPage is the Priming step's page: PRIMING, what priming is for, and
// once the day is read, the field Your day, holding it, and how to leave
// priming off; or, where the step failed, why. Its onward key moves on to
// the Service.
func (s Setup) primingPage() setupPage {
	p := setupPage{name: "PRIMING", purpose: primingPurpose, onward: onward("next: " + SetupService.Title())}
	switch pr := s.Priming; {
	case pr == nil:
	case pr.Failure != "":
		p.facts = []setup.Said{{Mark: setup.Failed, Label: primingNotSet, Detail: pr.Failure}}
	default:
		p.fields = []textField{{name: dayName, hint: dayHint, wrong: pr.Refused, field: pr.Day}}
		p.notes = []line{{{primingOff, mutedInk}}}
	}
	return p
}
