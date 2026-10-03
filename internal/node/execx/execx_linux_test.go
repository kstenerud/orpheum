// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestRun_CancellingKillsTheWholeProcessGroup(t *testing.T) {
	t.Parallel()
	fifo := filepath.Join(t.TempDir(), "ready")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{HoldFIFOPath: fifo})
	runner := newRunner(t, fake)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Cancel only once the fake has started a grandchild that holds stdout open.
	runner.onStarted = func() {
		ready, err := os.Open(fifo)
		if err != nil {
			t.Error(err)
			cancel()
			return
		}
		line, err := bufio.NewReader(ready).ReadString('\n')
		if closeErr := ready.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if err != nil || line != "ready\n" {
			t.Errorf("the fake said %q, %v; want ready", line, err)
		}
		cancel()
	}

	_, err := runner.Run(ctx, FFmpeg, confine.Access{}, nil, 1024)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	// Run returns once the grandchild's stdout closes, which is when it dies; if only the tool
	// were killed, Run would return after waitDelay with the grandchild still asleep.
	pid := fake.GrandchildPID(t)
	assertGone(t, pid)
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

// processState is the state letter in /proc/<pid>/stat, after the parenthesised command name.
func processState(stat string) string {
	parsed, _ := parseStat(stat)
	return parsed.state
}
