package cli

import (
	"cmp"

	"github.com/leeovery/switchboard/internal/status"
)

// projects reads each session's project as status.Projects reads it, against
// the machine's filesystem, home, temporary directory and user, a scratchpad
// decoded found as FindProject finds a directory's: status.ProjectOf alone
// where there's no Root.
func (a *app) projects() status.Projects {
	if a.Root == nil {
		return status.Projects{}
	}
	home, _ := a.HomeDir()
	return status.NewProjects(status.ProjectsConfig{
		Root: a.Root, Home: home, TempDir: cmp.Or(a.Getenv("TMPDIR"), "/tmp"), UID: a.UID, Find: a.FindProject,
		Repository: a.RepositoryOf,
	})
}
