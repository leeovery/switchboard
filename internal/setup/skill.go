package setup

import "github.com/leeovery/switchboard/internal/skill"

// Skill writes the skill where Claude Code reads it, or brings the copy
// there up to date, and says which it did, if either. Where it can do
// neither, it fails, saying so as the screen draws it: Not installed, and
// why.
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
		return Said{Mark: Done, Label: "Installed", Detail: at}, nil
	case found.Rewritten:
		logger.Info("brought the skill up to date", "path", s.SkillFile, "was", found.Was)
		return Said{Mark: Done, Label: "Updated", Detail: at}, nil
	}
	return Said{Mark: Done, Label: "Up to date", Detail: at}, nil
}

// notInstalled is what the skill step says where it failed, as err says.
func notInstalled(err error) Said {
	return Said{Mark: Failed, Label: NotInstalled, Detail: err.Error()}
}
