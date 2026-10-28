package launch_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/launch"
)

func TestStartedBy(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "marked", env: map[string]string{markEnv: "5150:1790000000:/opt/claude/bin/claude"}, want: true},
		{name: "marked long ago, which counts as much", env: map[string]string{markEnv: "5150:1:/opt/claude/bin/claude"}, want: true},
		{name: "unmarked", env: map[string]string{"PATH": "/usr/bin"}},
		{name: "marked only with what run set", env: map[string]string{setEnv: "set%3AANTHROPIC_BASE_URL=http%3A%2F%2F127.0.0.1%3A4747"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := launch.StartedBy(func(key string) string { return tt.env[key] }); got != tt.want {
				t.Errorf("StartedBy(%v) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}
