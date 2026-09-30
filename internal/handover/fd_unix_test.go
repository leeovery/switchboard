//go:build unix

package handover_test

import (
	"errors"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/handover"
)

func TestExecLeavesOpenTheListenersNamedAlone(t *testing.T) {
	dir := shortTempDir(t)
	before := inheritable(t)
	listeners := map[string]net.Listener{}
	for name, address := range map[string]string{"proxy": "127.0.0.1:0", "control": filepath.Join(dir, "control.sock")} {
		network := "tcp"
		if filepath.IsAbs(address) {
			network = "unix"
		}
		ln, err := net.Listen(network, address)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ln.Close() }()
		listeners[name] = ln
	}
	held, err := handover.Hold(listeners)
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	defer held.Close()

	var named, atExec []int
	exec := &recordedExec{}
	err = held.Exec(func(path string, argv, env []string) error {
		named, atExec = descriptors(t, handover.Named(env)), inheritable(t)
		return exec.run(path, argv, env)
	}, "/test/bin/switchboard", nil, nil)
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	taken, err := handover.Take(handover.Named(exec.env))
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(taken)
	if want := slices.Sorted(slices.Values(append(slices.Clone(before), named...))); !slices.Equal(atExec, want) {
		t.Errorf("as exec ran, the descriptors it leaves open were %v, want those open before, %v, and those named, %v, alone", atExec, before, named)
	}
	for _, ln := range listeners {
		if fd := descriptorOf(t, ln.(syscall.Conn)); !closesOnExec(fd) {
			t.Errorf("the listener's own descriptor, %d, isn't marked to close on exec", fd)
		}
	}
}

func TestTakePassesOverADescriptorThatIsntAListener(t *testing.T) {
	dir := shortTempDir(t)
	file, err := os.Create(filepath.Join(dir, "router.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	datagrams, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = datagrams.Close() }()
	tests := []struct {
		name string
		f    *os.File
		want string
	}{
		{name: "a file", f: file, want: "not a socket"},
		{name: "a connected socket", f: fileOf(t, conn.(*net.TCPConn)), want: "a connected socket"},
		{name: "a datagram socket", f: fileOf(t, datagrams.(*net.UDPConn)), want: "not a stream socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := hold(t, map[string]string{"control": "unix"}, dir)
			defer held.Close()
			exec := &recordedExec{}
			if err := held.Exec(exec.run, "/test/bin/switchboard", nil, nil); err != nil {
				t.Fatal(err)
			}
			fd := inheritableCopy(t, tt.f)
			foreign := "proxy=" + strconv.Itoa(fd)

			taken, err := handover.Take(handover.Named(exec.env) + "," + foreign)
			defer closeAll(taken)
			if err == nil || !strings.Contains(err.Error(), foreign+": "+tt.want) {
				t.Errorf("Take() error = %v, want it to name %s as %s", err, foreign, tt.want)
			}
			if got := slices.Sorted(maps.Keys(taken)); !slices.Equal(got, []string{"control"}) {
				t.Errorf("Take() took up %q, want the listener alone", got)
			}
			closes, open := flagsOf(fd)
			if !open {
				t.Fatalf("%s once passed over is closed, want it left open", tt.name)
			}
			if !closes {
				t.Errorf("%s once passed over isn't marked to close on exec, so every program the router runs inherits it", tt.name)
			}
		})
	}
}

// inheritable returns the descriptors open in this process that exec leaves
// open, in order.
func inheritable(t *testing.T) []int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	var fds []int
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if closes, open := flagsOf(fd); open && !closes {
			fds = append(fds, fd)
		}
	}
	slices.Sort(fds)
	return fds
}

// flagsOf reports whether the descriptor fd is marked to close on exec, and
// whether it's open at all.
func flagsOf(fd int) (closes, open bool) {
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	return flags&syscall.FD_CLOEXEC != 0, !errors.Is(errno, syscall.EBADF)
}

// closesOnExec reports whether the descriptor fd is marked to close on exec.
func closesOnExec(fd int) bool {
	closes, _ := flagsOf(fd)
	return closes
}

// inheritableCopy duplicates f's descriptor as one exec leaves open, as a
// descriptor a process was started with is, closed as the test ends.
func inheritableCopy(t *testing.T, f *os.File) int {
	t.Helper()
	fd, err := syscall.Dup(descriptorOf(t, f))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Close(fd) })
	return fd
}

// descriptorOf returns the descriptor c is on.
func descriptorOf(t *testing.T, c syscall.Conn) int {
	t.Helper()
	raw, err := c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	fd := -1
	if err := raw.Control(func(s uintptr) { fd = int(s) }); err != nil {
		t.Fatal(err)
	}
	return fd
}

// descriptors returns the descriptors value names, in Variable's form.
func descriptors(t *testing.T, value string) []int {
	t.Helper()
	var fds []int
	for pair := range strings.SplitSeq(value, ",") {
		_, digits, _ := strings.Cut(pair, "=")
		fd, err := strconv.Atoi(digits)
		if err != nil {
			t.Fatalf("%s = %q names no descriptor in %q", handover.Variable, value, pair)
		}
		fds = append(fds, fd)
	}
	return fds
}
