// Package childenv is the environment switchboard starts the programs it
// runs for itself in, such as claude --version and osascript: only what they
// need to run, so none of the tokens in switchboard's own reaches them.
package childenv

// kept are the variables such a program is given: where to find programs,
// the home directory, the user's own temporary directory rather than the
// /tmp everyone shares, and the language to speak.
var kept = []string{"PATH", "HOME", "TMPDIR", "LANG"}

// Minimal returns those of kept that getenv finds set, as exec.Cmd's Env
// takes them. It's never nil: a nil Env hands on the whole environment.
func Minimal(getenv func(string) string) []string {
	env := make([]string, 0, len(kept))
	for _, name := range kept {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}
