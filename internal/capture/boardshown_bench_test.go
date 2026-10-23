package capture

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// The ten accounts' Switchboard drawn in lanes, where they don't fit the
// view: scrolled an account at a time at 73 by 30 and 52 by 32, and a row
// at a time at 73 by 20.
func BenchmarkTheSwitchboardsLanes(b *testing.B) {
	f, err := ByName("sessions-ten-accounts-160")
	if err != nil {
		b.Fatal(err)
	}
	settled := f.settle(f.Size)
	for _, size := range []watch.Size{{Width: 73, Height: 30}, {Width: 52, Height: 32}, {Width: 73, Height: 20}} {
		b.Run(sizeName(size), func(b *testing.B) {
			m := deliver(settled, tea.WindowSizeMsg{Width: size.Width, Height: size.Height})
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}
