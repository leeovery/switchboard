package ledger_test

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
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

func TestASessionsNameIsGivenAsALineGivesItCleanedAndCut(t *testing.T) {
	const nameMost = 100
	tests := []struct {
		name, given, want string
	}{
		{name: "a terminal session's name", given: "deploy", want: "deploy"},
		{name: "none", given: "", want: ""},
		{name: "spaces kept between words", given: "fix login", want: "fix login"},
		{name: "control characters and runs of spaces cleaned", given: " fix\x1b[31m\tlogin\n ", want: "fix [31m login"},
		{name: "a Nerd Font glyph kept", given: "\uf1bb web", want: "\uf1bb web"},
		{name: "as long as a line holds, whole", given: strings.Repeat("a", nameMost), want: strings.Repeat("a", nameMost)},
		{name: "longer, cut at the end of a character", given: strings.Repeat("☃", nameMost), want: strings.Repeat("☃", nameMost/len("☃"))},
		{name: "cut after a space, the space let go", given: strings.Repeat("a", nameMost-1) + " b", want: strings.Repeat("a", nameMost-1)},
		{name: "a token hidden", given: "sk-ant-oat01-fake_name-token-shaped", want: redact.Placeholder},
		{name: "a token split by a control character hidden up to it", given: "sk-ant-oat01-fake\x07_name", want: redact.Placeholder + " _name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.LineName(tt.given)
			if got != tt.want {
				t.Errorf("LineName(%q) = %q, want %q", tt.given, got, tt.want)
			}
			if again := ledger.LineName(got); again != got {
				t.Errorf("LineName(%q) = %q, want it as it is, as a line given it gives it again", got, again)
			}
		})
	}
}

// TestASessionsNameHoldsNoTokenACharacterThatDoesntShowSplits holds LineName
// to hiding a token split by a zero-width space, which prose.Clean drops, so
// joining the token again: a name it gives holds none, nor does it once it's
// cleaned again to show.
func TestASessionsNameHoldsNoTokenACharacterThatDoesntShowSplits(t *testing.T) {
	for _, given := range []string{
		"sk-ant-\u200boat01-fake_name-token-shaped",
		"sk-ant-oat01-fake\u200b_name-token-shaped",
	} {
		got := ledger.LineName(given)
		if redact.HoldsToken(got) || redact.HoldsToken(prose.Clean(got)) {
			t.Errorf("LineName(%q) = %q, want no token in it, nor in it cleaned again", given, got)
		}
	}
}
