// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// These tests are scenarios from docs/contributors/design/plans/scenarios.md, named by their ids.

// S29: the partner starts a film whose node is down. Direct play is refused at once
// (TestPlayback_SaysWhichLegFailed); a prepare is queued at the
// highest priority and waits for the node without spending attempts, and the job view says it
// waits for that node and since when; the node's hello brings it back (D231 §6, §11, §12). What
// the player says while it waits is Phase 6's.
func TestScenario_S29_APrepareForAParkedNodeSaysWhatItWaitsFor(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	partner := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.roots = []string{"/media"} // shelveFilms enrolled node 1, "local"
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.nodes.isAway.Store(true)
	decided := harness.play(t, partner, chrome(2))
	if !decided.IsPreparing {
		t.Fatalf("the decision = %+v, want a copy being prepared", decided)
	}
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindPrepare, playback.HandlePrepare)
	for range 3 {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		harness.clock.Advance(time.Hour) // any while: nothing but its hello brings the node back
	}
	prepare := newestPrepareJob(t, harness.listedJobs(t, partner))
	if prepare.State != "queued" || prepare.AttemptCount != 0 || prepare.WaitingFor == nil ||
		prepare.WaitingFor.Name != "node local" || prepare.WaitingFor.Reason != "not connected" ||
		prepare.WaitingFor.Since != fakeNodeLastHeard.UnixMilli() || len(harness.nodes.commanded) != 0 {
		t.Errorf("the prepare while its node is down: %+v, waiting for %+v; want it waiting for node local, not "+
			"connected since it was last heard, no attempt spent and nothing sent", prepare, prepare.WaitingFor)
	}
	harness.nodes.isAway.Store(false)
	harness.queue.Unpark(t.Context(), jobs.NodeDestination(1)) // the node's hello
	go func() {
		command := <-harness.nodes.commanded
		playback.TakePrepareReport(t.Context(), 1, nodeproto.PrepareReport{
			JobID: command.Prepare.JobID, State: nodeproto.PrepareDone, SizeBytes: 9,
		})
	}()
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if done := newestPrepareJob(t, harness.listedJobs(t, partner)); done.State != "done" || done.AttemptCount != 1 {
		t.Errorf("once the node said hello the prepare is %+v; want it done at its first attempt", done)
	}
}

// S76: two nodes, one down. Work for the live node's files is unaffected; work for the down
// node's files parks on it, and a doomed attempt no longer blocks the queue (D231 §6).
func TestScenario_S76_ADownNodeBlocksNoOtherNodesWork(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)`,
		`INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 2, '/attic')`,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
		   probe_json, first_seen_at, last_seen_at)
		 SELECT 10, 1, 2, '/attic/Film (2003).mp4', CAST('/attic/Film (2003).mp4' AS BLOB), 10, X'1a1b', 'listed',
		   probe_json, 1, 1 FROM file WHERE id = 1`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, channel_count)
		 VALUES (10, 10, 0, 'audio', 'aac', 2)`)
	harness.nodes.awayNodeID = 2
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	for _, fileID := range []int64{10, 1} { // the down node's first, as a doomed attempt would have been
		measurement := jobs.Request{Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: fileID}, Reason: "test"}
		if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
			t.Fatal(err)
		}
	}
	playback := harness.server.dependencies.Load().Playback
	go func() {
		command := <-harness.nodes.commanded
		playback.TakeLoudness(t.Context(), 1, nodeproto.LoudnessResult{
			JobID: command.MeasureLoudness.JobID, Failure: "Invalid data found when processing input",
		})
	}()
	runner := harness.queue.NewRunner(jobs.KindMeasureLoudness, playback.HandleMeasureLoudness)
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, job := range harness.listedJobs(t, admin) {
		switch job.FileID {
		case 1:
			if job.State != "failed" || job.AttemptCount != 1 {
				t.Errorf("the live node's measurement = %+v; want it run at once, to FFmpeg's answer", job)
			}
		case 10:
			if job.State != "queued" || job.AttemptCount != 0 || job.Destination != "node:2" || job.WaitingFor == nil {
				t.Errorf("the down node's measurement = %+v; want it waiting for node attic, no attempt spent", job)
			}
		}
	}
}

// missingMedia records the last scan of node 1's root /media as unable to read it.
func (harness testHarness) missingMedia(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`)
}

func TestPrepare_APrepareOfAFileUnderAMissingRootParksOnItAndSendsNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, user, chrome(2))
	harness.missingMedia(t)
	playback := harness.server.dependencies.Load().Playback
	prepare := jobs.Job{ID: 81, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	err := playback.HandlePrepare(t.Context(), prepare)
	if parking, isParked := jobqueue.ParkingOf(err); !isParked ||
		parking.Destination != jobs.RootDestination(1, "/media") || len(harness.nodes.commanded) != 0 {
		t.Errorf("a prepare under a missing root: %v; want it parked on the root and nothing sent (D231 §5)", err)
	}
}

func TestSubtitles_AnExtractionWaitsForAMissingRootAndForANodeThatIsAway(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	playback := harness.server.dependencies.Load().Playback
	job := jobs.Job{ID: 93, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{StreamID: subtitleStreamID}}
	harness.nodes.isAway.Store(true)
	err := playback.HandleExtractSubtitle(t.Context(), job)
	if parking, isParked := jobqueue.ParkingOf(err); !isParked || parking.Destination != jobs.NodeDestination(1) {
		t.Errorf("an extraction for a node that is away: %v; want it parked on the node (D231 §11)", err)
	}
	harness.nodes.isAway.Store(false)
	harness.missingMedia(t)
	err = playback.HandleExtractSubtitle(t.Context(), job)
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.RootDestination(1, "/media") {
		t.Errorf("an extraction under a missing root: %v; want it parked on the root (D231 §5)", err)
	}
	if len(harness.nodes.commanded) != 0 {
		t.Error("a command was sent for an extraction that waits")
	}
}
