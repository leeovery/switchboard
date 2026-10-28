//go:build unix

package shell

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// runWaitDelay bounds the wait for a program's output once it has exited, or
// been killed: something it leaves running can hold its output open.
const runWaitDelay = time.Second

// Run runs the program at path with args, in env, as Runner says. It runs in
// a session of its own, so an interactive shell finds no terminal to take,
// and stops for, and in a process group of its own, which is killed whole
// once ctx ends, so nothing its startup files started outlives it.
func Run(ctx context.Context, path string, args, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.WaitDelay = env, runWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd.Output()
}
