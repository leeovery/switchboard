package setup_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
)

func TestServiceSaysHowFarItsSwapGotWhereItsContextEnds(t *testing.T) {
	killed := errors.New("signal: killed")
	tests := []struct {
		name string
		// lay sets the world up past the service step's own; at is the
		// launchctl run the context ends as, which fails with killed where
		// it's set, as launchctl killed by its context does.
		lay    func(w *world)
		at     string
		killed bool
		want   service.Swap
		// wantLabels are the rows said, in turn, by their marks and labels.
		wantLabels []string
	}{
		{
			name: "ended as the swap boots the old service out: swapped, installed, no failed row",
			at:   "bootout", want: service.Swapped,
			wantLabels: []string{"! Updated", "✓ Installed"},
		},
		{
			name: "launchctl killed as the step first asks launchd how the service stands, long before Install: none",
			at:   "print", killed: true, want: service.NotSwapped,
		},
		{
			name: "the swap failing at its end: failed",
			lay:  func(w *world) { w.launchd.bootoutSays = "Boot-out failed: 5: Input/output error" },
			at:   "bootout", want: service.SwapFailed,
			wantLabels: []string{"! Updated", "✗ Not set up"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.editPlist(t, withoutProcessType)
			if tt.lay != nil {
				tt.lay(w)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			w.launchd.before = func(args []string) error {
				if args[0] != tt.at {
					return nil
				}
				cancel()
				if tt.killed {
					return killed
				}
				return nil
			}

			got, err := w.setup(t, &user{}).Service(ctx)
			if err == nil || got.Swap != tt.want {
				t.Fatalf("Service() swap = %v, error = %v; want %v, and an error", got.Swap, err, tt.want)
			}
			if tt.want == service.Swapped && !errors.Is(err, context.Canceled) {
				t.Errorf("Service() error = %v, want the context's, for a caller to tell", err)
			}
			if tt.wantLabels == nil {
				return
			}
			marks := map[setup.Mark]string{setup.Done: "✓", setup.Warning: "!", setup.Failed: "✗"}
			var labels []string
			for _, said := range got.Said {
				labels = append(labels, marks[said.Mark]+" "+said.Label)
			}
			if !slices.Equal(labels, tt.wantLabels) {
				t.Errorf("Service() said %q, want %q", labels, tt.wantLabels)
			}
		})
	}
}
