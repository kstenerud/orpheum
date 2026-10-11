// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"testing"
)

// The served API keeps a person's own choices through the preferences service it is wired with,
// the one home of a person's choices (the architecture map's F6).
func TestServe_KeepsAPersonsOwnPreferences(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, configPath)
	kept := requestAPI(t, http.MethodPut, server.baseURL+"/me/preferences", token,
		map[string]any{"isLevellingOn": false, "isDialogueBoostOn": true})
	if kept.status != http.StatusNoContent {
		t.Errorf("PUT /me/preferences = %d %s, want 204", kept.status, kept.body)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}
