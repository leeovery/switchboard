//go:build darwin

package launch

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Process returns what the system knows of the running process with the
// given id: false where none runs.
func (ProcessTable) Process(pid int) (Process, bool) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(info.Proc.P_pid) != pid {
		return Process{}, false
	}
	started := info.Proc.P_starttime
	return Process{Parent: int(info.Eproc.Ppid), Started: time.Unix(started.Sec, int64(started.Usec)*int64(time.Microsecond))}, true
}

// Environ returns switchboard's variables of the environment the process
// with the given id started in, as switchboardsOf reads them, where the
// system lets it be read, as of a process of this user's: false otherwise.
func (ProcessTable) Environ(pid int) ([]string, bool) {
	args, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, false
	}
	return switchboardsOf(args)
}

// switchboardsOf returns switchboard's variables of the environment args
// lays out, as procArgs reads it: false where it can't.
func switchboardsOf(args []byte) ([]string, bool) {
	_, env, ok := procArgs(args)
	if !ok {
		return nil, false
	}
	return switchboards(env), true
}

// argsAlign is what the system pads the path a process started from to, in
// kern.procargs2, so its arguments start on a pointer's boundary, counted
// from the path's start.
const argsAlign = 8

// procArgs returns the arguments and the environment a process started with,
// as kern.procargs2 lays them out in args: the count of its arguments, a
// 32-bit integer in the machine's order; the path it was started from, ended
// by a NUL, and padded with NULs to argsAlign; then each argument, and each
// variable of its environment, ended by a NUL, the environment's last
// followed by an empty string or the end. Only the padding is passed over,
// so an empty first argument is read as one. The memory is read as it is
// now, which a process title can be written over, as libuv writes Node's:
// the title, then NULs to the arguments' end, so the empty strings past the
// arguments counted are passed over too. An environment of none reads so as
// the system's own strings that follow it, such as ptr_munge=, none of them
// switchboard's. It reports false where args isn't laid out so.
func procArgs(args []byte) (argv, env []string, ok bool) {
	if len(args) < 4 {
		return nil, nil, false
	}
	argc := int(int32(binary.NativeEndian.Uint32(args)))
	strs := args[4:]
	end := bytes.IndexByte(strs, 0)
	if end < 0 || argc < 0 {
		return nil, nil, false
	}
	start := end + 1
	for start < len(strs) && start%argsAlign != 0 && strs[start] == 0 {
		start++
	}
	fields := strings.Split(string(strs[start:]), "\x00")
	if argc > len(fields) {
		return nil, nil, false
	}
	env = fields[argc:]
	for len(env) > 0 && env[0] == "" {
		env = env[1:]
	}
	if end := slices.Index(env, ""); end >= 0 {
		env = env[:end]
	}
	return fields[:argc], env, true
}

// switchboards returns switchboard's variables of env, an environment as
// os.Environ gives it, in its order: those named SWITCHBOARD_….
func switchboards(env []string) []string {
	var own []string
	for _, variable := range env {
		if strings.HasPrefix(variable, "SWITCHBOARD_") {
			own = append(own, variable)
		}
	}
	return own
}
