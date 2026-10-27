package setuptoken

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/childenv"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/launch"
)

// environ is an environment as os.Environ gives it, "key=value", a variable
// each, as ownEnviron gives it: each key once.
type environ []string

// ownEnviron returns env, an environment as os.Environ gives it, as
// setup-token starts from it: each key once, at the value exec.Cmd gives a
// program of a key set twice, the last, and as Claude Code starts on its own
// login, as the user set it (see launch.OwnLogin).
func ownEnviron(env []string) environ {
	seen := make(map[string]bool)
	var last []string
	for _, variable := range slices.Backward(env) {
		if key, _, _ := strings.Cut(variable, "="); !seen[key] {
			seen[key] = true
			last = append(last, variable)
		}
	}
	slices.Reverse(last)
	return launch.OwnLogin(last)
}

// get returns key's value, or "" when it's unset.
func (e environ) get(key string) string {
	for _, variable := range e {
		if k, value, _ := strings.Cut(variable, "="); k == key {
			return value
		}
	}
	return ""
}

// forSetupToken returns the environment setup-token, the claude at path,
// starts in: this one, without what would send it elsewhere or add warnings
// to what it draws (see claude.SetupTokenUnset), with browser as the program
// it opens its sign-in address with, and claude's own directories first on
// PATH, as childenv.Beside puts them.
func (e environ) forSetupToken(path, browser string) []string {
	unset := append(claude.SetupTokenUnset(), claude.BrowserEnv)
	env := slices.DeleteFunc(slices.Clone(e), func(variable string) bool {
		key, _, _ := strings.Cut(variable, "=")
		return slices.Contains(unset, key)
	})
	return childenv.Beside(append(env, claude.BrowserEnv+"="+browser), path)
}
