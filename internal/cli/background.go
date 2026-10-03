package cli

import (
	"image/color"
	"io"
	"regexp"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/theme"
)

// listenEvery is how often the terminal is read for its answer while it's
// given its time.
const listenEvery = 5 * time.Millisecond

// TerminalBackground asks the terminal what its background is (OSC 11), as
// Deps.Background does, for the process's own output, giving it
// theme.AnswerWithin to say: nil where out isn't a terminal, the process has
// no terminal of its own to ask, or runs in the background, where asking
// would stop it, or where the terminal doesn't say in time.
func TerminalBackground(out io.Writer) color.Color {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return nil
	}
	c, closeConsole, err := openConsole()
	if err != nil {
		logger.Debug("no terminal to ask its background", "error", err)
		return nil
	}
	defer closeConsole()
	return askBackground(c, theme.AnswerWithin)
}

// console is a terminal as askBackground asks it: whether the process is in
// its foreground, put in raw mode, so its answer is neither shown nor held
// for a line's end, written to, and read without waiting.
type console interface {
	Foreground() bool
	// Raw puts the terminal in raw mode, and returns what puts it back.
	Raw() (restore func(), err error)
	Write(p []byte) (int, error)
	// ReadNow reads what the terminal has sent, without waiting for it: none,
	// and no error, where it has sent nothing.
	ReadNow(p []byte) (int, error)
}

// askBackground asks the terminal at c what its background is, giving it
// within to say: nil where the process runs in the background, as asking
// would stop it, or the terminal doesn't say in time. Its device attributes
// are asked after, which every terminal answers, so one that doesn't answer
// OSC 11 is known at once.
func askBackground(c console, within time.Duration) color.Color {
	if !c.Foreground() {
		logger.Debug("in the background: the terminal isn't asked its background")
		return nil
	}
	restore, err := c.Raw()
	if err != nil {
		logger.Debug("the terminal can't be asked its background", "error", err)
		return nil
	}
	defer restore()
	if _, err := c.Write([]byte(ansi.RequestBackgroundColor + ansi.RequestPrimaryDeviceAttributes)); err != nil {
		logger.Debug("the terminal can't be asked its background", "error", err)
		return nil
	}
	deadline := time.Now().Add(within)
	var heard []byte
	buf := make([]byte, 128)
	for {
		n, err := c.ReadNow(buf)
		heard = append(heard, buf[:n]...)
		background, done := answer(heard)
		switch {
		case err != nil:
			logger.Debug("the terminal's answer can't be read", "error", err)
			return background
		case done:
			return background
		case !time.Now().Before(deadline):
			logger.Debug("the terminal didn't say what its background is in time", "within", within)
			return background
		}
		time.Sleep(listenEvery)
	}
}

// The terminal's answers: to OSC 11, its background, and to DA1, its device
// attributes.
var (
	backgroundAnswer = regexp.MustCompile(`\x1b\]11;([^\x07\x1b]*)(?:\x07|\x1b\\)`)
	attributesAnswer = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)
)

// answer reads what the terminal has sent back, heard: the background its
// answer to OSC 11 gives, where it gave one, and whether it's done, its
// device attributes come, which it sends last. What else it sent, such as
// keys typed, is passed over.
func answer(heard []byte) (color.Color, bool) {
	var background color.Color
	if m := backgroundAnswer.FindSubmatch(heard); m != nil {
		background = ansi.XParseColor(string(m[1]))
	}
	return background, attributesAnswer.Match(heard)
}
