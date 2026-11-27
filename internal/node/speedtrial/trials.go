// SPDX-License-Identifier: AGPL-3.0-or-later

package speedtrial

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/chunks"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Runner runs FFmpeg: execx.Runner, or a test's fake.
type Runner interface {
	Run(ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, stdoutLimitBytes int,
	) ([]byte, error)
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// The watch on each FFmpeg run: the start an idle run may take behind a busy processor, and a whole
// run's bound, a 2160p clip made by x265 on the slowest node in use under the idle class.
const (
	runStartGrace  = 2 * time.Minute
	runStallWindow = 2 * time.Minute
	clipDeadline   = 30 * time.Minute
	trialDeadline  = 15 * time.Minute
	// The most ffmpeg -encoders may write: about twenty kilobytes in practice.
	encodersOutputBytes = 256 << 10
	// The most of the FFmpeg's words a row carries.
	failureBytes = 400
	// The suffix of the stamp that says the clip it follows is whole.
	clipStampSuffix = ".whole"
)

// Config is what Trials need.
type Config struct {
	Runner  Runner
	Encoder encode.Encoder
	Key     Key
	// ClipsDir keeps the clips, which can be made again: the node's cache.
	ClipsDir string
	// KeptDir keeps the results, which must outlive a restart: the node's persistent directory.
	KeptDir string
	Clock   clock.Clock
	Logger  *slog.Logger
	// Kinds are what is tried; nil is Kinds().
	Kinds []Kind
}

// Trials are a node's start-up speed trials (doc.go).
type Trials struct {
	config   Config
	yielding *yielding
	started  sync.Once
	// running is the background run Start made, and stop ends it (Stop).
	running sync.WaitGroup
	stop    context.CancelFunc

	mutex   sync.Mutex
	results []nodeproto.SpeedTrial
}

// New is the trials config says. Key's TrialRecipe is set here.
func New(config Config) *Trials {
	if config.Kinds == nil {
		config.Kinds = Kinds()
	}
	config.Key.TrialRecipe = trialRecipe
	return &Trials{config: config, yielding: newYielding(config.Clock)}
}

// Start runs the trials once, in the background, until they end, ctx does or Stop is called.
func (trials *Trials) Start(ctx context.Context) {
	trials.started.Do(func() {
		runCtx, stop := context.WithCancel(ctx)
		trials.mutex.Lock()
		trials.stop = stop
		trials.mutex.Unlock()
		trials.running.Go(func() {
			defer stop()
			if err := trials.Run(runCtx); err != nil && runCtx.Err() == nil {
				trials.config.Logger.WarnContext(runCtx, "the speed trials stopped",
					slogx.EventKey, "speedTrialsFailed", "why", err.Error())
			}
		})
	})
}

// Stop ends the trials Start began and waits until they have: a node that ends leaves no trial's
// FFmpeg running on, writing where the node's directories were.
func (trials *Trials) Stop() {
	trials.started.Do(func() {}) // none starts after
	trials.mutex.Lock()
	stop := trials.stop
	trials.mutex.Unlock()
	if stop != nil {
		stop()
	}
	trials.running.Wait()
}

// ChunksRunStarts says a run of chunks has started; the func it returns says it has ended. No trial
// goes while one does, and the one going when it starts is stopped at once and made again after.
func (trials *Trials) ChunksRunStarts() (ends func()) {
	return trials.yielding.runStarts()
}

// Results are every trial, once all have ended; nil until then.
func (trials *Trials) Results() []nodeproto.SpeedTrial {
	trials.mutex.Lock()
	defer trials.mutex.Unlock()
	return slices.Clone(trials.results)
}

// Run runs every trial not already kept under the key, keeping each as it ends, one at a time, each
// out of a run of chunks' way, and then publishes them all. A restart with the same key runs none.
func (trials *Trials) Run(ctx context.Context) error {
	started := trials.config.Clock.Now()
	results, err := readKept(trials.config.KeptDir, trials.config.Key)
	if err != nil {
		return err
	}
	if !results.IsComplete {
		encoders, err := trials.encoders(ctx)
		if err != nil {
			return err
		}
		for _, kind := range trials.config.Kinds {
			if err := trials.runKind(ctx, kind, encoders, &results); err != nil {
				return err
			}
		}
		results.IsComplete = true
		if err := writeKept(trials.config.KeptDir, results); err != nil {
			return err
		}
		trials.config.Logger.InfoContext(ctx, "the speed trials ended",
			slogx.EventKey, "speedTrialsDone", "trials", len(results.Trials),
			"took", trials.config.Clock.Now().Sub(started).Round(time.Second).String())
	}
	trials.mutex.Lock()
	trials.results = results.Trials
	trials.mutex.Unlock()
	return nil
}

// runKind runs kind's trials not already in results, keeping each in them as it ends.
func (trials *Trials) runKind(ctx context.Context, kind Kind, encoders map[string]bool, results *kept) error {
	for _, quality := range kind.qualities() {
		row := nodeproto.SpeedTrial{
			Codec: kind.Codec, BitDepth: kind.BitDepth, Height: kind.Height,
			MadeKbps: quality.kbps, MadeHeight: quality.height,
		}
		if slices.ContainsFunc(results.Trials, func(done nodeproto.SpeedTrial) bool { return isSameTrial(done, row) }) {
			continue
		}
		if !encoders[kind.Encoder] {
			row.NotTried = "this FFmpeg has no " + kind.Encoder + " encoder to make the clip with"
		} else if err := trials.step(ctx, func(stepCtx context.Context) error {
			made, err := trials.try(stepCtx, kind, quality, row)
			row = made
			return err
		}); err != nil {
			return err
		}
		trials.config.Logger.DebugContext(ctx, "a speed trial ended", slogx.EventKey, "speedTrial",
			"codec", row.Codec, "bitDepth", row.BitDepth, "height", row.Height, "madeKbps", row.MadeKbps,
			"madeHeight", row.MadeHeight, "speedPercent", row.SpeedPercent, "decodedOn", row.DecodedOn,
			"gpuDecodeFailure", row.GPUDecodeFailure, "notTried", row.NotTried, "failure", row.Failure)
		results.Trials = append(results.Trials, row)
		if err := writeKept(trials.config.KeptDir, *results); err != nil {
			return err
		}
	}
	return nil
}

// step runs do when no run of chunks is in its way, and again when one stopped it.
func (trials *Trials) step(ctx context.Context, do func(stepCtx context.Context) error) error {
	for {
		stepCtx, ends, err := trials.yielding.awaitClear(ctx)
		if err != nil {
			return err
		}
		err = do(stepCtx)
		isStopped := isStoppedByRun(stepCtx)
		ends()
		if !isStopped {
			return err
		}
	}
}

// try makes kind's clip when it is not already made, and the clip into quality, decoded on the card
// first where a run of chunks would be and on the CPU where that failed, as chunks.Maker.Make does. It
// fails only when ctx ends; FFmpeg's failure is the row's.
func (trials *Trials) try(ctx context.Context, kind Kind, quality quality, row nodeproto.SpeedTrial,
) (nodeproto.SpeedTrial, error) {
	clip, err := trials.clip(ctx, kind)
	if err != nil {
		if ctx.Err() != nil {
			return row, ctx.Err()
		}
		row.Failure = "make the clip: " + words(err)
		return row, nil
	}
	decoding := encode.DecodedOnCPU
	if trials.config.Encoder.DecodesOnGPU(kind.Codec) {
		decoding = encode.DecodedOnGPU
	}
	speed, err := trials.measure(ctx, clip, quality, decoding)
	if decoding == encode.DecodedOnGPU && chunks.IsDecodeFailure(ctx, err) {
		row.GPUDecodeFailure = words(err)
		decoding = encode.DecodedOnCPU
		speed, err = trials.measure(ctx, clip, quality, decoding)
	}
	if ctx.Err() != nil {
		return row, ctx.Err()
	}
	if err != nil {
		row.Failure = words(err)
		return row, nil
	}
	row.SpeedPercent, row.DecodedOn = speed, string(decoding)
	return row, nil
}

// measure makes clip into quality once, decoded as decoding says, with exactly the arguments a run of
// chunks makes that quality with (chunks.TrialArguments), written to nothing, in the idle class, and
// is how fast: the clip's length over the run's whole time by the node's clock, in percent of real
// time. It is the whole run's, not FFmpeg's own last speed: that word is the encode's alone, from
// its first picture on, and leaves out the start a run of chunks pays too (opening the card, the
// decoder and encoder starting), which on a short run is much of the whole; and a run stopped early
// leaves a speed that is not the run's.
func (trials *Trials) measure(ctx context.Context, clip string, quality quality, decoding encode.Decoding,
) (int, error) {
	encoder := trials.config.Encoder
	started := trials.config.Clock.Now()
	_, err := trials.config.Runner.RunWatched(ctx, execx.WatchedRun{
		Access: confine.Access{Read: []string{clip}, UsesGPU: encoder.UsesGPU()},
		Arguments: chunks.TrialArguments(execx.FileInput(clip), quality.kbps, quality.height, clipKeyframesAt,
			encoder, decoding),
		Watch:  execx.Watch{StartGrace: runStartGrace, StallWindow: runStallWindow, Deadline: trialDeadline},
		IsIdle: true,
	})
	if err != nil {
		return 0, err
	}
	took := max(trials.config.Clock.Now().Sub(started), time.Millisecond)
	return int(clipSeconds * time.Second * 100 / took), nil
}

// clip is the path of kind's clip, made when it is not already. A clip is whole only once a stamp
// beside it says so, written after its making ended; a stopped making leaves no stamp, and the next
// making writes over what it left (the node renames and removes nothing of its own here, D144).
func (trials *Trials) clip(ctx context.Context, kind Kind) (string, error) {
	path := filepath.Join(trials.config.ClipsDir, kind.name())
	stamp := path + clipStampSuffix
	if _, err := os.Stat(stamp); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(trials.config.ClipsDir, 0o700); err != nil {
		return "", fmt.Errorf("make the clips' directory: %w", err)
	}
	_, err := trials.config.Runner.RunWatched(ctx, execx.WatchedRun{
		Access:    confine.Access{Write: []string{trials.config.ClipsDir}},
		Arguments: kind.clipArguments(path),
		Watch:     execx.Watch{StartGrace: runStartGrace, StallWindow: runStallWindow, Deadline: clipDeadline},
		IsIdle:    true,
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(stamp, nil, keptMode); err != nil {
		return "", fmt.Errorf("stamp the clip whole: %w", err)
	}
	return path, nil
}

// encoders are the encoders this FFmpeg has, by name, from ffmpeg -encoders.
func (trials *Trials) encoders(ctx context.Context) (map[string]bool, error) {
	output, err := trials.config.Runner.Run(ctx, execx.FFmpeg, confine.Access{}, []string{"-encoders"},
		encodersOutputBytes)
	if err != nil {
		return nil, fmt.Errorf("list FFmpeg's encoders: %w", err)
	}
	return listedEncoders(string(output)), nil
}

// listedEncoders are the names ffmpeg -encoders lists: after the legend's "------" line, one an
// encoder, its flags then its name.
func listedEncoders(output string) map[string]bool {
	names := map[string]bool{}
	isListed := false
	for line := range strings.Lines(output) {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 1 && strings.HasPrefix(fields[0], "---"):
			isListed = true
		case isListed && len(fields) >= 2:
			names[fields[1]] = true
		}
	}
	return names
}

// words are err's FFmpeg's own words where it has them, its log's end, bounded.
func words(err error) string {
	said := err.Error()
	var failure *execx.RunError
	if errors.As(err, &failure) && strings.TrimSpace(failure.StderrTail) != "" {
		said = strings.TrimSpace(failure.StderrTail)
	}
	if len(said) > failureBytes {
		said = said[len(said)-failureBytes:]
	}
	return said
}

// isSameTrial is whether done is a trial of the same clip into the same quality as row.
func isSameTrial(done, row nodeproto.SpeedTrial) bool {
	return done.Codec == row.Codec && done.BitDepth == row.BitDepth && done.Height == row.Height &&
		done.MadeKbps == row.MadeKbps && done.MadeHeight == row.MadeHeight
}
