package testguard

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The variables no test may see: those that configure switchboard, Claude
// Code and the Anthropic API, which can hold real tokens; tmux's, which lead
// to the real tmux server; and proxies', as a proxy on loopback would carry a
// request past the dial guard to the network.
var (
	clearedPrefixes = []string{"SWITCHBOARD_", "CLAUDE_", "ANTHROPIC_"}
	// clearedNames count in either case, as proxies' lower-case names are
	// honoured too.
	clearedNames = []string{"TMUX", "TMUX_PANE", "HTTP_PROXY", "HTTPS_PROXY"}
)

// isolateEnv clears the variables no test may see, points HOME and XDG's base
// directories at home, and PATH at the stubs' directory alone.
func isolateEnv(home, stubs string) error {
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !cleared(name) {
			continue
		}
		if err := os.Unsetenv(name); err != nil {
			return fmt.Errorf("clear %s: %w", name, err)
		}
	}
	for name, value := range map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
		"PATH":            stubs,
	} {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	return nil
}

func cleared(name string) bool {
	hasPrefix := func(prefix string) bool { return strings.HasPrefix(name, prefix) }
	return slices.ContainsFunc(clearedPrefixes, hasPrefix) || slices.Contains(clearedNames, strings.ToUpper(name))
}
