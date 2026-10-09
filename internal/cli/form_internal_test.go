package cli

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestTheFormFlagsRefuseJSONWithPrettyBeforeTheCommandsOwnCheck(t *testing.T) {
	const both = "--json prints JSON, so it takes no --pretty"
	tests := []struct {
		name string
		// check is the command's own argument check: nil for none.
		check   cobra.PositionalArgs
		args    []string
		wantErr string
	}{
		{name: "no check, both", args: []string{"--json", "--pretty"}, wantErr: both},
		{name: "no check, both and an argument", args: []string{"extra", "--json", "--pretty"}, wantErr: both},
		{name: "no check, an argument, taken", args: []string{"extra", "--json"}},
		{name: "its own check, both", check: cobra.NoArgs, args: []string{"extra", "--json", "--pretty"}, wantErr: both},
		{name: "its own check, an argument", check: cobra.NoArgs, args: []string{"extra", "--pretty"}, wantErr: `unknown command "extra" for "verb"`},
		{name: "its own check, passed", check: cobra.NoArgs, args: []string{"--pretty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var form formFlags
			cmd := &cobra.Command{Use: "verb", Args: tt.check, RunE: func(*cobra.Command, []string) error { return nil }}
			form.add(cmd)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if got := errText(err); got != tt.wantErr {
				t.Errorf("verb %q fails with %q, want %q", tt.args, got, tt.wantErr)
			}
		})
	}
}

// errText is err's text, or "" for none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
