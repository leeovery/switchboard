package setuptoken

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/childenv"
	"github.com/leeovery/switchboard/internal/claude"
)

// environ is an environment as os.Environ gives it: "key=value", a variable
// each.
type environ []string

// get returns key's value, or "" when it's unset. Of a key set twice, the
// first counts, as it does for getenv.
func (e environ) get(key string) string {
	for _, variable := range e {
		if k, value, _ := strings.Cut(variable, "="); k == key {
			return value
		}
	}
	return ""
}

// forSetupToken returns the environment setup-token, the claude at path,
// starts in: this one, as the user set it, without what would send it
// elsewhere or add warnings to what it draws (see claude.SetupTokenUnset),
// with browser as the program it opens its sign-in address with, and claude's
// own directories first on PATH, as childenv.Beside puts them.
func (e environ) forSetupToken(path, browser string) []string {
	unset := append(claude.SetupTokenUnset(), claude.BrowserEnv)
	env := slices.DeleteFunc(slices.Clone(e), func(variable string) bool {
		key, _, _ := strings.Cut(variable, "=")
		return slices.Contains(unset, key)
	})
	return childenv.Beside(append(env, claude.BrowserEnv+"="+browser), path)
}
