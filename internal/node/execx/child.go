// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Child is a long-lived process: orpheum's own binary in another mode, started by StartSelf.
type Child struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
}

// StartSelf starts executable — orpheum's own binary — with arguments, as a long-lived child
// (D207): an empty environment (D204), the root directory as its working directory, stdin
// closed, and its own process group, which is killed whole when ctx ends. Its stdout and stderr
// go to output.
func StartSelf(ctx context.Context, executable string, arguments []string, output io.Writer) (*Child, error) {
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("orpheum's own path must be absolute, not %q", executable)
	}
	command := exec.CommandContext(ctx, executable, arguments...) //nolint:gosec // orpheum itself, argv as a slice
	command.Env = []string{}
	command.Dir = "/"
	command.Stdout = output
	command.Stderr = output
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = waitDelay
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s %v: %w", executable, arguments, err)
	}
	child := &Child{command: command, done: make(chan struct{})}
	go func() {
		child.err = command.Wait()
		close(child.done)
	}()
	return child, nil
}

// Done is closed when the child has exited and its output is drained.
func (child *Child) Done() <-chan struct{} { return child.done }

// Err is how the child ended; read it after Done is closed.
func (child *Child) Err() error {
	if child.err != nil {
		return fmt.Errorf("the child %d ended: %w", child.command.Process.Pid, child.err)
	}
	return nil
}

// Kill kills the child and everything it started, and waits for it to be gone.
func (child *Child) Kill() error {
	select {
	case <-child.done:
		return nil
	default:
	}
	if err := syscall.Kill(-child.command.Process.Pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill the child %d: %w", child.command.Process.Pid, err)
	}
	<-child.done
	return nil
}
