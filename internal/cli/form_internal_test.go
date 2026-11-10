package cli

import (
	"io"
	"strings"
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

func TestWriteJSONPutsTheSchemaFirstInAnyObject(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{name: "with fields", v: struct {
			A int `json:"a"`
		}{A: 2}, want: "{\n  \"schema\": 1,\n  \"a\": 2\n}\n"},
		{name: "without", v: struct{}{}, want: "{\n  \"schema\": 1\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := writeJSON(&b, tt.v); err != nil || b.String() != tt.want {
				t.Errorf("writeJSON() wrote %q, %v, want %q", b.String(), err, tt.want)
			}
		})
	}
}

func TestWriteJSONRefusesWhatIsntAnObject(t *testing.T) {
	var b strings.Builder
	if err := writeJSON(&b, []int{1}); err == nil || b.Len() != 0 {
		t.Errorf("writeJSON() of a list wrote %q, %v, want it refused, nothing written", b.String(), err)
	}
}

func TestWriteStoredPutsTheSchemaFirstBeforeTheLinesOwnFields(t *testing.T) {
	tests := []struct {
		name, line, want string
	}{
		{name: "with fields", line: `{"at":"2026-10-07T09:00:00Z","kind":"message"}`, want: `{"schema":1,"at":"2026-10-07T09:00:00Z","kind":"message"}` + "\n"},
		{name: "without", line: `{}`, want: `{"schema":1}` + "\n"},
		{name: "without, spaced", line: " { } \n", want: `{"schema":1}` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := writeStored(&b, []byte(tt.line)); err != nil || b.String() != tt.want {
				t.Errorf("writeStored(%q) wrote %q, %v, want %q", tt.line, b.String(), err, tt.want)
			}
		})
	}
}

func TestWriteStoredRefusesALineThatIsntAnObject(t *testing.T) {
	for _, line := range []string{`[1]`, `"text"`, ``, `not a line`} {
		var b strings.Builder
		if err := writeStored(&b, []byte(line)); err == nil || b.Len() != 0 {
			t.Errorf("writeStored(%q) wrote %q, %v, want it refused, nothing written", line, b.String(), err)
		}
	}
}

// errText is err's text, or "" for none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
