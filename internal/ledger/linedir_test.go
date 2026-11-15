package ledger_test

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/redact"
)

// pathMost is how many bytes of a directory or a project a line holds.
const pathMost = 1024

// scratchpad is a directory as long as those a session's tooling starts
// claude in, within Claude Code's scratchpad, past the 200 bytes a line holds
// of any other text.
const scratchpad = "/private/tmp/claude-501/-Users-tester-Code-acme-platform--claude-worktrees-agent-0f3a8c21/7c1e9d2a-4b6f-4e1a-9c3d-2f8b7a6e5d4c/scratchpad/29.18-18.7/worlds/b32-1/project/.claude/worktrees/agent-9d86a259/web"

func TestADirectoryIsGivenAsALineGivesIt(t *testing.T) {
	longest := "~/" + strings.Repeat("a", pathMost-len("~//project")) + "/project"
	longer := "~/" + strings.Repeat("a", pathMost) + "/project"
	tests := []struct {
		name, dir, want string
	}{
		{name: "the home as run tells it", dir: "~/Code/project", want: "~/Code/project"},
		{name: "outside the home", dir: "/srv/project", want: "/srv/project"},
		{name: "none", dir: "", want: ""},
		{name: "a scratchpad's, past 200 bytes, whole", dir: scratchpad, want: scratchpad},
		{name: "as long as a line holds, whole", dir: longest, want: longest},
		{name: "longer, cut from its front", dir: longer, want: "…" + strings.Repeat("a", pathMost-len("…/project")) + "/project"},
		{name: "a token hidden", dir: "~/Code/sk-ant-oat01-fake_dir-token-shaped/project", want: "~/Code/" + redact.Placeholder + "/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.LineDir(tt.dir)
			if got != tt.want {
				t.Errorf("LineDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
			if again := ledger.LineDir(got); again != got {
				t.Errorf("LineDir(%q) = %q, want it as it is, as a line given it cuts it again", got, again)
			}
		})
	}
}

func TestAProjectIsGivenAsALineGivesItWholeOrNotAtAll(t *testing.T) {
	longest := "~/" + strings.Repeat("a", pathMost-len("~/"))
	tests := []struct {
		name, project, want string
	}{
		{name: "the home as run tells it", project: "~/Code/api", want: "~/Code/api"},
		{name: "outside the home", project: "/srv/api", want: "/srv/api"},
		{name: "none", project: "", want: ""},
		{name: "a scratchpad's, past 200 bytes, whole", project: scratchpad, want: scratchpad},
		{name: "as long as a line holds, whole", project: longest, want: longest},
		{name: "longer than a line holds, none, as a cut one would name another", project: longest + "a", want: ""},
		{name: "a token hidden", project: "~/Code/sk-ant-oat01-fake_dir-token-shaped/api", want: "~/Code/" + redact.Placeholder + "/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.LineProject(tt.project)
			if got != tt.want {
				t.Errorf("LineProject(%q) = %q, want %q", tt.project, got, tt.want)
			}
			if again := ledger.LineProject(got); again != got {
				t.Errorf("LineProject(%q) = %q, want it as it is, as a line given it gives it again", got, again)
			}
		})
	}
}
