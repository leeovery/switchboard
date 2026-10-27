package claude

import "slices"

// localSubcommands are Claude Code's subcommands that switchboard has no part
// in: they look after Claude Code on this machine, or set up its login, and
// hold no conversation for the router to route.
var localSubcommands = []string{
	SetupTokenCommand, "update", "upgrade", "install", "doctor", "mcp", "plugin", "plugins",
	"auth", "import", "project", "auto-mode", "gateway",
}

// localFlags are Claude Code's flags that, given first, have it print its
// version or its help and exit, which switchboard has no part in either.
var localFlags = []string{"--version", "-v", "--help", "-h"}

// IsLocal reports whether args, Claude Code's arguments, run one of its local
// subcommands, or ask for its version or its help. Only the first argument
// counts: in -p doctor, doctor is a prompt.
func IsLocal(args []string) bool {
	return len(args) > 0 && (slices.Contains(localSubcommands, args[0]) || slices.Contains(localFlags, args[0]))
}
