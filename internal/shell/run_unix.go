//go:build unix

package shell

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Run runs the program at path with args, in env, as Runner says. It runs in
// a session of its own, so an interactive shell finds no terminal to take,
// and stops for, and in a process group of its own, which is killed whole as
// soon as the program exits, or ctx ends, so nothing its startup files
// started outlives it, nor holds its output open.
func Run(ctx context.Context, path string, args, env []string) ([]byte, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = read.Close() }()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Stdout = env, write
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process) }
	err = cmd.Start()
	_ = write.Close()
	if err != nil {
		return nil, err
	}
	out := make(chan []byte, 1)
	go func() {
		said, _ := io.ReadAll(read)
		out <- said
	}()
	// Stdout is a file, so Wait returns as the program exits, not once all
	// that holds its output open is done.
	err = cmd.Wait()
	_ = killGroup(cmd.Process)
	select {
	case said := <-out:
		return said, err
	case <-time.After(runWaitDelay):
		// Something that left the group holds the output open: what's read
		// of it so far is all there is.
		_ = read.Close()
		return <-out, err
	}
}

// runWaitDelay bounds the wait for the program's output once its group is
// killed, as something that left it can hold the output open.
const runWaitDelay = time.Second

// killGroup kills the process group p leads, whatever's left of it.
func killGroup(p *os.Process) error {
	return syscall.Kill(-p.Pid, syscall.SIGKILL)
}
