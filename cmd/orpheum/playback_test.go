// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore plays a scanned film end to end: the
// decision, then its bytes read by the core from the in-process node's byte socket and relayed.
func TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore(t *testing.T) {
	t.Parallel()
	server, token, filmID := playableFilm(t, filmOnNode(t, "nfo"))
	decided := call(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmID, "link": "local", "menu": map[string]any{
			"video": []string{"h264-sdr"}, "audio": []string{"aac-stereo"}, "subtitle": []string{},
			"containers": []string{"mkv"},
		},
	})
	var decision struct {
		Kind string `json:"kind"`
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
		"fileId": filmID, "link": "local", "menu": map[string]any{
			"video": []string{"h264-sdr"}, "audio": []string{"aac-stereo"}, "subtitle": []string{"ass"},
			"containers": []string{"mkv"},
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
		return
	}
}
