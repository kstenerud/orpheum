// SPDX-License-Identifier: GPL-3.0-or-later

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

// start runs a make_chunks job of film 1 with args, and returns where the answer its node's word
// resolves it with arrives.
func (fixture runsFixture) start(t *testing.T, id int64, args runArgs) <-chan error {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return fixture.resolver.Start(t, fixture.service.HandleMakeChunks, jobs.Job{
		ID: id, Kind: jobs.KindMakeChunks, Target: jobs.Target{FileID: 1}, ArgsJSON: string(encoded),
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
	ended := fixture.start(t, 7, runArgs{Stream: "q1200k480p", GridID: gridID, FirstChunk: 3, LastChunk: 5})
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
	if runs := queue.runs(t); len(runs) != 1 || runs[0].FirstChunk != 6 {
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
	ended := fixture.start(t, 8, runArgs{Stream: "sound0-boost", GridID: gridID, FirstChunk: 2, LastChunk: 2})
	sent := testutil.Receive(t, fixture.node.sent, "the run's command").MakeChunks
	want := nodeproto.PrepareAudio{AudioTrackIndex: 0, Language: "en", ChannelCount: 6, CarriedGainDB: -3.5}
	if sent == nil || sent.Stream.Sound == nil || *sent.Stream.Sound != want || !sent.Stream.IsBoosted ||
		sent.SoundLeadInMs != 10_000 || !slices.Equal(sent.BoundsMs, []int64{20_000, 30_000}) {
		t.Fatalf("the node was sent %+v; want the English 5.1 track boosted, levelled as a prepare levels it, "+
			"led in from chunk 1's start", sent)
	}
	select {
	case err := <-ended:
		t.Errorf("the run ended %v before its node said", err)
	default:
	}
	// The second track is stereo: it has no boosted mix to make.
	err := testutil.Receive(t, fixture.start(t, 9, runArgs{Stream: "sound1-boost", GridID: gridID, LastChunk: 1}),
		"the job of a sound the film does not have to end")
	if err == nil || !strings.Contains(err.Error(), "no sound sound1-boost") {
		t.Errorf("a run of a boosted stereo track ended %v; want it refused for good", err)
	}
}

func TestHandleMakeChunks_ARunOnAGridTheFilmNoLongerHasEndsForGood(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	args := runArgs{Stream: "picture", GridID: "0123456789abcdef", LastChunk: 1}
	err := testutil.Receive(t, fixture.start(t, 7, args), "the job to end")
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
		args := runArgs{Stream: "picture", GridID: mustGrid(t, fixture.service).ID(), FirstChunk: 3, LastChunk: 5}
		ended := fixture.start(t, 7, args)
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
