//go:build darwin

package backdrop_test

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
)

// fionread is FIONREAD of <sys/filio.h>, which x/sys doesn't name: how many
// bytes a terminal holds unread.
const fionread = 0x4004667f

// What was typed at a terminal and never read is thrown away as the screen
// reading it stops, and only that: a pseudo-terminal of the test's own,
// typed at through its master, holds nothing unread once Unread has run,
// and what was drawn on it, yet to reach the master, is all still there.
func TestUnreadThrowsAwayWhatWasTypedAtATerminalAndNothingDrawn(t *testing.T) {
	master, tty := pseudoTerminal(t)
	if _, err := io.WriteString(tty, "drawn"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(master, "typed\n"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); queued(t, tty, fionread) == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("what was typed never reached the terminal")
		}
	}
	drawn := queued(t, tty, unix.TIOCOUTQ)

	backdrop.Unread(tty)()

	if n := queued(t, tty, fionread); n != 0 {
		t.Errorf("the terminal holds %d bytes unread, want none: what was typed thrown away", n)
	}
	if n := queued(t, tty, unix.TIOCOUTQ); n != drawn || n == 0 {
		t.Errorf("the terminal holds %d bytes drawn yet to reach its master, want the %d it held: nothing drawn thrown away", n, drawn)
	}
}

// queued is how many bytes the terminal tty holds in the queue req asks of.
func queued(t *testing.T, tty *os.File, req uint) int {
	t.Helper()
	n, err := unix.IoctlGetInt(int(tty.Fd()), req)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// pseudoTerminal opens a pseudo-terminal, its master and its terminal, as
// grantpt, unlockpt and ptsname do, its terminal echoing nothing typed, so
// all it holds drawn is what's drawn on it. As the test ends the master is
// closed first: a terminal closed first waits on what it holds drawn to
// reach the master.
func pseudoTerminal(t *testing.T) (master, tty *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []uint{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
		if err := unix.IoctlSetInt(int(master.Fd()), req, 0); err != nil {
			_ = master.Close()
			t.Fatal(err)
		}
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(master.Fd()), &st); err != nil {
		_ = master.Close()
		t.Fatal(err)
	}
	tty, err = os.OpenFile(fmt.Sprintf("/dev/ttys%03d", unix.Minor(uint64(st.Rdev))), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = master.Close(), tty.Close() })
	modes, err := unix.IoctlGetTermios(int(tty.Fd()), unix.TIOCGETA)
	if err != nil {
		t.Fatal(err)
	}
	modes.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(int(tty.Fd()), unix.TIOCSETA, modes); err != nil {
		t.Fatal(err)
	}
	return master, tty
}
