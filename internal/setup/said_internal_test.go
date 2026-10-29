package setup

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
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

func TestFailedRow(t *testing.T) {
	failed := errors.New("launchctl bootstrap: Bootstrap failed: 5")
	notSetUp := Said{Mark: Failed, Label: "Not set up", Detail: failed.Error()}
	updated := Said{Mark: Warning, Label: "Updated", Detail: "reinstalled: its plist had no ProcessType"}
	walkOnly := Said{Mark: Failed, Sentence: "The line walk's alone."}
	ownRow := Said{Mark: Failed, Label: "Router", Detail: "not running"}
	tests := []struct {
		name string
		said []Said
		err  error
		want []Said
	}{
		{name: "done: as told", said: []Said{updated}, want: []Said{updated}},
		{name: "failed, saying nothing of it: Not set up", said: []Said{updated}, err: failed, want: []Said{updated, notSetUp}},
		{name: "failed with a row of its own: as told", said: []Said{ownRow}, err: failed, want: []Said{ownRow}},
		{name: "failed with a failed row the screen doesn't draw: Not set up", said: []Said{walkOnly}, err: failed, want: []Said{walkOnly, notSetUp}},
		{name: "interrupted: as told, an interrupt no failure of the step's", said: []Said{updated}, err: fmt.Errorf("the service is installed, but the wait for its router was %w", accounts.ErrInterrupted), want: []Said{updated}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failedRow(Serviced{Said: tt.said}, tt.err).Said; !slices.Equal(got, tt.want) {
				t.Errorf("failedRow() said %+v, want %+v", got, tt.want)
			}
		})
	}
}
