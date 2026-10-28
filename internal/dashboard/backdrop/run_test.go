package backdrop_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
)

func TestTheMouseMovingOrLetGoNeverReachesTheScreen(t *testing.T) {
	tests := []struct {
		name   string
		msg    tea.Msg
		heeded bool
	}{
		{name: "a drag, a cell on", msg: tea.MouseMotionMsg{X: 4, Y: 7, Button: tea.MouseLeft}},
		{name: "a button let go", msg: tea.MouseReleaseMsg{X: 4, Y: 7, Button: tea.MouseLeft}},
		{name: "a click", msg: tea.MouseClickMsg{X: 4, Y: 7, Button: tea.MouseLeft}, heeded: true},
		{name: "the wheel", msg: tea.MouseWheelMsg{X: 4, Y: 7, Button: tea.MouseWheelDown}, heeded: true},
		{name: "a key", msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, heeded: true},
	}
	for _, tt := range tests {
		if got := backdrop.Heeded(nil, tt.msg); (got != nil) != tt.heeded || tt.heeded && got != tt.msg {
			t.Errorf("%s reaches the screen as %v; want it to reach it %v", tt.name, got, tt.heeded)
		}
	}
}
