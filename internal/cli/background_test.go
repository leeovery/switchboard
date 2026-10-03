package cli_test

import (
	"fmt"
	"image/color"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/cli"
)

func TestTheTerminalIsAskedItsBackground(t *testing.T) {
	const da1 = "\x1b[?62;22c"
	tests := []struct {
		name string
		// replies are what the terminal sends back once asked, a read each.
		replies []string
		want    string
	}{
		{name: "an answer ended by BEL", replies: []string{"\x1b]11;rgb:fafa/fafa/fafa\x07" + da1}, want: "#FAFAFA"},
		{name: "an answer ended by ST", replies: []string{"\x1b]11;rgb:1a1a/1b1b/2626\x1b\\" + da1}, want: "#1A1B26"},
		{name: "an answer split across reads", replies: []string{"\x1b]11;rgb:fa", "fa/fafa/fafa\x07", "\x1b[?62", ";22c"}, want: "#FAFAFA"},
		{name: "an answer after keys typed ahead", replies: []string{"ls -la\r", "\x1b]11;rgb:1a1a/1b1b/2626\x07" + da1}, want: "#1A1B26"},
		{name: "a terminal that doesn't answer OSC 11, known by its device attributes", replies: []string{da1}, want: "none"},
		{name: "a terminal that answers nothing", want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeConsole{replies: tt.replies}
			within := 5 * time.Second
			if len(tt.replies) == 0 {
				within = 30 * time.Millisecond
			}
			began := time.Now()

			got := cli.AskBackground(c, within)

			if hexOf(got) != tt.want {
				t.Errorf("AskBackground() = %s, want %s", hexOf(got), tt.want)
			}
			if want := ansi.RequestBackgroundColor + ansi.RequestPrimaryDeviceAttributes; c.asked != want {
				t.Errorf("the terminal was asked %q, want %q", c.asked, want)
			}
			if !c.raw || !c.restored {
				t.Errorf("raw mode: %v, then put back: %v; want the answer read in raw mode, and the terminal put back", c.raw, c.restored)
			}
			if took := time.Since(began); took >= time.Second {
				t.Errorf("asking took %v, want it done once the terminal has answered, or %v has passed", took, within)
			}
		})
	}
}

func TestATerminalAnsweringLateHasItsAnswersReadBeforeItsPutBack(t *testing.T) {
	const background, da1 = "\x1b]11;rgb:fafa/fafa/fafa\x07", "\x1b[?62;22c"
	tests := []struct {
		name    string
		replies []string
		// attributes is set where the replies end with the terminal's device
		// attributes, which it sends last.
		attributes bool
	}{
		{name: "its background, then its device attributes", replies: []string{background, da1}, attributes: true},
		{name: "its background alone", replies: []string{background}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeConsole{replies: tt.replies, late: 80 * time.Millisecond}

			got := cli.AskBackground(c, 10*time.Millisecond)

			if got != nil {
				t.Errorf("AskBackground() = %s, want none: an answer after the wait is too late to take", hexOf(got))
			}
			if !c.restored || c.unsent > 0 {
				t.Errorf("put back: %v, with %d replies yet to come; want every one read first: once it's put back, they'd show in the shell", c.restored, c.unsent)
			}
			if tt.attributes && c.after > 0 {
				t.Errorf("read %d times once its device attributes came, want none: nothing comes after them", c.after)
			}
		})
	}
}

func TestAProcessInTheBackgroundAsksTheTerminalNothing(t *testing.T) {
	c := &fakeConsole{background: true, replies: []string{"\x1b]11;rgb:fafa/fafa/fafa\x07\x1b[?62c"}}

	if got := cli.AskBackground(c, time.Second); got != nil {
		t.Errorf("AskBackground() = %v, want nothing, taken for dark", got)
	}
	if c.asked != "" || c.raw {
		t.Errorf("the terminal was asked %q, in raw mode: %v; want it left alone, as asking would stop the process", c.asked, c.raw)
	}
}

// fakeConsole is a terminal that, once asked, sends back what replies holds,
// a read each, from late after it's asked, or with background, one whose
// foreground the process isn't in. It notes what it's asked, its raw mode,
// how many replies it had yet to send as it was put back, and how many
// times it was read after it had sent them all.
type fakeConsole struct {
	background    bool
	late          time.Duration
	replies       []string
	asked         string
	askedAt       time.Time
	raw, restored bool
	unsent, after int
}

func (c *fakeConsole) Foreground() bool {
	return !c.background
}

func (c *fakeConsole) Raw() (func(), error) {
	c.raw = true
	return func() { c.restored, c.unsent = true, len(c.replies) }, nil
}

func (c *fakeConsole) Write(p []byte) (int, error) {
	c.asked += string(p)
	c.askedAt = time.Now()
	return len(p), nil
}

func (c *fakeConsole) ReadNow(p []byte) (int, error) {
	switch {
	case c.asked == "" || time.Since(c.askedAt) < c.late:
		return 0, nil
	case len(c.replies) == 0:
		c.after++
		return 0, nil
	}
	n := copy(p, c.replies[0])
	c.replies = c.replies[1:]
	return n, nil
}

// hexOf is c written #RRGGBB, or "none".
func hexOf(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
