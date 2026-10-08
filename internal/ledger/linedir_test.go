package ledger_test

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/redact"
)

func TestADirectoryIsGivenAsALineGivesIt(t *testing.T) {
	long := "~/" + strings.Repeat("a", 300) + "/project"
	tests := []struct {
		name, dir, want string
	}{
		{name: "the home as run tells it", dir: "~/Code/project", want: "~/Code/project"},
		{name: "outside the home", dir: "/srv/project", want: "/srv/project"},
		{name: "none", dir: "", want: ""},
		{name: "long, cut from its front", dir: long, want: "…" + strings.Repeat("a", 200-len("…/project")) + "/project"},
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
