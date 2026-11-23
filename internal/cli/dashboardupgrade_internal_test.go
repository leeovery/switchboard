package cli

import (
	"slices"
	"testing"
	"time"
)

// A dashboard restarts as the dashboard command, run as the binary, with the
// config file --config named, and its own flags and interval, the interval
// only where it isn't the default; never the pool, which is carried.
func TestARestartedDashboardsCommandLineIsTheDashboards(t *testing.T) {
	const exe = "/test/bin/switchboard"
	tests := []struct {
		name   string
		opts   dashboardOptions
		config string
		want   []string
	}{
		{name: "as opened by default, as setup opens it", opts: dashboardOptions{interval: defaultInterval}, want: []string{exe, "dashboard"}},
		{name: "given no interval", opts: dashboardOptions{}, want: []string{exe, "dashboard"}},
		{name: "probing, without notifications", opts: dashboardOptions{probe: true, noNotify: true, interval: defaultInterval}, want: []string{exe, "dashboard", "--probe", "--no-notify"}},
		{name: "every 15 minutes", opts: dashboardOptions{interval: 15 * time.Minute}, want: []string{exe, "dashboard", "900s"}},
		{name: "of a config named", opts: dashboardOptions{interval: defaultInterval}, config: "/test/work.toml", want: []string{exe, "dashboard", "--config", "/test/work.toml"}},
		{name: "on a pool named", opts: dashboardOptions{pool: "q7mt", interval: defaultInterval}, want: []string{exe, "dashboard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.commandLine(exe, tt.config); !slices.Equal(got, tt.want) {
				t.Errorf("commandLine() = %q, want %q", got, tt.want)
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
