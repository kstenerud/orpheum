// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore plays a scanned film end to end: the
// decision, then its bytes read by the core from the in-process node's byte socket and relayed.
func TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore(t *testing.T) {
	t.Parallel()
	server, token, filmFileID := playableFilm(t, filmOnNode(t, "nfo"))
	decided := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmFileID, "playableFormats": map[string]any{
			"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"}, "subtitleFormats": []string{},
			"containers": []string{"mkv"},
		},
	})
	var decision struct {
		Delivery string `json:"delivery"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal([]byte(decided.body), &decision); err != nil || decision.Delivery != "direct" {
		t.Fatalf("POST /playback = %d %s, want direct", decided.status, decided.body)
	}
	root := strings.TrimSuffix(server.baseURL, "/api/v1")
	film := requestAPI(t, http.MethodGet, root+decision.URL, token, nil)
	if film.status != http.StatusOK || film.body != "film" {
		t.Errorf("the film's bytes = %d %q, want 200 \"film\" from the node", film.status, film.body)
	}
}

// playableFilm serves the shelved film's core with its node, signs in as the owner, and returns
// the server, the session token and the film's file id.
func playableFilm(t *testing.T, shelved filmFixture) (server runningServer, token string, filmFileID int64) {
	t.Helper()
	return playableFilmLogged(t, shelved, newCapturedLog("ready", filmIdentified))
}

// filmIdentified is what the core logs once a shelved film's library is identified, which
// playableFilmLogged waits for.
const filmIdentified = "identified a library"

// playableFilmLogged is playableFilm, logging to captured, which awaits filmIdentified.
func playableFilmLogged(
	t *testing.T, shelved filmFixture, captured capturedLog,
) (server runningServer, token string, filmFileID int64) {
	t.Helper()
	config, configPath, nodeID := shelved.config, shelved.configPath, shelved.nodeID
	server = serveWithNode(t, config, captured, filmIdentified)
	token = signedInOwner(t, server, configPath)
	for _, file := range liveFilingsOfNode(t, config, nodeID) {
		if strings.HasSuffix(file.path, ".mkv") {
			filmFileID = file.id
		}
	}
	return server, token, filmFileID
}

// TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore plays a film with an ASS
// track end to end: the play queues the extraction, the core's job commands the in-process node,
// the node runs FFmpeg on the film and answers, and the track's URL serves what FFmpeg wrote.
// The fake FFmpeg writes the same reply for every run, so the probe and the text are one reply.
func TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore(t *testing.T) {
	t.Parallel()
	const reply = `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[` +
		`{"index":0,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"eng"}}]}`
	server, token, filmFileID := playableFilm(t, filmProbedAs(t, "nfo", reply))
	decided := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmFileID, "playableFormats": map[string]any{
			"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"},
			"subtitleFormats": []string{"ass"},
			"containers":      []string{"mkv"},
		},
	})
	var decision struct {
		Subtitles []struct {
			ASSURL string `json:"assUrl"`
		} `json:"subtitles"`
	}
	if err := json.Unmarshal([]byte(decided.body), &decision); err != nil || len(decision.Subtitles) != 1 {
		t.Fatalf("POST /playback = %d %s, want one subtitle track", decided.status, decided.body)
	}
	root := strings.TrimSuffix(server.baseURL, "/api/v1")
	var text apiReply
	pollUntil(t, "the subtitle was extracted", func() bool {
		text = requestAPI(t, http.MethodGet, root+decision.Subtitles[0].ASSURL, token, nil)
		return text.status != http.StatusTooEarly
	})
	if text.status != http.StatusOK || text.body != reply {
		t.Errorf("the subtitle = %d %q, want 200 and what FFmpeg wrote", text.status, text.body)
	}
	// The job view reads the queue the runners run (cut-a §5.3).
	listed := requestAPI(t, http.MethodGet, server.baseURL+"/jobs", token, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"kind":"extract_subtitle"`) {
		t.Errorf("GET /jobs = %d %s, want the extraction listed", listed.status, listed.body)
	}
	// Doctor reads what the node's hello said about its confinement (D112).
	found := requestAPI(t, http.MethodGet, server.baseURL+"/system", token, nil)
	if found.status != http.StatusOK || !strings.Contains(found.body, `"check":"FFmpeg confinement on node`) {
		t.Errorf("GET /system = %d %s, want the node's confinement", found.status, found.body)
	}
}

// pollLimit is how long pollUntil waits for what a node does in the background.
const pollLimit = 30 * time.Second

// pollUntil calls isDone every 50 ms until it says true, and fails the test, saying what never
// happened, when pollLimit passes first.
func pollUntil(t *testing.T, what string, isDone func() bool) {
	t.Helper()
	deadline := time.After(pollLimit)
	for !isDone() {
		select {
		case <-deadline:
			t.Fatalf("not within %v: %s", pollLimit, what)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled reads a scanned film through end to end:
// the sweep after its identification queues the read-through, the core's job commands the
// in-process node, the node runs FFmpeg and reads its log and its output, the next playback is
// levelled by what it measured (D226), and its picture's timeline is kept (D247 §10). The fake
// FFmpeg answers every run alike: the probe from ffprobe and the picture's packets from ffmpeg on
// stdout, the loudness on stderr.
func TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled(t *testing.T) {
	t.Parallel()
	const probe = `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[` +
		`{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p"},` +
		`{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2}]}`
	measured := testutil.EBUR128Summary("stereo_1", "-35.7", "22.8", "-16.2")
	packets := "#tb 0: 1/1000\n0, 0, 0, 40, 900, 0x1\n0, 40, 40, 40, 30, 0x2, F=0x0\n"
	shelved := filmAnswered(t, "nfo", testutil.FakeReply{Stdout: probe, FFmpegStdout: packets, Stderr: measured})
	server, token, filmFileID := playableFilm(t, shelved)
	type levelling struct {
		IsMeasured bool    `json:"isMeasured"`
		GainDB     float64 `json:"gainDb"`
	}
	var levelled levelling
	pollUntil(t, "the film was measured", func() bool {
		decided := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
			"fileId": filmFileID, "playableFormats": map[string]any{
				"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"},
				"subtitleFormats": []string{},
				"containers":      []string{"mkv"},
			},
		})
		var decision struct {
			Levelling levelling `json:"levelling"`
		}
		if err := json.Unmarshal([]byte(decided.body), &decision); err != nil || decided.status != http.StatusOK {
			t.Fatalf("POST /playback = %d %s", decided.status, decided.body)
		}
		levelled = decision.Levelling
		return levelled.IsMeasured
	})
	if levelled.GainDB != 12.7 {
		t.Errorf("the levelling = %+v, want the node's measurement raised to -23 LUFS", levelled)
	}
	database := openDatabase(t, shelved.config)
	film, err := database.Read().GetPlayableFile(t.Context(), filmFileID)
	if err != nil {
		t.Fatal(err)
	}
	var timeline sqlcgen.VideoTimeline
	pollUntil(t, "the film's timeline was kept", func() bool {
		timeline, err = database.Read().GetVideoTimeline(t.Context(),
			sqlcgen.GetVideoTimelineParams{PayloadHash: film.PayloadHash, VideoStreamIndex: 0})
		return err == nil
	})
	if timeline.KeyframesMsJSON != "[0]" || timeline.BytesInEachSecondJSON != "[930]" {
		t.Errorf("the film's timeline in persistent.db = %+v (%v); want its one keyframe and its second's bytes, from "+
			"the same read", timeline, err)
	}
	// Measured, it has nothing left to wait for (D324 §6).
	waits := requestAPI(t, http.MethodGet, fmt.Sprintf("%s/files/%d/waits", server.baseURL, filmFileID), token, nil)
	if waits.status != http.StatusOK || strings.Contains(waits.body, "readThrough") {
		t.Errorf("once measured, the film's waits = %d %s; want none", waits.status, waits.body)
	}
}

// TestPlayback_WhatAWholeFilmPrepareLeftOnTheNodesDiskIsEvictedAtItsHello: the whole-film prepare is
// retired (D344 §1), so a rendition one left in the prepared directory, made or not, is evicted at the
// node's hello, as a making that never finished is (D238 §2).
func TestPlayback_WhatAWholeFilmPrepareLeftOnTheNodesDiskIsEvictedAtItsHello(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	server, _, filmFileID := playableFilm(t, shelved)
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	config := shelved.config
	// The disk's limits evict nothing at the node's start, whatever the test machine's disk holds: the
	// hello's eviction is the one under test.
	config.Node.PreparedDiskUsedLimitPercent, config.Node.PreparedDiskMinFreeMiB = 100, 0
	made := strconv.FormatInt(filmFileID, 10) + "/remux/0123456789abcdef"
	unfinished := strconv.FormatInt(filmFileID, 10) + "/h264/fedcba9876543210"
	for _, dir := range []string{made, unfinished} {
		writeFile(t, filepath.Join(config.Instance.preparedDir(), dir, "master.m3u8"), "#EXTM3U")
	}
	writeFile(t, filepath.Join(config.Instance.preparedDir(), made, "manifest.json"), `{"madeAtMs":1000}`)
	const evicted = "evicted the makings that never finished"
	again := serveWithNode(t, config, newCapturedLog("ready", evicted), evicted)
	if err := again.stop(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{made, unfinished} {
		if _, err := os.Stat(filepath.Join(config.Instance.preparedDir(), dir)); !os.IsNotExist(err) {
			t.Errorf("%s, a prepare's, is still on the node's disk: %v", dir, err)
		}
	}
}

// writeFile writes contents to path, making its folders.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
