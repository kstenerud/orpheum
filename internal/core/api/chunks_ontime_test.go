// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A film whose file has no index of its keyframes, as its node says at a player's read of the index
// alone, is offered cut on time at the player's next ask, encoded, its picture not copied, and read
// through ahead of the sweep for its keyframes; the playback keeps that grid once they are known,
// while a new playback is offered the picture copied (D269).
func TestChunks_AFilmWithNoIndexIsCutOnTimeAndItsPlaybackStaysSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	// A film cut on time is cut by its length, which its probe says: thirty seconds.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file SET probe_json = json_set(probe_json, '$.format.durationMs', 30000) WHERE id = 1`)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	if early := harness.getBytePath(t, base+"master.m3u8", user, true, ""); early.status != http.StatusTooEarly {
		t.Fatalf("before film 1's index is read: %d, want 425", early.status)
	}
	harness.findFilmOneHasNoIndex(t)
	onTime := harness.getBytePath(t, base+"master.m3u8", user, true, "")
	if onTime.status != http.StatusOK || strings.Contains(onTime.body, "picture/") ||
		!strings.Contains(onTime.body, "q1200k480p/") {
		t.Fatalf("film 1 with no index: %d\n%s\nwant its qualities, and no picture copied", onTime.status, onTime.body)
	}
	if ahead := harness.openJobs(t, jobs.KindReadThrough); len(ahead) != 1 ||
		ahead[0].Priority != jobs.PriorityAheadOfSweep {
		t.Errorf("the read-throughs queued = %+v; want film 1's ahead of the sweep", ahead)
	}
	harness.keepFilmOnesTimeline(t)
	if kept := harness.getBytePath(t, base+"master.m3u8", user, true, ""); kept.status != http.StatusOK ||
		strings.Contains(kept.body, "picture/") {
		t.Errorf("film 1 to the playback that began on time, its keyframes known: %d\n%s\nwant it still on time",
			kept.status, kept.body)
	}
	fresh := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	if copied := harness.getBytePath(t, fresh+"master.m3u8", user, true, ""); copied.status != http.StatusOK ||
		!strings.Contains(copied.body, "picture/") {
		t.Errorf("film 1 to a new playback, its keyframes known: %d\n%s\nwant its picture copied",
			copied.status, copied.body)
	}
}

// findFilmOneHasNoIndex runs the read-through a player's ask for film 1's chunks queued: it asks the
// picture's timeline from the index alone, and the node says the file has none.
func (harness testHarness) findFilmOneHasNoIndex(t *testing.T) {
	t.Helper()
	read := harness.openJobs(t, jobs.KindReadThrough)
	if len(read) != 1 {
		t.Fatalf("the read-throughs queued = %+v; want film 1's", read)
	}
	finished := harness.handle(t, harness.readThrough.HandleReadThrough, jobs.Job{
		ID: read[0].ID, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}, Priority: jobs.PriorityPlayNow,
	})
	command := harness.nextCommand(t).ReadThrough
	if command == nil || command.VideoTimeline == nil || !command.VideoTimeline.IsFromIndexOnly {
		t.Fatalf("a player's read-through asked %+v; want the picture's timeline from the index alone", command)
	}
	harness.answerReadThrough(t, read[0].ID, nodeproto.ReadThroughResult{HasNoIndex: true})
	if err := testutil.Receive(t, finished, "the read-through job to end"); err != nil {
		t.Fatal(err)
	}
	harness.finishJobs(t) // as the queue's runner does once its handler returns
}

// The sweep reads a file of a container whose index the node does not read, as an AVI, ahead of the
// others, its keyframes known no other way (D269 §4).
func TestReadThrough_TheSweepReadsAFileWithNoIndexReaderFirst(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelvePictures(t)
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file SET reported_path_raw = CAST('/media/Film (2002).avi' AS BLOB) WHERE id = 2`)
	if queued, err := harness.readThrough.QueueReadThroughs(t.Context()); err != nil || queued != 2 {
		t.Fatalf("the sweep queued %d, %v; want both films", queued, err)
	}
	priorities := map[int64]int64{}
	for _, job := range harness.openJobs(t, jobs.KindReadThrough) {
		if job.FileID != nil {
			priorities[*job.FileID] = job.Priority
		}
	}
	if priorities[1] != jobs.PriorityBackground || priorities[2] != jobs.PriorityAheadOfSweep {
		t.Errorf("the sweep's priorities by file = %v; want the MP4 in the background and the AVI ahead", priorities)
	}
}
