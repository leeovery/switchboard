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
// reading it stops: a pseudo-terminal of the test's own, typed at through
// its master, holds nothing unread once Unread has run.
func TestUnreadThrowsAwayWhatWasTypedAtATerminal(t *testing.T) {
	master, tty := pseudoTerminal(t)
	if _, err := io.WriteString(master, "typed\n"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); unreadOn(t, tty) == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("what was typed never reached the terminal")
		}
	}
	backdrop.Unread(tty)()
	if n := unreadOn(t, tty); n != 0 {
		t.Errorf("the terminal holds %d bytes unread, want none: what was typed thrown away", n)
	}
}

// unreadOn is how many bytes the terminal tty holds unread.
func unreadOn(t *testing.T, tty *os.File) int {
	t.Helper()
	n, err := unix.IoctlGetInt(int(tty.Fd()), fionread)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// pseudoTerminal opens a pseudo-terminal, its master and its terminal, as
// grantpt, unlockpt and ptsname do, both closed as the test ends.
func pseudoTerminal(t *testing.T) (master, tty *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = master.Close() })
	for _, req := range []uint{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
		if err := unix.IoctlSetInt(int(master.Fd()), req, 0); err != nil {
			t.Fatal(err)
		}
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(master.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	tty, err = os.OpenFile(fmt.Sprintf("/dev/ttys%03d", unix.Minor(uint64(st.Rdev))), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return master, tty
}
