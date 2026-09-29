package cli_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/cli"
)

func TestAHiddenReadTheUserInterruptsPutsTheTerminalBack(t *testing.T) {
	cantRestore := errors.New("inappropriate ioctl for device")
	tests := []struct {
		name string
		// interrupted has the user interrupt the read, which otherwise ends
		// with the line typed.
		interrupted bool
		restoreErr  error
		wantLine    string
		wantErr     string
		// wantRestored is set when the terminal is put back.
		wantRestored bool
	}{
		{name: "read to its end", wantLine: "test-token-work"},
		{name: "interrupted", interrupted: true, wantErr: "interrupted", wantRestored: true},
		{
			name:         "interrupted, the terminal not put back",
			interrupted:  true,
			restoreErr:   cantRestore,
			wantErr:      "interrupted, and the terminal couldn't be put back: inappropriate ioctl for device",
			wantRestored: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A read the user interrupts ends only once the test does.
			finished := make(chan struct{})
			t.Cleanup(func() { close(finished) })
			read := func() ([]byte, error) {
				if tt.interrupted {
					<-finished
					return nil, io.EOF
				}
				return []byte("test-token-work"), nil
			}
			restored := false
			restore := func() error {
				restored = true
				return tt.restoreErr
			}
			interrupts := make(chan os.Signal, 1)
			if tt.interrupted {
				interrupts <- os.Interrupt
			}

			line, err := cli.ReadUnseen(read, restore, interrupts)
			if string(line) != tt.wantLine || errorText(err) != tt.wantErr {
				t.Errorf("ReadUnseen() = %q, %v; want %q, and the error %q", line, err, tt.wantLine, tt.wantErr)
			}
			if tt.interrupted && (!errors.Is(err, accounts.ErrInterrupted) || (tt.restoreErr != nil && !errors.Is(err, tt.restoreErr))) {
				t.Errorf("ReadUnseen() error = %v, want one matching %v, and why the terminal wasn't put back", err, accounts.ErrInterrupted)
			}
			if restored != tt.wantRestored {
				t.Errorf("the terminal put back: %v, want %v", restored, tt.wantRestored)
			}
		})
	}
}

func TestAnInterruptAtAHiddenPromptEndsTheCommand130(t *testing.T) {
	const interrupted = "Error: read the token: interrupted\n"
	paste := func(id string) string {
		return "Paste " + id + "'s token, from claude setup-token run while signed in to that subscription (it won't show): \n"
	}
	tests := []struct {
		name string
		args []string
		// wantOut is how what the command printed on stdout ends, where
		// setup asks, with nothing asked after the token.
		wantOut    string
		wantStderr string
	}{
		{name: "accounts add", args: []string{"accounts", "add", "work"}, wantStderr: paste("work") + interrupted},
		{name: "accounts token", args: []string{"accounts", "token", "side"}, wantStderr: paste("side") + interrupted},
		{name: "setup", args: []string{"setup"}, wantOut: paste("personal"), wantStderr: interrupted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
			writeToken(t, deps, "side", "test-token-stale")
			binary := filepath.Join(t.TempDir(), "switchboard")
			if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			deps.Executable = func() (string, error) { return binary, nil }
			// The user at a terminal, who interrupts the token's typing.
			deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
				return func() ([]byte, error) { return nil, accounts.ErrInterrupted }, true
			}
			before := readFile(t, path)

			got := runWithInput(t, deps, "", tt.args...)
			if got.code != 130 || got.stderr != tt.wantStderr || !strings.HasSuffix(got.stdout, tt.wantOut) {
				t.Errorf("switchboard %s = %+v, want exit status 130, stderr\n%s\nand stdout ending\n%s", strings.Join(tt.args, " "), got, tt.wantStderr, tt.wantOut)
			}
			if readFile(t, path) != before {
				t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
			}
			for _, id := range []string{"work", "personal"} {
				checkNoTokenFile(t, deps, id)
			}
			checkTokenFile(t, deps, "side", "test-token-stale\n")
			if log := readLog(t, deps, "cli.log"); !hasLine(log, "level=INFO", `msg="command failed"`, `error="read the token: interrupted"`) {
				t.Errorf("cli.log reads\n%s\nwant the interrupt noted at info", log)
			}
		})
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
