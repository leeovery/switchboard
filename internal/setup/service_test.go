package setup_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
)

func TestTheServiceStep(t *testing.T) {
	const installed = "Installed <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist: " +
		"launchd starts the router now, at every login, and whenever it stops.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay     func(t *testing.T, w *world)
		answers []string
		want    string
		wantRan []string
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			answers: []string{""},
			want:    installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed, but launchd hasn't loaded it: installed again",
			lay:     func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			answers: []string{""},
			want:    installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed and up, nothing changed: left as it is",
			answers: []string{""},
			want:    "The service is installed, and the router is up: healthy, pid 4242.\n",
			wantRan: untouched,
		},
		{
			name:    "installed and up, a token changed: restarted",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{"test-token-side", ""},
			want:    "Restarted the router, to read the config and the tokens afresh.\nThe router is up: healthy, pid 4243.\n",
			wantRan: restarted,
		},
		{
			name:    "installed, its router not answering: restarted",
			lay:     func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			answers: []string{""},
			want:    "Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: restarted,
		},
		{
			name: "not installed, no account with a usable token: installed, with a warning",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
				w.removeToken(t, "work")
				w.removeToken(t, "side")
			},
			answers: []string{"", "", ""},
			want: installed + "Warning: no account has a usable token, so the router will have nothing to route to: switchboard accounts says why.\n" +
				"The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			shown := w.runs(t, tt.answers...)
			if got := section(t, shown, "3. The service"); got != tt.want {
				t.Errorf("the service step showed\n%s\nwant\n%s", got, tt.want)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}

func TestTheServiceStepInstallsTheServiceToRunThisSwitchboard(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(w.plist()); err != nil {
		t.Fatal(err)
	}
	w.launchd.loaded, w.launchd.pid = false, 0

	w.runs(t, "")
	plist, err := os.ReadFile(w.plist())
	if err != nil || !strings.Contains(string(plist), "<array>\n\t\t<string>"+w.switchboard+"</string>\n\t\t<string>serve</string>\n\t</array>") {
		t.Errorf("the plist reads\n%s(%v)\nwant it to run switchboard serve by the path switchboard was run by, %s", plist, err, w.switchboard)
	}
}

func TestSetupStopsWhenTheRouterDoesntAnswer(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(w.plist()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(w.skill); err != nil {
		t.Fatal(err)
	}
	w.launchd.loaded, w.launchd.pid, w.launchd.dead = false, 0, true
	synctest.Test(t, func(t *testing.T) {
		shown, err := w.run(t, "")
		want := "the router didn't answer within 5s of starting: see why with switchboard logs router, and in " + w.state + "/logs/launchd.log"
		if err == nil || err.Error() != want {
			t.Errorf("Run() error = %v, want %q", err, want)
		}
		if strings.Contains(shown, "4. The claude link") {
			t.Errorf("the terminal showed\n%s\nwant setup stopped at the service", shown)
		}
	})
	if _, err := os.Stat(w.skill); err == nil {
		t.Error("the skill is installed, want setup stopped before it")
	}
}
