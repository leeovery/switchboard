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
// process started from and the NULs that pad it, followed by what follows.
func laidOut(path string, padding int, argv, env []string, follows string) []byte {
	args := binary.NativeEndian.AppendUint32(nil, uint32(len(argv)))
	args = append(args, path...)
	args = append(args, make([]byte, 1+padding)...)
	for _, s := range slices.Concat(argv, env) {
		args = append(append(args, s...), 0)
	}
	return append(args, follows...)
}

func TestProcArgsReadsTheArgumentsAndEnvironmentAsTheyreLaidOut(t *testing.T) {
	argv := []string{"claude", "-p", "SWITCHBOARD_POOL=an argument, not a variable", ""}
	env := []string{"HOME=/home/tester", "SWITCHBOARD_POOL=q7mt", "CLAUDE_CODE_OAUTH_TOKEN=test-token-work"}
	tests := []struct {
		name    string
		args    []byte
		wantEnv []string
		wantOK  bool
	}{
		{name: "ending at an empty string", args: laidOut("/opt/claude", 6, argv, env, "\x00ptr_munge=\x00"), wantEnv: env, wantOK: true},
		{name: "ending at the end", args: laidOut("/opt/claude", 0, argv, env, ""), wantEnv: env, wantOK: true},
		{name: "no environment", args: laidOut("/opt/claude", 3, argv, nil, "\x00"), wantOK: true},
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
			if !slices.Equal(gotArgv, argv) || !slices.Equal(gotEnv, tt.wantEnv) {
				t.Errorf("procArgs() = %q, %q; want %q, %q", gotArgv, gotEnv, argv, tt.wantEnv)
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
func TestFindProcessFindsThisProcess(t *testing.T) {
	p, ok := FindProcess(os.Getpid())
	if !ok || p.Parent != os.Getppid() || p.Started.IsZero() || p.Started.After(time.Now()) {
		t.Errorf("FindProcess(this process) = %v, parent %d, started %v; want it running, started by %d before now", ok, p.Parent, p.Started, os.Getppid())
	}
	if i := slices.IndexFunc(p.Environ, func(v string) bool { return !strings.HasPrefix(v, "SWITCHBOARD_") }); i >= 0 {
		t.Errorf("FindProcess(this process) holds the variable %d of %d, which isn't switchboard's", i, len(p.Environ))
	}
}

func TestFindProcessFindsNoneThatIsntRunning(t *testing.T) {
	const none = 1 << 30
	if _, ok := FindProcess(none); ok {
		t.Errorf("FindProcess(%d) found one, want none: no process id runs so high", none)
	}
}
