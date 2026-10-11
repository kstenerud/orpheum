// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
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
	storeID, err := fixture.service.storeIDOf(t.Context(), 1, stream, run.gridID)
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
	checkQualityRun(t, *sent, gridID)
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
	service.reported(t, command, nodeproto.MakeChunksReport{State: nodeproto.PrepareDone, LastChunk: 5})
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
	want := nodeproto.PrepareAudio{AudioTrackIndex: 0, Language: "en", ChannelCount: 6, CarriedGainDB: -3.5}
	if sent == nil || sent.Stream.Sound == nil || *sent.Stream.Sound != want || !sent.Stream.IsBoosted ||
		sent.Stream.SoundKbps != 96 || sent.SoundLeadInMs != 10_000 ||
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

func TestHandleMakeChunks_EachEndIsTakenAsAPreparesIs(t *testing.T) {
	t.Parallel()
	stall := &nodeproto.Stall{Cause: nodeproto.StallStoodStill, LastPositionMs: 4000, WithoutAdvanceMs: 180_000}
	for what, check := range map[string]struct {
		report nodeproto.MakeChunksReport
		isWant func(err error) bool
	}{
		"FFmpeg failing fails the job": {
			nodeproto.MakeChunksReport{State: nodeproto.PrepareFailed, LastChunk: -1, Failure: "ffmpeg: bad"},
			func(err error) bool { return err != nil && strings.Contains(err.Error(), "ffmpeg: bad") },
		},
		"a stall is tried again": {
			nodeproto.MakeChunksReport{State: nodeproto.PrepareStalled, LastChunk: 3, Stall: stall},
			func(err error) bool { return err != nil && strings.HasPrefix(err.Error(), "stalled: ") },
		},
		"a full disk parks it on the node's prepared directory": {
			nodeproto.MakeChunksReport{
				State: nodeproto.PrepareRefused, LastChunk: -1, Refusal: "full", RefusalCode: nodeproto.RefusedNoRoom,
			},
			func(err error) bool {
				parking, isParked := jobqueue.ParkingOf(err)
				return isParked && parking.Destination == jobs.PreparedDirectoryDestination(1)
			},
		},
		"another refusal is tried again": {
			nodeproto.MakeChunksReport{State: nodeproto.PrepareRefused, LastChunk: -1, Refusal: "not the one named"},
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

// checkQualityRun fails the test unless sent is chunks 3 to 5 of film 1's picture at 1200k, 480p, its
// source by identity, bounded by the grid's starts and the last chunk's end.
func checkQualityRun(t *testing.T, sent nodeproto.MakeChunks, gridID string) {
	t.Helper()
	wantVideo := nodeproto.BytesFile{RootPath: "/m", Path: []byte("f1.mkv"), PayloadHash: "01"}
	if sent.Video.Target() != wantVideo.Target() || sent.VideoStreamIndex != 0 || sent.VideoCodec != "h264" ||
		sent.StoreDir != "1/chunks-q1200k480p/"+gridID || sent.FirstChunk != 3 || sent.SoundLeadInMs != 0 ||
		!slices.Equal(sent.BoundsMs, []int64{30_000, 40_000, 50_000, 60_000}) ||
		sent.Stream != (nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}) {
		t.Fatalf("the node was sent %+v; want chunks 3 to 5 of film 1's picture at 1200k, 480p, by its "+
			"identity, bounded by the grid's starts and the last's end", sent)
	}
}

// A copied picture's run seeks midway between its first keyframe and the next, so FFmpeg lands on
// its first keyframe and not the one before it (idiosyncrasies.md).
func TestHandleMakeChunks_APictureRunSeeksPastItsFirstKeyframe(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	gridID := mustGrid(t, fixture.service).ID()
	fixture.start(t, 10, testRun{"picture", gridID, runArgs{FirstChunk: 2, LastChunk: 3}})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	if sent == nil || sent.PictureSeekMs != 25_000 || !slices.Equal(sent.BoundsMs, []int64{20_000, 30_000, 40_000}) {
		t.Fatalf("the node was sent %+v; want chunks 2 and 3 read from a seek to 25 s, midway to the next "+
			"keyframe", sent)
	}
}

// The seek is midway to the next keyframe, or the film's end; at least FFmpeg's 131 ms past the
// keyframe, so a Matroska seek moved earlier still lands on it; and always before the next keyframe.
func TestPictureSeekMs_IsPastTheKeyframeAndBeforeTheNext(t *testing.T) {
	t.Parallel()
	keyframes := []int64{0, 10_000, 10_200, 10_300, 20_000}
	for _, test := range []struct {
		name        string
		startMs     int64
		wantSeekMs  int64
		explanation string
	}{
		{"far apart", 0, 5_000, "midway"},
		{"200 ms apart", 10_000, 10_131, "131 ms past it, beyond midway"},
		{"100 ms apart", 10_200, 10_299, "a millisecond before the next keyframe"},
		{"the last keyframe", 20_000, 25_000, "midway to the film's end"},
	} {
		if got := PictureSeekMs(keyframes, test.startMs, 30_000); got != test.wantSeekMs {
			t.Errorf("%s: the run at %d ms seeks to %d ms, want %d: %s",
				test.name, test.startMs, got, test.wantSeekMs, test.explanation)
		}
	}
}
