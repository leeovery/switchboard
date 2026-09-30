// Package handover hands a process's listening sockets to the program it
// replaces itself with, by exec, and takes them up in that program. The
// sockets stay open throughout, so a connection made meanwhile waits to be
// accepted, never refused.
package handover

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// Variable names, in the environment of the program a process replaced
// itself with, the listening sockets it handed over, each by name and
// descriptor: "control=9,proxy=8". An upgrade has one version set it and the
// next read it, so its form changes only in ways the next can still read.
const Variable = "SWITCHBOARD_LISTENERS"

// firstFD is the lowest descriptor a listener is handed over as: 0 to 2 are
// the standard input, output and error.
const firstFD = 3

// Held are listeners held open, by name, to hand over to the program this
// process replaces itself with.
type Held struct {
	files map[string]*os.File
}

// Hold holds open the listeners given, by name, for Exec to hand over: each
// socket is duplicated, so it outlives its listener's closing, and the
// connections made after that wait for the program handed it. A unix
// socket's listener no longer removes its path as it closes, as that program
// listens there. Hold fails, holding none, for a listener it can't
// duplicate.
func Hold(listeners map[string]net.Listener) (*Held, error) {
	h := &Held{files: make(map[string]*os.File, len(listeners))}
	for name, ln := range listeners {
		f, err := duplicate(ln)
		if err != nil {
			h.Close()
			return nil, fmt.Errorf("hold the %s listener: %w", name, err)
		}
		h.files[name] = f
	}
	for _, ln := range listeners {
		if u, ok := ln.(*net.UnixListener); ok {
			u.SetUnlinkOnClose(false)
		}
	}
	return h, nil
}

// duplicate returns a copy of the socket ln listens on, which exec closes.
func duplicate(ln net.Listener) (*os.File, error) {
	socket, ok := ln.(interface{ File() (*os.File, error) })
	if !ok {
		return nil, fmt.Errorf("a %T can't be handed over", ln)
	}
	return socket.File()
}

// Exec replaces this process with the program at path, run as argv with env,
// by exec, handing it the listeners held: they stay open in it, as the
// descriptors Variable, set in env in place of any value it had, names. It
// returns what exec does, which a real exec does only when it fails, and
// then closes what it would have handed over; Close closes the rest.
func (h *Held) Exec(exec func(path string, argv, env []string) error, path string, argv, env []string) error {
	// Descriptors exec leaves open, a program started alongside would inherit
	// too, holding the listeners open past this process: ForkLock keeps any
	// from starting meanwhile.
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	handing, err := h.inheritable()
	if err != nil {
		return err
	}
	if err := exec(path, argv, handing.set(env)); err != nil {
		handing.close()
		return err
	}
	return nil
}

// inheritable duplicates each listener held as a descriptor exec leaves open.
func (h *Held) inheritable() (handed, error) {
	handing := make(handed, len(h.files))
	for name, f := range h.files {
		fd, err := dupForExec(f)
		if err != nil {
			handing.close()
			return nil, fmt.Errorf("hand the %s listener over: %w", name, err)
		}
		handing[name] = fd
	}
	return handing, nil
}

// Close closes the listeners held: those never handed over, or, once they
// have been, this process's own copies, which exec closes.
func (h *Held) Close() {
	for _, f := range h.files {
		_ = f.Close()
	}
}

// Named returns the listeners env names as handed over: Variable's value in
// it, or "" when it has none.
func Named(env []string) string {
	for _, variable := range env {
		if value, ok := strings.CutPrefix(variable, Variable+"="); ok {
			return value
		}
	}
	return ""
}

// Take takes up the listeners value names, Variable's value as the process
// this one replaced set it, and returns them by name, a unix socket's
// listener removing its path as it closes, as one listened on afresh does.
// It fails, taking up none, for a value not in Variable's form; and says of
// each descriptor it passes over why, one that isn't a listening socket,
// leaving it alone, as it may be anything.
func Take(value string) (map[string]net.Listener, error) {
	named, err := parse(value)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]net.Listener, len(named))
	var passed []error
	for _, name := range slices.Sorted(maps.Keys(named)) {
		ln, err := take(named[name])
		if err != nil {
			passed = append(passed, fmt.Errorf("%s=%d: %w", name, named[name], err))
			continue
		}
		taken[name] = ln
	}
	return taken, errors.Join(passed...)
}

// take takes up the listener on the descriptor fd, once it's found to be
// one.
func take(fd int) (net.Listener, error) {
	if err := listening(fd); err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "listener")
	defer func() { _ = f.Close() }()
	ln, err := net.FileListener(f)
	if err != nil {
		return nil, err
	}
	if u, ok := ln.(*net.UnixListener); ok {
		u.SetUnlinkOnClose(true)
	}
	return ln, nil
}

// handed are the descriptors of listeners handed over, by name.
type handed map[string]int

// parse reads value in Variable's form: name=descriptor pairs, joined by
// commas, each name, lower-case letters, and each descriptor, a decimal
// number past the standard three, given once.
func parse(value string) (handed, error) {
	named := make(handed)
	fds := make(map[int]bool)
	for pair := range strings.SplitSeq(value, ",") {
		name, fd, ok := parsePair(pair)
		_, again := named[name]
		if !ok || again || fds[fd] {
			return nil, fmt.Errorf("%s isn't listeners' names and descriptors, each given once, such as control=9,proxy=8: %q", Variable, value)
		}
		named[name], fds[fd] = fd, true
	}
	return named, nil
}

// parsePair reads one name=descriptor pair, reporting whether it's one.
func parsePair(pair string) (name string, fd int, ok bool) {
	name, digits, found := strings.Cut(pair, "=")
	fd, err := strconv.Atoi(digits)
	ok = found && name != "" && !strings.ContainsFunc(name, notLower) &&
		err == nil && strconv.Itoa(fd) == digits && fd >= firstFD
	return name, fd, ok
}

func notLower(r rune) bool {
	return r < 'a' || r > 'z'
}

// String is h in Variable's form, its names in order.
func (h handed) String() string {
	pairs := make([]string, 0, len(h))
	for _, name := range slices.Sorted(maps.Keys(h)) {
		pairs = append(pairs, name+"="+strconv.Itoa(h[name]))
	}
	return strings.Join(pairs, ",")
}

// set returns env with Variable naming h, in place of any value it had.
func (h handed) set(env []string) []string {
	kept := slices.DeleteFunc(slices.Clone(env), func(variable string) bool {
		return strings.HasPrefix(variable, Variable+"=")
	})
	return append(kept, Variable+"="+h.String())
}

// close closes the descriptors h names.
func (h handed) close() {
	for _, fd := range h {
		closeFD(fd)
	}
}
