//go:build darwin

package launch

import (
	"encoding/binary"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"golang.org/x/sys/unix"
)

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
		{name: "ending at an empty string", args: LaidOut("/opt/claude", argv, env, "\x00ptr_munge=\x00"), wantEnv: env, wantOK: true},
		{name: "ending at the end", args: LaidOut("/opt/claude", argv, env, ""), wantEnv: env, wantOK: true},
		{name: "a path padded to nothing", args: LaidOut(unpadded, argv, env, "\x00"), wantEnv: env, wantOK: true},
		{name: "an empty first argument", args: LaidOut("/opt/claude", []string{"", "-p"}, env, "\x00"), wantArgv: []string{"", "-p"}, wantEnv: env, wantOK: true},
		{name: "an empty first argument, after a path padded to nothing", args: LaidOut(unpadded, []string{"", "-p"}, env, "\x00"), wantArgv: []string{"", "-p"}, wantEnv: env, wantOK: true},
		{
			name:     "a process title written over the arguments, as libuv writes Node's",
			args:     Titled(LaidOut("/opt/claude", argv, env, "\x00ptr_munge=\x00"), "/opt/claude", argv, "claude"),
			wantArgv: []string{"claude", "", "", ""}, wantEnv: env, wantOK: true,
		},
		{name: "no environment", args: LaidOut("/opt/claude", argv, nil, "\x00"), wantOK: true},
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
	if p.Owner != os.Geteuid() {
		t.Errorf("Process(this process) runs as the user %d, want %d, the one it runs as", p.Owner, os.Geteuid())
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

// laidOutTable is the processes running, as ProcessTable tells of them, each
// one's environment read from args, as kern.procargs2 gives it.
type laidOutTable map[int]laidOutProcess

type laidOutProcess struct {
	Process
	args []byte
}

func (t laidOutTable) Process(pid int) (Process, bool) {
	p, ok := t[pid]
	return p.Process, ok
}

func (t laidOutTable) Environ(pid int) ([]string, bool) {
	p, ok := t[pid]
	if !ok {
		return nil, false
	}
	return switchboardsOf(p.args)
}

// A session's claude that Node runs writes its title over the memory its
// environment is read from: what of it the title leaves is read, and what
// the title took, the session's mark of its claude, or of what run set, with
// it, is read from the environment the launch inherits.
func TestASessionsClaudeWhoseTitleIsWrittenOverWhatsReadIsFound(t *testing.T) {
	const (
		sessionPID = 5150
		withinPID  = 6001
		node       = "/usr/local/bin/node"
		acme       = "q7mt"
	)
	at := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	m := startedMark{pid: sessionPID, at: at, path: "/usr/local/bin/claude"}
	own := environ{"HOME=/home/tester", "PATH=/usr/bin:/bin"}
	sessionEnv := own.withOrUnset(PoolEnv, acme).markedAgainst(own, m).startingAt(m)
	argv := []string{node, "/usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js", "--resume"}
	laid := func(title string) []byte {
		return Titled(LaidOut(node, argv, sessionEnv, "\x00ptr_munge=\x00"), node, argv, title)
	}
	// toItsMark is the title written over everything ahead of the session's
	// mark of its claude, which is left whole, and what run set taken.
	toItsMark := func() []byte {
		return TitledAhead(LaidOut(node, argv, sessionEnv, "\x00ptr_munge=\x00"), node, "claude", startedEnv)
	}
	tests := []struct {
		name string
		// args is the session's claude's, as kern.procargs2 gives them, and
		// inherited the environment the launch within it starts from.
		args      []byte
		inherited []string
		inSession bool
	}{
		{name: "its title written over its arguments, as libuv writes Node's", args: laid("claude"), inherited: own, inSession: true},
		{
			name: "its title written over its environment too, the launch inheriting the session's",
			args: laid(strings.Repeat("claude ", 40)), inherited: sessionEnv, inSession: true,
		},
		{name: "its title written over its environment too, the launch's environment cleaned", args: laid(strings.Repeat("claude ", 40)), inherited: own},
		{
			name: "its title written over all its environment ahead of its mark of the claude, the launch inheriting the session's",
			args: toItsMark(), inherited: sessionEnv, inSession: true,
		},
		{name: "its title written over all its environment ahead of its mark of the claude, the launch's environment cleaned", args: toItsMark(), inherited: own},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Launcher{PID: withinPID, Environ: tt.inherited, Processes: laidOutTable{
				withinPID:  {Process: Process{Parent: sessionPID, Started: at.Add(time.Minute)}},
				sessionPID: {Process: Process{Parent: 1, Started: at}, args: tt.args},
			}}
			want := ""
			if tt.inSession {
				want = acme
			}
			if got := l.findOuter().among(config.Pools{{ID: acme}}); got.pool != want {
				t.Errorf("found within the session %+v, want one in pool %q", got, want)
			}
		})
	}
}
