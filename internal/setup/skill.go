package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/skill"
)

// skill writes the skill where Claude Code reads it, or brings the copy
// there up to date.
func (r *run) skill(context.Context) error {
	found, err := skill.Refresh(r.Skill)
	if err != nil {
		return err
	}
	switch {
	case !found.Installed:
		if err := skill.Install(r.Skill); err != nil {
			return err
		}
		logger.Info("installed the skill", "path", r.Skill)
		r.Terminal.sayf("Installed the skill, which tells Claude what switchboard does under claude: %s", r.Skill)
	case found.Rewritten:
		logger.Info("brought the skill up to date", "path", r.Skill, "was", found.Was)
		r.Terminal.sayf("Brought the skill up to date: %s", r.Skill)
	default:
		r.Terminal.sayf("The skill is installed, and up to date: %s", r.Skill)
	}
	return nil
}
