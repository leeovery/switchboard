package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/skill"
)

func TestTheSkillStep(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay  func(t *testing.T, w *world)
		want string
		// wantWritten is set when the skill is written.
		wantWritten bool
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.RemoveAll(filepath.Join(w.root, "claude")); err != nil {
					t.Fatal(err)
				}
			},
			want:        "Installed the skill, which tells Claude what switchboard does under claude: <root>/claude/skills/switchboard/SKILL.md\n",
			wantWritten: true,
		},
		{
			name:        "older than this switchboard's: brought up to date",
			lay:         func(t *testing.T, w *world) { writeFile(t, w.skill, "---\nname: switchboard\n---\n", 0o644) },
			want:        "Brought the skill up to date: <root>/claude/skills/switchboard/SKILL.md\n",
			wantWritten: true,
		},
		{
			name: "up to date: left as it is",
			want: "The skill is installed, and up to date: <root>/claude/skills/switchboard/SKILL.md\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}
			before := w.snapshot(t)

			shown := w.runs(t, "")
			if got := section(t, shown, "5. The skill"); got != tt.want {
				t.Errorf("the skill step showed\n%s\nwant\n%s", got, tt.want)
			}
			found, err := skill.Refresh(w.skill)
			if err != nil || !found.Installed || found.Rewritten {
				t.Errorf("the skill at %s: %+v, %v; want this switchboard's installed", w.skill, found, err)
			}
			if !tt.wantWritten {
				w.checkUnchanged(t, before)
			}
		})
	}
}
