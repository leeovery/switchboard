package launch

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// tmuxPatience is how long TmuxSession waits for tmux, which answers in
// milliseconds: a server that's stuck mustn't hold Claude Code's start.
const tmuxPatience = time.Second

// TmuxSession asks tmux, as PATH finds it, the name of the session the
// process runs in, as tmux display-message prints it, waiting tmuxPatience
// at most. tmux finds its server by TMUX, which the process inherits.
func TmuxSession() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxPatience)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return "", fmt.Errorf("ask tmux its session's name: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
