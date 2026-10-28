package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/sessionhook"
)

// withoutTheHook starts what setup says where the session hook can't be
// installed, before why.
const withoutTheHook = "Setup carries on without the session hook, so a session that doesn't go through switchboard won't say so: "

func TestTheSessionHook(t *testing.T) {
	const settings = "<root>/claude/settings.json"
	removeSettings := func(t *testing.T, w *world) {
		if err := os.Remove(w.settings); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay func(t *testing.T, w *world)
		// want is the link step's last line.
		want string
		// wantWritten is set where the settings file is written; wantLeft
		// where it's left as it was, though not as it should be.
		wantWritten, wantLeft bool
	}{
		{
			name:        "not installed: installed",
			lay:         removeSettings,
			want:        "Installed the session hook, which tells a session that doesn't go through switchboard so: " + settings + "\n",
			wantWritten: true,
		},
		{
			name: "not installed, and claude not linked: installed all the same",
			lay: func(t *testing.T, w *world) {
				removeSettings(t, w)
				claudeLink := filepath.Join(w.bin, "claude")
				if err := os.Remove(claudeLink); err != nil {
					t.Fatal(err)
				}
				writeFile(t, claudeLink, "#!/bin/sh\n", 0o700)
			},
			want:        "Installed the session hook, which tells a session that doesn't go through switchboard so: " + settings + "\n",
			wantWritten: true,
		},
		{
			name: "running another switchboard: put right",
			lay: func(t *testing.T, w *world) {
				if _, err := sessionhook.Install(w.settings, w.goInstalled(t)); err != nil {
					t.Fatal(err)
				}
			},
			want:        "Put the session hook right: " + settings + "\n",
			wantWritten: true,
		},
		{
			name: "in place: nothing written",
			want: "The session hook is in place: " + settings + "\n",
		},
		{
			name:     "settings that aren't an object: left as they are, and setup carries on",
			lay:      func(t *testing.T, w *world) { writeFile(t, w.settings, "[]\n", 0o644) },
			want:     withoutTheHook + settings + ": not a JSON object.\n",
			wantLeft: true,
		},
		{
			name: "switchboard run by another name: left as it is, and setup carries on",
			lay: func(t *testing.T, w *world) {
				version := filepath.Join(w.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard")
				w.switchboard = claudetest.Link(t, version, filepath.Join(w.home, "bin", "sb"))
				w.installService(t, w.switchboard)
				claudeLink := filepath.Join(w.bin, "claude")
				if err := os.Remove(claudeLink); err != nil {
					t.Fatal(err)
				}
				claudetest.Link(t, w.switchboard, claudeLink)
			},
			want:     withoutTheHook + "switchboard is running as sb here, so the session hook isn't installed: run setup as switchboard.\n",
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

			shown := w.runs(t, "")
			if got := section(t, shown, "4. The claude link"); !strings.HasSuffix(got, "\n"+tt.want) {
				t.Errorf("the claude link step showed\n%s\nwant it to end\n%s", got, tt.want)
			}
			if !strings.Contains(shown, "\n5. The skill\n") {
				t.Errorf("the terminal showed\n%s\nwant setup to carry on to the skill", shown)
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
