// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// testWatch is a watch with round numbers: a minute to start, two to stand still, an hour in all.
var testWatch = Watch{StartGrace: time.Minute, StallWindow: 2 * time.Minute, Deadline: time.Hour}

func TestWatchdog_OnlyAPositionThatAdvancesIsProgressNotLinesThatArrive(t *testing.T) {
	t.Parallel()
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	dog := newWatchdog(clockSource, testWatch)
	dog.observe(Progress{Position: 5 * time.Second, FrameCount: 120})
	clockSource.Advance(90 * time.Second)
	// FFmpeg keeps writing its progress block, with the position where it was.
	dog.observe(Progress{Position: 5 * time.Second, FrameCount: 120})
	if stalled := dog.verdict(FFmpeg); stalled != nil {
		t.Fatalf("stalled after 90 s of a two-minute window: %v", stalled)
	}
	clockSource.Advance(30 * time.Second)
	dog.observe(Progress{Position: 5 * time.Second, FrameCount: 120})
	stalled := dog.verdict(FFmpeg)
	if stalled == nil || stalled.Cause != StallStoodStill || stalled.LastPosition != 5*time.Second ||
		stalled.WithoutAdvance != 2*time.Minute {
		t.Fatalf("two minutes of blocks at 5 s: %+v; want stood still at 5 s for two minutes, the lines no progress",
			stalled)
	}

	moving := newWatchdog(clockSource, testWatch)
	for range 10 {
		clockSource.Advance(time.Minute)
		moving.observe(Progress{Position: moving.furthest.Position + time.Second})
	}
	if stalled := moving.verdict(FFmpeg); stalled != nil {
		t.Errorf("a run advancing each minute stalled: %v", stalled)
	}
	clockSource.Advance(time.Minute)
	// A frame count that grows is progress too, as a picture's run reports it.
	moving.observe(Progress{Position: moving.furthest.Position, FrameCount: 1})
	clockSource.Advance(90 * time.Second)
	if stalled := moving.verdict(FFmpeg); stalled != nil {
		t.Errorf("a run whose frame count grew 90 s ago stalled: %v", stalled)
	}
}

func TestWatchdog_ARunThatNeverMovesOrOutrunsItsDeadlineHasStalled(t *testing.T) {
	t.Parallel()
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	starting := newWatchdog(clockSource, testWatch)
	// Before FFmpeg has written anything, its position is N/A: read as zero, which is no movement.
	starting.observe(Progress{})
	clockSource.Advance(59 * time.Second)
	if stalled := starting.verdict(FFmpeg); stalled != nil {
		t.Fatalf("stalled within its start grace: %v", stalled)
	}
	clockSource.Advance(time.Second)
	if stalled := starting.verdict(FFmpeg); stalled == nil || stalled.Cause != StallNeverMoved {
		t.Errorf("a minute without moving: %+v; want never moved", stalled)
	}

	crawling := newWatchdog(clockSource, testWatch)
	for range 60 {
		clockSource.Advance(time.Minute)
		crawling.observe(Progress{Position: crawling.furthest.Position + time.Millisecond})
	}
	stalled := crawling.verdict(FFmpeg)
	if stalled == nil || stalled.Cause != StallPastDeadline || stalled.LastPosition != 60*time.Millisecond {
		t.Errorf("an hour of crawling: %+v; want past its deadline at 60 ms", stalled)
	}
	if !strings.Contains(stalled.Error(), "past_deadline") {
		t.Errorf("the stall says %q, not why", stalled.Error())
	}
}

func TestReadProgress_ReadsEachBlocksPositionFramesAndSpeed(t *testing.T) {
	t.Parallel()
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	var reported []Progress
	watch := testWatch
	watch.OnProgress = func(progress Progress) { reported = append(reported, progress) }
	dog := newWatchdog(clockSource, watch)
	dog.readProgress(strings.NewReader("frame=0\nout_time_us=N/A\nspeed=N/A\nprogress=continue\n" +
		"frame=48\nout_time_us=2000000\nspeed=1.14e+03x\nprogress=continue\n" +
		"frame=96\nout_time_us=4000000\nspeed=N/A\nprogress=end\n"))
	want := []Progress{
		{},
		{Position: 2 * time.Second, FrameCount: 48, RealtimeFactor: 1140},
		{Position: 4 * time.Second, FrameCount: 96, RealtimeFactor: 1140},
	}
	if len(reported) != len(want) {
		t.Fatalf("reported %+v, want %+v", reported, want)
	}
	for index := range want {
		if reported[index] != want[index] {
			t.Errorf("block %d: %+v, want %+v", index, reported[index], want[index])
		}
	}
	if dog.furthest.Position != 4*time.Second || !dog.hasMoved {
		t.Errorf("the watchdog saw %+v, want it at 4 s", dog.furthest)
	}
}

// sparseFile is a file of sizeBytes that takes no disk: a byte written at its end.
func sparseFile(t *testing.T, sizeBytes int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteAt([]byte{0}, sizeBytes-1)
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	return path
}

func TestReadingDeadline_IsReadingTheFileAtTheSlowestReadTheNodeIsConfiguredFor(t *testing.T) {
	t.Parallel()
	film := sparseFile(t, 60<<30)
	for _, check := range []struct {
		bytesPerSecond int64
		want           time.Duration
	}{
		{20 << 20, DeadlineFor(3073 * time.Second)},
		{40 << 20, DeadlineFor(1537 * time.Second)},
		{0, DeadlineFor(0)}, // a rate not known is an unknown length
	} {
		if got := ReadingDeadline(film, check.bytesPerSecond); got != check.want {
			t.Errorf("60 GiB at %d bytes a second: %s, want %s", check.bytesPerSecond, got, check.want)
		}
	}
}

func TestDeadlineFor_IsThreeTimesTheExpectedLengthAtLeastTenMinutesAndADayWhenUnknown(t *testing.T) {
	t.Parallel()
	for _, step := range []struct{ expected, want time.Duration }{
		{2 * time.Hour, 6 * time.Hour},
		{time.Minute, 10 * time.Minute},
		{0, 24 * time.Hour},
	} {
		if got := DeadlineFor(step.expected); got != step.want {
			t.Errorf("DeadlineFor(%s) = %s, want %s", step.expected, got, step.want)
		}
	}
}

// A stall says what the run's group was doing over the time it did not move, by /proc: waiting on
// a device when a process was in uninterruptible sleep, spinning when it kept a processor busy,
// stuck otherwise (D233 §5).
func TestWatchdog_AStallSaysWhetherItWaitedOnTheDiskSpunOrWasStuck(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		name  string
		later groupSample
		want  StallCondition
	}{
		{
			"a process in D",
			groupSample{
				processCount: 1, isWaitingOnDevice: true, cpuTicks: 100, waitingOnPath: "/cache/prepared/a.m4s",
			},
			ConditionWaitingOnDisk,
		},
		// Two minutes without moving, 100 s of processor time used: a loop.
		{"a busy processor", groupSample{processCount: 1, cpuTicks: 100 + 100*clockTicksPerSecond}, ConditionSpinning},
		// A second of processor time in two minutes: waiting on something that never came.
		{"an idle group", groupSample{processCount: 1, cpuTicks: 100 + clockTicksPerSecond}, ConditionStuck},
	} {
		clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
		dog := newWatchdog(clockSource, testWatch)
		dog.observeGroup(groupSample{processCount: 1, cpuTicks: 100}, 0)
		dog.observe(Progress{Position: time.Second})
		clockSource.Advance(testWatch.StallWindow)
		dog.observeGroup(check.later, 0)
		stalled := dog.verdict(FFmpeg)
		if stalled == nil || stalled.Condition != check.want {
			t.Errorf("%s: %+v; want stalled, %s", check.name, stalled, check.want)
		}
		// A run waiting on its disk says which file it was blocked on, whose disk did not answer.
		if stalled != nil && stalled.WaitingOnPath != check.later.waitingOnPath {
			t.Errorf("%s: waiting on %q, want %q", check.name, stalled.WaitingOnPath, check.later.waitingOnPath)
		}
	}
}

// Bytes read from or written to storage, or written to stdout, are movement though the position
// stands: a run reading through a file to its next frame, or writing a track whose position moves
// only when a subtitle comes. What the group did before its first sample is its start, not
// movement.
func TestWatchdog_BytesReadOrWrittenAreMovementThoughThePositionStands(t *testing.T) {
	t.Parallel()
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	reading := newWatchdog(clockSource, testWatch)
	reading.observe(Progress{Position: time.Second})
	reading.observeGroup(groupSample{processCount: 1, storageBytes: 1 << 20}, 0)
	for range 3 {
		clockSource.Advance(testWatch.StallWindow - time.Second)
		reading.observeGroup(groupSample{processCount: 1, storageBytes: reading.furthestStorageBytes + 1<<20}, 0)
	}
	if stalled := reading.verdict(FFmpeg); stalled != nil {
		t.Errorf("a run reading from storage stalled: %v", stalled)
	}

	writing := newWatchdog(clockSource, testWatch)
	writing.observe(Progress{Position: time.Second})
	writing.observeGroup(groupSample{processCount: 1}, 0)
	for written := range int64(3) {
		clockSource.Advance(testWatch.StallWindow - time.Second)
		writing.observeGroup(groupSample{processCount: 1}, (written+1)*100)
	}
	if stalled := writing.verdict(FFmpeg); stalled != nil {
		t.Errorf("a run writing to stdout stalled: %v", stalled)
	}

	starting := newWatchdog(clockSource, testWatch)
	starting.observeGroup(groupSample{processCount: 1, storageBytes: 1 << 30}, 0)
	clockSource.Advance(testWatch.StartGrace)
	starting.observeGroup(groupSample{processCount: 1, storageBytes: 1 << 30}, 0)
	if stalled := starting.verdict(FFmpeg); stalled == nil || stalled.Cause != StallNeverMoved {
		t.Errorf("a run that read only before its first sample: %+v; want never moved", stalled)
	}
}

// Each movement of a watched run, its position's or its bytes', is told to the movement hook its
// context carries, by which the node's work list counts how long a job has not moved.
func TestWatchdog_TellsTheMovementHookOfEachMovement(t *testing.T) {
	t.Parallel()
	clockSource := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	dog := newWatchdog(clockSource, testWatch)
	moves := 0
	dog.moved = func() { moves++ }
	dog.observe(Progress{Position: time.Second})
	dog.observe(Progress{Position: time.Second}) // no movement
	dog.observeGroup(groupSample{processCount: 1, storageBytes: 10}, 0)
	dog.observeGroup(groupSample{processCount: 1, storageBytes: 20}, 0)
	if moves != 2 {
		t.Errorf("the hook was told of %d movements, want 2: the position's, then the bytes'", moves)
	}
	ctx := WithMoved(t.Context(), func() { moves++ })
	Moved(ctx)
	Moved(t.Context()) // no hook: nothing
	if moves != 3 {
		t.Errorf("Moved told the hook %d times in all, want 3", moves)
	}
}
