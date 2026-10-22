// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// hangingProgress is what a hanging fake reports before it hangs: 4 s written.
const hangingProgress = "frame=100\nout_time_us=4000000\nspeed=2.0x\nprogress=continue\n"

// watchedRunner is a runner of fake whose watchdog reads clockSource and judges at each tick the
// test sends.
func watchedRunner(t *testing.T, fake testutil.FakeFFmpeg, clockSource *clocktest.Fake) (*Runner, chan time.Time) {
	t.Helper()
	runner := newRunner(t, fake)
	ticks := make(chan time.Time)
	runner.clock = clockSource
	runner.watchTicks = func() (<-chan time.Time, func()) { return ticks, func() {} }
	return runner, ticks
}

// runUntilStalled runs a hanging fake watched by testWatch, and once it has reported its progress,
// moves the clock past the stall window and ticks; it returns how the run ended.
func runUntilStalled(t *testing.T, runner *Runner, ticks chan time.Time, clockSource *clocktest.Fake) error {
	t.Helper()
	reported := make(chan struct{}, 1)
	watch := testWatch
	watch.OnProgress = func(Progress) {
		select {
		case reported <- struct{}{}:
		default:
		}
	}
	ended := make(chan error, 1)
	go func() {
		_, err := runner.RunWatched(t.Context(), WatchedRun{Arguments: []string{"-i", "x"}, Watch: watch})
		ended <- err
	}()
	testutil.Receive(t, reported, "the watch's report")
	clockSource.Advance(testWatch.StallWindow)
	ticks <- clockSource.Now()
	select {
	case err := <-ended:
		return err
	case <-time.After(waitDelay):
		t.Fatal("the stalled run was not stopped")
		return nil
	}
}

func TestRunWatched_AStalledRunIsQuitGracefullyAndSaysWhereItStood(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilQuit})
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	runner, ticks := watchedRunner(t, fake, clockSource)
	// Graces far longer than the test: it ends only because the graceful quit, SIGINT, worked.
	runner.quitGrace, runner.termGrace = time.Hour, time.Hour

	err := runUntilStalled(t, runner, ticks, clockSource)
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || stalled.Cause != StallStoodStill || stalled.LastPosition != 4*time.Second ||
		stalled.WithoutAdvance != testWatch.StallWindow || stalled.Tool != FFmpeg {
		t.Fatalf("a run that stood at 4 s for its window ended %v; want stalled, stood still at 4 s", err)
	}
	if runError, isRunError := errors.AsType[*RunError](err); isRunError {
		t.Errorf("a stall is also a RunError, FFmpeg's word on the bytes: %v", runError)
	}
	if arguments := fake.OnlyCall(t).Arguments; !strings.Contains(strings.Join(arguments, " "), "-progress pipe:3") {
		t.Errorf("a watched run ran with %q; want its progress on descriptor 3", arguments)
	}
	assertGone(t, fake.HungPID(t))
}

func TestRunWatched_AStalledRunThatIgnoresItsQuitIsKilledWithItsGroup(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilKilled})
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	runner, ticks := watchedRunner(t, fake, clockSource)
	runner.quitGrace, runner.termGrace = 10*time.Millisecond, 10*time.Millisecond

	err := runUntilStalled(t, runner, ticks, clockSource)
	if stalled, isStalled := errors.AsType[*StalledError](err); !isStalled || stalled.Cause != StallStoodStill {
		t.Fatalf("a run that ignored SIGINT and SIGTERM ended %v; want stalled", err)
	}
	assertGone(t, fake.HungPID(t))
}

// The words after -- that make this binary the node in TestRun_AToolDiesWithTheNode.
const actAsNode = "act-as-node"

// TestToolDeathHelper is the helper: as the node it starts a hanging fake from the directory it is
// given, and exits once the fake is hanging, without stopping it. Run as a plain test it does nothing.
func TestToolDeathHelper(t *testing.T) {
	words := flag.Args()
	if len(words) != 2 || words[0] != actAsNode {
		return
	}
	runner, err := NewRunner(words[1])
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = runner.Run(t.Context(), FFmpeg, confine.Access{}, nil, 1) }() //nolint:errcheck // it never ends
	for {
		if _, err := os.Stat(filepath.Join(words[1], "hung.pid")); err == nil {
			os.Exit(0) //nolint:forbidigo,revive // the node dying at once, with its tool running, is the test
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRun_AToolDiesWithTheNode(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Hang: testutil.HangUntilKilled})
	output, err := testutil.RunSelf(t, "-test.run=^TestToolDeathHelper$", "--", actAsNode, fake.Directory)
	if err != nil {
		t.Fatalf("the node: %v\n%s", err, output)
	}
	pid := fake.HungPID(t)
	// The kernel kills it as the node dies; give it a moment to be reaped by init.
	for range 1000 {
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err != nil {
			return
		}
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err == nil && processState(string(stat)) == "Z" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("the tool %d outlived the node that started it", pid)
}

// stateOf is the state letter /proc gives process pid once it is wantStopped's, "T" stopped or any
// other, or as it is when a second has passed: a signal takes effect a moment after it is sent.
func stateOf(t *testing.T, pid int, wantStopped bool) string {
	t.Helper()
	var state string
	for range 100 {
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		if state = processState(string(stat)); (state == "T") == wantStopped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return state
}

// A paused run is stopped where it stands, its process group sent SIGSTOP, and neither stalls nor
// spends its deadline however long it is paused; continued, it is judged again from where it
// stood, its paused time not counted (D240 §6).
func TestRunWatched_APausedRunNeitherStallsNorSpendsItsDeadlineAndIsJudgedAgainOnceContinued(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilQuit})
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	runner, ticks := watchedRunner(t, fake, clockSource)
	runner.quitGrace, runner.termGrace = time.Hour, time.Hour
	reported := make(chan struct{}, 1)
	watch := testWatch
	watch.OnProgress = func(Progress) {
		select {
		case reported <- struct{}{}:
		default:
		}
	}
	pausing := &Pausing{}
	ended := make(chan error, 1)
	go func() {
		_, err := runner.RunWatched(WithPausing(t.Context(), pausing),
			WatchedRun{Arguments: []string{"-i", "x"}, Watch: watch})
		ended <- err
	}()
	testutil.Receive(t, reported, "the watch's report")
	pausing.Pause()
	pausing.mutex.Lock()
	pid := pausing.process.Pid
	pausing.mutex.Unlock()
	if state := stateOf(t, pid, true); state != "T" {
		t.Errorf("the paused run's process is in state %q, want stopped (T)", state)
	}
	clockSource.Advance(testWatch.Deadline + testWatch.StallWindow)
	ticks <- clockSource.Now()
	select {
	case err := <-ended:
		t.Fatalf("a paused run ended %v; want it left paused, however long", err)
	case <-time.After(100 * time.Millisecond):
	}
	pausing.Continue()
	if state := stateOf(t, pid, false); state == "T" {
		t.Error("the continued run's process is still stopped")
	}
	clockSource.Advance(testWatch.StallWindow)
	ticks <- clockSource.Now()
	err := testutil.Receive(t, ended, "the continued run's end")
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || stalled.Cause != StallStoodStill || stalled.WithoutAdvance != testWatch.StallWindow {
		t.Errorf("the continued run, still for its window, ended %v; want stood still for the window alone", err)
	}
}

// A stalled run's condition is read from /proc for its whole process group: here a sample that
// says a process waits on a device.
func TestRunWatched_AStalledRunSaysWhatItsGroupWasDoing(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilQuit})
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	runner, ticks := watchedRunner(t, fake, clockSource)
	sampled := make(chan int, 1)
	runner.sampleGroup = func(pgid int) groupSample {
		select {
		case sampled <- pgid:
		default:
		}
		return groupSample{processCount: 1, isWaitingOnDevice: true}
	}

	err := runUntilStalled(t, runner, ticks, clockSource)
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || stalled.Condition != ConditionWaitingOnDisk || stalled.IsLeftRunning {
		t.Fatalf("a run whose group waited on a device ended %v; want stalled on the disk, not left running", err)
	}
	if pgid := testutil.Receive(t, sampled, "the group's sample"); pgid != fake.HungPID(t) {
		t.Errorf("sampled process group %d, want the run's, %d", pgid, fake.HungPID(t))
	}
}

// A process waiting on a device outlives SIGKILL until the device answers. Its run is answered as
// stalled on the disk once the kill's grace has passed, so its slot is freed, and it is listed as
// stuck until it ends (D233 §10). Here the signals are not sent, which is how the kernel treats
// them.
func TestRunWatched_ARunThatOutlivesItsKillIsAnsweredAndListedAsStuckUntilItEnds(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Progress: hangingProgress, Hang: testutil.HangUntilKilled})
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	runner, ticks := watchedRunner(t, fake, clockSource)
	runner.quitGrace, runner.termGrace, runner.killGrace = 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond
	runner.signalRun = func(*os.Process, syscall.Signal) error { return nil }

	err := runUntilStalled(t, runner, ticks, clockSource)
	stalled, isStalled := errors.AsType[*StalledError](err)
	if !isStalled || !stalled.IsLeftRunning || stalled.Condition != ConditionWaitingOnDisk {
		t.Fatalf("a run that outlived its kill ended %v; want stalled on the disk and left running", err)
	}
	pid := fake.HungPID(t)
	if stuck := runner.Stuck(); len(stuck) != 1 || stuck[0].PID != pid || stuck[0].Tool != FFmpeg {
		t.Fatalf("stuck: %+v; want the run's process, %d", stuck, pid)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	for range 1000 {
		if len(runner.Stuck()) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("a stuck process that ended is still listed: %+v", runner.Stuck())
}

// A cancelled run whose process outlives its kill is answered as cancelled once the kill's grace
// has passed, and listed as stuck until it ends: a cancel never waits on a dead disk.
func TestRun_ACancelledRunThatOutlivesItsKillIsAnsweredAndListedAsStuck(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Hang: testutil.HangUntilKilled})
	runner := newRunner(t, fake)
	runner.killGrace = 10 * time.Millisecond
	runner.signalRun = func(*os.Process, syscall.Signal) error { return nil }
	ctx, cancel := context.WithCancel(t.Context())
	ended := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, FFmpeg, confine.Access{Read: []string{"/media/films"}}, nil, 1)
		ended <- err
	}()
	for range 1000 {
		if recorded, err := os.ReadFile(filepath.Join(fake.Directory, "hung.pid")); err == nil &&
			strings.HasSuffix(string(recorded), "\n") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	pid := fake.HungPID(t)
	cancel()
	err := testutil.Receive(t, ended, "the cancelled run's end")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled run that outlived its kill ended %v; want cancelled", err)
	}
	if stuck := runner.Stuck(); len(stuck) != 1 || stuck[0].PID != pid || stuck[0].Reading[0] != "/media/films" {
		t.Fatalf("stuck: %+v; want the run's process, %d, reading /media/films", stuck, pid)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
}

// /proc/<pid>/stat's command name may hold spaces and parentheses; the fields after its last
// parenthesis are read.
func TestParseStat_ReadsTheStateGroupAndTimesAfterTheCommandName(t *testing.T) {
	t.Parallel()
	stat := "4242 (ff (mpeg) x) D 1 4240 4240 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 1 0 1000 0 0"
	got, isParsed := parseStat(stat)
	if want := (procStat{state: "D", groupID: 4240, cpuTicks: 300}); !isParsed || got != want {
		t.Errorf("parseStat(%q) = %+v, %t; want %+v", stat, got, isParsed, want)
	}
	if _, isParsed := parseStat("4242 (ffmpeg) R 1"); isParsed {
		t.Error("a stat cut short was parsed")
	}
}

// A process blocked on a device names the file its call waits on by the call's first argument, a
// file descriptor, which /proc links to the file; a call with no file behind it names none.
func TestBlockedPathFrom_IsTheFileTheBlockedCallsDescriptorNames(t *testing.T) {
	t.Parallel()
	links := map[int64]string{3: "/media/films/Wing.mkv", 4: "pipe:[123]"}
	linkOf := func(fd int64) (string, error) {
		if link, isLinked := links[fd]; isLinked {
			return link, nil
		}
		return "", errors.New("no such descriptor")
	}
	for call, want := range map[string]string{
		"0 0x3 0x7f00 0x1000 0x0 0x0 0x0 0x7ffd 0x7f00":  "/media/films/Wing.mkv",
		"17 0x4 0x7f00 0x1000 0x0 0x0 0x0 0x7ffd 0x7f00": "",
		"0 0x9 0x7f00 0x1000 0x0 0x0 0x0 0x7ffd 0x7f00":  "",
		"running":          "",
		"-1 0x7ffd 0x7f00": "",
		"":                 "",
	} {
		if got := blockedPathFrom(call, linkOf); got != want {
			t.Errorf("blockedPathFrom(%q) = %q, want %q", call, got, want)
		}
	}
}

// A run that ends because its disk failed a read, as a network share mounted to give up on a host
// that is gone says, is a stall waiting on its disk, not FFmpeg's word on the bytes; a run that fails
// otherwise is a RunError as before (D233 §10; staging, 2026-10-21).
func TestRunWatched_ARunItsDiskFailedIsAStallOnItsDisk(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		stderr      string
		isDiskStall bool
	}{
		{"[matroska @ 0x1] Error reading: Input/output error\n", true},
		{"/media/film.mkv: Stale file handle\n", true},
		{"[hevc @ 0x1] Invalid data found when processing input\n", false},
	} {
		fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stderr: check.stderr, ExitStatus: 1})
		run := WatchedRun{Arguments: []string{"-i", "x"}, Watch: testWatch}
		_, err := newRunner(t, fake).RunWatched(t.Context(), run)
		stalled, isStalled := errors.AsType[*StalledError](err)
		if isStalled != check.isDiskStall ||
			(isStalled && (stalled.Cause != StallDiskFailed || stalled.Condition != ConditionWaitingOnDisk)) {
			t.Errorf("a run that logged %q ended %v; want a stall on its disk: %t", check.stderr, err,
				check.isDiskStall)
		}
	}
}

// FFmpeg whose demuxer's read fails for its disk ends its input there and exits 0 with an answer made
// of part of the file, so the log decides: that run is a stall on its disk, even when the line is far
// from the end of the log, while a run whose only errors are frames it concealed has succeeded
// (ffmpeg-http-input-2026-10-22).
func TestRunWatched_ARunWhoseDemuxerCouldNotReadItsDiskFailsThoughItExitsZero(t *testing.T) {
	t.Parallel()
	summary := strings.Repeat("  Integrated loudness:\n    I:         -34.8 LUFS\n", 200)
	for _, check := range []struct {
		stderr      string
		isDiskStall bool
	}{
		{"[in#0/matroska,webm @ 0x1] Error during demuxing: Input/output error\n" + summary, true},
		{"[in#0/mov @ 0x1] Error during demuxing: Stale file handle\n", true},
		{"[h264 @ 0x1] error while decoding MB 10 49\n[h264 @ 0x1] concealing 2319 DC errors\n" + summary, false},
		{"[in#0/matroska,webm @ 0x1] Error during demuxing: Invalid data found when processing input\n", false},
	} {
		fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stderr: check.stderr})
		run := WatchedRun{Arguments: []string{"-i", "x"}, Watch: testWatch, LogLimitBytes: 512}
		_, err := newRunner(t, fake).RunWatched(t.Context(), run)
		stalled, isStalled := errors.AsType[*StalledError](err)
		if isStalled != check.isDiskStall || (!isStalled && err != nil) ||
			(isStalled && (stalled.Cause != StallDiskFailed || stalled.Condition != ConditionWaitingOnDisk)) {
			t.Errorf("a run that exited 0 having logged %q ended %v; want a stall on its disk: %t",
				check.stderr[:min(80, len(check.stderr))], err, check.isDiskStall)
		}
	}
}
