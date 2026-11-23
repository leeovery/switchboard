package cli

import (
	"slices"
	"testing"
)

// A dashboard restarted shows the pool shown, whatever pool its command line
// named, and the rest of the command line as it was.
func TestARestartedDashboardsCommandLineShowsThePoolShown(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		pool string
		want []string
	}{
		{name: "naming none", argv: []string{"switchboard", "dashboard", "15m"}, pool: "q7mt", want: []string{"switchboard", "dashboard", "15m", "--pool=q7mt"}},
		{name: "naming another", argv: []string{"switchboard", "dashboard", "--pool", "b2n8", "--probe"}, pool: "q7mt", want: []string{"switchboard", "dashboard", "--probe", "--pool=q7mt"}},
		{name: "naming another, as one argument", argv: []string{"switchboard", "--pool=b2n8", "dashboard"}, pool: "q7mt", want: []string{"switchboard", "dashboard", "--pool=q7mt"}},
		{name: "naming it", argv: []string{"switchboard", "dashboard", "--pool", "q7mt"}, pool: "q7mt", want: []string{"switchboard", "dashboard", "--pool=q7mt"}},
		{name: "showing no pool", argv: []string{"switchboard", "dashboard", "--pool", "b2n8", "--no-notify"}, want: []string{"switchboard", "dashboard", "--no-notify"}},
		{name: "its flags ended", argv: []string{"switchboard", "dashboard", "--", "15m"}, pool: "q7mt", want: []string{"switchboard", "dashboard", "--pool=q7mt", "--", "15m"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argv := slices.Clone(tt.argv)
			if got := showingPool(argv, tt.pool); !slices.Equal(got, tt.want) {
				t.Errorf("showingPool(%q, %q) = %q, want %q", tt.argv, tt.pool, got, tt.want)
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
