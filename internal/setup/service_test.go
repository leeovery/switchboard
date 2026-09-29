package setup_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
)

// restarted are the launchctl subcommands a run of setup runs that restarts
// the router: it asks whether the service is loaded, then restarts it.
var restarted = []string{"print", "print", "kickstart"}

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
			name:    "installed and up, a token changed: left to the router, which takes it up",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{"test-token-side", ""},
			want:    "The service is installed, and the router is up: healthy, pid 4242. It takes up setup's changes on its own.\n",
			wantRan: untouched,
		},
		{
			name: "installed and up, the config changed: left to the router, which takes it up",
			lay: func(t *testing.T, w *world) {
				w.writeConfig(t, strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n"))
			},
			answers: []string{"", "07:00-22:00"},
			want:    "The service is installed, and the router is up: healthy, pid 4242. It takes up setup's changes on its own.\n",
			wantRan: untouched,
		},
		{
			name:    "installed from another switchboard: installed afresh, to run this one",
			lay:     func(t *testing.T, w *world) { w.installService(t, w.goInstalled(t)) },
			answers: []string{""},
			want: "The service runs <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard, so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4244.\n",
			wantRan: []string{"print", "print", "bootout", "bootstrap"},
		},
		{
			name:    "installed with a plist naming no program: installed afresh, to run this switchboard",
			lay:     func(t *testing.T, w *world) { writeFile(t, w.plist(), "<plist/>\n", 0o644) },
			answers: []string{""},
			want: "The service runs a program its plist doesn't name, not this switchboard, <root>/brew/bin/switchboard, so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootout", "bootstrap"},
		},
		{
			name:    "installed, its router not answering: restarted",
			lay:     func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			answers: []string{""},
			want:    "Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: restarted,
		},
		{
			name:    "installed, its router answering only once setup restarts it: restarted as it finishes its requests",
			lay:     func(_ *testing.T, w *world) { w.launchd.unanswered = 1 },
			answers: []string{""},
			want: "The router is finishing its requests in flight, then launchd starts it again.\n" +
				"Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "kill"},
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
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay func(t *testing.T, w *world)
	}{
		{
			name: "not installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
		},
		{
			name: "installed from another switchboard",
			lay:  func(t *testing.T, w *world) { w.installService(t, w.goInstalled(t)) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)

			w.runs(t, "")
			plist, err := os.ReadFile(w.plist())
			if err != nil || !strings.Contains(string(plist), "<array>\n\t\t<string>"+w.switchboard+"</string>\n\t\t<string>serve</string>\n\t</array>") {
				t.Errorf("the plist reads\n%s(%v)\nwant it to run switchboard serve by the path switchboard was run by, %s", plist, err, w.switchboard)
			}
		})
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
