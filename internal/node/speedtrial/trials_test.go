// SPDX-License-Identifier: AGPL-3.0-or-later

package speedtrial

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// allEncoders is ffmpeg -encoders' output naming every encoder a clip is made with.
const allEncoders = ` V..... = Video
 ------
 V....D libx264              H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D libx265              H.265 / HEVC (codec hevc)
 V....D libvpx-vp9           libvpx VP9 (codec vp9)
 V....D libsvtav1            SVT-AV1(Scalable Video Technology for AV1) encoder (codec av1)
 V.S..D mpeg2video           MPEG-2 video
 V....D mpeg4                MPEG-4 part 2
`

// fakeFFmpeg stands for FFmpeg: it lists encoders, makes a clip as an empty file, and takes each
// trial a second of the fake clock, failing one decoding on the card when told to.
type fakeFFmpeg struct {
	clock    *clocktest.Fake
	encoders string
	// failsOnGPU makes every run decoding on the card fail as FFmpeg does.
	failsOnGPU bool
	// started, when set, is told of each trial as it starts, and the trial then waits for its
	// context to end.
	started chan []string

	mutex  sync.Mutex
	clips  []string
	trials [][]string
	// stopped counts the trials that ended with their context.
	stopped int
}

func (fake *fakeFFmpeg) Run(context.Context, execx.Tool, confine.Access, []string, int) ([]byte, error) {
	return []byte(fake.encoders), nil
}

func (fake *fakeFFmpeg) RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error) {
	if !run.IsIdle {
		panic("a speed trial's run is not in the idle class")
	}
	arguments := run.Arguments
	if slices.Contains(arguments, "lavfi") {
		fake.mutex.Lock()
		fake.clips = append(fake.clips, arguments[len(arguments)-1])
		fake.mutex.Unlock()
		return execx.Ran{}, os.WriteFile(arguments[len(arguments)-1], nil, 0o600)
	}
	fake.mutex.Lock()
	fake.trials = append(fake.trials, arguments)
	fake.mutex.Unlock()
	if fake.started != nil {
		fake.started <- arguments
		<-ctx.Done()
		fake.mutex.Lock()
		fake.stopped++
		fake.mutex.Unlock()
		return execx.Ran{}, ctx.Err()
	}
	if fake.failsOnGPU && slices.Contains(arguments, "-hwaccel") {
		return execx.Ran{}, &execx.RunError{Tool: execx.FFmpeg, ExitStatus: 218, StderrTail: "No support for codec"}
	}
	fake.clock.Advance(4 * time.Second)
	return execx.Ran{}, nil
}

func (fake *fakeFFmpeg) trialRuns() [][]string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return slices.Clone(fake.trials)
}

func newFake() *fakeFFmpeg {
	return &fakeFFmpeg{clock: clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)), encoders: allEncoders}
}

func testKey() Key {
	return Key{FFmpegVersion: "8.0", FFmpegSha256: "aa", Encoder: encode.EncoderX264, QualityRecipe: 3}
}

func newTrials(t *testing.T, fake *fakeFFmpeg, kept string, key Key, encoder encode.Encoder, log *bytes.Buffer,
) *Trials {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	if log != nil {
		logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return New(Config{
		Runner: fake, Encoder: encoder, Key: key, ClipsDir: t.TempDir(), KeptDir: kept,
		Clock: fake.clock, Logger: logger,
	})
}

// Each kind is made into a clip once and tried once at its top quality and once at the lowest, with
// the speed the whole run took, and the trials are published only when all have ended.
func TestRun_TriesEachKindOnceAtItsTopAndLowestQuality(t *testing.T) {
	t.Parallel()
	fake := newFake()
	var log bytes.Buffer
	trials := newTrials(t, fake, t.TempDir(), testKey(), encode.EncoderX264, &log)
	if got := trials.Results(); got != nil {
		t.Fatalf("results before the trials ran are %v; want none", got)
	}
	if err := trials.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(fake.clips) != len(Kinds()) {
		t.Errorf("made %d clips; want one a kind, %d", len(fake.clips), len(Kinds()))
	}
	results := trials.Results()
	if len(results) != 2*len(Kinds()) || len(fake.trialRuns()) != 2*len(Kinds()) {
		t.Fatalf("%d results of %d runs; want two a kind, %d", len(results), len(fake.trialRuns()), 2*len(Kinds()))
	}
	for index, kind := range Kinds() {
		top, lowest := results[2*index], results[2*index+1]
		want := nodeproto.SpeedTrial{
			Codec: kind.Codec, BitDepth: kind.BitDepth, Height: kind.Height, MadeKbps: 8000, MadeHeight: kind.Height,
			SpeedPercent: 300, DecodedOn: "cpu",
		}
		if top != want {
			t.Errorf("kind %d's top trial is %+v; want %+v", index, top, want)
		}
		want.MadeKbps, want.MadeHeight = 400, 360
		if lowest != want {
			t.Errorf("kind %d's lowest trial is %+v; want %+v", index, lowest, want)
		}
	}
	if fake.trialRuns()[0][len(fake.trialRuns()[0])-2] != "null" ||
		!slices.Contains(fake.trialRuns()[0], "-force_key_frames") {
		t.Errorf("a trial's arguments are %q; want a run of chunks' quality, written to nothing", fake.trialRuns()[0])
	}
	if strings.Count(log.String(), "logEvent=speedTrial ") != 2*len(Kinds()) ||
		strings.Count(log.String(), "logEvent=speedTrialsDone") != 1 {
		t.Errorf("the log is %s; want a line a trial and one when all ended", log.String())
	}
}

// A kind whose encoder this FFmpeg lacks is reported not tried, not as a failure, and runs nothing.
func TestRun_AKindWithoutItsEncoderIsNotTried(t *testing.T) {
	t.Parallel()
	fake := newFake()
	fake.encoders = strings.ReplaceAll(allEncoders, "libsvtav1", "libaom-av1")
	trials := newTrials(t, fake, t.TempDir(), testKey(), encode.EncoderX264, nil)
	if err := trials.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	notTried := 0
	for _, row := range trials.Results() {
		if row.Codec == "av1" {
			if row.NotTried == "" || row.Failure != "" || row.SpeedPercent != 0 {
				t.Errorf("the AV1 row is %+v; want not tried, and no failure", row)
			}
			notTried++
		} else if row.NotTried != "" {
			t.Errorf("row %+v is not tried; want tried", row)
		}
	}
	if notTried != 2 || len(fake.trialRuns()) != 2*(len(Kinds())-1) {
		t.Errorf("%d AV1 rows not tried, %d runs; want 2, and none for AV1", notTried, len(fake.trialRuns()))
	}
}

// On a VAAPI node a kind the card decodes is tried decoded on it first, and where that fails as
// FFmpeg's own failure, made again on the CPU and reported so, with FFmpeg's words.
func TestRun_ACardsDecodeFailureFallsToTheCPUAndIsReported(t *testing.T) {
	t.Parallel()
	fake := newFake()
	fake.failsOnGPU = true
	key := testKey()
	key.Encoder = encode.EncoderVAAPI
	trials := newTrials(t, fake, t.TempDir(), key, encode.EncoderVAAPI, nil)
	if err := trials.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	row := trials.Results()[0]
	if row.DecodedOn != "cpu" || row.GPUDecodeFailure != "No support for codec" || row.SpeedPercent != 300 {
		t.Errorf("the first trial is %+v; want made on the CPU after the card's failure, which it names", row)
	}
	if runs := fake.trialRuns(); !slices.Contains(runs[0], "-hwaccel") || slices.Contains(runs[1], "-hwaccel") {
		t.Errorf("the first two runs are %q; want the card's, then the CPU's", runs[:2])
	}
}

// A restart under the same key runs nothing and publishes what was kept; a changed key runs every
// trial again.
func TestRun_KeptResultsRunNothingUntilTheKeyChanges(t *testing.T) {
	t.Parallel()
	kept := t.TempDir()
	first := newFake()
	if err := newTrials(t, first, kept, testKey(), encode.EncoderX264, nil).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	again := newFake()
	restarted := newTrials(t, again, kept, testKey(), encode.EncoderX264, nil)
	if err := restarted.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(again.trialRuns()) != 0 || len(again.clips) != 0 || len(restarted.Results()) != 2*len(Kinds()) {
		t.Errorf("a restart ran %d trials and published %d; want none run, all kept published",
			len(again.trialRuns()), len(restarted.Results()))
	}
	changed := testKey()
	changed.FFmpegVersion = "8.1"
	newer := newFake()
	if err := newTrials(t, newer, kept, changed, encode.EncoderX264, nil).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(newer.trialRuns()) != 2*len(Kinds()) {
		t.Errorf("a changed key ran %d trials; want every one, %d", len(newer.trialRuns()), 2*len(Kinds()))
	}
}

// No trial starts while a run of chunks goes, nor until it has been over for quietAfterRun; a run
// starting stops the trial going at once, and the trial is made again after.
func TestRun_NoTrialGoesWhileARunOfChunksDoes(t *testing.T) {
	t.Parallel()
	fake := newFake()
	fake.started = make(chan []string)
	trials := newTrials(t, fake, t.TempDir(), testKey(), encode.EncoderX264, nil)
	trials.config.Kinds = trials.config.Kinds[:1]
	ends := trials.ChunksRunStarts()
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan error, 1)
	go func() { ran <- trials.Run(ctx) }()
	select {
	case <-fake.started:
		t.Fatal("a trial started while a run of chunks went")
	case <-time.After(50 * time.Millisecond):
	}
	ends()
	if !fake.clock.AwaitArmed(t.Context(), 1) {
		t.Fatal("the trials do not wait out the quiet after the run")
	}
	fake.clock.Advance(quietAfterRun - time.Second)
	select {
	case <-fake.started:
		t.Fatal("a trial started before the quiet after the run had passed")
	case <-time.After(50 * time.Millisecond):
	}
	fake.clock.Advance(time.Second)
	first := testutil.Receive(t, fake.started, "the trial after the quiet")
	stopping := trials.ChunksRunStarts()
	if again := testutil.Receive(t, waitForTrialOrNone(fake.started), "a trial or none"); again != nil {
		t.Fatal("a trial started again while the run that stopped it went")
	}
	stopping()
	fake.clock.AwaitArmed(t.Context(), 1)
	fake.clock.Advance(quietAfterRun)
	if again := testutil.Receive(t, fake.started, "the stopped trial again"); !slices.Equal(again, first) {
		t.Errorf("after the run the trial made is %q; want the one it stopped, %q", again, first)
	}
	cancel()
	if err := testutil.Receive(t, ran, "the trials' end"); err == nil {
		t.Error("the trials stopped by their context say they ended")
	}
}

// waitForTrialOrNone is the next trial's arguments, or nil when none starts in a moment.
func waitForTrialOrNone(started chan []string) <-chan []string {
	result := make(chan []string, 1)
	go func() {
		select {
		case arguments := <-started:
			result <- arguments
		case <-time.After(50 * time.Millisecond):
			result <- nil
		}
	}()
	return result
}

// A node that ends stops the trial going and waits until its FFmpeg has ended: the node's tests
// found a trial writing on in a directory already being removed (the gate, 2026-11-27).
func TestStop_EndsTheTrialGoingAndWaitsForIt(t *testing.T) {
	t.Parallel()
	fake := newFake()
	fake.started = make(chan []string)
	trials := newTrials(t, fake, t.TempDir(), testKey(), encode.EncoderX264, nil)
	trials.config.Kinds = trials.config.Kinds[:1]
	trials.Start(t.Context())
	testutil.Receive(t, fake.started, "the trial to start")
	trials.Stop()
	fake.mutex.Lock()
	stopped := fake.stopped
	fake.mutex.Unlock()
	if stopped != 1 {
		t.Fatalf("Stop returned with %d trials ended; the one going is to have ended", stopped)
	}
}

func TestListedEncoders_AreTheNamesAfterTheLegend(t *testing.T) {
	t.Parallel()
	names := listedEncoders(allEncoders)
	if !names["libx264"] || !names["mpeg4"] || names["="] || names["Video"] || len(names) != 6 {
		t.Errorf("the names are %v; want the six encoders, not the legend's words", names)
	}
}
