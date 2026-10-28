package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/skill"
)

// Skill writes the skill where Claude Code reads it, or brings the copy
// there up to date, and says which it did, if either. Where it can do
// neither, it fails, saying so as the screen draws it, which the line walk
// doesn't print: Not installed, and why.
func (s *Setup) Skill() (Said, error) {
	found, err := skill.Refresh(s.SkillFile)
	if err != nil {
		return notInstalled(err), err
	}
	at := s.fromHome(s.SkillFile)
	switch {
	case !found.Installed:
		if err := skill.Install(s.SkillFile); err != nil {
			return notInstalled(err), err
		}
		logger.Info("installed the skill", "path", s.SkillFile)
		return Said{Mark: Done, Label: "Installed", Detail: at, Sentence: "Installed the skill, which tells Claude what switchboard does under claude: " + s.SkillFile}, nil
	case found.Rewritten:
		logger.Info("brought the skill up to date", "path", s.SkillFile, "was", found.Was)
		return Said{Mark: Done, Label: "Updated", Detail: at, Sentence: "Brought the skill up to date: " + s.SkillFile}, nil
	}
	return Said{Mark: Done, Label: "Up to date", Detail: at, Sentence: "The skill is installed, and up to date: " + s.SkillFile}, nil
}

// notInstalled is what the skill step says where it failed, as err says.
func notInstalled(err error) Said {
	return Said{Mark: Failed, Label: "Not installed", Detail: err.Error()}
}

// skill is the line walk's skill step.
func (r *run) skill(context.Context) error {
	said, err := r.Skill()
	if err != nil {
		return err
	}
	r.Terminal.tell(said)
	return nil
}
