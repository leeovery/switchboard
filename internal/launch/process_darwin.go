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

// FindProcess returns what the system knows of the running process with the
// given id: false where none runs. Its environment is switchboard's variables
// of the one it started in, where the system lets it be read, as of a process
// of this user's; none otherwise.
func FindProcess(pid int) (Process, bool) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(info.Proc.P_pid) != pid {
		return Process{}, false
	}
	started := info.Proc.P_starttime
	p := Process{Parent: int(info.Eproc.Ppid), Started: time.Unix(started.Sec, int64(started.Usec)*int64(time.Microsecond))}
	if args, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
		if _, env, ok := procArgs(args); ok {
			p.Environ = switchboards(env)
		}
	}
	return p, true
}

// procArgs returns the arguments and the environment a process started with,
// as kern.procargs2 lays them out in args: the count of its arguments, a
// 32-bit integer in the machine's order; the path it was started from, ended
// by one NUL or more; then each argument, and each variable of its
// environment, ended by a NUL, the environment's last followed by an empty
// string or the end. It reports false where args isn't laid out so.
func procArgs(args []byte) (argv, env []string, ok bool) {
	if len(args) < 4 {
		return nil, nil, false
	}
	argc := int(int32(binary.NativeEndian.Uint32(args)))
	_, strs, found := bytes.Cut(args[4:], []byte{0})
	fields := strings.Split(string(bytes.TrimLeft(strs, "\x00")), "\x00")
	if !found || argc < 0 || argc > len(fields) {
		return nil, nil, false
	}
	env = fields[argc:]
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
