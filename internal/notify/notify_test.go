package notify

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/redact"
)

func TestQuote(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "plain", text: "work · Work has room again", want: `"work · Work has room again"`},
		{name: "empty", text: "", want: `""`},
		{name: "double quotes", text: `the "Work" account`, want: `"the \"Work\" account"`},
		{name: "a backslash", text: `C:\Work`, want: `"C:\\Work"`},
		{name: "a backslash before a quote", text: `\"`, want: `"\\\""`},
		{name: "a trailing backslash", text: `Work\`, want: `"Work\\"`},
		{name: "text that would close the string and run more script", text: `" & (do shell script "true") & "`, want: `"\" & (do shell script \"true\") & \""`},
		{name: "wide characters", text: "東京 · Tōkyō", want: `"東京 · Tōkyō"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quote(tt.text); got != tt.want {
				t.Errorf("quote(%q) = %s, want %s", tt.text, got, tt.want)
			}
		})
	}
}

func TestDesktopNotify(t *testing.T) {
	var got []string
	desktop := Desktop{goos: "darwin", run: func(_ context.Context, _ []string, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}

	if err := desktop.Notify(t.Context(), `work · "Work": Week at 91%`); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	want := []string{"osascript", "-e", `display notification "work · \"Work\": Week at 91%" with title "Switchboard"`}
	if !slices.Equal(got, want) {
		t.Errorf("Notify() ran %q, want %q", got, want)
	}
}

func TestDesktopRunsOsascriptWithoutTheTokens(t *testing.T) {
	tmp := t.TempDir()
	vars := map[string]string{
		"CLAUDE_TOKEN_WORK":       "test-token-work",
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-oauth",
		"PATH":                    "/usr/bin:/bin",
		"HOME":                    "/home/tester",
		"TMPDIR":                  tmp,
		"LANG":                    "en_GB.UTF-8",
	}
	var env []string
	desktop := NewDesktop(func(key string) string { return vars[key] }, func() (string, error) { return "/home/tester", nil })
	desktop.goos = "darwin"
	desktop.app = nil
	desktop.run = func(_ context.Context, e []string, _ string, _ ...string) error {
		env = e
		return nil
	}

	if err := desktop.Notify(t.Context(), "work · Work has room again"); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if want := []string{"PATH=/usr/bin:/bin", "HOME=/home/tester", "TMPDIR=" + tmp, "LANG=en_GB.UTF-8"}; !slices.Equal(env, want) {
		t.Errorf("osascript ran with the environment %q, want %q alone", env, want)
	}
}

func TestDesktopNotifyFails(t *testing.T) {
	failed := errors.New("exit status 1")
	desktop := Desktop{goos: "darwin", run: func(context.Context, []string, string, ...string) error { return failed }}

	if err := desktop.Notify(t.Context(), "work · Work has room again"); !errors.Is(err, failed) {
		t.Errorf("Notify() error = %v, want it to wrap %v", err, failed)
	}
}

func TestDesktopPostsNothingOffMacOS(t *testing.T) {
	desktop := Desktop{goos: "linux", run: func(_ context.Context, _ []string, name string, _ ...string) error {
		t.Errorf("Notify() ran %s off macOS", name)
		return nil
	}}

	if err := desktop.Notify(t.Context(), "work · Work has room again"); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
}

func TestOff(t *testing.T) {
	if err := (Off{}).Notify(t.Context(), "work · Work has room again"); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
}

func TestAFailingProgramSaysWhyAsItsLastLineDoes(t *testing.T) {
	failed := exitStatus(1)
	tests := []struct {
		name   string
		err    error
		stderr string
		want   string
	}{
		{name: "succeeding", stderr: "a warning\n"},
		{name: "saying nothing", err: failed, want: "exit status 1"},
		{name: "saying why last", err: failed, stderr: "first\nThe operation couldn't be completed.\n\n", want: "exit status 1: The operation couldn't be completed."},
		{name: "saying something token-shaped", err: failed, stderr: "refused sk-ant-oat01-abc_DEF-123\n", want: "exit status 1: refused " + redact.Placeholder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withReason(tt.err, []byte(tt.stderr))
			switch {
			case tt.err == nil && got != nil:
				t.Errorf("withReason() = %v, want nil", got)
			case tt.err != nil && (got == nil || got.Error() != tt.want || !errors.Is(got, tt.err)):
				t.Errorf("withReason() = %v, want %q, wrapping %v", got, tt.want, tt.err)
			}
		})
	}
}

func TestTheEndOfWhatAProgramSaysIsKept(t *testing.T) {
	var kept tail
	for range 100 {
		if _, err := kept.Write([]byte("0123456789")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := kept.Write([]byte("\nthe last line")); err != nil {
		t.Fatal(err)
	}
	if len(kept.b) != reasonSize || !strings.HasSuffix(string(kept.b), "\nthe last line") {
		t.Errorf("kept %d bytes, ending %q, want the last %d", len(kept.b), kept.b[max(0, len(kept.b)-20):], reasonSize)
	}
}

func TestTheEndOfWhatAProgramSaysStartsAtAWholeCharacter(t *testing.T) {
	var kept tail
	// 600 bytes of three-byte characters: the last 512 begin a third of the
	// way through one.
	if _, err := kept.Write([]byte(strings.Repeat("€", 200))); err != nil {
		t.Fatal(err)
	}
	if want := reasonSize - reasonSize%3; !utf8.Valid(kept.b) || len(kept.b) != want {
		t.Errorf("kept %d bytes, starting %q, want the last %d, from the first whole character", len(kept.b), kept.b[:min(len(kept.b), 3)], want)
	}
	if got := withReason(exitStatus(1), kept.b); !utf8.ValidString(got.Error()) {
		t.Errorf("withReason() = %q, want it UTF-8", got)
	}
}
