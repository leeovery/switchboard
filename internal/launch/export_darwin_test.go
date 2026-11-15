//go:build darwin

package launch

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
)

// What kern.procargs2 gives of a process, laid out for the package's tests,
// and read as ProcessTable reads it, for those outside the package too, so a
// whole launch reads a session's claude as the system gives it.

// SwitchboardsOf is switchboardsOf.
var SwitchboardsOf = switchboardsOf

// LaidOut lays argv and env out as kern.procargs2 does, after the path the
// process started from, padded with NULs as the system pads it, followed by
// what follows.
func LaidOut(path string, argv, env []string, follows string) []byte {
	args := binary.NativeEndian.AppendUint32(nil, uint32(len(argv)))
	args = append(append(args, path...), 0)
	for (len(args)-4)%argsAlign != 0 {
		args = append(args, 0)
	}
	for _, s := range slices.Concat(argv, env) {
		args = append(append(args, s...), 0)
	}
	return append(args, follows...)
}

// Titled is args, argv and env laid out after path as LaidOut lays them out,
// with title written over its arguments as libuv writes a process title,
// Node's: the title, then NULs to the arguments' end. A title longer than
// the arguments goes on over the environment, ended by a NUL.
func Titled(args []byte, path string, argv []string, title string) []byte {
	return TitledTo(args, path, title, argsStart(path)+max(len(strings.Join(argv, "\x00"))+1, len(title)+1))
}

// TitledAhead is args, laid out after path as LaidOut lays them out, with
// title written over everything ahead of the environment's variable named
// name, then NULs, the variable left whole.
func TitledAhead(args []byte, path, title, name string) []byte {
	return TitledTo(args, path, title, bytes.Index(args, []byte(name+"=")))
}

// TitledTo is args, laid out after path as LaidOut lays them out, with title
// written over everything up to end, then NULs.
func TitledTo(args []byte, path, title string, end int) []byte {
	over := args[argsStart(path):end]
	clear(over)
	copy(over, title)
	return args
}

// argsStart is where the arguments start in what LaidOut lays out after
// path.
func argsStart(path string) int {
	start := 4 + len(path) + 1
	for (start-4)%argsAlign != 0 {
		start++
	}
	return start
}
