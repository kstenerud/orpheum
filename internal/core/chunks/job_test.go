// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// runsFixture is a chunks service whose runs are sent to node, and the queue its runs are asked of.
type runsFixture struct {
	service  *Service
	queue    *fakeQueue
	node     fakeNode
	resolver *nodestest.Resolver
}

// runsOnAFakeNode is a chunks service whose runs node 1 is sent.
func runsOnAFakeNode(t *testing.T) runsFixture {
	t.Helper()
	fixture := runsFixture{
		queue: newFakeQueue(), node: fakeNode{sent: make(chan nodeproto.Command, 1)}, resolver: nodestest.NewResolver(),
	}
	fixture.service, _ = chunksFixture(t, fixture.queue, fixture.node)
	fixture.service.exchanges.Handle(jobs.KindMakeChunks, fixture.service.Assignee(fixture.resolver.Of(nil)))
	return fixture
}

// testRun is a run of film 1 a test starts: its store's stream and grid, and its chunks.
type testRun struct {
	stream, gridID string
	runArgs
}

// start runs a make_chunks job of a run of film 1, targeting its store's row, and returns where
// the answer its node's word resolves it with arrives.
func (fixture runsFixture) start(t *testing.T, id int64, run testRun) <-chan error {
	t.Helper()
	encoded, err := json.Marshal(run.runArgs)
	if err != nil {
		t.Fatal(err)
	}
	stream, isStream := StreamNamed(run.stream)
	if !isStream {
		t.Fatalf("no stream %q", run.stream)
	}
	storeID, err := fixture.service.storeIDOf(t.Context(), 1, stream,
		testStoreKey(t, fixture.service, stream, run.gridID))
	if err != nil {
		t.Fatal(err)
	}
	return fixture.resolver.Start(t, fixture.service.HandleMakeChunks, jobs.Job{
		ID: id, Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: storeID}, ArgsJSON: string(encoded),
		Priority: jobs.PriorityPlayNow,
	})
}

// reported hands node 1's report on how the run command made ended to the job, as the node channel
// does.
func (service *Service) reported(t *testing.T, command nodeproto.Command, report nodeproto.MakeChunksReport) {
	t.Helper()
	err := service.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
		JobID: command.JobID, AttemptToken: command.AttemptToken, Work: &command.Work, MakeChunks: &report,
	})
	if err != nil {
		t.Errorf("the node's report was not taken: %v", err)
	}
}

func TestHandleMakeChunks_SendsTheRunOfTheFilmsPictureAndTakesItsChunksAsTheyAreMade(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service, queue := fixture.service, fixture.queue
	gridID := mustGrid(t, service).ID()
	ended := fixture.start(t, 7, testRun{"q1200k480p", gridID, runArgs{FirstChunk: 3, LastChunk: 5}})
	command := testutil.Receive(t, fixture.node.sent, "the run's command")
	sent := command.MakeChunks
	if command.Type != nodeproto.CommandMakeChunks || sent == nil {
		t.Fatalf("the node was sent %+v; want a run of chunks", command)
	}
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}
	checkQualityRun(t, *sent, testStoreKey(t, service, quality, gridID))
	madeThrough := 4
	service.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: 7, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning, ChunksMadeThrough: &madeThrough,
	})
	if share := queue.progress[7]; share < 0.66 || share > 0.67 {
		t.Errorf("after chunk 4 of 3 to 5 the job is %v done; want two thirds", share)
	}
	// Chunks 3 and 4 are made and 5 on its way: a player at chunk 3 needs no run before chunk 6.
	ensured(t, service, Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}, 3)
	if runs := queue.runs(t, service); len(runs) != 1 || runs[0].firstChunk != 6 {
		t.Errorf("the runs asked for after progress = %+v; want one from chunk 6", runs)
	}
	service.reported(t, command, nodeproto.MakeChunksReport{State: nodeproto.RunDone, LastChunk: 5})
	if err := testutil.Receive(t, ended, "the run's job to end"); err != nil {
		t.Errorf("the run ended %v; want done", err)
	}
}

func TestHandleMakeChunks_ASoundRunStartsAChunkEarlyWithItsTracksGainAndMix(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	gridID := mustGrid(t, fixture.service).ID()
	ended := fixture.start(t, 8, testRun{"sound0-boost-96k", gridID, runArgs{FirstChunk: 2, LastChunk: 2}})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	want := nodeproto.SoundTrack{AudioTrackIndex: 0, Language: "en", ChannelCount: 6, CarriedGainDB: -3.5}
	if sent == nil || sent.Stream.Sound == nil || *sent.Stream.Sound != want || !sent.Stream.IsBoosted ||
		sent.Stream.SoundKbps != 96 || sent.LeadInMs != 10_000 || !sent.IsLedIn ||
		!slices.Equal(sent.BoundsMs, []int64{20_000, 30_000}) {
		t.Fatalf("the node was sent %+v; want the English 5.1 track boosted at 96 kb/s, levelled as a prepare "+
			"levels it, led in from chunk 1's start", sent)
	}
	select {
	case err := <-ended:
		t.Errorf("the run ended %v before its node said", err)
	default:
	}
	// The second track is stereo: it has no boosted mix to make.
	err := testutil.Receive(t, fixture.start(t, 9, testRun{"sound1-boost-96k", gridID, runArgs{LastChunk: 1}}),
		"the job of a sound the film does not have to end")
	if err == nil || !strings.Contains(err.Error(), "no sound sound1-boost-96k") {
		t.Errorf("a run of a boosted stereo track ended %v; want it refused for good", err)
	}
}

func TestHandleMakeChunks_ARunOnAGridTheFilmNoLongerHasEndsForGood(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	run := testRun{"picture", "0123456789abcdef", runArgs{LastChunk: 1}}
	err := testutil.Receive(t, fixture.start(t, 7, run), "the job to end")
	if !errors.Is(err, ErrGridChanged) {
		t.Errorf("a run on a grid the film does not have ended %v; want ErrGridChanged", err)
	}
}

func TestHandleMakeChunks_EachEndOfARunIsTakenAsItsNodeSays(t *testing.T) {
	t.Parallel()
	stall := &nodeproto.Stall{Cause: nodeproto.StallStoodStill, LastPositionMs: 4000, WithoutAdvanceMs: 180_000}
	for what, check := range map[string]struct {
		report nodeproto.MakeChunksReport
		isWant func(err error) bool
	}{
		"FFmpeg failing fails the job": {
			nodeproto.MakeChunksReport{State: nodeproto.RunFailed, LastChunk: -1, Failure: "ffmpeg: bad"},
			func(err error) bool { return err != nil && strings.Contains(err.Error(), "ffmpeg: bad") },
		},
		"a stall is tried again": {
			nodeproto.MakeChunksReport{State: nodeproto.RunStalled, LastChunk: 3, Stall: stall},
			func(err error) bool { return err != nil && strings.HasPrefix(err.Error(), "stalled: ") },
		},
		"a full disk parks it on the node's prepared directory": {
			nodeproto.MakeChunksReport{
				State: nodeproto.RunRefused, LastChunk: -1, Refusal: "full", RefusalCode: nodeproto.RefusedNoRoom,
			},
			func(err error) bool {
				parking, isParked := jobqueue.ParkingOf(err)
				return isParked && parking.Destination == jobs.PreparedDirectoryDestination(1)
			},
		},
		"another refusal is tried again": {
			nodeproto.MakeChunksReport{State: nodeproto.RunRefused, LastChunk: -1, Refusal: "not the one named"},
			func(err error) bool {
				_, isParked := jobqueue.ParkingOf(err)
				return err != nil && !isParked && strings.Contains(err.Error(), "not the one named")
			},
		},
	} {
		fixture := runsOnAFakeNode(t)
		run := testRun{"picture", mustGrid(t, fixture.service).ID(), runArgs{FirstChunk: 3, LastChunk: 5}}
		ended := fixture.start(t, 7, run)
		fixture.service.reported(t, testutil.Receive(t, fixture.node.sent, "the run's command"), check.report)
		if err := testutil.Receive(t, ended, "the run's job to end"); !check.isWant(err) {
			t.Errorf("%s: the job ended %v", what, err)
		}
	}
}

// A run its node refuses because its film is not at the place it read, renamed away, is tried again
// reading the copy beside it, which the runs after it read too, until a scan sees the film there
// again; with no other place it is a refusal tried again on its back-off (D363 §8). A play of a film
// renamed away waited on runs refused there though its copy stood (staging, 2026-10-27, VQ81).
func TestHandleMakeChunks_ARunRefusedForItsFilmGoneFromItsPlaceReadsTheCopyBesideIt(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	service.absences = library.NewAbsences(slog.New(slog.DiscardHandler))
	backup := librarytest.LibraryOn(t, service.database, 1,
		librarytest.Library{Name: "Backup", Kind: "movie", MetadataSource: "nfo", Root: "/m2"})
	librarytest.File(t, service.database, librarytest.FileSpec{
		LibraryID: backup, NodeID: 1, Path: "/m2/f1.mkv", Filing: "copy", CopyOf: 1, FirstSeenAt: 3,
	})
	gone := nodeproto.MakeChunksReport{
		State: nodeproto.RunRefused, LastChunk: -1, Refusal: "no such file", RefusalCode: nodeproto.RefusedGone,
	}
	run := testRun{"picture", mustGrid(t, service).ID(), runArgs{FirstChunk: 3, LastChunk: 5}}
	readAt := func(id int64) (nodeproto.Command, <-chan error) {
		t.Helper()
		ended := fixture.start(t, id, run)
		return testutil.Receive(t, fixture.node.sent, "the run's command"), ended
	}
	for index, want := range []string{"/m/f1.mkv", "/m2/f1.mkv", "/m/f1.mkv"} {
		command, ended := readAt(int64(7 + index))
		read := command.MakeChunks.Video
		if got := read.RootPath + "/" + string(read.Path); got != want {
			t.Fatalf("the run read %s; want %s", got, want)
		}
		service.reported(t, command, gone)
		err := testutil.Receive(t, ended, "the refused run to end")
		if _, isParked := jobqueue.ParkingOf(err); err == nil || isParked || jobs.IsMarkedPermanent(err) {
			t.Errorf("a run refused at %s for its film gone ended %v; want it tried again", want, err)
		}
	}
}

// checkQualityRun fails the test unless sent is chunks 3 to 5 of film 1's picture at 1200k, 480p, its
// source by identity, bounded by the grid's starts and the last chunk's end.
func checkQualityRun(t *testing.T, sent nodeproto.MakeChunks, storeKey string) {
	t.Helper()
	wantVideo := nodeproto.BytesFile{RootPath: "/m", Path: []byte("f1.mkv"), PayloadHash: "01"}
	if sent.Video.Target() != wantVideo.Target() || sent.VideoStreamIndex != 0 || sent.VideoCodec != "h264" ||
		sent.StoreDir != "1/chunks-q1200k480p/"+storeKey || sent.FirstChunk != 3 || sent.LeadInMs != 0 ||
		!slices.Equal(sent.BoundsMs, []int64{30_000, 40_000, 50_000, 60_000}) ||
		sent.Stream != (nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}) {
		t.Fatalf("the node was sent %+v; want chunks 3 to 5 of film 1's picture at 1200k, 480p, by its "+
			"identity, bounded by the grid's starts and the last's end", sent)
	}
}

// A copied picture's run is led in from the chunk before, so FFmpeg times an open GOP's pictures
// shown before the run's first keyframe as the film does (idiosyncrasies.md); the film's first chunk
// is read from itself.
func TestHandleMakeChunks_APictureRunIsLedInFromTheChunkBeforeIt(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	gridID := mustGrid(t, fixture.service).ID()
	fixture.start(t, 10, testRun{"picture", gridID, runArgs{FirstChunk: 2, LastChunk: 3}})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	if sent == nil || sent.LeadInMs != 10_000 || !sent.IsLedIn ||
		!slices.Equal(sent.BoundsMs, []int64{20_000, 30_000, 40_000}) {
		t.Fatalf("the node was sent %+v; want chunks 2 and 3 led in from 10 s", sent)
	}
	fixture.start(t, 11, testRun{"picture", gridID, runArgs{FirstChunk: 0, LastChunk: 1}})
	sent = testutil.Receive(t, fixture.node.sent, "the first chunk's command").MakeChunks
	if sent == nil || sent.LeadInMs != 0 || sent.IsLedIn {
		t.Fatalf("the node was sent %+v; want chunks 0 and 1 with no lead-in", sent)
	}
	// Chunk 0 starts at the film's start, so a run from chunk 1 is led in from 0 ms, and says so: a
	// lead-in of zero alone is none, and the node refused the run's seek past chunk 0's keyframe (file
	// 17099, staging, 2026-11-16).
	fixture.start(t, 12, testRun{"picture", gridID, runArgs{FirstChunk: 1, LastChunk: 2}})
	sent = testutil.Receive(t, fixture.node.sent, "the second chunk's command").MakeChunks
	if sent == nil || sent.LeadInMs != 0 || !sent.IsLedIn {
		t.Fatalf("the node was sent %+v; want chunks 1 and 2 led in from the film's start", sent)
	}
}

// A film whose picture's B-frames carry no display times, as its probe says, has every run of its
// picture, copied or encoded, say it is to be re-timed, and its sound's not; an ordinary film's
// picture is not re-timed (design/plans/audio-switch-and-retiming-2026-11-01.md §2.4; file 5667).
func TestHandleMakeChunks_APictureWhoseBFramesCarryNoDisplayTimesIsReTimed(t *testing.T) {
	t.Parallel()
	ordinary := runsOnAFakeNode(t)
	ordinary.start(t, 10, testRun{"picture", mustGrid(t, ordinary.service).ID(), runArgs{FirstChunk: 2, LastChunk: 3}})
	if sent := testutil.Receive(t, ordinary.node.sent, "the run's command").MakeChunks; sent == nil ||
		sent.NeedsPictureRetiming {
		t.Errorf("an ordinary film's picture run was sent %+v; want it not re-timed", sent)
	}

	fixture := runsOnAFakeNode(t)
	librarytest.Probe(t, fixture.service.database, librarytest.ProbeSpec{
		FileID: 1, DurationMs: filmMinutes * 60_000,
		ProbeJSON: `{"format":{"formatNames":["mov","mp4"],"durationMs":1200000,"bitRateBps":4000000},"streams":[` +
			`{"kind":"video","codec":"hevc","video":{"width":1920,"height":1080,"needsRetiming":true}},` +
			`{"streamIndex":1,"kind":"audio","codec":"aac","audio":{"channelCount":2}}]}`,
	})
	gridID := mustGrid(t, fixture.service).ID()
	for index, test := range []struct {
		stream string
		want   bool
	}{{"picture", true}, {"q1200k480p", true}, {"sound0-96k-raw", false}} {
		fixture.start(t, int64(11+index), testRun{test.stream, gridID, runArgs{FirstChunk: 2, LastChunk: 3}})
		sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
		if sent == nil || sent.NeedsPictureRetiming != test.want {
			t.Errorf("a %s run was sent %+v; want it said re-timed: %v", test.stream, sent, test.want)
		}
	}
}

// A store whose runs FFmpeg failed at the same chunk twice is not asked of its node again: the
// player asking is told the film cannot be made, and why is logged once; a single failure is tried
// again. A sound whose every run failed alike was queued again every 4.4 s for as long as its player
// waited (file 15102 on the production NAS, 2026-10-27).
func TestEnsure_AChunkWhoseRunsFailedTwiceIsNotMadeAgain(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	var logs bytes.Buffer
	service.logger = slog.New(slog.NewTextHandler(&logs, nil))
	picture := Stream{Kind: nodeproto.StreamPicture}
	failure := nodeproto.MakeChunksReport{
		State: nodeproto.RunFailed, LastChunk: -1, Failure: "raw 00000.mp4 holds less",
	}
	for attempt := range 2 {
		ensured(t, service, picture, 0)
		fixture.queue.mutex.Lock()
		request := fixture.queue.requests[attempt]
		fixture.queue.mutex.Unlock()
		job := jobs.Job{
			ID: int64(20 + attempt), Kind: jobs.KindMakeChunks, Target: request.Target, ArgsJSON: request.ArgsJSON,
			Priority: request.Priority,
		}
		ended := fixture.resolver.Start(t, service.HandleMakeChunks, job)
		service.reported(t, testutil.Receive(t, fixture.node.sent, "the run's command"), failure)
		ending := testutil.Receive(t, ended, "the run's job to end")
		if !jobs.IsMarkedPermanent(ending) {
			t.Fatalf("a failed run's job ended %v; want it failed for good", ending)
		}
		if err := service.RecordMakeChunksFailure(t.Context(), job, ending.Error()); err != nil {
			t.Fatal(err)
		}
	}
	err := service.Ensure(t.Context(), 1, OnKeyframes, picture, 0)
	if !errors.Is(err, ErrCannotBeMade) || !strings.Contains(err.Error(), "raw 00000.mp4 holds less") {
		t.Errorf("a third ask was %v; want it refused, ErrCannotBeMade, with FFmpeg's word", err)
	}
	if runs := fixture.queue.runs(t, service); len(runs) != 2 {
		t.Errorf("the runs queued were %+v; want the two that failed alone", runs)
	}
	if logged := logs.String(); strings.Count(logged, "level=WARN") != 1 ||
		!strings.Contains(logged, "raw 00000.mp4 holds less") || !strings.Contains(logged, "chunk=0") {
		t.Errorf("the log says %q; want one warning of the chunk given up on and why", logged)
	}
}

// A RealMedia film's cook is interleaved in super-blocks, which FFmpeg loses the one of a seek lands
// in, and the run says so; its RealAudio 14.4, not interleaved, is not (file 15102 on the production NAS,
// 2026-10-27; idiosyncrasies.md).
func TestHandleMakeChunks_ARealMediaSoundInterleavedInSuperBlocksIsSaidToBe(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	librarytest.Probe(t, fixture.service.database, librarytest.ProbeSpec{
		FileID: 1, DurationMs: filmMinutes * 60_000,
		ProbeJSON: `{"format":{"formatNames":["rm"],"durationMs":1200000,"bitRateBps":400000},"streams":[` +
			`{"kind":"video","codec":"rv40","video":{"width":424,"height":240}},` +
			`{"streamIndex":1,"kind":"audio","codec":"cook","audio":{"channelCount":2}},` +
			`{"streamIndex":2,"kind":"audio","codec":"ra_144","audio":{"channelCount":1}}]}`,
	})
	gridID := mustGrid(t, fixture.service).ID()
	for index, want := range []bool{true, false} {
		stream := "sound" + strconv.Itoa(index) + "-96k-raw"
		fixture.start(t, int64(11+index), testRun{stream, gridID, runArgs{FirstChunk: 0, LastChunk: 3}})
		sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
		if sent == nil || sent.IsSoundInterleaved != want {
			t.Errorf("the node was sent %+v for %s; want it said interleaved: %v", sent, stream, want)
		}
	}
}

// Each step of a run looks again at who wants it: a run whose players all fell silent, and so want
// nothing, stops at its next step, with no ask to stop it (D309 §3).
func TestTakeProgress_ARunNoPlayerWantsStopsAtItsNextStep(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	grid := mustGrid(t, service)
	dir := testStoreDir(t, service,
		Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}},
		grid.ID())
	wanted := wantedBy(service)
	wanted.say(Interest{StreamName: "q1200k480p", PositionMs: 30_000})
	fixture.start(t, 7, testRun{"q1200k480p", grid.ID(), runArgs{FirstChunk: 3, LastChunk: 5}})
	command := testutil.Receive(t, fixture.node.sent, "the run's command")
	service.runs.knowGrid(dir, grid)
	for step, players := range [][]Interest{{{StreamName: "q1200k480p", PositionMs: 30_000}}, nil} {
		wanted.say(players...)
		madeThrough := 3 + step
		service.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
			JobID: 7, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning,
			ChunksMadeThrough: &madeThrough,
		})
		if isWanted := players != nil; service.runs.hasRun(dir) != isWanted {
			t.Errorf("at step %d, with players %v, the run is kept %t; want %t", step, players, !isWanted, isWanted)
		}
	}
}

// A run cancelled whose node still says its progress, as one does between the cancel and its end,
// is not taken as running again: it was, and the chunks it was to make counted as on their way,
// which no run made (staging, 2026-10-18).
func TestTakeProgress_ARunCancelledIsNotTakenAsRunningAgainByItsProgress(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	grid := mustGrid(t, service)
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}
	dir := testStoreDir(t, service, quality, grid.ID())
	wanted := wantedBy(service)
	fixture.start(t, 7, testRun{"q1200k480p", grid.ID(), runArgs{FirstChunk: 3, LastChunk: 5}})
	command := testutil.Receive(t, fixture.node.sent, "the run's command")
	service.runs.knowGrid(dir, grid)
	if err := service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	// The player wants it again before the cancelled run's last word arrives.
	wanted.say(Interest{StreamName: "q1200k480p", PositionMs: 40_000})
	madeThrough := 3
	service.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: 7, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning,
		ChunksMadeThrough: &madeThrough,
	})
	if service.runs.hasRun(dir) {
		t.Fatal("the cancelled run's progress took it as running again")
	}
	// Its chunks after the one made start a run of their own.
	if err := service.askedFor(t.Context(), 1, OnKeyframes, quality, 4); err != nil {
		t.Fatal(err)
	}
	if runs := fixture.queue.runs(t, service); len(runs) != 1 || runs[0].firstChunk != 4 {
		t.Errorf("the runs asked for = %+v; want one from chunk 4", runs)
	}
}

// A run the node evicted, still running there, is not taken as running again by a progress it said
// before it stopped, and once its end comes it is no longer held as cancelled: the eviction forgot the
// run as a cancel does (staging, 2026-10-18).
func TestTakeProgress_ARunEvictedIsNotTakenAsRunningAgainAndItsEndIsForgotten(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	grid := mustGrid(t, service)
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}
	dir := testStoreDir(t, service, quality, grid.ID())
	fixture.start(t, 7, testRun{"q1200k480p", grid.ID(), runArgs{FirstChunk: 3, LastChunk: 5}})
	command := testutil.Receive(t, fixture.node.sent, "the run's command")
	service.runs.evict(dir, 3, 1, service.clock.Now())
	madeThrough := 3
	service.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: 7, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning,
		ChunksMadeThrough: &madeThrough,
	})
	if service.runs.hasRun(dir) {
		t.Fatal("the evicted run's progress took it as running again")
	}
	service.reported(t, command, nodeproto.MakeChunksReport{State: nodeproto.RunDone, LastChunk: 3})
	service.runs.mutex.Lock()
	defer service.runs.mutex.Unlock()
	if len(service.runs.cancelledJobs) != 0 {
		t.Errorf("after its end, the jobs held as cancelled = %v; want none", service.runs.cancelledJobs)
	}
}

// stretchedTrackZero is chunksFixture's probe of its films, their first audio track's first packet
// lasting 31.9 s, as file 18306's does (D534).
func stretchedTrackZero(durationMs int64) string {
	return fmt.Sprintf(`{"format":{"durationMs":%d,"bitRateBps":4000000},"streams":[`+
		`{"kind":"video","codec":"h264","video":{"width":1920,"height":1080}},`+
		`{"streamIndex":1,"kind":"audio","codec":"ac3","language":"en",`+
		`"audio":{"channelCount":6,"firstPacketStretchMs":31902}},`+
		`{"streamIndex":2,"kind":"audio","codec":"aac","audio":{"channelCount":2}}]}`, durationMs)
}

// A track whose first packet alone lasts seconds is never offered copied, though its player plays it
// as it is: a player takes a copy's times and played it that much late; the top plays the sound made
// of it (D534 §2). The sound made's runs read past the stretch, as its probe says; another track's do
// not.
func TestStoreFileOf_ATrackWhoseFirstPacketIsStretchedIsNeverCopied(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	librarytest.Probe(t, service.database, librarytest.ProbeSpec{
		FileID: 1, DurationMs: filmMinutes * 60_000, ProbeJSON: stretchedTrackZero(filmMinutes * 60_000),
	})
	_, err := service.StoreFileOf(t.Context(), playsOwnSound(), "sound0-own", "00009.m4s")
	if !errors.Is(err, ErrNoStream) {
		t.Errorf("track 0 copied, its first packet stretched = %v; want ErrNoStream", err)
	}
	fixture := runsOnAFakeNode(t)
	librarytest.Probe(t, fixture.service.database, librarytest.ProbeSpec{
		FileID: 1, DurationMs: filmMinutes * 60_000, ProbeJSON: stretchedTrackZero(filmMinutes * 60_000),
	})
	gridID := mustGrid(t, fixture.service).ID()
	for index, want := range []int64{31_902, 0} {
		stream := "sound" + strconv.Itoa(index) + "-96k-raw"
		fixture.start(t, int64(11+index), testRun{stream, gridID, runArgs{FirstChunk: 0, LastChunk: 3}})
		sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
		if sent == nil || sent.SoundFirstPacketStretchMs != want {
			t.Errorf("the node was sent %+v for %s; want its first packet's stretch, %d ms", sent, stream, want)
		}
	}
}
