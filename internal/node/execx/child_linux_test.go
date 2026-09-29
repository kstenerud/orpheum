// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// syncBuffer is a buffer the child's output can be written to while the test reads it.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (out *syncBuffer) Write(data []byte) (int, error) {
	out.mutex.Lock()
	defer out.mutex.Unlock()
	return out.buffer.Write(data) //nolint:wrapcheck // a bytes.Buffer never fails
}

func (out *syncBuffer) String() string {
	out.mutex.Lock()
	defer out.mutex.Unlock()
	return out.buffer.String()
}

// holdingChild starts the fake as a long-lived child and waits until it is running.
func holdingChild(ctx context.Context, t *testing.T) (*Child, testutil.FakeFFmpeg) {
	t.Helper()
	fifo := filepath.Join(t.TempDir(), "ready")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{HoldFIFOPath: fifo, Stderr: "node: listening\n"})
	arguments := []string{"node", "--config", "/etc/o.toml"}
	child, err := StartSelf(ctx, filepath.Join(fake.Directory, "ffmpeg"), arguments, &syncBuffer{})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := os.Open(fifo)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(ready).ReadString('\n')
	if closeErr := ready.Close(); closeErr != nil {
		t.Error(closeErr)
	}
	if err != nil || line != "ready\n" {
		t.Fatalf("the child said %q, %v; want ready", line, err)
	}
	return child, fake
}

func TestStartSelf_StartsAChildWithAnEmptyEnvironmentAndKillsItWhole(t *testing.T) {
	t.Parallel()
	child, fake := holdingChild(t.Context(), t)
	call := fake.OnlyCall(t)
	if !slices.Equal(call.Arguments, []string{"node", "--config", "/etc/o.toml"}) {
		t.Errorf("the child ran with %q", call.Arguments)
	}
	if !slices.Equal(call.Environment, []string{"PWD=/"}) {
		t.Errorf("the child's environment is %q, want nothing but the shell's PWD=/ (D204)", call.Environment)
	}
	select {
	case <-child.Done():
		t.Fatal("the child ended before it was killed")
	default:
	}
	if err := child.Kill(); err != nil {
		t.Fatal(err)
	}
	<-child.Done()
	if child.Err() == nil {
		t.Error("a killed child reports no error")
	}
	if err := child.Kill(); err != nil {
		t.Errorf("killing a child that is gone: %v, want nothing to do", err)
	}
	assertGone(t, fake.GrandchildPID(t))
}

func TestStartSelf_EndingTheContextKillsTheChild(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	child, fake := holdingChild(ctx, t)
	cancel()
	<-child.Done()
	assertGone(t, fake.GrandchildPID(t))
}

func TestStartSelf_TheChildsOutputReachesTheCaller(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "out\n", Stderr: "err\n", ExitStatus: 3})
	output := &syncBuffer{}
	child, err := StartSelf(t.Context(), filepath.Join(fake.Directory, "ffmpeg"), nil, output)
	if err != nil {
		t.Fatal(err)
	}
	<-child.Done()
	if got := output.String(); !strings.Contains(got, "out\n") || !strings.Contains(got, "err\n") {
		t.Errorf("output = %q, want the child's stdout and stderr", got)
	}
	if err := child.Err(); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("Err() = %v, want the exit status", err)
	}
	if _, err := StartSelf(t.Context(), "orpheum", nil, output); err == nil {
		t.Error("a relative path to orpheum was accepted")
	}
}

// assertGone fails the test if pid, a grandchild that held the child's output open, is still
// asleep. The child is gone only once that output has closed, which a process does partway
// through exiting: by then it is gone, a zombie, or running its exit (R or D). Only a grandchild
// that was never killed is still sleeping (S), holding the output open.
func assertGone(t *testing.T, pid int) {
	t.Helper()
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil && processState(string(stat)) == "S" {
		t.Errorf("the grandchild %d is still asleep: the process group was not killed", pid)
	}
}
