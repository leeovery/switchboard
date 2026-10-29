package setup_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/setup"
)

// rootedSaid is said with the world's root written <root>, as the tests write
// it.
func (w *world) rootedSaid(said []setup.Said) []setup.Said {
	out := make([]setup.Said, len(said))
	for i, s := range said {
		s.Detail = strings.ReplaceAll(s.Detail, w.root, "<root>")
		s.Sentence = strings.ReplaceAll(s.Sentence, w.root, "<root>")
		out[i] = s
	}
	return out
}

func TestServiceSaysWhatTheServicePageShows(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.installServiceWith(t, service.InstallOptions{Executable: w.switchboard, LogLevel: "debug"})
	w.editPlist(t, withoutProcessType)
	w.launchd.version = "0.2.10"

	got, err := w.setup(t, &user{}).Service(t.Context())
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	want := []setup.Said{
		{
			Mark:     setup.Warning,
			Label:    "Updated",
			Detail:   "reinstalled: its plist had no ProcessType · kept its --log-level",
			Sentence: "The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.",
		},
		{
			Mark:     setup.Done,
			Label:    "Installed",
			Detail:   "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist",
			Sentence: strings.TrimSuffix(installedAt, "\n"),
		},
		{
			Mark:     setup.Done,
			Label:    "Router",
			Detail:   "healthy · 127.0.0.1:4747 · v0.2.10",
			Sentence: "The router is up: healthy, pid 4244.",
		},
	}
	if said := w.rootedSaid(got.Said); !reflect.DeepEqual(said, want) {
		t.Errorf("Service() said\n%#v\nwant\n%#v", said, want)
	}
	if got.Router == nil || got.Router.PID != 4244 || got.Router.Version != "0.2.10" {
		t.Errorf("Service() router = %+v, want the one launchd started, pid 4244, v0.2.10", got.Router)
	}
}

func TestServiceSaysEachOutcomeAsTheLineWalkDoes(t *testing.T) {
	// said is a Said without its sentence, which the line walk's tests hold.
	type said struct {
		mark          setup.Mark
		label, detail string
	}
	const plist = "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist"
	tests := []struct {
		name string
		// lay changes the world, which is done, before the step.
		lay        func(t *testing.T, w *world)
		want       []said
		wantRouter bool
	}{
		{
			name:       "installed and up: the router alone",
			want:       []said{{setup.Done, "Router", "healthy · 127.0.0.1:4747"}},
			wantRouter: true,
		},
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			want:       []said{{setup.Done, "Installed", plist}, {setup.Done, "Router", "healthy · 127.0.0.1:4747"}},
			wantRouter: true,
		},
		{
			name: "installed from another switchboard, with a config file and a log level: updated, keeping both, never their values",
			lay: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.goInstalled(t), Config: w.config, LogLevel: "warn"})
			},
			want: []said{
				{setup.Warning, "Updated", "reinstalled: it ran <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard · kept its --config and --log-level"},
				{setup.Done, "Installed", plist},
				{setup.Done, "Router", "healthy · 127.0.0.1:4747"},
			},
			wantRouter: true,
		},
		{
			name: "given another config file than the router serves: updated, keeping the log level alone",
			lay: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.switchboard, Config: filepath.Join(w.root, "elsewhere.toml"), LogLevel: "debug"})
				w.configFlag = w.config
			},
			want: []said{
				{setup.Warning, "Updated", "reinstalled: setup is given another config file than the one the router served · kept its --log-level"},
				{setup.Done, "Installed", plist},
				{setup.Done, "Router", "healthy · 127.0.0.1:4747"},
			},
			wantRouter: true,
		},
		{
			name: "its router not answering: restarted",
			lay:  func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			want: []said{
				{setup.Warning, "Restarted", "the router didn't answer"},
				{setup.Done, "Router", "healthy · 127.0.0.1:4747"},
			},
			wantRouter: true,
		},
		{
			name: "a router run by hand answering: failed, with no router of the service's",
			lay:  func(_ *testing.T, w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			want: []said{{setup.Failed, "Router run by hand", "a router run some other way answers, pid 5000, while launchd has the service loaded, but runs no router: " +
				"setup leaves them as they are; stop the router run by hand, then run switchboard service install"}},
		},
		{
			name: "a plist that can't be read back, a router run by hand answering: failed, saying why, redacted",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
				w.launchd.pid, w.launchd.byHand = 0, 5000
			},
			want: []said{{setup.Failed, "Router run by hand", "the service's plist can't be read back (" + unreadBinary + "), and a router run some other way answers, pid 5000, " +
				"while launchd has the service loaded, but runs no router: setup leaves them as they are; stop the router run by hand, then run switchboard service install"}},
		},
		{
			name: "where the router finds its state can't be worked out, a router run by hand answering: failed, as one clause",
			lay: func(t *testing.T, w *world) {
				setsHomeEmpty(t, w, "XDG_STATE_HOME", "state")
				w.launchd.pid, w.launchd.byHand = 0, 5000
			},
			want: []said{{setup.Failed, "Router run by hand", "where the service's router finds its state or its config can't be worked out from its plist " +
				"(locate state directory: the service's plist sets HOME empty, where the router finds no home directory), and a router run some other way answers, pid 5000, " +
				"while launchd has the service loaded, but runs no router: setup leaves them as they are; stop the router run by hand, then run switchboard service install"}},
		},
	}
	// laid is a world, done, as lay changes it.
	laid := func(t *testing.T, lay func(t *testing.T, w *world)) *world {
		w := newWorld(t)
		w.done(t)
		if lay != nil {
			lay(t, w)
		}
		return w
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := laid(t, tt.lay)

			got, _ := w.setup(t, &user{}).Service(t.Context())
			var marked []said
			var sentences strings.Builder
			for _, s := range w.rootedSaid(got.Said) {
				marked = append(marked, said{s.Mark, s.Label, s.Detail})
				if s.Sentence != "" {
					sentences.WriteString(s.Sentence + "\n")
				}
			}
			walked, _ := laid(t, tt.lay).run(t, "")
			if printed := section(t, walked, "3. The service"); sentences.String() != printed {
				t.Errorf("Service() said the sentences\n%s\nbut the line walk printed\n%s", sentences.String(), printed)
			}
			if !reflect.DeepEqual(marked, tt.want) {
				t.Errorf("Service() said\n%+v\nwant\n%+v", marked, tt.want)
			}
			if (got.Router != nil) != tt.wantRouter {
				t.Errorf("Service() router = %+v, want one: %v", got.Router, tt.wantRouter)
			}
		})
	}
}

func TestServiceNeedsAnAccount(t *testing.T) {
	w := newWorld(t)
	got, err := w.setup(t, &user{}).Service(t.Context())
	if err == nil || !strings.HasPrefix(err.Error(), "no account is configured") {
		t.Fatalf("Service() error = %v, want no account configured", err)
	}
	want := []setup.Said{{Mark: setup.Failed, Label: "Not set up", Detail: err.Error()}}
	if !reflect.DeepEqual(got.Said, want) {
		t.Errorf("Service() said %+v, want %+v: a failure the step said nothing of, as the screen draws it, the line walk printing none", got.Said, want)
	}
}

func TestServiceEndsAFailureWithAFailedRow(t *testing.T) {
	tests := []struct {
		name string
		lay  func(t *testing.T, w *world)
		// want are the marks and labels said, in turn, and wantDetail what the
		// last one's detail says, and never what's token-shaped.
		want       []string
		wantDetail string
	}{
		{
			name: "failing after a warning: the warning, then Not set up, why redacted",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, withoutProcessType)
				w.launchd.bootoutSays = "Boot-out failed: " + tokenShaped
			},
			want:       []string{"! Updated", "✗ Not set up"},
			wantDetail: "io.github.leeovery.switchboard: Boot-out failed: " + redact.Placeholder,
		},
		{
			name:       "failing as a row of its own says: that row alone",
			lay:        func(_ *testing.T, w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			want:       []string{"✗ Router run by hand"},
			wantDetail: "a router run some other way answers, pid 5000",
		},
	}
	marks := map[setup.Mark]string{setup.Done: "✓", setup.Warning: "!", setup.Failed: "✗"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)

			got, err := w.setup(t, &user{}).Service(t.Context())
			if err == nil {
				t.Fatal("Service() error = nil, want it to fail")
			}
			var said []string
			for _, s := range got.Said {
				said = append(said, marks[s.Mark]+" "+s.Label)
				if strings.Contains(s.Detail, tokenShaped) {
					t.Errorf("Service() said %+v, want what's token-shaped redacted", s)
				}
			}
			if !slices.Equal(said, tt.want) {
				t.Errorf("Service() said %q, want %q", said, tt.want)
			}
			if last := got.Said[len(got.Said)-1]; !strings.Contains(last.Detail, tt.wantDetail) {
				t.Errorf("Service()'s last row says %q, want it to hold %q", last.Detail, tt.wantDetail)
			}
		})
	}
}

func TestPrimingAndSetDay(t *testing.T) {
	dayless := strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n")
	tests := []struct {
		name   string
		config string
		day    string
		// want is the day Priming returns once SetDay is done, and
		// wantConfig the config it leaves.
		want       string
		wantConfig string
	}{
		{name: "set, where priming is off", config: dayless, day: "07:00-22:00", want: "07:00-22:00", wantConfig: dayless + "\n[prime]\nday = \"07:00-22:00\"\n"},
		{name: "set again, over another", config: doneConfig, day: "07:00-22:00", want: "07:00-22:00", wantConfig: dayless + "\n[prime]\nday = \"07:00-22:00\"\n"},
		{name: "cleared: priming turned off", config: doneConfig, day: "", want: "", wantConfig: dayless + "\n[prime]\n"},
		{name: "cleared, where priming is off: left as it is", config: dayless, day: "", want: "", wantConfig: dayless},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.writeConfig(t, tt.config)
			s := w.setup(t, &user{})

			set, err := s.SetDay(tt.day)
			if err != nil {
				t.Fatalf("SetDay(%q) error = %v", tt.day, err)
			}
			got, err := s.Priming()
			switch {
			case err != nil:
				t.Fatalf("Priming() error = %v", err)
			case got != set:
				t.Errorf("SetDay(%q) = %v, but Priming() = %v", tt.day, set, got)
			case dayOf(got) != tt.want:
				t.Errorf("Priming() = %q, want %q", dayOf(got), tt.want)
			}
			if got := w.readConfig(t); got != tt.wantConfig {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.wantConfig)
			}
		})
	}
}

// dayOf is a day as [prime] gives it, "" for none.
func dayOf(day config.Day) string {
	if day == (config.Day{}) {
		return ""
	}
	return day.String()
}

func TestSetDayTellsTheServiceOfAChangedDayAlone(t *testing.T) {
	const takesUp = " · takes up setup's changes on its own"
	tests := []struct {
		name, day   string
		wantTakesUp bool
	}{
		{name: "a day changed", day: "07:00-22:00", wantTakesUp: true},
		{name: "a day left as it was", day: "08:00-23:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			s := w.setup(t, &user{})

			if _, err := s.SetDay(tt.day); err != nil {
				t.Fatalf("SetDay(%q) error = %v", tt.day, err)
			}
			got, err := s.Service(t.Context())
			if err != nil || len(got.Said) != 1 {
				t.Fatalf("Service() = %+v, %v; want the router alone", got, err)
			}
			if takes := strings.HasSuffix(got.Said[0].Detail, takesUp); takes != tt.wantTakesUp {
				t.Errorf("Service() said %q, want the router taking up setup's changes: %v", got.Said[0].Detail, tt.wantTakesUp)
			}
		})
	}
}

func TestSetDayNeverMakesTheConfigFile(t *testing.T) {
	tests := []struct {
		name, day string
		wantErr   bool
	}{
		{name: "a day cleared: nothing to do", day: ""},
		{name: "a day set: no account to prime", day: "08:00-23:00", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)

			_, err := w.setup(t, &user{}).SetDay(tt.day)
			if (err != nil) != tt.wantErr || (err != nil && !strings.HasPrefix(err.Error(), "no account is configured")) {
				t.Errorf("SetDay(%q) error = %v, want no account configured: %v", tt.day, err, tt.wantErr)
			}
			if _, err := os.Stat(w.config); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the config file: %v, want none made", err)
			}
		})
	}
}

func TestSetDayRefusesADayThatIsntOne(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, doneConfig)

	_, err := w.setup(t, &user{}).SetDay("8-23")
	if want := `prime.day "8-23": must be two times of day, HH:MM, joined by -, such as 08:00-23:00`; err == nil || err.Error() != want {
		t.Errorf("SetDay() error = %v, want %q", err, want)
	}
	if got := w.readConfig(t); got != doneConfig {
		t.Errorf("the config reads\n%s\nwant it as it was\n%s", got, doneConfig)
	}
}

func TestSkillSays(t *testing.T) {
	const at = "<root>/claude/skills/switchboard/SKILL.md"
	tests := []struct {
		name string
		lay  func(t *testing.T, w *world)
		want setup.Said
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.RemoveAll(filepath.Join(w.root, "claude")); err != nil {
					t.Fatal(err)
				}
			},
			want: setup.Said{Mark: setup.Done, Label: "Installed", Detail: at, Sentence: "Installed the skill, which tells Claude what switchboard does under claude: " + at},
		},
		{
			name: "older: updated",
			lay:  func(t *testing.T, w *world) { writeFile(t, w.skill, "---\nname: switchboard\n---\n", 0o644) },
			want: setup.Said{Mark: setup.Done, Label: "Updated", Detail: at, Sentence: "Brought the skill up to date: " + at},
		},
		{
			name: "up to date",
			want: setup.Said{Mark: setup.Done, Label: "Up to date", Detail: at, Sentence: "The skill is installed, and up to date: " + at},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			got, err := w.setup(t, &user{}).Skill()
			if err != nil {
				t.Fatalf("Skill() error = %v", err)
			}
			if said := w.rootedSaid([]setup.Said{got})[0]; said != tt.want {
				t.Errorf("Skill() = %+v, want %+v", said, tt.want)
			}
		})
	}
}

func TestSkillSaysWhyItFailed(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	claudeDir := filepath.Join(w.root, "claude")
	if err := os.RemoveAll(claudeDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, claudeDir, "not a directory\n", 0o644)

	got, err := w.setup(t, &user{}).Skill()
	if err == nil {
		t.Fatal("Skill() error = nil, want why the skill couldn't be written")
	}
	want := setup.Said{Mark: setup.Failed, Label: "Not installed", Detail: err.Error()}
	if got != want {
		t.Errorf("Skill() = %+v, want %+v: failed, the error's words its detail, and no sentence for the line walk", got, want)
	}
}

func TestSessionHookSays(t *testing.T) {
	const at = "<root>/claude/settings.json"
	const does = "Claude Code says so when a session doesn't start through switchboard · " + at
	tests := []struct {
		name string
		lay  func(t *testing.T, w *world)
		want setup.Said
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.settings); err != nil {
					t.Fatal(err)
				}
			},
			want: setup.Said{Mark: setup.Done, Label: "Session hook", Detail: does, Sentence: "Installed the session hook, which tells a session that doesn't go through switchboard so: " + at},
		},
		{
			name: "running another switchboard: put right",
			lay: func(t *testing.T, w *world) {
				if _, err := sessionhook.Install(w.settings, w.goInstalled(t)); err != nil {
					t.Fatal(err)
				}
			},
			want: setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "put right: " + does, Sentence: "Put the session hook right: " + at},
		},
		{
			name: "in place",
			want: setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "in place: " + does, Sentence: "The session hook is in place: " + at},
		},
		{
			name: "settings that aren't an object: a warning, carrying on",
			lay:  func(t *testing.T, w *world) { writeFile(t, w.settings, "[]\n", 0o644) },
			want: setup.Said{
				Mark:     setup.Warning,
				Label:    "Session hook",
				Detail:   "setup carries on without it, so a session that doesn't go through switchboard won't say so: " + at + ": not a JSON object",
				Sentence: withoutTheHook + at + ": not a JSON object.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			got := w.setup(t, &user{}).SessionHook()
			if said := w.rootedSaid([]setup.Said{got})[0]; said != tt.want {
				t.Errorf("SessionHook() = %+v, want %+v", said, tt.want)
			}
		})
	}
}

func TestServiceRedactsReasonsInBothForms(t *testing.T) {
	tests := []struct {
		name string
		lay  func(t *testing.T, w *world)
		// wantDetail is in a Said's detail, redacted.
		wantDetail string
	}{
		{
			name: "why the plist can't be read",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
				w.plutilSays = "plutil: not a plist, but " + tokenShaped
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			wantDetail: "reinstalled: its plist couldn't be read back (",
		},
		{
			name: "why the router couldn't start",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, withoutProcessType)
				w.unstartable = errors.New("the config holds " + tokenShaped)
			},
			wantDetail: "it couldn't start as: the config holds ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)

			got, _ := w.setup(t, &user{}).Service(t.Context())
			var details []string
			for _, said := range got.Said {
				if strings.Contains(said.Detail, tokenShaped) || strings.Contains(said.Sentence, tokenShaped) {
					t.Errorf("Service() said %+v, want what's token-shaped redacted", said)
				}
				details = append(details, said.Detail)
			}
			if !strings.Contains(strings.Join(details, "\n"), tt.wantDetail) {
				t.Errorf("Service() said details\n%s\nwant one holding %q", strings.Join(details, "\n"), tt.wantDetail)
			}
		})
	}
}
