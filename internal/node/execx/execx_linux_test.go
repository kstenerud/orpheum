// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{HoldFIFO: fifo})
	runner := newRunner(t, fake)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Cancel only once the fake has started a grandchild that holds stdout open.
	runner.started = func() {
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
	pid := fake.Grandchild(t)
	assertGone(t, pid)
}

// processState is the state letter in /proc/<pid>/stat, after the parenthesised command name.
func processState(stat string) string {
	_, after, _ := strings.Cut(stat[strings.LastIndex(stat, ")"):], " ")
	state, _, _ := strings.Cut(after, " ")
	return state
}
