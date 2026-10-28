package setup

import (
	"testing"

	"github.com/leeovery/switchboard/internal/router"
)

func TestRouterUp(t *testing.T) {
	tests := []struct {
		name string
		h    router.Health
		want Said
	}{
		{
			name: "healthy, with its address and version",
			h:    router.Health{OK: true, Listen: "127.0.0.1:4747", Version: "0.2.10", PID: 4242},
			want: Said{Mark: Done, Label: "Router", Detail: "healthy · 127.0.0.1:4747 · v0.2.10"},
		},
		{
			name: "unhealthy: a warning, saying why",
			h:    router.Health{Reason: "no account has a usable token", Listen: "127.0.0.1:4747", Version: "0.2.10"},
			want: Said{Mark: Warning, Label: "Router", Detail: "unhealthy: no account has a usable token · 127.0.0.1:4747 · v0.2.10"},
		},
		{
			name: "not listening yet, a build without a release's version",
			h:    router.Health{OK: true, Version: "dev"},
			want: Said{Mark: Done, Label: "Router", Detail: "healthy · dev"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routerUp(tt.h); got != tt.want {
				t.Errorf("routerUp() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
