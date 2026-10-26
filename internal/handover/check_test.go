package handover_test

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/switchboard/internal/handover"
)

func TestAskRunsTheProgramAsItWouldBeRunWithTheCheckFlag(t *testing.T) {
	var path string
	var argv, env []string
	run := func(_ context.Context, p string, a, e []string) ([]byte, error) {
		path, argv, env = p, a, e
		return nil, nil
	}
	started := []string{"/test/bin/switchboard", "serve", "--config", "/test/config.toml"}
	environ := []string{"HOME=/home/tester", handover.Variable + "=control=9,proxy=8", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}

	if _, err := handover.Ask(t.Context(), run, "/test/bin/switchboard", started, environ); err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if path != "/test/bin/switchboard" {
		t.Errorf("ran %s, want the program asked", path)
	}
	if want := append(slices.Clone(started), "--check"); !slices.Equal(argv, want) {
		t.Errorf("ran it as %q, want %q: as it would be run, asked to check", argv, want)
	}
	if want := []string{"HOME=/home/tester", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}; !slices.Equal(env, want) {
		t.Errorf("ran it in the environment %q, want %q: as it would be run, but naming no listener, as it takes none up", env, want)
	}
	if !slices.Equal(started, []string{"/test/bin/switchboard", "serve", "--config", "/test/config.toml"}) {
		t.Errorf("the command line asked with became %q, want it left as it was", started)
	}
}

func TestAskTellsARefusalApartFromAProgramThatCantAnswer(t *testing.T) {
	tests := []struct {
		name string
		// out is what the program prints, and err how it ends.
		out         string
		err         error
		wantRefusal string
		wantErr     error
	}{
		{name: "able to start"},
		{
			name:        "refusing, saying why",
			out:         "invalid config /test/config.toml:\naccount work: its label is longer than 100 characters\n",
			err:         exited(handover.Refused),
			wantRefusal: "invalid config /test/config.toml:\naccount work: its label is longer than 100 characters",
		},
		{
			name:        "refusing, saying nothing",
			err:         exited(handover.Refused),
			wantRefusal: "it gave no reason",
		},
		{
			name:        "refusing, saying nothing that reads",
			out:         "\x00\x07\n",
			err:         exited(handover.Refused),
			wantRefusal: "it gave no reason",
		},
		{
			name:        "refusing, quoting a token",
			out:         "invalid config: account work: sk-ant-oat01-test-token-work isn't a label\n",
			err:         exited(handover.Refused),
			wantRefusal: "invalid config: account work: [redacted] isn't a label",
		},
		{
			name:    "from before the check, not knowing the flag",
			out:     "Error: unknown flag: --check\n",
			err:     exited(1),
			wantErr: exited(1),
		},
		{name: "crashing", err: exited(2), wantErr: exited(2)},
		{name: "killed", err: exited(-1), wantErr: exited(-1)},
		{name: "not there for a moment, as an upgrade moves its link on", err: fs.ErrNotExist, wantErr: fs.ErrNotExist},
		{name: "still checking as the time to answer runs out", err: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(context.Context, string, []string, []string) ([]byte, error) {
				return []byte(tt.out), tt.err
			}

			refusal, err := handover.Ask(t.Context(), run, "/test/bin/switchboard", []string{"/test/bin/switchboard", "serve"}, nil)
			if refusal != tt.wantRefusal || !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Errorf("Ask() = %q, %v, want %q, %v", refusal, err, tt.wantRefusal, tt.wantErr)
			}
		})
	}
}

// exited is a program's exit with the status it gives, as *exec.ExitError
// tells it, -1 for one killed by a signal.
type exited int

func (e exited) Error() string {
	return "exit status " + strconv.Itoa(int(e))
}

func (e exited) ExitCode() int {
	return int(e)
}
