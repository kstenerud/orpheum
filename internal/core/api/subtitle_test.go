// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// subtitleStreamID is the stream row of film 2's ASS track, as shelveSubtitles records it.
const subtitleStreamID = 5

// shelveSubtitles records film 2's subtitle tracks as a scan would: its ASS and its PGS.
func (harness testHarness) shelveSubtitles(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.database,
		`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec) VALUES
		 (5, 2, 2, 2, 'subtitle', 'ass'), (6, 2, 2, 3, 'subtitle', 'hdmv_pgs_subtitle')`)
}

// extract runs the extraction job of film 2's ASS track, answering the node's command with
// answer, and returns the command and how the job ended.
func (harness testHarness) extract(
	t *testing.T, jobID int64, answer nodeproto.SubtitleText,
) (nodeproto.Command, error) {
	t.Helper()
	playback := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	job := jobs.Job{ID: jobID, Kind: jobs.KindExtractSubtitle, Subject: subtitleStreamID}
	go func() { finished <- playback.HandleSubtitle(t.Context(), job) }()
	command := <-harness.nodes.commanded
	answer.Job = jobID
	playback.Extracted(t.Context(), 1, answer)
	return command, <-finished
}

// subtitleJobs counts the extraction jobs ever queued for film 2's ASS track.
func (harness testHarness) subtitleJobs(t *testing.T) int {
	t.Helper()
	all, err := harness.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, job := range all {
		if job.Kind == string(jobs.KindExtractSubtitle) && job.Subject != nil && *job.Subject == subtitleStreamID {
			count++
		}
	}
	return count
}

// finishJobs marks every job done, as the queue's runner would once its handler returned.
func (harness testHarness) finishJobs(t *testing.T) {
	t.Helper()
	all, err := harness.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	err = harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		for _, job := range all {
			finished := cachegen.FinishJobParams{State: "done", ID: job.ID}
			if finishErr := queries.FinishJob(t.Context(), finished); finishErr != nil {
				return fmt.Errorf("finish job %d: %w", job.ID, finishErr)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// offeredASSTrack is the one track a playback of film 2 offers, once checked to be its ASS
// track and not its PGS.
func offeredASSTrack(t *testing.T, decided decision) subtitleTrack {
	t.Helper()
	if len(decided.Subtitles) != 1 {
		t.Fatalf("the subtitles offered = %+v, want the ASS track only, not the PGS", decided.Subtitles)
	}
	track := decided.Subtitles[0]
	if track.StreamIndex != 2 || track.Language != "en" || !track.IsDefault ||
		!strings.HasSuffix(track.ASSURL, "/2.ass") || !strings.HasSuffix(track.VTTURL, "/2.vtt") {
		t.Fatalf("the track offered = %+v, want stream 2, English, the default", track)
	}
	return track
}

func TestSubtitles_ATextTrackIsOfferedAndQueuedWhenFirstPlayedAndIsTooEarlyUntilExtracted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	track := offeredASSTrack(t, harness.play(t, user, chrome(2)))
	if queued := harness.subtitleJobs(t); queued != 1 {
		t.Errorf("playing the file queued %d extractions of its text track, want 1", queued)
	}
	early := harness.fetch(t, track.ASSURL, user, true, "")
	if early.status != http.StatusTooEarly || early.header.Get("Retry-After") != "1" ||
		!strings.Contains(early.body, `"code":"preparing"`) {
		t.Errorf("before it is extracted: %d %v %s, want 425 preparing", early.status, early.header, early.body)
	}
	base := strings.TrimSuffix(track.ASSURL, "2.ass")
	for _, name := range []string{"2.srt", "3.ass", "notes"} {
		if odd := harness.fetch(t, base+name, user, true, ""); odd.status != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, odd.status)
		}
	}
}

// checkExtractCommand fails unless command asks for stream 2 of film 2 by root, path and hash.
func checkExtractCommand(t *testing.T, command nodeproto.Command, jobID int64) {
	t.Helper()
	sent := command.ExtractSubtitle
	if command.Type != nodeproto.CommandExtractSubtitle || sent == nil || sent.Job != jobID || sent.Sidecar != nil ||
		sent.Video == nil || sent.Video.Root != "/media" || string(sent.Video.Path) != "Film (2002).mkv" ||
		sent.Video.Hash != "0c0d" || sent.StreamIndex != 2 {
		t.Fatalf("the node was sent %+v, want stream 2 of the MKV by root, path and hash", command)
	}
}

func TestSubtitles_AnExtractedTrackIsServedAsASSAndWebVTTAndNotExtractedAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	track := offeredASSTrack(t, harness.play(t, user, chrome(2)))
	extracted := nodeproto.SubtitleText{ASS: []byte("[Script Info]"), VTT: []byte("WEBVTT")}
	command, err := harness.extract(t, 90, extracted)
	if err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	checkExtractCommand(t, command, 90)
	for url, want := range map[string]struct{ contentType, body string }{
		track.ASSURL: {"text/x-ssa; charset=utf-8", "[Script Info]"},
		track.VTTURL: {"text/vtt; charset=utf-8", "WEBVTT"},
	} {
		served := harness.fetch(t, url, user, true, "")
		if served.status != http.StatusOK || served.body != want.body ||
			served.header.Get("Content-Type") != want.contentType {
			t.Errorf("%s: %d %v %q, want %q as %s",
				url, served.status, served.header, served.body, want.body, want.contentType)
		}
	}
	harness.finishJobs(t)
	harness.play(t, user, chrome(2))
	if queued := harness.subtitleJobs(t); queued != 1 {
		t.Errorf("playing again queued %d extractions in all, want none after the first", queued)
	}
}

func TestSubtitles_AFailedExtractionSaysSoAndIsTriedAgainWhenNextPlayed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	if _, err := harness.extract(t, 91, nodeproto.SubtitleText{Refused: "ffmpeg: bad"}); err == nil ||
		!strings.Contains(err.Error(), "ffmpeg: bad") {
		t.Errorf("the job ended %v, want FFmpeg's failure", err)
	}
	failed := harness.fetch(t, decided.Subtitles[0].ASSURL, user, true, "")
	if failed.status != http.StatusBadGateway || !strings.Contains(failed.body, `"code":"prepare_failed"`) ||
		!strings.Contains(failed.body, "ffmpeg: bad") {
		t.Errorf("a failed track: %d %s, want prepare_failed with why", failed.status, failed.body)
	}
	again := harness.play(t, user, chrome(2))
	if early := harness.fetch(t, again.Subtitles[0].ASSURL, user, true, ""); early.status != http.StatusTooEarly {
		t.Errorf("played again after a failure: %d, want 425 while it is tried again", early.status)
	}
}
