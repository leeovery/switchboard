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

// Zsh writes zsh for a shell to eval, which has it start Claude Code through
// run: a claude function that starts it through the switchboard at binary; a
// launcher for each account, named prefix and the account's id, that starts
// it pinned to that account; and an export of the first account's token, for
// programs that start claude themselves. The export names the token's
// variable, so the output holds no token. The accounts are as config.Load
// validates them, whose ids and variables' names are safe in zsh as they
// stand.
func Zsh(w io.Writer, binary, prefix string, accounts []config.Account) error {
	if err := CheckPrefix(prefix); err != nil {
		return err
	}
	run := quote(binary) + " run"
	var b strings.Builder
	fmt.Fprintf(&b, "function %s { %s -- \"$@\"; }\n", claude.Command, run)
	for _, a := range accounts {
		fmt.Fprintf(&b, "function %s%s { %s --account %s -- \"$@\"; }\n", prefix, a.ID, run, a.ID)
	}
	if len(accounts) > 0 {
		token := accounts[0].TokenEnv
		fmt.Fprintf(&b, "if [[ -n ${%s-} ]]; then\n  export %s=\"${%s}\"\nfi\n", token, claude.TokenEnv, token)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// quote quotes s for zsh, whatever it holds: in single quotes, with each
// single quote of its own closed, escaped and reopened.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
