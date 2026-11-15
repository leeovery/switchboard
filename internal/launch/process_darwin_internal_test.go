//go:build darwin

package launch

import (
	"encoding/binary"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// laidOut lays argv and env out as kern.procargs2 does, after the path the
// process started from, padded with NULs as the system pads it, followed by
// what follows.
func laidOut(path string, argv, env []string, follows string) []byte {
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

func TestProcArgsReadsTheArgumentsAndEnvironmentAsTheyreLaidOut(t *testing.T) {
	argv := []string{"claude", "-p", "SWITCHBOARD_POOL=an argument, not a variable", ""}
	env := []string{"HOME=/home/tester", "SWITCHBOARD_POOL=q7mt", "CLAUDE_CODE_OAUTH_TOKEN=test-token-work"}
	// unpadded is a path that, with its NUL, ends on the boundary the system
	// pads to, so no padding follows it.
	const unpadded = "/opt/claude/bin"
	tests := []struct {
		name string
		args []byte
		// wantArgv is argv unless it's given.
		wantArgv, wantEnv []string
		wantOK            bool
	}{
		{name: "ending at an empty string", args: laidOut("/opt/claude", argv, env, "\x00ptr_munge=\x00"), wantEnv: env, wantOK: true},
		{name: "ending at the end", args: laidOut("/opt/claude", argv, env, ""), wantEnv: env, wantOK: true},
		{name: "a path padded to nothing", args: laidOut(unpadded, argv, env, "\x00"), wantEnv: env, wantOK: true},
		{name: "an empty first argument", args: laidOut("/opt/claude", []string{"", "-p"}, env, "\x00"), wantArgv: []string{"", "-p"}, wantEnv: env, wantOK: true},
		{name: "an empty first argument, after a path padded to nothing", args: laidOut(unpadded, []string{"", "-p"}, env, "\x00"), wantArgv: []string{"", "-p"}, wantEnv: env, wantOK: true},
		{name: "no environment", args: laidOut("/opt/claude", argv, nil, "\x00"), wantOK: true},
		{name: "too short for a count", args: []byte{1, 0}},
		{name: "a path never ended", args: append(binary.NativeEndian.AppendUint32(nil, 1), "/opt/claude"...)},
		{name: "more arguments counted than there are", args: append(binary.NativeEndian.AppendUint32(nil, 99), "/opt/claude\x00claude\x00"...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgv, gotEnv, ok := procArgs(tt.args)
			if ok != tt.wantOK {
				t.Fatalf("procArgs() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			wantArgv := argv
			if tt.wantArgv != nil {
				wantArgv = tt.wantArgv
			}
			if !slices.Equal(gotArgv, wantArgv) || !slices.Equal(gotEnv, tt.wantEnv) {
				t.Errorf("procArgs() = %q, %q; want %q, %q", gotArgv, gotEnv, wantArgv, tt.wantEnv)
			}
		})
	}
}

func TestSwitchboardsKeepsNoVariableButSwitchboards(t *testing.T) {
	env := []string{"HOME=/home/tester", "SWITCHBOARD_POOL=q7mt", "CLAUDE_CODE_OAUTH_TOKEN=test-token-work", "SWITCHBOARD_STARTED=5150:1:/opt/claude", "XSWITCHBOARD_POOL=no"}
	if got, want := switchboards(env), []string{"SWITCHBOARD_POOL=q7mt", "SWITCHBOARD_STARTED=5150:1:/opt/claude"}; !slices.Equal(got, want) {
		t.Errorf("switchboards(%q) = %q, want %q", env, got, want)
	}
}

// The system's own layout of this test's process is read as procArgs reads
// it: its arguments are its own. Its environment is read, but never looked
// at, as the real one it started in.
func TestProcArgsReadsThisProcessAsTheSystemLaysItOut(t *testing.T) {
	args, err := unix.SysctlRaw("kern.procargs2", os.Getpid())
	if err != nil {
		t.Fatalf("kern.procargs2 of this process: %v", err)
	}
	if argv, _, ok := procArgs(args); !ok || !slices.Equal(argv, os.Args) {
		t.Errorf("procArgs(this process) = %q, %v; want its arguments, %q", argv, ok, os.Args)
	}
}

// This test's process is found as the system has it, its environment never
// looked at but to see it holds nothing but switchboard's variables.
func TestProcessTableFindsThisProcess(t *testing.T) {
	p, ok := ProcessTable{}.Process(os.Getpid())
	if !ok || p.Parent != os.Getppid() || p.Started.IsZero() || p.Started.After(time.Now()) {
		t.Errorf("Process(this process) = %v, parent %d, started %v; want it running, started by %d before now", ok, p.Parent, p.Started, os.Getppid())
	}
	env, ok := ProcessTable{}.Environ(os.Getpid())
	if !ok {
		t.Fatal("Environ(this process) read none, want the environment it started in")
	}
	if i := slices.IndexFunc(env, func(v string) bool { return !strings.HasPrefix(v, "SWITCHBOARD_") }); i >= 0 {
		t.Errorf("Environ(this process) holds the variable %d of %d, which isn't switchboard's", i, len(env))
	}
}

func TestProcessTableFindsNoneThatIsntRunning(t *testing.T) {
	const none = 1 << 30
	if _, ok := (ProcessTable{}).Process(none); ok {
		t.Errorf("Process(%d) found one, want none: no process id runs so high", none)
	}
	if _, ok := (ProcessTable{}).Environ(none); ok {
		t.Errorf("Environ(%d) read one, want none: no process id runs so high", none)
	}
}
