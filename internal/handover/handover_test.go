package handover_test

import (
	"bufio"
	"errors"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/handover"
)

func TestAListenerHandedOverServesAConnectionMadeBeforeItsTakenUp(t *testing.T) {
	tests := []struct {
		name    string
		network string
		// address is where to listen, given a directory of the test's own.
		address func(dir string) string
	}{
		{name: "on TCP", network: "tcp", address: func(string) string { return "127.0.0.1:0" }},
		{name: "on a unix socket", network: "unix", address: func(dir string) string { return filepath.Join(dir, "control.sock") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln, err := net.Listen(tt.network, tt.address(shortTempDir(t)))
			if err != nil {
				t.Fatal(err)
			}
			address := ln.Addr().String()
			held, err := handover.Hold(map[string]net.Listener{"proxy": ln})
			if err != nil {
				t.Fatalf("Hold() error = %v", err)
			}
			// The server stops taking connections, as it does before exec.
			_ = ln.Close()

			conn, err := net.Dial(tt.network, address)
			if err != nil {
				t.Fatalf("a connection made between the server stopping and the listener's taking up: %v, want it waiting to be accepted", err)
			}
			defer func() { _ = conn.Close() }()
			if _, err := conn.Write([]byte("hello\n")); err != nil {
				t.Fatal(err)
			}
			exec := &recordedExec{}
			if err := held.Exec(exec.run, "/test/bin/switchboard", []string{"/test/bin/switchboard", "serve"}, nil); err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			held.Close()

			taken, err := handover.Take(handover.Named(exec.env))
			if err != nil {
				t.Fatalf("Take() error = %v", err)
			}
			serveOne(t, taken["proxy"])
			if answer, err := bufio.NewReader(conn).ReadString('\n'); err != nil || answer != "hello\n" {
				t.Errorf("the connection was answered %q, %v, want its line echoed by the listener taken up", answer, err)
			}
			_ = taken["proxy"].Close()
			if _, err := os.Stat(address); tt.network == "unix" && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the socket's path once the listener taken up closed: %v, want it gone, as a listener's own closing removes it", err)
			}
		})
	}
}

func TestExecNamesTheListenersHandedOver(t *testing.T) {
	tests := []struct {
		name string
		env  []string
	}{
		{name: "in an environment without the variable", env: []string{"HOME=/home/tester", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}},
		{name: "in place of the value it had", env: []string{"HOME=/home/tester", handover.Variable + "=control=41,proxy=40", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := shortTempDir(t)
			held := hold(t, map[string]string{"proxy": "tcp", "control": "unix"}, dir)
			defer held.Close()
			exec := &recordedExec{}
			argv := []string{"/test/bin/switchboard", "serve", "--config", "/test/config.toml"}

			if err := held.Exec(exec.run, "/test/bin/switchboard", argv, tt.env); err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			if exec.path != "/test/bin/switchboard" || !slices.Equal(exec.argv, argv) {
				t.Errorf("exec ran %s as %q, want /test/bin/switchboard as %q", exec.path, exec.argv, argv)
			}
			var rest, values []string
			for _, variable := range exec.env {
				if value, ok := strings.CutPrefix(variable, handover.Variable+"="); ok {
					values = append(values, value)
					continue
				}
				rest = append(rest, variable)
			}
			if want := []string{"HOME=/home/tester", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}; !slices.Equal(rest, want) {
				t.Errorf("exec's environment, but for %s, is %q, want %q", handover.Variable, rest, want)
			}
			if len(values) != 1 || values[0] != handover.Named(exec.env) {
				t.Fatalf("exec's environment sets %s to %q, want one value, which Named gives", handover.Variable, values)
			}
			taken, err := handover.Take(values[0])
			if err != nil {
				t.Fatalf("Take(%q) error = %v", values[0], err)
			}
			defer closeAll(taken)
			if got := slices.Sorted(maps.Keys(taken)); !slices.Equal(got, []string{"control", "proxy"}) {
				t.Errorf("%s = %q named %q, want each listener held", handover.Variable, values[0], got)
			}
			if !strings.HasPrefix(values[0], "control=") {
				t.Errorf("%s = %q, want the listeners named in order", handover.Variable, values[0])
			}
		})
	}
}

func TestAnExecThatFailsLeavesTheListenerClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	held, err := handover.Hold(map[string]net.Listener{"proxy": ln})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	_ = ln.Close()
	refused := errors.New("exec format error")

	err = held.Exec(func(string, []string, []string) error { return refused }, "/test/bin/switchboard", nil, nil)
	if !errors.Is(err, refused) {
		t.Errorf("Exec() = %v, want exec's error", err)
	}
	held.Close()
	if conn, err := net.Dial("tcp", ln.Addr().String()); err == nil {
		_ = conn.Close()
		t.Error("a connection to the listener's address was taken, want it refused: nothing is left holding the socket open")
	}
}

func TestHoldRefusesAListenerItCantDuplicate(t *testing.T) {
	good, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = good.Close() }()

	held, err := handover.Hold(map[string]net.Listener{"proxy": good, "control": opaqueListener{good}})
	if held != nil || err == nil || !strings.Contains(err.Error(), "control") {
		t.Errorf("Hold() = %v, %v, want an error naming the listener it can't duplicate, holding none", held, err)
	}
}

func TestTakeRefusesAValueNotInItsForm(t *testing.T) {
	for _, value := range []string{
		"",
		"proxy",
		"proxy=",
		"=9",
		"proxy=nine",
		"proxy=-9",
		"proxy=+9",
		"proxy=09",
		"proxy=9.0",
		"proxy= 9",
		"proxy=2",
		"proxy=0",
		"Proxy=9",
		"proxy-1=9",
		"proxy=9,",
		"proxy=9;control=10",
		"proxy=9,proxy=10",
		"proxy=9,control=9",
		"proxy=99999999999999999999",
	} {
		t.Run(value, func(t *testing.T) {
			taken, err := handover.Take(value)
			if taken != nil || err == nil || !strings.Contains(err.Error(), handover.Variable) {
				t.Errorf("Take(%q) = %v, %v, want none taken, and an error naming %s", value, taken, err, handover.Variable)
			}
		})
	}
}

// recordedExec notes what it was run with, and succeeds, standing in for an
// exec that replaced the process: what it was handed stays open for a test
// to take up.
type recordedExec struct {
	path      string
	argv, env []string
}

func (e *recordedExec) run(path string, argv, env []string) error {
	e.path, e.argv, e.env = path, argv, env
	return nil
}

// hold listens on each network named, by the name given, a unix socket in
// dir, and holds the listeners, closing them, as a server stopping does.
func hold(t *testing.T, networks map[string]string, dir string) *handover.Held {
	t.Helper()
	listeners := make(map[string]net.Listener)
	for name, network := range networks {
		address := "127.0.0.1:0"
		if network == "unix" {
			address = filepath.Join(dir, name+".sock")
		}
		ln, err := net.Listen(network, address)
		if err != nil {
			t.Fatal(err)
		}
		listeners[name] = ln
	}
	held, err := handover.Hold(listeners)
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	for _, ln := range listeners {
		_ = ln.Close()
	}
	return held
}

// serveOne accepts one connection on ln, and echoes the line it sends.
func serveOne(t *testing.T, ln net.Listener) {
	t.Helper()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
}

// fileOf returns a copy of the socket c is on, closed as the test ends.
func fileOf(t *testing.T, c syscall.Conn) *os.File {
	t.Helper()
	socket, ok := c.(interface{ File() (*os.File, error) })
	if !ok {
		t.Fatalf("%T has no File", c)
	}
	f, err := socket.File()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func closeAll(listeners map[string]net.Listener) {
	for _, ln := range listeners {
		_ = ln.Close()
	}
}

// opaqueListener is a listener whose socket can't be duplicated.
type opaqueListener struct {
	net.Listener
}

// shortTempDir returns a directory of the test's own with a path short enough
// to hold a unix socket: t.TempDir's can be too long on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ho")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
