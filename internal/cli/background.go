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

const (
	// listenEvery is how often the terminal is read for its answer while
	// it's given its time.
	listenEvery = 5 * time.Millisecond
	// lateFor is how long a terminal that didn't answer in time is kept raw
	// after, its late answers read and dropped, so they aren't echoed into
	// the shell once it's put back.
	lateFor = 100 * time.Millisecond
)

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
// OSC 11 is known at once. One that doesn't answer in time is kept raw for
// lateFor more, or until its device attributes come, what it sends then
// dropped, so its answers aren't left in the shell's input.
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
	heard, done, err := listen(c, nil, time.Now().Add(within))
	switch {
	case err != nil:
		logger.Debug("the terminal's answer can't be read", "error", err)
	case !done:
		logger.Debug("the terminal didn't say what its background is in time", "within", within)
		if _, _, err := listen(c, heard, time.Now().Add(lateFor)); err != nil {
			logger.Debug("the terminal's late answer can't be read", "error", err)
		}
	}
	return backgroundIn(heard)
}

// listen reads what the terminal at c sends back, after heard, until its
// device attributes come, which it sends last, or until passes, and returns
// all it has heard, and whether they came.
func listen(c console, heard []byte, until time.Time) ([]byte, bool, error) {
	buf := make([]byte, 128)
	for {
		n, err := c.ReadNow(buf)
		heard = append(heard, buf[:n]...)
		switch {
		case err != nil:
			return heard, false, err
		case attributesAnswer.Match(heard):
			return heard, true, nil
		case !time.Now().Before(until):
			return heard, false, nil
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

// backgroundIn is the background the terminal's answer to OSC 11 gives, in
// what it has sent back, heard, where it gave one. What else it sent, such as
// keys typed, is passed over.
func backgroundIn(heard []byte) color.Color {
	m := backgroundAnswer.FindSubmatch(heard)
	if m == nil {
		return nil
	}
	return ansi.XParseColor(string(m[1]))
}
