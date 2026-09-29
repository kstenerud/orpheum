// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore plays a scanned film end to end: the
// decision, then its bytes read by the core from the in-process node's byte socket and relayed.
func TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore(t *testing.T) {
	t.Parallel()
	server, token, filmID := playableFilm(t, filmOnNode(t, "nfo"))
	decided := call(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmID, "network": "local", "playableFormats": map[string]any{
			"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"}, "subtitleFormats": []string{},
			"containers": []string{"mkv"},
		},
	})
	var decision struct {
		Kind string `json:"delivery"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal([]byte(decided.body), &decision); err != nil || decision.Kind != "direct" {
		t.Fatalf("POST /playback = %d %s, want direct", decided.status, decided.body)
	}
	root := strings.TrimSuffix(server.baseURL, "/api/v1")
	film := call(t, http.MethodGet, root+decision.URL, token, nil)
	if film.status != http.StatusOK || film.body != "film" {
		t.Errorf("the film's bytes = %d %q, want 200 \"film\" from the node", film.status, film.body)
	}
}

// signInDevice is the device the tests sign in on (D217 §8).
var signInDevice = map[string]string{"name": "Firefox on Linux", "platform": "web"}

// playableFilm serves the shelved film's core with its node, signs in as the owner, and returns
// the server, the session token and the film's file id.
func playableFilm(t *testing.T, shelved shelf) (server running, token string, filmID int64) {
	t.Helper()
	config, configPath, nodeID := shelved.config, shelved.configPath, shelved.nodeID
	const identified = "identified a library"
	server = serveWithNode(t, config, newCapturedLog("ready", identified), identified)
	setupCode := runCommand(t, "setup-code", "--config", configPath)
	created := call(t, http.MethodPost, server.baseURL+"/setup", "",
		map[string]string{"setupCode": strings.TrimSpace(setupCode.stdout), "name": "owner", "password": "pw"})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	session := call(t, http.MethodPost, server.baseURL+"/session", "",
		map[string]any{"name": "owner", "password": "pw", "device": signInDevice})
	var signedIn struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(session.body), &signedIn); err != nil || signedIn.Token == "" {
		t.Fatalf("sign in: %d %s", session.status, session.body)
	}
	files, err := openDatabase(t, config).Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".mkv") {
			filmID = file.ID
		}
	}
	return server, signedIn.Token, filmID
}

// TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore plays a film with an ASS
// track end to end: the play queues the extraction, the core's job commands the in-process node,
// the node runs FFmpeg on the film and answers, and the track's URL serves what FFmpeg wrote.
// The fake FFmpeg writes the same reply for every run, so the probe and the text are one reply.
func TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore(t *testing.T) {
	t.Parallel()
	const reply = `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[` +
		`{"index":0,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"eng"}}]}`
	server, token, filmID := playableFilm(t, filmProbedAs(t, "nfo", reply))
	decided := call(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmID, "network": "local", "playableFormats": map[string]any{
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
	for {
		text := call(t, http.MethodGet, root+decision.Subtitles[0].ASSURL, token, nil)
		if text.status == http.StatusTooEarly {
			select {
			case <-t.Context().Done():
				t.Fatal("the subtitle was never extracted")
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		if text.status != http.StatusOK || text.body != reply {
			t.Errorf("the subtitle = %d %q, want 200 and what FFmpeg wrote", text.status, text.body)
		}
		// The job view reads the queue the runners run (cut-a §5.3).
		listed := call(t, http.MethodGet, server.baseURL+"/jobs", token, nil)
		if listed.status != http.StatusOK || !strings.Contains(listed.body, `"kind":"extract_subtitle"`) {
			t.Errorf("GET /jobs = %d %s, want the extraction listed", listed.status, listed.body)
		}
		// Doctor reads what the node's hello said about its confinement (D112).
		found := call(t, http.MethodGet, server.baseURL+"/doctor", token, nil)
		if found.status != http.StatusOK || !strings.Contains(found.body, `"check":"FFmpeg confinement on node`) {
			t.Errorf("GET /doctor = %d %s, want the node's confinement", found.status, found.body)
		}
		return
	}
}

// TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled measures a scanned film end to end: the
// sweep after its identification queues the measurement, the core's job commands the in-process
// node, the node runs FFmpeg and reads its log, and the next playback is levelled by what it
// measured (D226). The fake FFmpeg answers every run alike: the probe on stdout, the loudness on
// stderr.
func TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled(t *testing.T) {
	t.Parallel()
	const probe = `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[` +
		`{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p"},` +
		`{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2}]}`
	const measured = "[ebur128@stereo_1 @ 0x55d1] Summary:\n\n  Integrated loudness:\n    I:         -35.7 LUFS\n" +
		"    Threshold: -46.0 LUFS\n\n  Loudness range:\n    LRA:        22.8 LU\n\n  True peak:\n" +
		"    Peak:      -16.2 dBFS\n"
	shelved := filmAnswered(t, "nfo", testutil.FakeReply{Stdout: probe, Stderr: measured})
	server, token, filmID := playableFilm(t, shelved)
	deadline := time.After(30 * time.Second)
	for {
		decided := call(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
			"fileId": filmID, "network": "local", "playableFormats": map[string]any{
				"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"},
				"subtitleFormats": []string{},
				"containers":      []string{"mkv"},
			},
		})
		var decision struct {
			Levelling struct {
				IsMeasured bool    `json:"isMeasured"`
				GainDB     float64 `json:"gainDb"`
			} `json:"levelling"`
		}
		if err := json.Unmarshal([]byte(decided.body), &decision); err != nil || decided.status != http.StatusOK {
			t.Fatalf("POST /playback = %d %s", decided.status, decided.body)
		}
		if !decision.Levelling.IsMeasured {
			select {
			case <-deadline:
				t.Fatal("the film was not measured within 30 seconds")
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		if decision.Levelling.GainDB != 12.7 {
			t.Errorf("the levelling = %+v, want the node's measurement raised to -23 LUFS", decision.Levelling)
		}
		return
	}
}
