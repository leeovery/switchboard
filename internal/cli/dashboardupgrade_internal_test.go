package cli

import (
	"slices"
	"testing"
)

// A dashboard restarts by its command line as it was, but for the --pool it
// named, the pool shown being carried in its place.
func TestARestartedDashboardsCommandLineNamesNoPool(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{name: "naming none", argv: []string{"switchboard", "dashboard", "15m"}, want: []string{"switchboard", "dashboard", "15m"}},
		{name: "naming one", argv: []string{"switchboard", "dashboard", "--pool", "b2n8", "--probe"}, want: []string{"switchboard", "dashboard", "--probe"}},
		{name: "naming one, as one argument", argv: []string{"switchboard", "--pool=b2n8", "dashboard"}, want: []string{"switchboard", "dashboard"}},
		{name: "naming one last, without its id", argv: []string{"switchboard", "dashboard", "--pool"}, want: []string{"switchboard", "dashboard"}},
		{name: "its flags ended", argv: []string{"switchboard", "dashboard", "--", "--pool", "15m"}, want: []string{"switchboard", "dashboard", "--", "--pool", "15m"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argv := slices.Clone(tt.argv)
			if got := withoutPool(argv); !slices.Equal(got, tt.want) {
				t.Errorf("withoutPool(%q) = %q, want %q", tt.argv, got, tt.want)
			}
			if !slices.Equal(argv, tt.argv) {
				t.Errorf("the command line given became %q, want it left as it was", argv)
			}
		})
	}
}

// An upgrade's version is read as --version prints it, and only so.
func TestAnUpgradesVersionIsReadAsVersionPrintsIt(t *testing.T) {
	tests := []struct {
		out     string
		want    string
		wantErr bool
	}{
		{out: "switchboard version 1.4.0\n", want: "1.4.0"},
		{out: "switchboard version dev", want: "dev"},
		{out: "", wantErr: true},
		{out: "switchboard version \n", wantErr: true},
		{out: "Error: unknown flag: --version\n", wantErr: true},
		{out: "switchboard version 1.4.0 (built today)\n", wantErr: true},
	}
	for _, tt := range tests {
		got, err := versionIn([]byte(tt.out))
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("versionIn(%q) = %q, %v; want %q, an error %v", tt.out, got, err, tt.want, tt.wantErr)
		}
	}
}
