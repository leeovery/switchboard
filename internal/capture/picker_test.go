package capture

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAFixturesKeysOpenThePickerOverTheOverviewOnTheThemeBeforeTheFrames(t *testing.T) {
	f := fixture(t, "page-routing")
	f.keys = append(keysOf("t"), tea.KeyPressMsg{Code: tea.KeyUp})
	frame := ansi.Strip(f.Frame(f.Size))

	for _, want := range []string{"│ Themes", "│ ▌ exchange", "│   nord                     ●", "USAGE", "1 work"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the frame\n%s\nhas no %q", frame, want)
		}
	}
}
