package setup

// Step is a step of setup, in their order, as its screen's tabs and its
// summary name them.
type Step int

const (
	StepAccounts Step = iota
	StepPriming
	StepService
	StepLink
	StepSkill
	StepDone
)

// Steps are setup's steps, in their order.
func Steps() []Step {
	return []Step{StepAccounts, StepPriming, StepService, StepLink, StepSkill, StepDone}
}

// Title is the step's name.
func (s Step) Title() string {
	switch s {
	case StepAccounts:
		return "Accounts"
	case StepPriming:
		return "Priming"
	case StepService:
		return "Service"
	case StepLink:
		return "claude link"
	case StepSkill:
		return "Skill"
	}
	return "Done"
}
