// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// A person signed in asks for a system video by what it is: while no node can hold it, it is
// unavailable; once one can, its first use starts it being made; the debug view lists it and rebuilds
// it, and only an admin may (D435 §5–§7).
func TestSystemVideos_AnyoneAsksForOneAndTheAdminSeesAndRebuildsThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	const clip = "/system-videos/sound-timing/h264-aac"
	if got := harness.do(t, http.MethodGet, clip, partner, nil); got.status != http.StatusServiceUnavailable {
		t.Errorf("GET %s with no node = %d %s; want 503", clip, got.status, got.body)
	}
	harness.shelveFilms(t)
	harness.nodes.roots = []string{"/media", "/var/orpheum/system-videos"}
	harness.nodes.systemFolder = "/var/orpheum/system-videos"
	asked := harness.do(t, http.MethodGet, clip, partner, nil)
	var video struct {
		FileName string `json:"fileName"`
		IsReady  bool   `json:"isReady"`
	}
	if err := json.Unmarshal([]byte(asked.body), &video); err != nil || asked.status != http.StatusOK ||
		video.FileName != "sound-timing.h264-aac.mp4" || video.IsReady {
		t.Errorf("GET %s = %d %s; want the clip, being made", clip, asked.status, asked.body)
	}
	if got := harness.do(t, http.MethodGet, "/system-videos/sound-timing/vp9-opus", partner, nil); got.status/100 != 4 {
		t.Errorf("a codec no recipe makes = %d; want refused", got.status)
	}
	if got := harness.do(t, http.MethodGet, "/system/videos", partner, nil); got.status != http.StatusForbidden {
		t.Errorf("GET /system/videos as a user = %d; want 403", got.status)
	}
	listed := harness.do(t, http.MethodGet, "/system/videos", admin, nil)
	var view struct {
		NodeName string `json:"nodeName"`
		Folder   string `json:"folder"`
		Videos   []struct {
			Purpose string `json:"purpose"`
		} `json:"videos"`
	}
	if err := json.Unmarshal([]byte(listed.body), &view); err != nil || listed.status != http.StatusOK ||
		len(view.Videos) != 1 || view.Videos[0].Purpose != "sound-timing" {
		t.Errorf("GET /system/videos = %d %s; want the clip", listed.status, listed.body)
	}
	rebuilt := harness.do(t, http.MethodPost, "/system/videos/rebuild", admin, nil)
	if rebuilt.status != http.StatusNoContent {
		t.Errorf("POST /system/videos/rebuild = %d %s; want 204", rebuilt.status, rebuilt.body)
	}
}
