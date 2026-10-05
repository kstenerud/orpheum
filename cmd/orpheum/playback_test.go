// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
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
	config, configPath, nodeID := shelved.config, shelved.configPath, shelved.nodeID
	const identified = "identified a library"
	server = serveWithNode(t, config, newCapturedLog("ready", identified), identified)
	token = signedInOwner(t, server, configPath)
	files, err := openDatabase(t, config).Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".mkv") {
			filmFileID = file.ID
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
	found := requestAPI(t, http.MethodGet, server.baseURL+"/doctor", token, nil)
	if found.status != http.StatusOK || !strings.Contains(found.body, `"check":"FFmpeg confinement on node`) {
		t.Errorf("GET /doctor = %d %s, want the node's confinement", found.status, found.body)
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
}

// TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted rebuilds the index from
// the node's disk (D238 §2): a rendition whose manifest names the film as it is, which cache.db does
// not know, is adopted complete at the node's hello, and a run directory without a manifest, a
// making that never finished, is evicted.
func TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	server, _, filmFileID := playableFilm(t, shelved)
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	config := shelved.config
	film, err := openDatabase(t, config).Read().GetPlayableFile(t.Context(), filmFileID)
	if err != nil {
		t.Fatal(err)
	}
	made := strconv.FormatInt(filmFileID, 10) + "/remux/0123456789abcdef"
	unfinished := strconv.FormatInt(filmFileID, 10) + "/h264-8m/fedcba9876543210"
	for _, dir := range []string{made, unfinished} {
		writeFile(t, filepath.Join(config.Instance.preparedDir(), dir, "master.m3u8"), "#EXTM3U")
	}
	manifest, err := json.Marshal(nodeproto.RenditionManifest{
		Prepare: nodeproto.Prepare{
			Video:   nodeproto.BytesFile{PayloadHash: hex.EncodeToString(film.PayloadHash)},
			Profile: nodeproto.ProfileRemux, RenditionDir: made, SegmentLengthSeconds: 2,
		},
		MadeAtMs: 1000, SizeBytes: 7, VideoEncoder: "copy",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(config.Instance.preparedDir(), made, nodeproto.ManifestName), string(manifest))
	const evicted = "evicted the renditions the core does not keep"
	again := serveWithNode(t, config, newCapturedLog("ready", evicted), evicted)
	if err = again.stop(); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(config.Instance.CacheDir, "cache.db")
	cache, err := store.OpenCache(t.Context(), cachePath, config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cache.Close() }() //nolint:errcheck // read only
	adopted, err := cache.Read().GetRenditionOfFileAndProfile(t.Context(),
		cachegen.GetRenditionOfFileAndProfileParams{SourceFile: filmFileID, Profile: "remux"})
	if err != nil || adopted.State != "complete" || adopted.RunID != "0123456789abcdef" {
		t.Errorf("the film's remux in the index is %+v (%v), want the node's adopted complete", adopted, err)
	}
	if _, err := os.Stat(filepath.Join(config.Instance.preparedDir(), unfinished)); !os.IsNotExist(err) {
		t.Errorf("the unfinished making is still on the node's disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.Instance.preparedDir(), made, "master.m3u8")); err != nil {
		t.Errorf("the adopted rendition is gone from the node's disk: %v", err)
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
