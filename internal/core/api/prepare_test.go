// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: 77, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	go func() { finished <- playback.HandlePrepare(t.Context(), job) }()
	command := <-harness.nodes.commanded
	sent := command.Prepare
	if command.Type != nodeproto.CommandPrepare || sent == nil || sent.JobID != 77 ||
		!strings.HasPrefix(sent.RenditionDir, "2/remux/") ||
		sent.Video.RootPath != "/media" || string(sent.Video.Path) != "Film (2002).mkv" ||
		sent.Video.PayloadHash != "0c0d" || len(sent.AudioTracks) != 1 || sent.AudioTracks[0].ChannelCount != 8 {
		t.Fatalf("the node was sent %+v, want the MKV by root, path and hash, with its 8-channel track", command)
	}
	running := nodeproto.PrepareReport{JobID: 77, State: nodeproto.PrepareRunning, PreparedThroughMs: 4000}
	playback.TakePrepareReport(t.Context(), 1, running)
	done := nodeproto.PrepareReport{JobID: 77, State: nodeproto.PrepareDone, SizeBytes: 1234}
	playback.TakePrepareReport(t.Context(), 1, done)
	if err := <-finished; err != nil {
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
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: 78, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	go func() { finished <- playback.HandlePrepare(t.Context(), job) }()
	<-harness.nodes.commanded
	failed := nodeproto.PrepareReport{JobID: 78, State: nodeproto.PrepareFailed, Failure: "ffmpeg: bad"}
	playback.TakePrepareReport(t.Context(), 1, failed)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "ffmpeg: bad") {
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
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: 79, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}}
	go func() { finished <- playback.HandlePrepare(t.Context(), job) }()
	<-harness.nodes.commanded
	refused := nodeproto.PrepareReport{JobID: 79, State: nodeproto.PrepareRefused, Refusal: "the disk has no room"}
	playback.TakePrepareReport(t.Context(), 1, refused)
	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "the disk has no room") {
			t.Errorf("the job ended %v, want the node's refusal, to be tried again", err)
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

func TestPrepare_ARenditionItsNodeNoLongerHasIsPreparedAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	playback := harness.server.dependencies.Load().Playback
	first := make(chan error, 1)
	go func() {
		first <- playback.HandlePrepare(t.Context(), jobs.Job{
			ID: 80, Kind: jobs.KindPrepare,
			Target: jobs.Target{RenditionID: 1},
		})
	}()
	<-harness.nodes.commanded
	done := nodeproto.PrepareReport{JobID: 80, State: nodeproto.PrepareDone, SizeBytes: 9}
	playback.TakePrepareReport(t.Context(), 1, done)
	if err := <-first; err != nil {
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
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE setting SET value_json = '6', is_modified = 1 WHERE key = 'hls_segment_length_seconds'")
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
		`UPDATE file SET probe_json = json_set(probe_json, '$.streams[1].language', 'de') WHERE id = 2`)
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
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	go func() {
		finished <- playback.HandlePrepare(t.Context(), jobs.Job{
			ID: jobID, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1},
		})
	}()
	command := <-harness.nodes.commanded
	done := nodeproto.PrepareReport{JobID: jobID, State: nodeproto.PrepareDone, SizeBytes: 9}
	playback.TakePrepareReport(t.Context(), 1, done)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	return *command.Prepare
}
