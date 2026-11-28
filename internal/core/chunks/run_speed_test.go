// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// keyedTrials are a node's trials, with the key its speed is measured under as the test sets it.
type keyedTrials struct {
	triedAt
	key string
}

func (trials keyedTrials) SpeedKey(int64) string { return trials.key }

// ranRun is a run of a quality of file 1 on node 1, as its node reported it.
type ranRun struct {
	streamName   string
	speedPercent int
	filmMadeMs   int64
	wasBeside    bool
}

// fairRun is a run of a quality alone, of a minute of film, at speedPercent.
func fairRun(streamName string, speedPercent int) ranRun {
	return ranRun{streamName: streamName, speedPercent: speedPercent, filmMadeMs: 60_000}
}

// keepRuns keeps runs' reports as the core takes their ends.
func keepRuns(t *testing.T, service *Service, runs ...ranRun) {
	t.Helper()
	for _, run := range runs {
		stream, isStream := StreamNamed(run.streamName)
		if !isStream {
			t.Fatalf("%s is no stream", run.streamName)
		}
		command := nodeproto.MakeChunks{
			StoreDir: "1/chunks-" + run.streamName + "/0123456789abcdef",
			Stream: nodeproto.ChunkStream{
				Kind: stream.Kind, BitRateKbps: stream.Quality.Kbps, Height: stream.Quality.Height,
			},
		}
		err := service.takeEnd(t.Context(), jobs.Job{}, 1, command, nodeproto.MakeChunksReport{
			State: nodeproto.RunDone, LastChunk: 10, SpeedPercent: run.speedPercent, FilmMadeMs: run.filmMadeMs,
			DecodedOn: "cpu", WasBesideAnotherRun: run.wasBeside,
		})
		if err != nil {
			t.Fatalf("the end of a run of %s: %v", run.streamName, err)
		}
	}
}

// startOf is how file 1 starts on its ladder with no link remembered.
func startOf(t *testing.T, service *Service) Offer {
	t.Helper()
	offer, err := service.StartOnLadder(t.Context(), Playback{FileID: 1, HasStartingAudio: true}, 95_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

// threeAt400 are three fair runs of the top quality at 400 %.
var threeAt400 = []ranRun{fairRun("q4000k1080p", 400), fairRun("q4000k1080p", 400), fairRun("q4000k1080p", 400)}

// A node's fair runs of a kind of film and a quality, alone on it, of half a minute of film or more,
// take over from its trials once three are kept, by their middle one, the median, not their mean; a
// run beside another, or of less film, counts for nothing; a kind the trials never tried gets its own
// figure so; and a quality the runs have not taken over keeps the trials' reading
// (start-quality-by-node-speed.md §3). The trials here make 1080p at 200 % and 720p at 342 %: a start
// they judge alone is held to q3000k720p.
func TestStartOnLadder_ANodesFairRunsTakeOverFromItsTrials(t *testing.T) {
	t.Parallel()
	slowAtTheTop := kindTried("h264", 8, 1080, 200, 600)
	for _, start := range []struct {
		name         string
		trials       triedAt
		runs         []ranRun
		want         string
		speedPercent int
		from         SpeedFrom
	}{
		{name: "no runs", trials: slowAtTheTop, want: "q3000k720p", speedPercent: 342, from: SpeedFromTrial},
		{
			name: "three fair runs", trials: slowAtTheTop, runs: threeAt400, want: "q4000k1080p", speedPercent: 400,
			from: SpeedFromRuns,
		},
		{
			name: "two fair runs", trials: slowAtTheTop, runs: threeAt400[:2], want: "q3000k720p", speedPercent: 342,
			from: SpeedFromTrial,
		},
		{
			name: "one run beside another", trials: slowAtTheTop, want: "q3000k720p", speedPercent: 342,
			from: SpeedFromTrial, runs: append(threeAt400[:2:2],
				ranRun{streamName: "q4000k1080p", speedPercent: 400, filmMadeMs: 60_000, wasBeside: true}),
		},
		{
			name: "one run under half a minute", trials: slowAtTheTop, want: "q3000k720p", speedPercent: 342,
			from: SpeedFromTrial, runs: append(threeAt400[:2:2],
				ranRun{streamName: "q4000k1080p", speedPercent: 400, filmMadeMs: nodeproto.SpeedWindowFilmMs - 1}),
		},
		// Their mean, 470 %, would start the top; their median, 260 %, does not, and the trials'
		// reading of 720p stands.
		{
			name: "the median, not the mean", trials: slowAtTheTop, want: "q3000k720p", speedPercent: 342,
			from: SpeedFromTrial,
			runs: []ranRun{fairRun("q4000k1080p", 250), fairRun("q4000k1080p", 900), fairRun("q4000k1080p", 260)},
		},
		// The runs judge 1080p and 720p too slow, where the trials passed 720p; 540p, which no run
		// made, keeps the trials' reading between them.
		{
			name: "a quality between measured ones", trials: slowAtTheTop, want: "q2000k540p", speedPercent: 457,
			from: SpeedFromTrial, runs: []ranRun{
				fairRun("q4000k1080p", 900), fairRun("q4000k1080p", 250), fairRun("q4000k1080p", 260),
				fairRun("q3000k720p", 200), fairRun("q3000k720p", 200), fairRun("q3000k720p", 200),
				fairRun("q1200k480p", 300), fairRun("q1200k480p", 310), fairRun("q1200k480p", 990),
			},
		},
		// Only VP9 was tried, slowly: borrowed, its line would hold the start to 360p.
		{
			name: "a kind never tried", trials: kindTried("vp9", 8, 1080, 100, 200), runs: threeAt400,
			want: "q4000k1080p", speedPercent: 400, from: SpeedFromRuns,
		},
		{name: "no trials", runs: threeAt400, want: "q4000k1080p", speedPercent: 400, from: SpeedFromRuns},
	} {
		service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
		service.encoders = start.trials
		keepRuns(t, service, start.runs...)
		if offer := startOf(t, service); offer.StreamName != start.want ||
			offer.ExpectedSpeedPercent != start.speedPercent || offer.ExpectedSpeedFrom != start.from {
			t.Errorf("%s: the start is %s at %d %% from %q; want %s at %d %% from %q", start.name, offer.StreamName,
				offer.ExpectedSpeedPercent, offer.ExpectedSpeedFrom, start.want, start.speedPercent, start.from)
		}
	}
}

// Only the last runSpeedsKept fair runs of a kind and a quality are kept, and their middle one taken.
func TestKeepRunSpeed_KeepsOnlyTheLastRuns(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	service.encoders = triedAt(nil)
	keepRuns(t, service, fairRun("q4000k1080p", 100), fairRun("q4000k1080p", 100), fairRun("q4000k1080p", 100),
		fairRun("q4000k1080p", 900), fairRun("q4000k1080p", 900), fairRun("q4000k1080p", 900))
	rows, err := service.cache.Read().ListNodeRunSpeedsOfKind(t.Context(), cachegen.ListNodeRunSpeedsOfKindParams{
		NodeID: 1, SpeedKey: testSpeedKey, Codec: "h264", BitDepth: int64(nodeproto.ProbeVideo{}.BitDepth()),
		Height: 1080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != runSpeedsKept {
		t.Errorf("%d runs are kept; want the last %d", len(rows), runSpeedsKept)
	}
	if offer := startOf(t, service); offer.ExpectedSpeedPercent != 900 {
		t.Errorf("the start is expected at %d %%; want 900, the middle of the last five", offer.ExpectedSpeedPercent)
	}
}

// A node's runs' figures outlast a reopened cache, and start over when its FFmpeg, encoder or recipe
// changes, which its speed key says (start-quality-by-node-speed.md §3).
func TestKeepRunSpeed_FiguresOutlastAReopenedCacheAndStartOverOnANewKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cache.db")
	open := func() *store.Cache {
		cache, err := store.OpenCache(t.Context(), path, storetest.OpenOptions())
		if err != nil {
			t.Fatal(err)
		}
		return cache
	}
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	trials := keyedTrials{triedAt: kindTried("h264", 8, 1080, 200, 600), key: "7.1 ab12 x264 1"}
	service.encoders, service.cache = trials, open()
	keepRuns(t, service, threeAt400...)
	if err := service.cache.Close(); err != nil {
		t.Fatal(err)
	}
	service.cache = open()
	t.Cleanup(func() {
		if err := service.cache.Close(); err != nil {
			t.Error(err)
		}
	})
	if offer := startOf(t, service); offer.ExpectedSpeedFrom != SpeedFromRuns {
		t.Errorf("after the cache reopened the start is %s from %q; want its runs'", offer.StreamName,
			offer.ExpectedSpeedFrom)
	}
	service.encoders = keyedTrials{triedAt: trials.triedAt, key: "7.2 cd34 x264 1"}
	keepRuns(t, service, threeAt400[:2]...)
	if offer := startOf(t, service); offer.ExpectedSpeedFrom != SpeedFromTrial {
		t.Errorf("under a new key with two runs the start is from %q; want the trials'", offer.ExpectedSpeedFrom)
	}
	service.encoders = trials
	if offer := startOf(t, service); offer.ExpectedSpeedFrom != SpeedFromTrial {
		t.Errorf("back under the old key the start is from %q; want the trials', its runs started over",
			offer.ExpectedSpeedFrom)
	}
}

// measuringHold records node 1's holds to measure, how many commands its node had been sent at
// each, so a test sees a hold come before the run it is for is given, and how many times the hold
// was ended. It holds nothing back for an encode behind playback.
type measuringHold struct {
	sent       chan nodeproto.Command
	holds      []time.Time
	sentAtEach []int
	ended      int
}

func (*measuringHold) HoldBackFor(context.Context, int64, time.Time) int { return 0 }

func (hold *measuringHold) HoldBackToMeasure(_ context.Context, nodeID int64, until time.Time) int {
	if nodeID == 1 {
		hold.holds = append(hold.holds, until)
		hold.sentAtEach = append(hold.sentAtEach, len(hold.sent))
	}
	return 0
}

func (hold *measuringHold) EndHoldToMeasure(_ context.Context, nodeID int64) {
	if nodeID == 1 {
		hold.ended++
	}
}

// openTrickplayOf queues file fileID's trickplay job, its scrub-bar pictures still to be made.
func openTrickplayOf(t *testing.T, service *Service, fileID int64) {
	t.Helper()
	err := service.cache.Jobs().Enqueue(t.Context(), jobs.KindTrickplay, store.JobTarget{FileID: fileID},
		func(jobqueue.Open[store.JobTarget]) (jobqueue.Change, error) {
			return jobqueue.Change{
				Action: jobqueue.ActionInsert, ArgsJSON: "{}", Priority: jobs.PriorityPlayNow,
				RunnableAt: service.clock.Now(), CreatedAt: service.clock.Now(),
			}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

// gpuTried are trials of a node that encodes on a GPU.
type gpuTried struct{ triedAt }

func (gpuTried) EncodesOnAGPU(int64) bool { return true }

// A played run of a quality, of the speed window's film or more, on a node whose runs have not taken
// over for the film's kind and the quality, holds the node's other work back before the run is
// given, until twice the window's expected time at the speed it is expected at, with its start: the
// trials make 1080p at 200 %, so the half minute takes 15 s, and the hold lasts 35 s. Judged by
// nothing, it lasts measuringHoldMost. A node that encodes on a GPU is held too. Nothing is held
// once the runs have taken over, nor for a run no one waits to play, one short of the window, or one
// of a node whose runs are not kept (D572), nor while the film's own scrub-bar pictures are still to
// be made (D573).
func TestHandleMakeChunks_APlayedRunNotYetMeasuredHoldsItsNodesOtherWorkBackForItsWindow(t *testing.T) {
	t.Parallel()
	slowAtTheTop := triedAt(kindTried("h264", 8, 1080, 200, 600))
	for _, start := range []struct {
		name       string
		encoders   Encoders
		runs       []ranRun
		isUnplayed bool
		isShort    bool
		isPictured bool          // its film's trickplay job is open
		want       time.Duration // 0 holds nothing
	}{
		{name: "not yet measured", encoders: slowAtTheTop, want: 35 * time.Second},
		{name: "on a GPU", encoders: gpuTried{slowAtTheTop}, want: 35 * time.Second},
		{name: "judged by nothing", encoders: triedAt(nil), want: measuringHoldMost},
		{name: "two fair runs", encoders: slowAtTheTop, runs: threeAt400[:2], want: 35 * time.Second},
		{name: "the runs took over", encoders: slowAtTheTop, runs: threeAt400},
		{name: "no one waits to play it", encoders: slowAtTheTop, isUnplayed: true},
		{name: "short of the window", encoders: slowAtTheTop, isShort: true},
		{name: "runs not kept", encoders: keyedTrials{triedAt: slowAtTheTop}},
		{name: "its scrub-bar pictures still to be made", encoders: slowAtTheTop, isPictured: true},
	} {
		node := fakeNode{sent: make(chan nodeproto.Command, 1)}
		service, _ := chunksFixture(t, newFakeQueue(), node)
		service.exchanges.Handle(jobs.KindMakeChunks, service.Assignee(nodestest.NewResolver().Of(nil)))
		service.encoders = start.encoders
		keepRuns(t, service, start.runs...)
		hold := &measuringHold{sent: node.sent}
		service.holdBack = hold
		if start.isPictured {
			openTrickplayOf(t, service, 1)
		}
		grid := mustGrid(t, service)
		last := grid.chunkAt(nodeproto.SpeedWindowFilmMs + 10_000)
		if start.isShort {
			last = grid.chunkAt(nodeproto.SpeedWindowFilmMs-1) - 1
		}
		quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 4000, Height: 1080}}
		encoded, err := json.Marshal(runArgs{FirstChunk: 0, LastChunk: last})
		if err != nil {
			t.Fatal(err)
		}
		storeID, err := service.storeIDOf(t.Context(), 1, quality, testStoreKey(t, service, quality, grid.ID()))
		if err != nil {
			t.Fatal(err)
		}
		priority := jobs.PriorityPlayNow
		if start.isUnplayed {
			priority = jobs.PriorityBackground
		}
		answer := service.HandleMakeChunks(t.Context(), jobs.Job{
			ID: 7, Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: storeID}, ArgsJSON: string(encoded),
			Priority: priority,
		})
		if !jobs.IsAssigned(answer) {
			t.Fatalf("%s: the run was not given: %v", start.name, answer)
		}
		jobqueue.HandOver(t.Context(), answer) // as a runner does, once it recorded the job assigned
		testutil.Receive(t, node.sent, start.name+": the run's command")
		var want []time.Time
		if start.want > 0 {
			want = []time.Time{service.clock.Now().Add(start.want)}
		}
		if !slices.EqualFunc(hold.holds, want, time.Time.Equal) {
			t.Errorf("%s: the holds = %v; want %v", start.name, hold.holds, want)
		}
		if slices.ContainsFunc(hold.sentAtEach, func(sent int) bool { return sent > 0 }) {
			t.Errorf("%s: the node was held back after the run was given; want before, so it starts alone", start.name)
		}
	}
}

// givenToMeasure gives a played run of a quality, of a minute of film, to node 1, not yet measured,
// so its other work is held back to measure it; and returns the run's command, as sent.
func givenToMeasure(t *testing.T) (*Service, *measuringHold, nodeproto.Command) {
	t.Helper()
	node := fakeNode{sent: make(chan nodeproto.Command, 1)}
	service, _ := chunksFixture(t, newFakeQueue(), node)
	service.exchanges.Handle(jobs.KindMakeChunks, service.Assignee(decidingResolver{}))
	service.encoders = triedAt(kindTried("h264", 8, 1080, 200, 600))
	hold := &measuringHold{sent: node.sent}
	service.holdBack = hold
	grid := mustGrid(t, service)
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 4000, Height: 1080}}
	encoded, err := json.Marshal(runArgs{FirstChunk: 0, LastChunk: grid.chunkAt(2 * nodeproto.SpeedWindowFilmMs)})
	if err != nil {
		t.Fatal(err)
	}
	storeID, err := service.storeIDOf(t.Context(), 1, quality, testStoreKey(t, service, quality, grid.ID()))
	if err != nil {
		t.Fatal(err)
	}
	answer := service.HandleMakeChunks(t.Context(), jobs.Job{
		ID: 7, Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: storeID}, ArgsJSON: string(encoded),
		Priority: jobs.PriorityPlayNow,
	})
	if !jobs.IsAssigned(answer) {
		t.Fatalf("the run was not given: %v", answer)
	}
	jobqueue.HandOver(t.Context(), answer)
	command := testutil.Receive(t, node.sent, "the run's command")
	if len(hold.holds) != 1 {
		t.Fatalf("the holds to measure = %v; want one", hold.holds)
	}
	return service, hold, command
}

// decidingResolver resolves a job by its answer at once, as its runner would.
type decidingResolver struct{}

func (decidingResolver) ResolveBy(_ context.Context, id int64, decide func(job jobs.Job) error) (bool, error) {
	_ = decide(jobs.Job{ID: id, Kind: jobs.KindMakeChunks}) //nolint:errcheck // the test reads the hold, not the answer
	return true, nil
}

// madeThrough says to the core that the run of command made its chunks through chunk.
func madeThrough(t *testing.T, service *Service, command nodeproto.Command, chunk int) {
	t.Helper()
	service.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: command.JobID, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning,
		ChunksMadeThrough: &chunk,
	})
}

// A hold to measure a played run ends as soon as the core hears the run made its speed window's film,
// not at the time the hold was given, and once: the run's end after lifts nothing more. A run that ends
// before its window is made ends its hold too (D573).
func TestTakeProgress_AHoldToMeasureEndsAsTheRunsWindowIsMade(t *testing.T) {
	t.Parallel()
	service, hold, command := givenToMeasure(t)
	window := command.MakeChunks.BoundsMs[0] + nodeproto.SpeedWindowFilmMs
	beforeWindow := 0
	for command.MakeChunks.BoundsMs[beforeWindow+2] < window {
		beforeWindow++
	}
	madeThrough(t, service, command, beforeWindow)
	if hold.ended != 0 {
		t.Errorf("short of its window the run's hold was ended %d times; want it standing", hold.ended)
	}
	madeThrough(t, service, command, beforeWindow+1)
	if hold.ended != 1 {
		t.Errorf("its window made, the run's hold was ended %d times; want once", hold.ended)
	}
	service.reported(t, command, nodeproto.MakeChunksReport{State: nodeproto.RunDone, LastChunk: beforeWindow + 3})
	if hold.ended != 1 {
		t.Errorf("after its window, the run's end ended its hold %d times in all; want once", hold.ended)
	}

	cut, cutHold, cutCommand := givenToMeasure(t)
	madeThrough(t, cut, cutCommand, 0)
	cut.reported(t, cutCommand, nodeproto.MakeChunksReport{State: nodeproto.RunFailed, LastChunk: 0, Failure: "x"})
	if cutHold.ended != 1 {
		t.Errorf("a run ended short of its window ended its hold %d times; want once", cutHold.ended)
	}
}
