// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore plays a scanned film end to end: the
// decision, then its bytes read by the core from the in-process node's byte socket and relayed.
func TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	config, configPath, nodeID := shelved.config, shelved.configPath, shelved.nodeID
	const identified = "identified a library"
	server := serveWithNode(t, config, newCapturedLog("ready", identified), identified)
	setupCode := runCommand(t, "setup-code", "--config", configPath)
	created := call(t, http.MethodPost, server.baseURL+"/setup", "",
		map[string]string{"setupCode": strings.TrimSpace(setupCode.stdout), "name": "owner", "password": "pw"})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	session := call(t, http.MethodPost, server.baseURL+"/session", "",
		map[string]string{"name": "owner", "password": "pw"})
	var signedIn struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(session.body), &signedIn); err != nil || signedIn.Token == "" {
		t.Fatalf("sign in: %d %s", session.status, session.body)
	}
	token := signedIn.Token
	files, err := openDatabase(t, config).Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	var filmID int64
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".mkv") {
			filmID = file.ID
		}
	}
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
