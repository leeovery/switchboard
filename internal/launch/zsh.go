package launch

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
)

// prefixPattern is what a launcher's name may start with. The prefix and an
// account's id make the name of a zsh function, which must be a plain word,
// and mustn't look like an option.
var prefixPattern = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_-]*)?$`)

// CheckPrefix fails for a prefix that can't start a launcher's name.
func CheckPrefix(prefix string) error {
	if !prefixPattern.MatchString(prefix) {
		return fmt.Errorf("invalid prefix %q: use letters, digits, '-' and '_', not starting with '-'", prefix)
	}
	return nil
}

// Integration is the shell integration: what has the shell start Claude Code
// through run.
type Integration struct {
	// Binary is the switchboard binary the functions run, by its absolute
	// path.
	Binary string
	// Config is the config file the functions have run read, by its absolute
	// path, or "" for the one run finds itself.
	Config string
	// Prefix starts each account's launcher's name.
	Prefix string
	// Accounts are the configured accounts, as config.Load validates them,
	// whose ids and variables' names are safe in zsh as they stand; none when
	// the config can't be read.
	Accounts config.Accounts
	// Getenv reads the accounts' token variables as the integration is
	// written, which choose the one its export names.
	Getenv func(key string) string
}

// Zsh writes the integration as zsh for a shell to eval: a claude function
// that starts Claude Code through run; a launcher for each account, named the
// prefix and the account's id, that starts it pinned to that account; and,
// for programs that start claude themselves, an export of the token of the
// first account whose token is set as it's written, as run chooses without
// the router, else the first account's. The export names the token's
// variable, so the output holds no token. Without accounts, it writes the
// claude function alone, which run keeps working.
func (i Integration) Zsh(w io.Writer) error {
	if err := CheckPrefix(i.Prefix); err != nil {
		return err
	}
	run := quote(i.Binary)
	if i.Config != "" {
		run += " --config " + quote(i.Config)
	}
	run += " run"
	var b strings.Builder
	fmt.Fprintf(&b, "function %s { %s -- \"$@\"; }\n", claude.Command, run)
	for _, a := range i.Accounts {
		fmt.Fprintf(&b, "function %s%s { %s --account %s -- \"$@\"; }\n", i.Prefix, a.ID, run, a.ID)
	}
	if len(i.Accounts) > 0 {
		token := i.exported().TokenEnv
		fmt.Fprintf(&b, "if [[ -n ${%s-} ]]; then\n  export %s=\"${%s}\"\nfi\n", token, claude.TokenEnv, token)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// exported is the account whose token the export names: the first with a
// token, else the first. There must be one.
func (i Integration) exported() config.Account {
	if a, _, ok := firstWithToken(i.Accounts, i.Getenv); ok {
		return a
	}
	return i.Accounts[0]
}

// quote quotes s for zsh, whatever it holds: in single quotes, with each
// single quote of its own closed, escaped and reopened.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
