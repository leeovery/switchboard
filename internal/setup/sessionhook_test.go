package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/setup"
)

func TestSessionHookSays(t *testing.T) {
	const at = "<root>/claude/settings.json"
	const does = "Claude Code says so when a session doesn't start through switchboard · " + at
	const without = "setup carries on without it, so a session that doesn't go through switchboard won't say so: "
	tests := []struct {
		name string
		// lay changes the world, which is done, before the session hook.
		lay  func(t *testing.T, w *world)
		want setup.Said
		// wantWritten is set where the settings file is written; wantLeft
		// where it's left as it was, though not as it should be.
		wantWritten, wantLeft bool
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.settings); err != nil {
					t.Fatal(err)
				}
			},
			want:        setup.Said{Mark: setup.Done, Label: "Session hook", Detail: does},
			wantWritten: true,
		},
		{
			name: "running another switchboard: put right",
			lay: func(t *testing.T, w *world) {
				if _, err := sessionhook.Install(w.settings, w.goInstalled(t)); err != nil {
					t.Fatal(err)
				}
			},
			want:        setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "put right: " + does},
			wantWritten: true,
		},
		{
			name: "in place: nothing written",
			want: setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "in place: " + does},
		},
		{
			name:     "settings that aren't an object: left as they are, a warning, carrying on",
			lay:      func(t *testing.T, w *world) { writeFile(t, w.settings, "[]\n", 0o644) },
			want:     setup.Said{Mark: setup.Warning, Label: "Session hook", Detail: without + at + ": not a JSON object"},
			wantLeft: true,
		},
		{
			name: "switchboard run by another name: left as it is, a warning, carrying on",
			lay: func(t *testing.T, w *world) {
				version := filepath.Join(w.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard")
				w.switchboard = claudetest.Link(t, version, filepath.Join(w.home, "bin", "sb"))
			},
			want: setup.Said{
				Mark:   setup.Warning,
				Label:  "Session hook",
				Detail: without + "switchboard is running as sb here, so the session hook isn't installed: run setup as switchboard",
			},
			wantLeft: true,
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

			got := w.setup(t).SessionHook()
			if said := w.rootedSaid([]setup.Said{got})[0]; said != tt.want {
				t.Errorf("SessionHook() = %+v, want %+v", said, tt.want)
			}
			if tt.wantWritten {
				before[w.settings] = w.snapshot(t)[w.settings]
			}
			w.checkUnchanged(t, before)
			if tt.wantLeft {
				return
			}
			if got, err := sessionhook.Install(w.settings, w.switchboard); err != nil || got != sessionhook.InPlace {
				t.Errorf("the session hook in %s: %v, %v; want it in place, running %s", w.settings, got, err, w.switchboard)
			}
		})
	}
}
