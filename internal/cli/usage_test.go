package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard"
)

func TestUsagePrettyOffATerminalIsPrintedWithoutColourUnlessCLIColorForceAsks(t *testing.T) {
	truecolor := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}
	tests := []struct {
		name string
		// force is CLICOLOR_FORCE, "" for unset.
		force      string
		wantColour bool
	}{
		{name: "without CLICOLOR_FORCE, none"},
		{name: "with CLICOLOR_FORCE, in colour", force: "1", wantColour: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(truecolor)
			if tt.force != "" {
				env["CLICOLOR_FORCE"] = tt.force
			}

			got := offATerminal.run(t, goldenDeps(t, env), "usage", "--pretty")
			coloured, escaped := strings.Contains(got.stdout, "38;2;"), strings.Contains(got.stdout, "\x1b[")
			if got.code != 0 || coloured != tt.wantColour || escaped != tt.wantColour {
				t.Fatalf("switchboard usage --pretty, off a terminal, = %+v, want exit status 0, and in colour: %v", got, tt.wantColour)
			}
			// Without colour, a bar's fill is drawn in █; in colour, its cells
			// are spaces on the ramp's colours.
			noColour := maps.Clone(env)
			noColour["NO_COLOR"] = "1"
			want := ansi.Strip(run(t, goldenDeps(t, noColour), "usage").stdout)
			if tt.wantColour {
				want = strings.ReplaceAll(want, "█", " ")
			}
			if !strings.Contains(want, "█") && !tt.wantColour {
				t.Fatalf("switchboard usage under NO_COLOR printed\n%s\nwant its bars' fills drawn in █", want)
			}
			if ansi.Strip(got.stdout) != want {
				t.Errorf("switchboard usage --pretty, off a terminal, printed, stripped of its escapes,\n%s\nwant what it prints on one under NO_COLOR%s\n%s",
					ansi.Strip(got.stdout), map[bool]string{true: ", its █ fills spaces", false: ""}[tt.wantColour], want)
			}
		})
	}
}

func TestUsageRefreshWithoutTheRouterProbesAsUsageDoes(t *testing.T) {
	// usage runs switchboard usage with args, set up as goldenDeps sets it
	// up, and returns how it went and what the API was asked.
	usage := func(args ...string) (result, []string) {
		api := newClaudeAPI(t)
		deps := statusDeps(t, api.URL, nil)
		onATerminal.on(&deps)
		writeToken(t, deps, "personal", "test-token-personal")
		return run(t, deps, append([]string{"usage"}, args...)...), api.questions()
	}
	want, probes := usage()
	if want.code != 0 || want.stderr != "" || len(probes) == 0 || !strings.Contains(want.stdout, "\n○ probed · router not running\n") {
		t.Fatalf("switchboard usage = %+v, asking the API %q; want exit status 0, nothing on stderr, and every account probed, as the printout says", want, probes)
	}
	for _, label := range []string{"Work", "Personal", "Side"} {
		if printedRow(want.stdout, "5-HOUR", label) == "" {
			t.Errorf("switchboard usage printed\n%s\nwant a row of %s's 5-hour window", want.stdout, label)
		}
	}
	for _, flag := range []string{"--refresh", "-r"} {
		t.Run(flag, func(t *testing.T) {
			got, asked := usage(flag)
			if got != want {
				t.Errorf("switchboard usage %s =\n%+v\nwant what usage prints\n%+v", flag, got, want)
			}
			if !slices.Equal(asked, probes) {
				t.Errorf("switchboard usage %s asked the API %q, want %q, every account probed as usage probes it", flag, asked, probes)
			}
		})
	}
}

func TestUsageAsJSONPrintsWhatStatusJSONPrints(t *testing.T) {
	tests := []struct {
		name string
		// args are given to usage and to status --json alike, and printout
		// to usage alone.
		args, printout []string
	}{
		{name: "as the document stands"},
		{name: "refreshed", args: []string{"--refresh"}},
		{name: "refreshed, with -r for short", args: []string{"-r"}},
		{name: "probed as asked", args: []string{"--probe"}},
		{name: "laid out by account, which shapes the printout alone", printout: []string{"--by", "account"}},
		{name: "of windows given, which shape the printout alone", printout: []string{"--fable", "--5h"}},
	}
	for _, tt := range tests {
		for _, form := range jsonForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				deps := statusDeps(t, fakeClaudeAPI(t), nil)

				got := form.run(t, deps, slices.Concat([]string{"usage"}, tt.args, tt.printout)...)
				want := run(t, deps, append([]string{"status", "--json"}, tt.args...)...)
				if want.code != 0 || !json.Valid([]byte(want.stdout)) {
					t.Fatalf("switchboard status --json %s = %+v, want exit status 0 and a document", strings.Join(tt.args, " "), want)
				}
				if got != want {
					t.Errorf("switchboard usage %s %s, %s, =\n%+v\nwant what status --json prints\n%+v",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), form.name, got, want)
				}
			})
		}
	}
}

func TestUsageAsJSONIgnoresAWindowNoAccountReports(t *testing.T) {
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			deps := statusDeps(t, sharedWindowsAPI(t), nil)

			got := form.run(t, deps, "usage", "--fable")
			if want := run(t, deps, "status", "--json"); got.code != 0 || got != want {
				t.Errorf("switchboard usage --fable %s, %s, =\n%+v\nwant what status --json prints, the flag passed over\n%+v",
					strings.Join(form.args, " "), form.name, got, want)
			}
		})
	}
}

func TestUsagePrintsTheWindowsGivenLaidOutAsAsked(t *testing.T) {
	tests := []struct {
		name string
		// layout is [usage] by in the config, "" for none.
		layout  string
		args    []string
		by      dashboard.By
		windows []string
	}{
		{name: "every window by window, where neither the config nor --by says", by: dashboard.ByWindow},
		{name: "by window, as the config says", layout: "window", by: dashboard.ByWindow},
		{name: "by account, as the config says", layout: "account", by: dashboard.ByAccount},
		{name: "by account, as --by says", args: []string{"--by", "account"}, by: dashboard.ByAccount},
		{name: "by window, as --by says over the config", layout: "account", args: []string{"--by", "window"}, by: dashboard.ByWindow},
		{name: "by account, as --by says over the config", layout: "window", args: []string{"--by=account"}, by: dashboard.ByAccount},
		{name: "by account, as --by says in any case", args: []string{"--by", "Account"}, by: dashboard.ByAccount},
		{name: "by account, as the config says in any case", layout: "ACCOUNT", by: dashboard.ByAccount},
		{name: "the 5-hour window alone", args: []string{"--5h"}, windows: []string{"5h"}},
		{name: "the week alone", args: []string{"--week"}, windows: []string{"7d"}},
		{name: "Fable's week alone, by its model's name", args: []string{"--fable"}, windows: []string{"7d_oi"}},
		{name: "the windows given, in Claude's order", args: []string{"--fable", "--week", "--5h"}, windows: []string{"5h", "7d", "7d_oi"}},
		{name: "the windows given, by account", args: []string{"--week", "--by", "account", "--5h"}, by: dashboard.ByAccount, windows: []string{"5h", "7d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := fakeClaudeAPI(t)
			text := fmt.Sprintf("upstream = %q\n", upstream) + threeAccounts
			if tt.layout != "" {
				text += "\n[usage]\nby = \"" + tt.layout + "\"\n"
			}
			deps := statusDeps(t, upstream, map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, text), "COLUMNS": "160"})

			got := run(t, deps, append([]string{"usage", "--pretty"}, tt.args...)...)
			rows := dashboard.Printout{Width: 160, Look: dashboard.NoColour(), By: tt.by, Windows: tt.windows, Policy: claude.Policy, Name: claude.WindowName}.Draw(statusJSON(t, deps).At(testNow, claude.Policy))
			if want := (result{stdout: ansi.Strip("\n" + strings.Join(rows, "\n") + "\n")}); got != want {
				t.Errorf("switchboard usage --pretty %s =\n%+v\nwant the printout by %v of %q, off a terminal, without colour\n%+v",
					strings.Join(tt.args, " "), got, tt.by, tt.windows, want)
			}
		})
	}
}

func TestUsageRefusesAWindowNoAccountReportsNamingThoseThereAre(t *testing.T) {
	tests := []struct {
		name     string
		upstream func(*testing.T) string
		args     []string
		want     string
	}{
		{
			name: "a model's window", upstream: sharedWindowsAPI, args: []string{"--fable"},
			want: "no account reports the Fable week: give --5h or --week",
		},
		{
			name: "a model's window, among others reported", upstream: sharedWindowsAPI, args: []string{"--5h", "--fable", "--week"},
			want: "no account reports the Fable week: give --5h or --week",
		},
		{
			name: "a model's window, where no account is read", upstream: refusingAPI, args: []string{"--fable", "--5h"},
			want: "no account reports the Fable week: give --5h or --week",
		},
	}
	for _, tt := range tests {
		for _, form := range prettyForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, statusDeps(t, tt.upstream(t), nil), append([]string{"usage"}, tt.args...)...)
				if want := (result{stderr: "Error: " + tt.want + "\n", code: 1}); got != want {
					t.Errorf("switchboard usage %s %s, %s, = %+v, want %+v", strings.Join(tt.args, " "), strings.Join(form.args, " "), form.name, got, want)
				}
			})
		}
	}
}

func TestUsagePrintsEveryAccountBeforeAnyWindowIsRead(t *testing.T) {
	tests := []struct {
		args     []string
		headings []string
	}{
		{headings: []string{"5-HOUR", "WEEK"}},
		{args: []string{"--5h"}, headings: []string{"5-HOUR"}},
		{args: []string{"--week"}, headings: []string{"WEEK"}},
		{args: []string{"--week", "--5h"}, headings: []string{"5-HOUR", "WEEK"}},
	}
	words := map[string]string{"Work": "not read yet", "Personal": "no token", "Side": "not read yet"}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			deps := statusDeps(t, refusingAPI(t), map[string]string{"COLUMNS": "120"})

			got := run(t, deps, append([]string{"usage", "--pretty"}, tt.args...)...)
			if got.code != 0 {
				t.Fatalf("switchboard usage --pretty %s, no account read, = %+v, want it to print", strings.Join(tt.args, " "), got)
			}
			for _, heading := range tt.headings {
				for label, want := range words {
					if row := printedRow(got.stdout, heading, label); !strings.HasSuffix(row, "—  "+want) {
						t.Errorf("switchboard usage --pretty %s, no account read, printed %s's row under %s as %q, want it to end %q:\n%s",
							strings.Join(tt.args, " "), label, heading, row, "—  "+want, got.stdout)
					}
				}
			}
		})
	}
}

func TestUsageRefusesALayoutItHasNone(t *testing.T) {
	for _, given := range []string{"matrix", "accounts", " account", ""} {
		got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "usage", "--by", given)
		want := fmt.Sprintf("Error: invalid argument %q for \"--by\" flag: must be window or account\n", given)
		if got.code != 1 || got.stdout == "" || !strings.HasPrefix(got.stderr, want) {
			t.Errorf("switchboard usage --by %q = %+v, want exit status 1, the usage, and an error starting %q", given, got, want)
		}
	}
}

// sharedWindowsAPI serves a probe of the work account, of any model, with
// the windows every model shares alone, and rejects anything else. It returns
// the API's URL.
func sharedWindowsAPI(t *testing.T) string {
	t.Helper()
	return reportingAPI(t, "5h", "7d")
}

// reportingAPI serves a probe of the work account, of any model, with the
// windows with the keys given alone, each a fifth used, and rejects anything
// else. It returns the API's URL.
func reportingAPI(t *testing.T, keys ...string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token-work" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`)
			return
		}
		for _, key := range keys {
			w.Header().Set("anthropic-ratelimit-unified-"+key+"-utilization", "0.2")
			w.Header().Set("anthropic-ratelimit-unified-"+key+"-reset", "1790974800")
		}
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// refusingAPI rejects every request, so no account reports a window. It
// returns the API's URL.
func refusingAPI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// printedRow is the row usage printed on stdout, stripped of its escapes and
// of the spaces it ends with, of the account labelled, in the section under
// heading, as the windows stacked print it: "" where there's none.
func printedRow(stdout, heading, label string) string {
	_, section, ok := strings.Cut(ansi.Strip(stdout), "\n"+heading+"\n")
	if !ok {
		return ""
	}
	for row := range strings.SplitSeq(section, "\n") {
		if fields := strings.Fields(row); len(fields) > 1 && fields[1] == label {
			return strings.TrimRight(row, " ")
		}
	}
	return ""
}

// goldenDeps are statusDeps on a terminal, so usage draws its printout, with
// env added to their environment, but for a token for personal, which
// fakeClaudeAPI refuses: the reason for one missing names the test's own
// state directory, so what two of them print would differ.
func goldenDeps(t *testing.T, env map[string]string) cli.Deps {
	t.Helper()
	deps := statusDeps(t, fakeClaudeAPI(t), env)
	onATerminal.on(&deps)
	writeToken(t, deps, "personal", "test-token-personal")
	return deps
}
