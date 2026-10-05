// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// shelveAudioStreams gives the shelved films the audio streams a scan records: the MP4's stereo
// track and the MKV's eight-channel one.
func (harness testHarness) shelveAudioStreams(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, channel_count)
		 VALUES (1, 1, 0, 'audio', 'aac', 2), (2, 2, 1, 'audio', 'truehd', 8)`)
}

// readThroughWith runs a read-through job for fileID, answers the node's command with answer, and
// returns the command and how the job ended.
func (harness testHarness) readThroughWith(
	t *testing.T, jobID, fileID int64, answer nodeproto.ReadThroughResult,
) (*nodeproto.ReadThrough, error) {
	t.Helper()
	job := jobs.Job{ID: jobID, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: fileID}}
	finished := harness.handle(t, harness.readThrough.HandleReadThrough, job)
	command := harness.nextCommand(t)
	harness.answerReadThrough(t, jobID, answer)
	//nolint:wrapcheck // the job's own error, which the test reads as it is
	return command.ReadThrough, testutil.Receive(t, finished, "the read-through job to end")
}

// filesToReadThrough is what a sweep would find to read through now.
func (harness testHarness) filesToReadThrough(t *testing.T) []int64 {
	t.Helper()
	ids, err := harness.readThrough.FilesToReadThrough(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestReadThrough_AFileFFmpegCannotReadIsNotAskedAgainButOneTheNodeDidNotTryIs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	_, err := harness.readThroughWith(t, 93, 2,
		nodeproto.ReadThroughResult{Refusal: "the file at this path is not the one named"})
	if err == nil || !strings.Contains(err.Error(), "not the one named") {
		t.Errorf("a measurement the node would not try ended %v, want its reason", err)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after the node would not try film 2, a sweep finds %v, want it asked again", ids)
	}
	_, err = harness.readThroughWith(t, 94, 2, nodeproto.ReadThroughResult{Failure: "ffmpeg: Invalid data found"})
	if err == nil || !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("a failed measurement ended %v, want FFmpeg's words", err)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after FFmpeg could not measure film 2, a sweep finds %v, want only film 1", ids)
	}
}

func TestReadThrough_AReadThroughTheNodeRefusedIsTriedAgainOnItsBackoff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	measurement := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 2}, Reason: "test"}
	if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	runner := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	command := harness.nextCommand(t)
	refusal := "not the one named"
	answer := nodeproto.ReadThroughResult{Refusal: refusal, RefusalCode: nodeproto.RefusedChanged}
	harness.answerReadThrough(t, command.JobID, answer)
	if err := testutil.Receive(t, ran, "the measurement runner to end"); err != nil {
		t.Fatal(err)
	}
	job := newestOf(t, harness.listedJobs(t, admin), jobs.KindReadThrough)
	if job.State != "queued" || !strings.Contains(job.LastFailure, refusal) ||
		!strings.Contains(job.LastFailure, "changed since the scan") {
		t.Errorf("the refused measurement = %+v, want queued to be tried again, with the refusal for a changed file",
			job)
	}
}

// A read-through is waited for as long as reading its file takes at the slowest its node says it
// reads; a node that did not say is not sent one it could not be waited for.
func TestReadThrough_AReadThroughOnANodeThatDidNotSayHowFastItReadsIsNotSent(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.isReadRateUnknown = true
	job := jobs.Job{ID: 5, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}}
	err := harness.readThrough.HandleReadThrough(t.Context(), job)
	if err == nil || !strings.Contains(err.Error(), "has not said how fast it reads") {
		t.Errorf("measuring on a node that did not say how fast it reads: %v, want it not waited for", err)
	}
	if len(harness.nodes.queued) != 0 {
		t.Errorf("the node was sent %+v, want nothing", harness.nodes.queued)
	}
}

func TestReadThrough_AFileUnderAMissingRootIsNotSweptAndItsReadThroughWaitsForTheRoot(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`)
	if ids := harness.filesToReadThrough(t); len(ids) != 0 {
		t.Errorf("with its root unreadable, a sweep finds %v to measure; want none (G6)", ids)
	}
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	measurement := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 2}, Reason: "test"}
	if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	runner := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(harness.nodes.commanded) != 0 {
		t.Error("the node was asked to measure a file its missing root holds")
	}
	views, err := harness.queue.List(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if job := views[0]; job.State != "queued" || job.AttemptCount != 0 ||
		job.Destination != jobs.RootDestination(1, "/media") || job.WaitingFor == nil {
		t.Errorf("the measurement = %+v; want queued on the missing root with no attempt spent", job)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE root_state SET last_scan_outcome = 'complete'`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media', 9, 0)`)
	if ids := harness.filesToReadThrough(t); len(ids) != 0 {
		t.Errorf("with the disk under its root missing, a sweep finds %v to measure; want none (G6)", ids)
	}
	storetest.ExecRaw(t, harness.databasePath, `DELETE FROM root_mount`)
	harness.queue.Unpark(t.Context(), jobs.RootDestination(1, "/media"))
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		command, isSent := harness.commandWithin(t)
		if !isSent {
			return
		}
		harness.answerReadThrough(t, command.JobID,
			nodeproto.ReadThroughResult{Failure: "Invalid data found when processing input"})
	}()
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The job is given to the node; the node's answer resolves it.
	testutil.Receive(t, answered, "the node's answer")
	if job := newestOf(t, harness.listedJobs(t, admin), jobs.KindReadThrough); job.State != "failed" ||
		job.AttemptCount != 1 {
		t.Errorf("once the root came back, the measurement = %+v; want it run once, to FFmpeg's answer", job)
	}
}

// A root whose disk work stalled is unavailable until a scan reads it again, though its last scan
// was complete: the sweep passes its files over by the same rule a read-through parks by (D233 §10).
func TestReadThrough_AFileUnderARootWhoseDiskStalledIsNotSwept(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, unavailable_since)
		 VALUES (1, '/media', 'complete', 5000, 6000)`)
	if ids := harness.filesToReadThrough(t); len(ids) != 0 {
		t.Errorf("with its root's disk stalled, a sweep finds %v to measure; want none", ids)
	}
	storetest.ExecRaw(t, harness.databasePath, `UPDATE root_state SET unavailable_since = NULL`)
	if ids := harness.filesToReadThrough(t); len(ids) == 0 {
		t.Error("once a scan read the root again, a sweep finds nothing to measure; want its films")
	}
}

// shelvePictures gives the shelved films the picture streams a scan records, and a cover beside the
// MKV's, which is not a picture to play. The MP4's probe numbers both its streams 0, which its sound's
// row has taken, so its picture's row is numbered apart: the sweep asks only whether it has one.
func (harness testHarness) shelvePictures(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, disposition_flags)
		 VALUES (1, 1, 10, 'video', 'h264', 0), (2, 2, 0, 'video', 'h264', 0), (2, 2, 9, 'video', 'mjpeg', 1024)`)
}

// timelineOf is the timeline kept for the bytes with payloadHash, of stream 0.
func (harness testHarness) timelineOf(t *testing.T, payloadHash []byte) (cachegen.VideoTimeline, bool) {
	t.Helper()
	timeline, err := store.OrNotFound(harness.cache.Read().GetVideoTimeline(t.Context(),
		cachegen.GetVideoTimelineParams{PayloadHash: payloadHash, VideoStreamIndex: 0}))
	if errors.Is(err, store.ErrNotFound) {
		return cachegen.VideoTimeline{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return timeline, true
}

// shelveFilmsToReadThrough shelves the films with their sound and pictures, ready to be commanded,
// and film 1's loudness, as if measured before timelines were read: film 1 lacks only its timeline.
func (harness testHarness) shelveFilmsToReadThrough(t *testing.T) {
	t.Helper()
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.shelvePictures(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertLoudness(t.Context(), sqlcgen.UpsertLoudnessParams{
			PayloadHash: []byte{0x0a, 0x0b}, StreamIndex: 0, MeasuredMix: "stereo", IntegratedLUFS: new(-20.0),
			MeasuredAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadThrough_APictureIsReadForItsTimelineOnlyWhileItsBytesLackIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilmsToReadThrough(t)
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Fatalf("a sweep finds %v, want both films, whose pictures have no timeline", ids)
	}
	timeline := nodeproto.VideoTimeline{KeyframesMs: []int64{0, 10_010}, BytesInEachSecond: []int64{900, 40}}
	command, err := harness.readThroughWith(t, 80, 1, nodeproto.ReadThroughResult{VideoTimeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || len(command.LoudnessTracks) != 0 || command.VideoTimeline == nil ||
		command.VideoTimeline.VideoStreamIndex != 0 {
		t.Fatalf("film 1, its loudness kept, was asked %+v; want only stream 0's timeline", command)
	}
	kept, isKept := harness.timelineOf(t, []byte{0x0a, 0x0b})
	if !isKept || kept.KeyframesMsJSON != "[0,10010]" || kept.BytesInEachSecondJSON != "[900,40]" ||
		kept.Failure != nil {
		t.Errorf("the timeline kept for film 1's bytes = %+v; want its keyframes and seconds", kept)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{2}) {
		t.Errorf("after reading film 1 through, a sweep finds %v, want only film 2", ids)
	}
	job := jobs.Job{ID: 81, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}}
	if err := harness.readThrough.HandleReadThrough(t.Context(), job); err != nil || len(harness.nodes.commanded) != 0 {
		t.Errorf("reading film 1 through again: %v, with %d commands sent; want nothing asked of the node",
			err, len(harness.nodes.commanded))
	}
}

// FFmpeg's word that a file cannot be read is kept for both the parts it was asked for, and its
// cover is not its picture.
func TestReadThrough_OneReadAsksForBothPartsAndFFmpegsWordIsKeptForBoth(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilmsToReadThrough(t)
	command, err := harness.readThroughWith(t, 82, 2, nodeproto.ReadThroughResult{Failure: "Invalid data found"})
	if err == nil || command == nil || len(command.LoudnessTracks) != 1 || command.VideoTimeline == nil ||
		command.VideoTimeline.VideoStreamIndex != 0 {
		t.Errorf("film 2's read-through asked %+v and ended %v; want one read for its track's loudness and stream "+
			"0's timeline, and FFmpeg's failure", command, err)
	}
	if failed, isKept := harness.timelineOf(t, []byte{0x0c, 0x0d}); !isKept || failed.Failure == nil ||
		failed.KeyframesMsJSON != "[]" {
		t.Errorf("film 2's timeline after FFmpeg failed = %+v (%t); want FFmpeg's word, with no keyframes",
			failed, isKept)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("once FFmpeg could not read film 2, a sweep finds %v; want only film 1", ids)
	}
}

// A player waiting on a film's timeline has its read-through queued at play priority, and a sweep's
// read-through of the film already queued is raised to it, so it runs before background work.
func TestReadThrough_ReadForPlayRunsAtPlayPriority(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelvePictures(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	if queued, err := harness.readThrough.QueueReadThroughs(t.Context()); err != nil || queued != 2 {
		t.Fatalf("the sweep queued %d, %v; want both films, whose pictures have no timeline", queued, err)
	}
	if added, err := harness.readThrough.ReadForPlay(t.Context(), 2); err != nil || added {
		t.Fatalf("ReadForPlay of a film the sweep queued: %t, %v; want it raised, not added", added, err)
	}
	runner := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	command := harness.nextCommand(t)
	if command.JobID == 0 || command.Priority != jobs.PriorityPlayNow || !command.IsPlayDriven ||
		command.ReadThrough == nil || command.ReadThrough.Video.PayloadHash != "0c0d" {
		t.Errorf("the first read-through sent = %+v; want film 2's, at play priority", command)
	}
	harness.answerReadThrough(t, command.JobID, nodeproto.ReadThroughResult{Refusal: "a test"})
	if err := testutil.Receive(t, ran, "the read-through runner to end"); err != nil {
		t.Fatal(err)
	}
}
