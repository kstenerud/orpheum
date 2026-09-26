// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// renditionFile puts one file of film 2's remux, in its current run, on the fake node.
func (harness testHarness) renditionFile(t *testing.T, name, contents string) {
	t.Helper()
	rendition, err := harness.cache.Read().GetRendition(t.Context(),
		cachegen.GetRenditionParams{SourceFile: 2, Profile: "remux"})
	if err != nil {
		t.Fatal(err)
	}
	harness.bytes.mutex.Lock()
	defer harness.bytes.mutex.Unlock()
	harness.bytes.files[nodeproto.PathBytesPrepared+"2/remux/"+rendition.RunID+"/"+name] = contents
}

func TestPrepare_APreparedPlaybackIsTooEarlyUntilItsFileExists(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(2))
	if decided.Kind != "hls" || !strings.HasSuffix(decided.URL, "/master.m3u8") ||
		!strings.HasPrefix(decided.URL, Prefix+"/bytes/media/hls/") {
		t.Fatalf("the decision = %+v, want a rendition's master playlist", decided)
	}
	early := harness.fetch(t, decided.URL, user, true, "")
	if early.status != http.StatusTooEarly || early.header.Get("Retry-After") != "1" ||
		!strings.Contains(early.body, `"code":"preparing"`) {
		t.Errorf("before the first segment: %d %v %s, want 425 preparing with Retry-After 1",
			early.status, early.header, early.body)
	}
	harness.renditionFile(t, "master.m3u8", "#EXTM3U\n")
	master := harness.fetch(t, decided.URL, user, true, "")
	if master.status != http.StatusOK || master.body != "#EXTM3U\n" ||
		master.header.Get("Content-Type") != "application/vnd.apple.mpegurl" ||
		master.header.Get("Cache-Control") != "no-cache" {
		t.Errorf("once written: %d %v %q, want the playlist, not cached while it grows",
			master.status, master.header, master.body)
	}
	base := strings.TrimSuffix(decided.URL, "master.m3u8")
	harness.renditionFile(t, "audio0-boost/seg00001.m4s", "segment")
	if segment := harness.fetch(t, base+"audio0-boost/seg00001.m4s", user, true, ""); segment.status !=
		http.StatusOK || segment.header.Get("Content-Type") != "video/iso.segment" {
		t.Errorf("a segment: %d %v", segment.status, segment.header)
	}
	if odd := harness.fetch(t, base+"audio0/notes.txt", user, true, ""); odd.status != http.StatusNotFound {
		t.Errorf("a name that is not a rendition's: %d, want 404", odd.status)
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(decided.URL, Prefix+"/bytes/media/hls/"), "/")
	if direct := harness.fetch(t, Prefix+"/bytes/media/direct/"+token, user, true, ""); direct.status !=
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
	if len(decided.Audio) != 2 || !decided.Audio[1].IsDialogueBoost || !decided.IsPreparing {
		t.Fatalf("the decision = %+v, want a standard and a boosted mix, preparing", decided)
	}
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: 77, Kind: jobs.KindPrepare, Subject: 1}
	go func() { finished <- playback.HandlePrepare(t.Context(), job) }()
	command := <-harness.nodes.commanded
	sent := command.Prepare
	if command.Type != nodeproto.CommandPrepare || sent == nil || sent.Job != 77 ||
		!strings.HasPrefix(sent.RenditionDir, "2/remux/") ||
		sent.Source.Root != "/media" || string(sent.Source.Path) != "Film (2002).mkv" ||
		sent.Source.Hash != "0c0d" || len(sent.Audio) != 1 || sent.Audio[0].Channels != 8 {
		t.Fatalf("the node was sent %+v, want the MKV by root, path and hash, with its 8-channel track", command)
	}
	running := nodeproto.PrepareReport{Job: 77, State: nodeproto.PrepareRunning, PreparedMs: 4000}
	playback.Reported(t.Context(), 1, running)
	playback.Reported(t.Context(), 1, nodeproto.PrepareReport{Job: 77, State: nodeproto.PrepareDone, SizeBytes: 1234})
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
	job := jobs.Job{ID: 78, Kind: jobs.KindPrepare, Subject: 1}
	go func() { finished <- playback.HandlePrepare(t.Context(), job) }()
	<-harness.nodes.commanded
	failed := nodeproto.PrepareReport{Job: 78, State: nodeproto.PrepareFailed, Error: "ffmpeg: bad"}
	playback.Reported(t.Context(), 1, failed)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "ffmpeg: bad") {
		t.Errorf("the job ended %v, want FFmpeg's failure", err)
	}
	playlist := harness.fetch(t, decided.URL, user, true, "")
	if playlist.status != http.StatusBadGateway || !strings.Contains(playlist.body, `"code":"prepare_failed"`) {
		t.Errorf("a failed rendition's playlist: %d %s, want prepare_failed", playlist.status, playlist.body)
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
		first <- playback.HandlePrepare(t.Context(), jobs.Job{ID: 80, Kind: jobs.KindPrepare, Subject: 1})
	}()
	<-harness.nodes.commanded
	playback.Reported(t.Context(), 1, nodeproto.PrepareReport{Job: 80, State: nodeproto.PrepareDone, SizeBytes: 9})
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	// Complete, but the node has no playlist for it: it was evicted, or its directory lost.
	lost := harness.fetch(t, decided.URL, user, true, "")
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
	playback := harness.server.dependencies.Load().Playback
	run := func(job int64) nodeproto.Prepare {
		finished := make(chan error, 1)
		go func() {
			finished <- playback.HandlePrepare(t.Context(), jobs.Job{ID: job, Kind: jobs.KindPrepare, Subject: 1})
		}()
		command := <-harness.nodes.commanded
		playback.Reported(t.Context(), 1, nodeproto.PrepareReport{Job: job, State: nodeproto.PrepareDone, SizeBytes: 9})
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		return *command.Prepare
	}
	first := run(81)
	if first.SegmentLengthSeconds != 2 {
		t.Errorf("the first copy asked for %d-second segments, want hls_segment_length_s's 2",
			first.SegmentLengthSeconds)
	}
	if same := harness.play(t, user, chrome(2)); same.IsPreparing {
		t.Errorf("played again with the same setting: %+v, want the copy as it is", same)
	}
	storetest.ExecRaw(t, harness.database,
		"UPDATE setting SET value = '6', is_modified = 1 WHERE scope = 'server' AND key = 'hls_segment_length_s'")
	if changed := harness.play(t, user, chrome(2)); !changed.IsPreparing {
		t.Errorf("played after hls_segment_length_s changed: %+v, want the copy made again", changed)
	}
	again := run(82)
	if again.SegmentLengthSeconds != 6 {
		t.Errorf("the copy made again asked for %d-second segments, want 6", again.SegmentLengthSeconds)
	}
	// A new making writes where no earlier one did, so no request reaches the old copy's files.
	if again.RenditionDir == first.RenditionDir || !strings.HasPrefix(again.RenditionDir, "2/remux/") {
		t.Errorf("made again in %q after %q, want a new run under 2/remux", again.RenditionDir, first.RenditionDir)
	}
}
