// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The policies /proc/<pid>/stat names (sched(7)).
const (
	policyNormal = "0"
	policyIdle   = "5"
)

// trickplay.md §3: a run asked to be idle starts in SCHED_IDLE, and one not asked, as an encode's,
// in the normal class.
func TestRunWatched_AnIdleRunStartsInTheIdleSchedulingClassAndAnotherDoesNot(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		isIdle bool
		want   string
	}{{isIdle: true, want: policyIdle}, {isIdle: false, want: policyNormal}} {
		reply := testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilQuit}
		fake := testutil.InstallFakeFFmpeg(t, reply)
		runner, _ := watchedRunner(t, fake, clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
		reported := make(chan struct{}, 1)
		watch := testWatch
		watch.OnProgress = func(Progress) {
			select {
			case reported <- struct{}{}:
			default:
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		ended := make(chan error, 1)
		go func() {
			run := WatchedRun{Arguments: []string{"-i", "x"}, Watch: watch, IsIdle: check.isIdle}
			_, err := runner.RunWatched(ctx, run)
			ended <- err
		}()
		testutil.Receive(t, reported, "the run's report")
		if got := schedulingPolicy(t, fake.HungPID(t)); got != check.want {
			t.Errorf("a run with IsIdle %v has policy %s; want %s", check.isIdle, got, check.want)
		}
		cancel()
		if err := testutil.Receive(t, ended, "the cancelled run's end"); !errors.Is(err, context.Canceled) {
			t.Errorf("the cancelled run ended %v; want cancelled", err)
		}
	}
}

// schedulingPolicy is the policy field of a process's /proc stat, the 41st, counted after the name,
// which may hold spaces.
func schedulingPolicy(t *testing.T, pid int) string {
	t.Helper()
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		t.Fatal(err)
	}
	afterName := string(stat[bytes.LastIndexByte(stat, ')')+1:])
	fields := strings.Fields(afterName) // the first is field 3
	const policyField = 41
	if len(fields) < policyField-2 {
		t.Fatalf("the stat %q has no policy", stat)
	}
	return fields[policyField-3]
}
