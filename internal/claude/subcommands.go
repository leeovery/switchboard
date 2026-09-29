package claude

import "slices"

// localSubcommands are Claude Code's subcommands that switchboard has no part
// in: they look after Claude Code on this machine, or set up its login, and
// hold no conversation for the router to route.
var localSubcommands = []string{
	"setup-token", "update", "upgrade", "install", "doctor", "mcp", "plugin", "plugins",
	"auth", "import", "project", "auto-mode", "gateway",
}

// IsLocal reports whether args, Claude Code's arguments, run one of its local
// subcommands. Only the first can name one: in -p doctor, doctor is a
// prompt.
func IsLocal(args []string) bool {
	return len(args) > 0 && slices.Contains(localSubcommands, args[0])
}
