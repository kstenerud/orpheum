// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// subtitleStreamID is the stream row of film 2's ASS track, as shelveSubtitles records it, and
// subtitleFileID the file it is in, film 2, which its extraction job works on (D340).
const (
	subtitleStreamID = 5
	subtitleFileID   = 2
)

// shelveSubtitles records film 2's subtitle tracks as a scan would: its ASS, English and the
// default, and its PGS; and film 1's own track, which no playback of film 2 may reach.
func (harness testHarness) shelveSubtitles(t *testing.T) {
	t.Helper()
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 5, BytesFile: 2, StreamIndex: 2, Kind: "subtitle", Codec: "ass", Language: "en", DispositionFlags: 1,
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 6, BytesFile: 2, StreamIndex: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle",
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 8, BytesFile: 1, StreamIndex: 2, Kind: "subtitle", Codec: "mov_text", Language: "en",
	})
}

// sidecarStreamID is the stream row of film 2's Spanish subtitle file, as shelveSidecar records it,
// and sidecarFileID the file's.
const (
	sidecarStreamID = 7
	sidecarFileID   = 10
)

// shelveSidecar records "Film (2002).es.srt" beside film 2 as a scan would: the file, attached,
// with its payload hash, and its one track.
func (harness testHarness) shelveSidecar(t *testing.T) {
	t.Helper()
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 10, LibraryID: 1, NodeID: 1, Path: "/media/Film (2002).es.srt", SizeBytes: 5,
		PayloadHash: []byte{0x1e, 0x1f}, Filing: "attached", FilingReason: "subtitle", SidecarKind: "subtitle",
		AttachedToFile: 2,
	})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 7, BytesFile: 10, VideoFile: 2, Kind: "subtitle", Codec: "subrip", ContainerLanguage: "es", Language: "es",
	})
}

// extract runs the extraction job of film 2's text tracks, answering the node's command with
// answer, and returns the command and how the job ended.
func (harness testHarness) extract(
	t *testing.T, jobID int64, answer nodeproto.SubtitleText,
) (nodeproto.Command, error) {
	t.Helper()
	job := jobs.Job{ID: jobID, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: subtitleFileID}}
	finished := harness.handle(t, harness.subtitles.HandleExtractSubtitle, job)
	command := harness.nextCommand(t)
	harness.answerSubtitle(t, jobID, answer)
	//nolint:wrapcheck // the job's own error, which the test reads as it is
	return command, testutil.Receive(t, finished, "the extraction job to end")
}

// trackText is a node's answer that made the text of a video's track at streamIndex.
func trackText(streamIndex int, ass, vtt string) nodeproto.SubtitleText {
	return nodeproto.SubtitleText{
		Tracks: []nodeproto.TrackText{{StreamIndex: streamIndex, ASS: []byte(ass), VTT: []byte(vtt)}},
	}
}

// storeExtractedText keeps text as extracted for the track at streamIndex of the file with
// payloadHash.
func (harness testHarness) storeExtractedText(t *testing.T, payloadHash []byte, streamIndex int64, text string) {
	t.Helper()
	err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.KeepSubtitleText(t.Context(), sqlcgen.KeepSubtitleTextParams{
			SourceHash: payloadHash, StreamIndex: streamIndex, ASS: []byte(text), VTT: []byte(text), MadeAt: 2,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// subtitleJobCount counts the extraction jobs ever queued for film 2's text tracks.
func (harness testHarness) subtitleJobCount(t *testing.T) int {
	t.Helper()
	all, err := harness.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, job := range all {
		if job.Kind == string(jobs.KindExtractSubtitle) && job.FileID != nil && *job.FileID == subtitleFileID {
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
			finished := cachegen.FinishJobParams{State: "done", AttemptCount: job.AttemptCount, ID: job.ID}
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
	if track.StreamID != subtitleStreamID || track.Language != "en" || !track.IsDefault ||
		!strings.HasSuffix(track.ASSURL, "/5.ass") || !strings.HasSuffix(track.VTTURL, "/5.vtt") {
		t.Fatalf("the track offered = %+v, want stream 5, English, the default", track)
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
	if queued := harness.subtitleJobCount(t); queued != 1 {
		t.Errorf("playing the file queued %d extractions of its text track, want 1", queued)
	}
	early := harness.getBytePath(t, track.ASSURL, user, true, "")
	if early.status != http.StatusTooEarly || early.header.Get("Retry-After") != "1" ||
		!strings.Contains(early.body, `"code":"preparing"`) {
		t.Errorf("before it is extracted: %d %v %s, want 425 preparing", early.status, early.header, early.body)
	}
	base := strings.TrimSuffix(track.ASSURL, "5.ass")
	// 8 is another film's track, extracted: a playback reaches only its own file's.
	harness.storeExtractedText(t, []byte{0x0a, 0x0b}, 2, "film 1's text")
	for _, name := range []string{"5.srt", "6.ass", "8.ass", "99.ass", "notes"} {
		if odd := harness.getBytePath(t, base+name, user, true, ""); odd.status != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, odd.status)
		}
	}
}

// checkExtractCommand fails unless command asks for stream 2 of film 2 by root, path and hash, and
// not its PGS.
func checkExtractCommand(t *testing.T, command nodeproto.Command, jobID int64) {
	t.Helper()
	sent := command.ExtractSubtitle
	if command.Type != nodeproto.CommandExtractSubtitle || sent == nil || command.JobID != jobID ||
		sent.Sidecar != nil || sent.Video == nil || sent.Video.RootPath != "/media" ||
		string(sent.Video.Path) != "Film (2002).mkv" || sent.Video.PayloadHash != "0c0d" ||
		!slices.Equal(sent.StreamIndexes, []int{2}) {
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
	command, err := harness.extract(t, 90, trackText(2, "[Script Info]", "WEBVTT"))
	if err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	checkExtractCommand(t, command, 90)
	for url, want := range map[string]struct{ contentType, body string }{
		track.ASSURL: {"text/x-ssa; charset=utf-8", "[Script Info]"},
		track.VTTURL: {"text/vtt; charset=utf-8", "WEBVTT"},
	} {
		served := harness.getBytePath(t, url, user, true, "")
		if served.status != http.StatusOK || served.body != want.body ||
			served.header.Get("Content-Type") != want.contentType {
			t.Errorf("%s: %d %v %q, want %q as %s",
				url, served.status, served.header, served.body, want.body, want.contentType)
		}
	}
	harness.finishJobs(t)
	harness.play(t, user, chrome(2))
	if queued := harness.subtitleJobCount(t); queued != 1 {
		t.Errorf("playing again queued %d extractions in all, want none after the first", queued)
	}
}

func TestSubtitles_ExtractedTextIsKeptInPersistentDBWhichOutlivesCacheDB(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	if _, err := harness.extract(t, 93, trackText(2, "[Script Info]", "WEBVTT")); err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	// Making it again reads the whole video, so it is not cache.db's to lose (the architecture
	// map's F1; D231 section 3).
	kept, err := harness.database.Read().GetSubtitleText(t.Context(), sqlcgen.GetSubtitleTextParams{
		SourceHash: []byte{0x0c, 0x0d}, StreamIndex: 2,
	})
	if err != nil || string(kept.ASS) != "[Script Info]" || string(kept.VTT) != "WEBVTT" || kept.Failure != nil {
		t.Errorf("persistent.db keeps %+v (%v), want the text the node made", kept, err)
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
	failedTrack := nodeproto.SubtitleText{Tracks: []nodeproto.TrackText{{StreamIndex: 2, Failure: "ffmpeg: bad"}}}
	if _, err := harness.extract(t, 91, failedTrack); err == nil ||
		!strings.Contains(err.Error(), "ffmpeg: bad") {
		t.Errorf("the job ended %v, want FFmpeg's failure", err)
	}
	failed := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if failed.status != http.StatusBadGateway || !strings.Contains(failed.body, `"code":"prepare_failed"`) ||
		!strings.Contains(failed.body, "ffmpeg: bad") {
		t.Errorf("a failed track: %d %s, want prepare_failed with why", failed.status, failed.body)
	}
	again := harness.play(t, user, chrome(2))
	if early := harness.getBytePath(t, again.Subtitles[0].ASSURL, user, true, ""); early.status != http.StatusTooEarly {
		t.Errorf("played again after a failure: %d, want 425 while it is tried again", early.status)
	}
}

func TestSubtitles_ARefusedExtractionIsNotKeptAndTheTrackKeepsWaiting(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	refusal := "the file is not the one named: it changed since the scan"
	if _, err := harness.extract(t, 92, nodeproto.SubtitleText{Refusal: refusal}); err == nil ||
		!strings.Contains(err.Error(), refusal) {
		t.Errorf("the job ended %v, want the node's refusal, to be tried again", err)
	}
	waiting := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if waiting.status != http.StatusTooEarly {
		t.Errorf("a refused track: %d %s, want 425 while it is tried again, not failed", waiting.status, waiting.body)
	}
}

func TestSubtitles_ASubtitleFileBesideTheVideoIsOfferedAndConvertedWhole(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.shelveSidecar(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	if len(decided.Subtitles) != 2 || decided.Subtitles[0].StreamID != subtitleStreamID ||
		decided.Subtitles[1].StreamID != sidecarStreamID || decided.Subtitles[1].Language != "es" {
		t.Fatalf("the subtitles offered = %+v, want the video's own track, then the Spanish file", decided.Subtitles)
	}
	job := jobs.Job{ID: 92, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: sidecarFileID}}
	finished := harness.handle(t, harness.subtitles.HandleExtractSubtitle, job)
	command := harness.nextCommand(t)
	sent := command.ExtractSubtitle
	if sent == nil || sent.Video != nil || sent.Sidecar == nil || sent.Sidecar.RootPath != "/media" ||
		string(sent.Sidecar.Path) != "Film (2002).es.srt" || sent.Sidecar.PayloadHash != "1e1f" {
		t.Fatalf("the node was sent %+v, want the subtitle file whole, by root, path and hash", command)
	}
	extracted := nodeproto.SubtitleText{ASS: []byte("[Script Info] es"), VTT: []byte("WEBVTT es")}
	harness.answerSubtitle(t, 92, extracted)
	if err := testutil.Receive(t, finished, "the extraction job to end"); err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	if served := harness.getBytePath(t, decided.Subtitles[1].ASSURL, user, true, ""); served.status != http.StatusOK ||
		served.body != "[Script Info] es" {
		t.Errorf("the subtitle file's track: %d %q", served.status, served.body)
	}
}

func TestSubtitles_AnExtractionThatGaveUpSaysSoAndTheNextPlayAsksAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	decided := harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 94, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: subtitleFileID}}
	if err := harness.subtitles.RecordFailure(t.Context(), job, "node 1 said nothing five times"); err != nil {
		t.Fatal(err)
	}
	failed := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if failed.status != http.StatusBadGateway || !strings.Contains(failed.body, "said nothing five times") {
		t.Errorf("a track whose extraction gave up: %d %s, want prepare_failed with why (D231 section 10)",
			failed.status, failed.body)
	}
	again := harness.play(t, user, chrome(2))
	if early := harness.getBytePath(t, again.Subtitles[0].ASSURL, user, true, ""); early.status != http.StatusTooEarly {
		t.Errorf("played again: %d, want 425 while it is made again", early.status)
	}
}

// demotedKinds are the kinds of the jobs a demotion of unwanted play work moved to the background.
func demotedKinds(t *testing.T, harness testHarness) []jobs.Kind {
	t.Helper()
	demoted, err := harness.queue.DemoteUnwanted(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	views, err := harness.queue.List(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	kindByID := map[int64]jobs.Kind{}
	for _, view := range views {
		kindByID[view.ID] = view.Kind
	}
	kinds := make([]jobs.Kind, 0, len(demoted))
	for _, job := range demoted {
		kinds = append(kinds, kindByID[job.ID])
	}
	return kinds
}

// A player asking for a text still being extracted is a person waiting for it, so its extraction
// keeps play's priority while the player asks, and falls to the background only once it stops, as a
// player's other play-driven work does (D234 §3).
func TestSubtitles_APlayerAskingForTheTextKeepsItsExtractionWanted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	track := offeredASSTrack(t, harness.play(t, user, chrome(2)))
	for range 3 { // half an hour of a player asking every nine minutes
		harness.clock.Advance(9 * time.Minute)
		if early := harness.getBytePath(t, track.ASSURL, user, true, ""); early.status != http.StatusTooEarly {
			t.Fatalf("the text while it is extracted: %d, want 425", early.status)
		}
	}
	if kinds := demotedKinds(t, harness); slices.Contains(kinds, jobs.KindExtractSubtitle) {
		t.Errorf("while the player asks, demoted %v; want the extraction kept at play's priority", kinds)
	}
	harness.clock.Advance(11 * time.Minute)
	if kinds := demotedKinds(t, harness); !slices.Contains(kinds, jobs.KindExtractSubtitle) {
		t.Errorf("ten minutes after the player stopped, demoted %v; want the extraction", kinds)
	}
}
