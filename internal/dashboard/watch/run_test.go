package watch

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheBackgroundIsPutBackHoweverTheWatchStops(t *testing.T) {
	stops := []struct {
		name string
		// key is what's typed to stop the watch, which the filter turns into
		// as, where it's set: the message Bubble Tea makes of a signal, or a
		// panic.
		key       string
		as        func() tea.Msg
		wantError bool
	}{
		{name: "q", key: "q"},
		{name: "ctrl+c", key: "\x03"},
		{name: "an interrupt", key: "x", as: func() tea.Msg { return tea.InterruptMsg{} }},
		{name: "a terminate signal, which Bubble Tea takes as a quit", key: "x", as: func() tea.Msg { return tea.QuitMsg{} }},
		{name: "a panic Bubble Tea recovers from", key: "x", as: func() tea.Msg { panic("a bug") }, wantError: true},
	}
	terminals := []struct {
		name string
		// answer is what the terminal says when asked its background, ""
		// for nothing.
		answer string
		want   string
	}{
		{name: "the terminal said its background", answer: "\x1b]11;rgb:1a1a/1b1b/2626\x07", want: ansi.SetBackgroundColor("#1a1b26")},
		{name: "the terminal said nothing", want: ansi.ResetBackgroundColor},
	}
	for _, stop := range stops {
		for _, terminal := range terminals {
			t.Run(stop.name+", "+terminal.name, func(t *testing.T) {
				out, err := watchUntil(t, themedConfig(), terminal.answer, stop.key, stop.as)

				if (err != nil) != stop.wantError {
					t.Errorf("run() error = %v, want an error: %v", err, stop.wantError)
				}
				if !strings.HasSuffix(out, terminal.want) {
					t.Errorf("the watch's output ends %q, want the background put back with %q", tail(out), terminal.want)
				}
			})
		}
	}
}

func TestAWatchsPanicLeavesNothingBehindWhateverTheEnvironmentSays(t *testing.T) {
	t.Setenv("TEA_DEBUG", "true")
	t.Chdir(t.TempDir())

	if _, err := watchUntil(t, themedConfig(), "", "x", func() tea.Msg { panic("a bug") }); err == nil {
		t.Fatal("run() error = nil, want the panic it recovered from")
	}
	if logs, _ := filepath.Glob("bubbletea-panic-*.log"); len(logs) > 0 {
		t.Errorf("the panic left %q behind, want nothing written, whatever TEA_DEBUG says", logs)
	}
}

func TestABackgroundNeverSetIsLeftAlone(t *testing.T) {
	cfg := themedConfig()
	cfg.Choice = theme.One(theme.Terminal)
	cfg.Pair = cfg.Themes.List().Pair(cfg.Choice)
	for name, cfg := range map[string]Config{"without colour": noColourConfig(), "in the terminal's own colours": cfg} {
		t.Run(name, func(t *testing.T) {
			out, err := watchUntil(t, cfg, "\x1b]11;rgb:1a1a/1b1b/2626\x07", "q", nil)
			if err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if strings.Contains(out, "\x1b]11;#") || strings.Contains(out, ansi.ResetBackgroundColor) {
				t.Errorf("the watch's output %q sets or resets the background, want it left alone", tail(out))
			}
		})
	}
}

// watchUntil runs a watch of cfg on a terminal that answers its question
// about its background with answer, and once the watch has its answer, or
// has given up on one, types key, which the watch's filter turns into as,
// where that's set. It returns what the watch wrote, and how it stopped.
func watchUntil(t *testing.T, cfg Config, answer, key string, as func() tea.Msg) (string, error) {
	t.Helper()
	// Bubble Tea reads these itself: set, they have it write a trace, or a
	// log of a panic it recovers from in the working directory.
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	var out lockedBuffer
	in, typing := io.Pipe()
	t.Cleanup(func() { _ = typing.Close() })
	var settle sync.Once
	settled := make(chan struct{})
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		switch msg := msg.(type) {
		case tea.BackgroundColorMsg, unansweredMsg:
			settle.Do(func() { close(settled) })
		case tea.KeyPressMsg:
			if msg.String() == key && as != nil {
				return as()
			}
		}
		return msg
	}
	if answer == "" {
		cfg.After = func(_ time.Duration, msg tea.Msg) tea.Cmd {
			if _, ok := msg.(unansweredMsg); ok {
				return func() tea.Msg { return msg }
			}
			return nil
		}
	}
	go func() {
		if answer != "" {
			_, _ = io.WriteString(typing, answer)
		}
		if cfg.Themes != nil {
			<-settled
		}
		_, _ = io.WriteString(typing, key)
	}()
	err := run(t.Context(), cfg, &out, tea.WithInput(in), tea.WithoutSignals(), tea.WithFilter(filter))
	return out.String(), err
}

// themedConfig is a watch of calm, in colour, in nord.
func themedConfig() Config {
	cfg := noColourConfig()
	themes := &fakeThemes{listing: listing()}
	cfg.Themes, cfg.Choice = themes, theme.One("nord")
	cfg.Pair = themes.listing.Pair(cfg.Choice)
	return cfg
}

// noColourConfig is a watch of calm, without colour, whose timers never
// fire.
func noColourConfig() Config {
	clock := &fakeClock{now: start}
	return Config{
		Source: &fakeSource{doc: calm()}, Notifier: &fakeNotifier{}, Now: clock.Now, Interval: interval, Policy: policy,
		Size: Size{Width: 80, Height: 24}, After: func(time.Duration, tea.Msg) tea.Cmd { return nil },
	}
}

// tail is the end of what a watch wrote, as much as says how it stopped.
func tail(out string) string {
	return out[max(len(out)-80, 0):]
}

// lockedBuffer is a buffer Bubble Tea's goroutines can all write to.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
