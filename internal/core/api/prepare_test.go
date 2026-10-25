// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// putRenditionFile puts one file of film 2's remux, in its current run, on the fake node.
func (harness testHarness) putRenditionFile(t *testing.T, name, contents string) {
	t.Helper()
	rendition, err := harness.cache.Read().GetRenditionOfFileAndProfile(t.Context(),
		cachegen.GetRenditionOfFileAndProfileParams{SourceFile: 2, Profile: "remux"})
	if err != nil {
		t.Fatal(err)
	}
	harness.nodeBytes.mutex.Lock()
	defer harness.nodeBytes.mutex.Unlock()
	harness.nodeBytes.files[nodeproto.PathBytesPrepared+"2/remux/"+rendition.RunID+"/"+name] = contents
}

func TestPrepare_APreparedPlaybackIsTooEarlyUntilItsFileExists(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(2))
	if decided.Delivery != "hls" || !strings.HasSuffix(decided.URL, "/master.m3u8") ||
		!strings.HasPrefix(decided.URL, Prefix+"/bytes/media/hls/") {
		t.Fatalf("the decision = %+v, want a rendition's master playlist", decided)
	}
	early := harness.getBytePath(t, decided.URL, user, true, "")
	if early.status != http.StatusTooEarly || early.header.Get("Retry-After") != "1" ||
		!strings.Contains(early.body, `"code":"preparing"`) {
		t.Errorf("before the first segment: %d %v %s, want 425 preparing with Retry-After 1",
			early.status, early.header, early.body)
	}
	harness.putRenditionFile(t, "master.m3u8", "#EXTM3U\n")
	master := harness.getBytePath(t, decided.URL, user, true, "")
	if master.status != http.StatusOK || master.body != "#EXTM3U\n" ||
		master.header.Get("Content-Type") != "application/vnd.apple.mpegurl" ||
		master.header.Get("Cache-Control") != "no-cache" {
		t.Errorf("once written: %d %v %q, want the playlist, not cached while it grows",
			master.status, master.header, master.body)
	}
	base := strings.TrimSuffix(decided.URL, "master.m3u8")
	harness.putRenditionFile(t, "audio0-boost/seg00001.m4s", "segment")
	if segment := harness.getBytePath(t, base+"audio0-boost/seg00001.m4s", user, true, ""); segment.status !=
		http.StatusOK || segment.header.Get("Content-Type") != "video/iso.segment" {
		t.Errorf("a segment: %d %v", segment.status, segment.header)
	}
	if odd := harness.getBytePath(t, base+"audio0/notes.txt", user, true, ""); odd.status != http.StatusNotFound {
		t.Errorf("a name that is not a rendition's: %d, want 404", odd.status)
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(decided.URL, Prefix+"/bytes/media/hls/"), "/")
	if direct := harness.getBytePath(t, Prefix+"/bytes/media/direct/"+token, user, true, ""); direct.status !=
		http.StatusNotFound {
		t.Errorf("a prepared playback's token as a direct one: %d, want 404", direct.status)
	}
}

func TestPrepare_AJobCommandsTheNodeAndRecordsTheRenditionComplete(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	decided := harness.play(t, user, chrome(2))
	if len(decided.Audio) != 2 || !decided.Audio[1].IsDialogueBoosted || !decided.IsPreparing {
		t.Fatalf("the decision = %+v, want a standard and a boosted mix, preparing", decided)
	}
	job := jobs.Job{ID: 77, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	finished := harness.handle(t, harness.renditions.HandlePrepare, job)
	command := harness.nextCommand(t)
	sent := command.Prepare
	if command.Type != nodeproto.CommandPrepare || sent == nil || command.JobID != 77 ||
		!strings.HasPrefix(sent.RenditionDir, "2/remux/") ||
		sent.Video.RootPath != "/media" || string(sent.Video.Path) != "Film (2002).mkv" ||
		sent.Video.PayloadHash != "0c0d" || len(sent.AudioTracks) != 1 || sent.AudioTracks[0].ChannelCount != 8 {
		t.Fatalf("the node was sent %+v, want the MKV by root, path and hash, with its 8-channel track", command)
	}
	harness.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: 77, AttemptToken: command.AttemptToken, State: nodeproto.ProgressRunning, PreparedThroughMs: 4000,
	})
	done := nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 1234}
	harness.answerPrepare(t, 77, done)
	if err := testutil.Receive(t, finished, "the prepare job to end"); err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	again := harness.play(t, user, chrome(2))
	if again.IsPreparing {
		t.Errorf("after the node said done: %+v, want the rendition complete", again)
	}
}

func TestPrepare_AFailedRenditionFailsThePlaybackAndIsNotRetried(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	decided := harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 78, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	finished := harness.handle(t, harness.renditions.HandlePrepare, job)
	harness.nextCommand(t)
	failed := nodeproto.PrepareReport{State: nodeproto.PrepareFailed, Failure: "ffmpeg: bad"}
	harness.answerPrepare(t, 78, failed)
	err := testutil.Receive(t, finished, "the prepare job to end")
	if err == nil || !strings.Contains(err.Error(), "ffmpeg: bad") {
		t.Errorf("the job ended %v, want FFmpeg's failure", err)
	}
	playlist := harness.getBytePath(t, decided.URL, user, true, "")
	if playlist.status != http.StatusBadGateway || !strings.Contains(playlist.body, `"code":"prepare_failed"`) {
		t.Errorf("a failed rendition's playlist: %d %s, want prepare_failed", playlist.status, playlist.body)
	}
}

func TestPrepare_ARefusedPrepareIsTriedAgainAndThePlaybackKeepsWaiting(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	decided := harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 79, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	finished := harness.handle(t, harness.renditions.HandlePrepare, job)
	harness.nextCommand(t)
	refused := nodeproto.PrepareReport{
		State: nodeproto.PrepareRefused, Refusal: "not the one named", RefusalCode: nodeproto.RefusedChanged,
	}
	harness.answerPrepare(t, 79, refused)
	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "not the one named") ||
			!strings.Contains(err.Error(), "changed since the scan") {
			t.Errorf("the job ended %v, want the node's refusal for a changed file, said so, to be tried again", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the job did not end at the node's refusal")
	}
	playlist := harness.getBytePath(t, decided.URL, user, true, "")
	if playlist.status != http.StatusTooEarly {
		t.Errorf("a refused rendition's playlist: %d %s, want 425 while it is tried again, not failed",
			playlist.status, playlist.body)
	}
}

func TestPrepare_ARefusalForWantOfRoomParksTheNodesPreparesUntilItsStatusShowsRoom(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 79, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	finished := harness.handle(t, harness.renditions.HandlePrepare, job)
	harness.nextCommand(t)
	harness.answerPrepare(t, 79, nodeproto.PrepareReport{
		State: nodeproto.PrepareRefused, Refusal: "the disk is past its limit",
		RefusalCode: nodeproto.RefusedNoRoom,
	})
	err := testutil.Receive(t, finished, "the prepare job to end")
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.PreparedDirectoryDestination(1) || !parking.AwaitsSignal ||
		!strings.Contains(err.Error(), "past its limit") {
		t.Errorf("a prepare refused for want of room: %v (%+v); want it parked on the node's prepared directory "+
			"until its status shows room (D231 §5)", err, parking)
	}
}

func TestPrepare_APrepareForANodeThatIsNotConnectedParksOnItAndSendsNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, user, chrome(2))
	harness.nodes.isAway.Store(true)
	job := jobs.Job{ID: 80, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	err := harness.renditions.HandlePrepare(t.Context(), job)
	if parking, isParked := jobqueue.ParkingOf(err); !isParked || parking.Destination != jobs.NodeDestination(1) {
		t.Errorf("a prepare for a node that is away: %v; want it parked on the node (D231 §11)", err)
	}
	if len(harness.nodes.commanded) != 0 {
		t.Error("a command was sent to a node that is away")
	}
}

func TestPrepare_ARenditionItsNodeNoLongerHasIsPreparedAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	first := harness.handle(t, harness.renditions.HandlePrepare, jobs.Job{
		ID: 80, Kind: jobs.KindPrepare,
		Target: jobs.Target{RenditionID: 1},
	})
	harness.nextCommand(t)
	done := nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 9}
	harness.answerPrepare(t, 80, done)
	if err := testutil.Receive(t, first, "the first prepare job to end"); err != nil {
		t.Fatal(err)
	}
	// Complete, but the node has no playlist for it: it was evicted, or its directory lost.
	lost := harness.getBytePath(t, decided.URL, user, true, "")
	if lost.status != http.StatusTooEarly {
		t.Errorf("a complete rendition the node lost: %d %s, want 425 while it is prepared again",
			lost.status, lost.body)
	}
	if again := harness.play(t, user, chrome(2)); !again.IsPreparing {
		t.Errorf("after it was lost: %+v, want it being prepared again", again)
	}
}

func TestPrepare_ACopyMadeWithAnotherSegmentLengthIsMadeAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	first := harness.prepareRenditionOne(t, 81)
	if first.SegmentLengthSeconds != 2 {
		t.Errorf("the first copy asked for %d-second segments, want hls_segment_length_seconds's 2",
			first.SegmentLengthSeconds)
	}
	if same := harness.play(t, user, chrome(2)); same.IsPreparing {
		t.Errorf("played again with the same setting: %+v, want the copy as it is", same)
	}
	harness.setSetting(t, "hls_segment_length_seconds", "6")
	if changed := harness.play(t, user, chrome(2)); !changed.IsPreparing {
		t.Errorf("played after hls_segment_length_seconds changed: %+v, want the copy made again", changed)
	}
	again := harness.prepareRenditionOne(t, 82)
	if again.SegmentLengthSeconds != 6 {
		t.Errorf("the copy made again asked for %d-second segments, want 6", again.SegmentLengthSeconds)
	}
	// A new making writes where no earlier one did, so no request reaches the old copy's files.
	if again.RenditionDir == first.RenditionDir || !strings.HasPrefix(again.RenditionDir, "2/remux/") {
		t.Errorf("made again in %q after %q, want a new run under 2/remux", again.RenditionDir, first.RenditionDir)
	}
}

// A relabelled audio track keeps the file's payload hash (D179), but the master playlist names
// each track's language, so a copy made before the relabel is made again with the new one.
func TestPrepare_ACopyMadeBeforeAnAudioTrackWasRelabelledIsMadeAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	first := harness.prepareRenditionOne(t, 83)
	if len(first.AudioTracks) != 1 || first.AudioTracks[0].Language != "und" {
		t.Fatalf("the first copy's audio = %+v, want its one track untagged", first.AudioTracks)
	}
	if same := harness.play(t, user, chrome(2)); same.IsPreparing {
		t.Errorf("played again unchanged: %+v, want the copy as it is", same)
	}
	// A rescan after mkvpropedit tagged the track German: the same payload, a new probe.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_set(probe_json, '$.streams[1].language', 'de') WHERE file_id = 2`)
	if relabelled := harness.play(t, user, chrome(2)); !relabelled.IsPreparing {
		t.Errorf("played after the track was relabelled: %+v, want the copy made again", relabelled)
	}
	again := harness.prepareRenditionOne(t, 84)
	if len(again.AudioTracks) != 1 || again.AudioTracks[0].Language != "de" {
		t.Errorf("the copy made again tagged its audio %+v, want German", again.AudioTracks)
	}
	if same := harness.play(t, user, chrome(2)); same.IsPreparing {
		t.Errorf("played again after the new copy: %+v, want it as it is", same)
	}
}

// prepareRenditionOne runs a prepare job for rendition 1 as jobID, has the fake node say it is done,
// and returns the command the node was sent.
func (harness testHarness) prepareRenditionOne(t *testing.T, jobID int64) nodeproto.Prepare {
	t.Helper()
	finished := harness.handle(t, harness.renditions.HandlePrepare, jobs.Job{
		ID: jobID, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1},
	})
	command := harness.nextCommand(t)
	done := nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 9}
	harness.answerPrepare(t, jobID, done)
	if err := testutil.Receive(t, finished, "the prepare job to end"); err != nil {
		t.Fatal(err)
	}
	return *command.Prepare
}

// A player asking for a rendition still being prepared is a person still waiting for it: its job
// keeps its play priority for ten minutes from the last ask, and falls to background ten minutes
// after the player stops asking (D234 §3, S87).
func TestPrepare_APlayerAskingForTheRenditionKeepsItsPrepareWanted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(2))
	for range 3 { // half an hour of a player asking every nine minutes
		harness.clock.Advance(9 * time.Minute)
		if early := harness.getBytePath(t, decided.URL, user, true, ""); early.status != http.StatusTooEarly {
			t.Fatalf("the playlist while preparing: %d, want 425", early.status)
		}
	}
	// Film 2's start also read its keyframes for its ladder (D260 §1), which no player asks for here.
	prepares := harness.openJobs(t, jobs.KindPrepare)
	if len(prepares) != 1 {
		t.Fatalf("the prepares open = %+v; want film 2's", prepares)
	}
	isPrepare := func(job jobs.Demoted) bool { return job.ID == prepares[0].ID }
	demoted, err := harness.queue.DemoteUnwanted(t.Context())
	if err != nil || slices.ContainsFunc(demoted, isPrepare) {
		t.Errorf("while the player asks, demoted %+v (%v), want not the prepare", demoted, err)
	}
	harness.clock.Advance(11 * time.Minute)
	demoted, err = harness.queue.DemoteUnwanted(t.Context())
	if err != nil || !slices.ContainsFunc(demoted, isPrepare) {
		t.Errorf("ten minutes after the player stopped, demoted %+v (%v), want the prepare", demoted, err)
	}
}

// A prepare job that fails for good, whatever ended it, fails its rendition's making: the player is
// told it failed rather than waiting on a making no job does, and the next play prepares it again
// (G8).
func TestPrepare_APrepareJobThatFailsForGoodFailsItsRendition(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 80, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	if err := harness.renditions.RecordPrepareFailure(t.Context(), job, "refused five times"); err != nil {
		t.Fatal(err)
	}
	playlist := harness.getBytePath(t, decided.URL, user, true, "")
	if playlist.status != http.StatusBadGateway || !strings.Contains(playlist.body, `"code":"prepare_failed"`) {
		t.Errorf("the playlist of a rendition whose job failed: %d %s, want prepare_failed", playlist.status,
			playlist.body)
	}
	if again := harness.play(t, user, chrome(2)); !again.IsPreparing {
		t.Errorf("the next play = %+v; want the copy prepared again", again)
	}
}

// A making restarted while an earlier run's job is with its node gets a job of its own, queued after
// that one, rather than being taken for the earlier job's work and left with none (G13).
func TestPrepare_AMakingRestartedWhileItsJobIsWithTheNodeIsQueuedAfterIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	prepares := harness.queue.NewRunner(jobs.KindPrepare, harness.renditions.HandlePrepare)
	if err := prepares.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	first := newestPrepareJob(t, harness.listedJobs(t, user))
	if first.State != "running" {
		t.Fatalf("the first making's job = %+v; want it with the node", first)
	}
	// The making fails, as a lost copy would, and the next play makes it again under a new run.
	lost := jobs.Job{Target: jobs.Target{RenditionID: 1}}
	if err := harness.renditions.RecordPrepareFailure(t.Context(), lost, "lost"); err != nil {
		t.Fatal(err)
	}
	harness.play(t, user, chrome(2))
	var states []string
	for _, job := range harness.listedJobs(t, user) {
		if job.Kind == "prepare" {
			states = append(states, job.State)
		}
	}
	slices.Sort(states)
	if !slices.Equal(states, []string{"queued", "running"}) {
		t.Errorf("the prepare jobs are %v; want the first still with the node and the new run's queued after it",
			states)
	}
}
