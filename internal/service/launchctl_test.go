package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/service"
)

func TestLaunchctlRunsWithoutTheTokens(t *testing.T) {
	// A launchctl on PATH that prints its arguments and what it's given of
	// the environment, a line each, in place of the real one.
	bin := t.TempDir()
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" \"token=${CLAUDE_CODE_OAUTH_TOKEN-unset}\" \"home=${HOME-unset}\" \"path=${PATH-unset}\"\n"
	if err := os.WriteFile(filepath.Join(bin, "launchctl"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HOME", "/home/tester")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "test-token-work")

	out, err := service.Launchctl(t.Context(), "print", "gui/501/io.github.leeovery.switchboard")
	want := "print\ngui/501/io.github.leeovery.switchboard\ntoken=unset\nhome=/home/tester\npath=" + bin + "\n"
	if err != nil || string(out) != want {
		t.Errorf("launchctl printed\n%s(%v)\nwant\n%s", out, err, want)
	}
}
