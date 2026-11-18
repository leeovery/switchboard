package launch_test

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
)

// insideTmux is TMUX as tmux sets it inside one of its sessions: its
// server's socket, its process id and the session's index, never its name.
const insideTmux = "TMUX=/private/tmp/tmux-501/default,4242,0"

func TestRunTellsTheRouterTheNameOfTheTerminalSessionClaudeCodeStartsIn(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		// tmux is what tmux answers, asked, and failed whether it fails.
		tmux   string
		failed bool
		// want is the header's value sent, "" for none, and wantAsked
		// whether tmux was asked.
		want      string
		wantAsked bool
	}{
		{name: "zmx's", environ: []string{"ZMX_SESSION=deploy"}, want: "deploy"},
		{name: "zmx's, inside tmux too, tmux not asked", environ: []string{"ZMX_SESSION=deploy", insideTmux}, tmux: "review", want: "deploy"},
		{name: "tmux's, asked", environ: []string{insideTmux}, tmux: "review", want: "review", wantAsked: true},
		{name: "tmux's, its line's end let go", environ: []string{insideTmux}, tmux: "review\n", want: "review", wantAsked: true},
		{name: "tmux failing, none, and claude starts all the same", environ: []string{insideTmux}, failed: true, wantAsked: true},
		{name: "outside both, none, tmux not asked", tmux: "review"},
		{name: "zmx's set to nothing, inside tmux, tmux's", environ: []string{"ZMX_SESSION=", insideTmux}, tmux: "review", want: "review", wantAsked: true},
		{name: "past ASCII, encoded", environ: []string{"ZMX_SESSION=café"}, want: "caf%C3%A9"},
		{name: "control characters cleaned", environ: []string{"ZMX_SESSION=fix\x1b[31m\tlogin\n"}, want: "fix [31m login"},
		{name: "a line of its own in the name, which would end the header, cleaned", environ: []string{insideTmux}, tmux: "x\nX-Switchboard-Account: side", want: "x X-Switchboard-Account: side", wantAsked: true},
		{name: "a long name, cut", environ: []string{"ZMX_SESSION=" + strings.Repeat("a", 300)}, want: strings.Repeat("a", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.environ...)
			asked := false
			h.launcher.TmuxSession = func() (string, error) {
				asked = true
				if tt.failed {
					return "", errors.New("no server running on /private/tmp/tmux-501/default")
				}
				return tt.tmux, nil
			}

			if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			headers, set := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]
			var want string
			if tt.want != "" {
				want = "X-Switchboard-Name: " + tt.want
			}
			if headers != want || set != (want != "") {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q (set: %v), want %q", headers, set, want)
			}
			if asked != tt.wantAsked {
				t.Errorf("tmux asked: %v, want %v", asked, tt.wantAsked)
			}
		})
	}
}

func TestRunNamesTheSessionBesideTheUsersOwnHeadersAndInPlaceOfOneInherited(t *testing.T) {
	tests := []struct {
		name    string
		headers string
		// zmx is the zmx session's name, "" outside one.
		zmx  string
		want string
	}{
		{name: "the user's own kept", headers: "X-Trace: on\nX-Team: core", zmx: "deploy", want: "X-Trace: on\nX-Team: core\nX-Switchboard-Name: deploy"},
		{name: "one inherited, in place of it", headers: "x-switchboard-name : outer\r\nX-Trace: on", zmx: "deploy", want: "X-Trace: on\nX-Switchboard-Name: deploy"},
		{name: "one inherited, outside every terminal session, gone", headers: "X-Switchboard-Name: outer\nX-Trace: on", want: "X-Trace: on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			environ := []string{"ANTHROPIC_CUSTOM_HEADERS=" + tt.headers}
			if tt.zmx != "" {
				environ = append(environ, "ZMX_SESSION="+tt.zmx)
			}
			h := newHarness(t, environ...)

			if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]; got != tt.want {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunTellsARouterFromBeforeNoName(t *testing.T) {
	// The health of a router from before it took every X-Switchboard- header
	// off what it sends upstream, as it answers.
	var older router.Health
	if err := json.Unmarshal([]byte(`{"ok": true, "listen": "`+proxyAddr+`", "version": "0.1.0", "pid": 4242}`), &older); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Name: outer", insideTmux)
	h.launcher.TmuxSession = func() (string, error) {
		t.Error("tmux asked, want it not: the name is never told such a router")
		return "review", nil
	}

	if err := h.launcher.Run(t.Context(), route(&fakeRouter{health: older}, ""), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"], "X-Trace: on"; got != want {
		t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q, want %q: no name, which the router would send on", got, want)
	}
}

func TestOwnLoginTellsNoName(t *testing.T) {
	session := []string{"HOME=" + homeDir, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Name: deploy", "ZMX_SESSION=deploy"}

	got := envMap(launch.OwnLogin(session))

	want := map[string]string{"HOME": homeDir, "ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on", "ZMX_SESSION": "deploy"}
	if !maps.Equal(got, want) {
		t.Errorf("OwnLogin() = %q, want %q: the name told the router gone, the terminal's own variable kept", got, want)
	}
}
