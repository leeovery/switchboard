package sessionhook_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sessionhook"
)

// wantNotice is the notice, exactly, that a session not started through
// switchboard hears as it starts.
// It tells the user alone: nothing goes into Claude's context, which every
// headless run would carry in its prompt.
const wantNotice = `{"systemMessage":"This session isn't going through switchboard, as the claude that started it isn't switchboard's: ` +
	`from a terminal, switchboard setup puts that right; an app that runs its own Claude Code isn't routed."}` + "\n"

func TestNotice(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// speaks is set where an unmarked session hears the notice.
		speaks bool
	}{
		{name: "starting", input: `{"session_id":"abc","hook_event_name":"SessionStart","source":"startup"}`, speaks: true},
		{name: "resumed", input: `{"source":"resume"}`, speaks: true},
		{name: "cleared", input: `{"source":"clear"}`},
		{name: "compacted", input: `{"source":"compact"}`},
		{name: "a source Claude Code doesn't give yet", input: `{"source":"handoff"}`},
		{name: "no source", input: `{"session_id":"abc"}`, speaks: true},
		{name: "a null source", input: `{"source":null}`, speaks: true},
		{name: "a source that isn't a string", input: `{"source":5}`, speaks: true},
		{name: "no input", input: "", speaks: true},
		{name: "junk", input: "not json", speaks: true},
		{name: "JSON that isn't an object", input: `["startup"]`, speaks: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionhook.Notice([]byte(tt.input), true); got != nil {
				t.Errorf("Notice(%q, started) = %q, want nothing", tt.input, got)
			}
			want := ""
			if tt.speaks {
				want = wantNotice
			}
			if got := string(sessionhook.Notice([]byte(tt.input), false)); got != want {
				t.Errorf("Notice(%q, unmarked) = %q, want %q", tt.input, got, want)
			}
		})
	}
}

func TestReadInput(t *testing.T) {
	const input = `{"source":"startup"}`
	got, err := sessionhook.ReadInput(strings.NewReader(input))
	if err != nil || string(got) != input {
		t.Errorf("ReadInput() = %q, %v, want %q", got, err, input)
	}

	limit := strings.Repeat(" ", 64<<10)
	if got, err := sessionhook.ReadInput(strings.NewReader(limit)); err != nil || len(got) != len(limit) {
		t.Errorf("ReadInput(64 KiB) = %d bytes, %v, want them all", len(got), err)
	}
	if got, err := sessionhook.ReadInput(strings.NewReader(limit + " ")); err == nil || got != nil {
		t.Errorf("ReadInput(past 64 KiB) = %d bytes, %v, want an error", len(got), err)
	}
	failing := errors.New("broken pipe")
	if _, err := sessionhook.ReadInput(errReader{failing}); !errors.Is(err, failing) {
		t.Errorf("ReadInput(a failing reader) error = %v, want %v", err, failing)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestPath(t *testing.T) {
	noHome := func() (string, error) { return "", errors.New("no home") }
	home := func() (string, error) { return "/home/test", nil }
	tests := []struct {
		name    string
		env     map[string]string
		homeDir func() (string, error)
		want    string
		wantErr bool
	}{
		{name: "CLAUDE_CONFIG_DIR", env: map[string]string{"CLAUDE_CONFIG_DIR": "/claude"}, homeDir: noHome, want: "/claude/settings.json"},
		{name: "the home's .claude", homeDir: home, want: "/home/test/.claude/settings.json"},
		{name: "no home", homeDir: noHome, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sessionhook.Path(func(key string) string { return tt.env[key] }, tt.homeDir)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("Path() = %q, %v; want %q, an error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
