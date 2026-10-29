package dashboard

// What the service step's page says: what the service is for, and what it
// says while the step is under way, which can take half a minute.
const (
	servicePurpose  = "launchd keeps the router running: it starts at login, and restarts it if it stops."
	serviceUnderWay = "Seeing to the service and its router"
)

// servicePage is the Service step's page: THE SERVICE, what the service is
// for, and while the step is under way, that it is, as waiting has it; once
// it's done, what it found and did, a fact a row, and enter on to the claude
// link.
func (s Setup) servicePage() setupPage {
	p := setupPage{
		name:    "THE SERVICE",
		purpose: []line{{{servicePurpose, mutedInk}}},
		onward:  onward("next: " + SetupLink.Title()),
	}
	switch {
	case s.UnderWay:
		p.underWay = waiting(s.Now, serviceUnderWay)
	case s.Service != nil:
		p.facts = s.Service.Said
	}
	return p
}
