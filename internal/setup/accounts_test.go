package setup_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// restarted are the launchctl subcommands a run of setup runs that restarts
// the router: it asks whether the service is loaded, then restarts it.
var restarted = []string{"print", "print", "kickstart"}

// untouched are those of a run that leaves the service as it is.
var untouched = []string{"print"}

const (
	// tokenShaped is shaped like a Claude token, though it's none, as a user
	// might paste one where it shows.
	tokenShaped = "sk-ant-oat01-fake_token-shaped"
	// looksLikeAToken is what setup says of an answer shaped like one.
	looksLikeAToken = "That looks like a token, which isn't asked for here: setup asks for each token where it won't show.\n"
)

func TestTheAccountsStep(t *testing.T) {
	const listed = "work · Work: its token is usable.\n"
	const noMore = "Add another account? [y/N] \n"
	const primary = "The primary, the account the browser and the Claude apps are signed into, is work · Work.\n"
	const sideToken = "<root>/state/switchboard/tokens/side"
	const pasteSide = "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n"
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay     func(t *testing.T, w *world)
		answers []string
		want    string
		// check checks what setup left, beyond what it showed.
		check func(t *testing.T, w *world)
		// wantRan are the launchctl subcommands run.
		wantRan []string
	}{
		{
			name:    "every token usable, no account added",
			answers: []string{""},
			want:    listed + "side · Side: its token is usable.\n" + noMore + primary,
			wantRan: untouched,
		},
		{
			name:    "a missing token, taken",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{"test-token-side", ""},
			want: listed + "side · Side has no usable token: token missing: write it to " + sideToken + "\n" +
				pasteSide + "Saved side's token.\n" + noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkToken(t, "side", "test-token-side") },
			wantRan: restarted,
		},
		{
			name:    "a missing token, left for later",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{"", ""},
			want: listed + "side · Side has no usable token: token missing: write it to " + sideToken + "\n" +
				pasteSide + "side has none still: no token given. Run setup again, or switchboard accounts token side, to give it one.\n" + noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkNoToken(t, "side") },
			wantRan: untouched,
		},
		{
			name:    "a missing token the API refuses",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{refusedToken, ""},
			want: listed + "side · Side has no usable token: token missing: write it to " + sideToken + "\n" +
				pasteSide + "side has none still: the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: " +
				"make another with claude setup-token, run while signed in to that subscription. Run setup again, or switchboard accounts token side, to give it one.\n" +
				noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkNoToken(t, "side") },
			wantRan: untouched,
		},
		{
			name: "a token others can read, replaced",
			lay: func(t *testing.T, w *world) {
				if err := os.Chmod(w.tokens().Path("side"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			answers: []string{"test-token-side", ""},
			want: listed + "side · Side has no usable token: other users can read the token file (mode 0644): chmod 600 " + sideToken + "\n" +
				pasteSide + "Saved side's token.\n" + noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkToken(t, "side", "test-token-side") },
			wantRan: restarted,
		},
		{
			name:    "an account added",
			answers: []string{"y", "spare", "Spare", "test-token-spare", ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: spare\nIts label, to show it by (Enter for spare): Spare\n" +
				"Paste spare's token, from claude setup-token run while signed in to that subscription (it won't show): \n" +
				"Added spare.\n" + noMore + primary,
			check: func(t *testing.T, w *world) {
				w.checkToken(t, "spare", "test-token-spare")
				want := strings.Replace(doneConfig, "label = \"Side\"\n", "label = \"Side\"\n\n[[account]]\nid    = \"spare\"\nlabel = \"Spare\"\n", 1)
				if got := w.readConfig(t); got != want {
					t.Errorf("the config reads\n%s\nwant\n%s", got, want)
				}
			},
			wantRan: restarted,
		},
		{
			name:    "an account added, asked for an id again until it's one an account can have",
			answers: []string{"y", "my work", "spare", "", "test-token-spare", ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: my work\n" +
				"account \"my work\": id must start with a letter or digit and contain only letters, digits, '-' and '_'.\n" +
				"Its id, for good: letters, digits, - and _, such as work: spare\nIts label, to show it by (Enter for spare): \n" +
				"Paste spare's token, from claude setup-token run while signed in to that subscription (it won't show): \n" +
				"Added spare.\n" + noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkToken(t, "spare", "test-token-spare") },
			wantRan: restarted,
		},
		{
			name: "an account added, a token pasted where it shows never taken",
			answers: []string{
				"y", tokenShaped, "spare", tokenShaped, "Spare", "test-token-spare", "",
			},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: " + tokenShaped + "\n" + looksLikeAToken +
				"Its id, for good: letters, digits, - and _, such as work: spare\n" +
				"Its label, to show it by (Enter for spare): " + tokenShaped + "\n" + looksLikeAToken +
				"Its label, to show it by (Enter for spare): Spare\n" +
				"Paste spare's token, from claude setup-token run while signed in to that subscription (it won't show): \n" +
				"Added spare.\n" + noMore + primary,
			check: func(t *testing.T, w *world) {
				w.checkToken(t, "spare", "test-token-spare")
				if got := w.readConfig(t); strings.Contains(got, tokenShaped) {
					t.Errorf("the config reads\n%s\nwant no token in it", got)
				}
			},
			wantRan: restarted,
		},
		{
			name:    "an account added, keeping the usable token its token file holds",
			lay:     func(t *testing.T, w *world) { w.writeToken(t, "spare", "test-token-spare") },
			answers: []string{"y", "spare", "Spare", ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: spare\nIts label, to show it by (Enter for spare): Spare\n" +
				"Added spare, keeping the token in <root>/state/switchboard/tokens/spare.\n" + noMore + primary,
			check:   func(t *testing.T, w *world) { w.checkToken(t, "spare", "test-token-spare") },
			wantRan: restarted,
		},
		{
			name:    "an account whose token the API refuses, not added",
			answers: []string{"y", "spare", "", refusedToken, ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: spare\nIts label, to show it by (Enter for spare): \n" +
				"Paste spare's token, from claude setup-token run while signed in to that subscription (it won't show): \n" +
				"spare isn't added: the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: " +
				"make another with claude setup-token, run while signed in to that subscription.\n" + noMore + primary,
			check: func(t *testing.T, w *world) {
				w.checkNoToken(t, "spare")
				if got := w.readConfig(t); got != doneConfig {
					t.Errorf("the config reads\n%s\nwant it as it was", got)
				}
			},
			wantRan: untouched,
		},
		{
			name:    "an account configured already, not added again",
			answers: []string{"y", "side", "Side again", ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: side\nIts label, to show it by (Enter for side): Side again\n" +
				"side isn't added: account \"side\" is already configured.\n" + noMore + primary,
			wantRan: untouched,
		},
		{
			name:    "no id given for another account",
			answers: []string{"y", ""},
			want: listed + "side · Side: its token is usable.\nAdd another account? [y/N] y\n" +
				"Its id, for good: letters, digits, - and _, such as work: \n" + primary,
			wantRan: untouched,
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
			if got := section(t, shown, "1. Accounts"); got != tt.want {
				t.Errorf("the accounts step showed\n%s\nwant\n%s", got, tt.want)
			}
			if tt.check != nil {
				tt.check(t, w)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}

func TestTheAccountsStepSettlesThePrimary(t *testing.T) {
	const twoUnmarked = "[[account]]\nid    = \"work\"\nlabel = \"Work\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n\n[prime]\nday = \"08:00-23:00\"\n"
	const asked = "Which account are the browser and the Claude apps signed into? It's the primary: " +
		"Claude Code's own token is its, and the router leaves a share of its quota for the apps.\n"
	const question = "The primary, of work, side (Enter for work): "
	tests := []struct {
		name    string
		config  string
		answers []string
		want    string
		// wantConfig is the config setup leaves.
		wantConfig string
		wantRan    []string
	}{
		{
			name:       "more than one, none marked: the one named, marked",
			config:     twoUnmarked,
			answers:    []string{"", "side"},
			want:       asked + question + "side\nThe primary is side · Side.\n",
			wantConfig: strings.Replace(twoUnmarked, "label = \"Side\"\n", "label = \"Side\"\nprimary = true\n", 1),
			wantRan:    restarted,
		},
		{
			name:       "more than one, none marked: the first, as Enter gives, marked",
			config:     twoUnmarked,
			answers:    []string{"", "nope", ""},
			want:       asked + question + "nope\nThere's no account \"nope\".\n" + question + "\nThe primary is work · Work.\n",
			wantConfig: strings.Replace(twoUnmarked, "label = \"Work\"\n", "label = \"Work\"\nprimary = true\n", 1),
			wantRan:    restarted,
		},
		{
			name:       "one marked: not asked",
			config:     doneConfig,
			answers:    []string{""},
			want:       "The primary, the account the browser and the Claude apps are signed into, is work · Work.\n",
			wantConfig: doneConfig,
			wantRan:    untouched,
		},
		{
			name:       "one account, not marked: not asked",
			config:     "[[account]]\nid = \"work\"\n\n[prime]\nday = \"08:00-23:00\"\n",
			answers:    []string{""},
			want:       "The primary, the account the browser and the Claude apps are signed into, is work · work.\n",
			wantConfig: "[[account]]\nid = \"work\"\n\n[prime]\nday = \"08:00-23:00\"\n",
			wantRan:    untouched,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.writeConfig(t, tt.config)

			shown := w.runs(t, tt.answers...)
			_, got, _ := strings.Cut(section(t, shown, "1. Accounts"), "Add another account? [y/N] \n")
			if got != tt.want {
				t.Errorf("the accounts step ended\n%s\nwant\n%s", got, tt.want)
			}
			if config := w.readConfig(t); config != tt.wantConfig {
				t.Errorf("the config reads\n%s\nwant\n%s", config, tt.wantConfig)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}
