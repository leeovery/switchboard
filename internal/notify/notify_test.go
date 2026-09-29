package notify

import (
	"context"
	"errors"
	"slices"
	"testing"
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

	if err := desktop.Notify(`work · "Work": Week at 91%`); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	want := []string{"osascript", "-e", `display notification "work · \"Work\": Week at 91%" with title "Switchboard"`}
	if !slices.Equal(got, want) {
		t.Errorf("Notify() ran %q, want %q", got, want)
	}
}

func TestDesktopRunsOsascriptWithoutTheTokens(t *testing.T) {
	tmp := t.TempDir()
	for name, value := range map[string]string{
		"CLAUDE_TOKEN_WORK":       "test-token-work",
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-oauth",
		"PATH":                    "/usr/bin:/bin",
		"HOME":                    "/home/tester",
		"TMPDIR":                  tmp,
		"LANG":                    "en_GB.UTF-8",
	} {
		t.Setenv(name, value)
	}
	var env []string
	desktop := NewDesktop()
	desktop.goos = "darwin"
	desktop.run = func(_ context.Context, e []string, _ string, _ ...string) error {
		env = e
		return nil
	}

	if err := desktop.Notify("work · Work has room again"); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if want := []string{"PATH=/usr/bin:/bin", "HOME=/home/tester", "TMPDIR=" + tmp, "LANG=en_GB.UTF-8"}; !slices.Equal(env, want) {
		t.Errorf("osascript ran with the environment %q, want %q alone", env, want)
	}
}

func TestDesktopNotifyFails(t *testing.T) {
	failed := errors.New("exit status 1")
	desktop := Desktop{goos: "darwin", run: func(context.Context, []string, string, ...string) error { return failed }}

	if err := desktop.Notify("work · Work has room again"); !errors.Is(err, failed) {
		t.Errorf("Notify() error = %v, want it to wrap %v", err, failed)
	}
}

func TestDesktopPostsNothingOffMacOS(t *testing.T) {
	desktop := Desktop{goos: "linux", run: func(_ context.Context, _ []string, name string, _ ...string) error {
		t.Errorf("Notify() ran %s off macOS", name)
		return nil
	}}

	if err := desktop.Notify("work · Work has room again"); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
}

func TestOff(t *testing.T) {
	if err := (Off{}).Notify("work · Work has room again"); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
}
