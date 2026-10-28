package dashboard

import "github.com/leeovery/switchboard/internal/setup"

// skillPurpose is what the skill step is for, as its page says it.
const skillPurpose = "The skill tells Claude about switchboard, so it can answer questions about your accounts, usage and sessions."

// skillPage is the Skill step's page: SKILL, what the skill is for, what the
// step did of it, once it has, and enter to finish.
func (s Setup) skillPage() setupPage {
	p := setupPage{name: "SKILL", purpose: line{{skillPurpose, secondaryInk}}, onward: onward("finish")}
	if s.Skill != nil {
		p.facts = []setup.Said{*s.Skill}
	}
	return p
}
