// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveSurroundFilm adds file 4 to shelveFilms' library: an MP4 whose sound is 5.1 AAC, which a
// browser listing aac-multi plays as it is; it has a length, so it is cut on time when converted.
func (harness testHarness) shelveSurroundFilm(t *testing.T) {
	t.Helper()
	probe := `{"format":{"formatNames":["mov","mp4","m4a"],"durationMs":1200000},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"aac","codecProfile":"LC","audio":{"channelCount":6}}]}`
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
		 first_seen_at, last_seen_at) VALUES
		 (4, 1, 1, '/media/Loud (2008).mp4', CAST('/media/Loud (2008).mp4' AS BLOB), 10, X'1a1b', 'listed', 1, 1)`,
		`INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (4, '`+probe+`', 1200000)`)
}

// thisDeviceID is the device a session is on, from its list of devices.
func (harness testHarness) thisDeviceID(t *testing.T, token string) int64 {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/me/devices", token, nil)
	var listed struct {
		Devices []struct {
			ID           int64 `json:"id"`
			IsThisDevice bool  `json:"isThisDevice"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(answer.body), &listed); err != nil {
		t.Fatalf("GET /me/devices = %d %s", answer.status, answer.body)
	}
	for _, device := range listed.Devices {
		if device.IsThisDevice {
			return device.ID
		}
	}
	t.Fatalf("no device of the session's in %s", answer.body)
	return 0
}

// surroundFormats are Chrome's playable formats for file 4, with aac-multi, so its 5.1 sound plays as it is.
func surroundFormats() map[string]any {
	body := chrome(4)
	body["playableFormats"] = map[string]any{
		"videoCodecs": []string{"hevc-sdr", "h264-sdr"}, "audioCodecs": []string{"aac-stereo", "aac-multi"},
		"subtitleFormats": []string{"ass", "webvtt"}, "containers": []string{"mp4"},
	}
	return body
}

func (harness testHarness) playFor(t *testing.T, token string, body map[string]any) decision {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/playback", token, body)
	var answered decision
	if err := json.Unmarshal([]byte(answer.body), &answered); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /playback = %d %s", answer.status, answer.body)
	}
	return answered
}

// A film's surround sound plays as it is where the browser plays it, with nothing asked of the
// device, until its person turns the dialogue boost on: the server must make that mix, so the film
// plays from its ladder, its boosted mix among its sounds (D336 §1, §2, §4, D344 §1).
func TestPlayback_ASurroundSoundPlaysAsItIsUntilThePersonTurnsTheBoostOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSurroundFilm(t)
	first := harness.playFor(t, user, surroundFormats())
	if first.Delivery != "direct" || len(first.NotDirectReasons) != 0 {
		t.Errorf("5.1 the browser plays, the boost off: %s %v, want direct: it plays as it is",
			first.Delivery, first.NotDirectReasons)
	}
	turnOn := map[string]bool{"isDialogueBoostOn": true}
	if status := harness.do(t, http.MethodPut, "/me/preferences", user, turnOn).status; status != http.StatusNoContent {
		t.Fatalf("PUT the boost: %d", status)
	}
	got := harness.playFor(t, user, surroundFormats())
	isBoosted := false
	for _, sound := range got.Tracks.Sounds {
		isBoosted = isBoosted || sound.IsBoosted
	}
	if got.Delivery != "hls" || !slices.Equal(got.NotDirectReasons, []string{"dialogue_boost"}) || !isBoosted {
		t.Errorf("5.1 with the boost on: %s %v, a boosted mix %v; want converted for dialogue_boost, with it",
			got.Delivery, got.NotDirectReasons, isBoosted)
	}
	if stereo := harness.playFor(t, user, chrome(1)); stereo.Delivery != "direct" {
		t.Errorf("a stereo film with the boost on: %s, want direct: there is no centre to raise", stereo.Delivery)
	}
}

func TestFilePreferences_TheDialogueBoostIsThePersonsInEveryFile(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	boostOf := func(fileID int64) bool {
		t.Helper()
		answer := harness.do(t, http.MethodPost, "/playback", user, chrome(fileID))
		var answered struct {
			Preferences struct {
				IsDialogueBoostOn bool `json:"isDialogueBoostOn"`
			} `json:"preferences"`
		}
		if err := json.Unmarshal([]byte(answer.body), &answered); err != nil {
			t.Fatalf("POST /playback = %d %s", answer.status, answer.body)
		}
		return answered.Preferences.IsDialogueBoostOn
	}
	if boostOf(2) {
		t.Error("before any choice: the boost is on, want off (D222)")
	}
	turnOn := map[string]bool{"isDialogueBoostOn": true}
	if status := harness.do(t, http.MethodPut, "/me/preferences", user, turnOn).status; status != http.StatusNoContent {
		t.Fatalf("PUT the boost: %d", status)
	}
	if !boostOf(2) || !boostOf(1) {
		t.Error("after turning it on in one file: want it on in that file and the next (D83)")
	}
	turnOff := map[string]bool{"isDialogueBoostOn": false}
	if status := harness.do(t, http.MethodPut, "/me/preferences", user,
		turnOff).status; status != http.StatusNoContent {
		t.Fatalf("PUT the boost off: %d", status)
	}
	if boostOf(2) {
		t.Error("after turning it off: want it off everywhere")
	}
}

// No one is asked how their device makes sound (D336 §4): nothing the API documents names it, and a
// change to a device that says only that is refused, naming nothing it changes.
func TestDevices_NothingTheAPIDocumentsAsksHowADeviceMakesSound(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	for name, schema := range document.Components.Schemas {
		if _, isNamed := schema.Properties["audioOutput"]; isNamed {
			t.Errorf("%s names audioOutput, want no schema to ask how a device makes sound", name)
		}
	}
	device := "/me/devices/" + strconv.FormatInt(harness.thisDeviceID(t, user), 10)
	changed := harness.do(t, http.MethodPut, device, user, map[string]string{"audioOutput": "receiver"})
	if changed.status < 400 || changed.status >= 500 {
		t.Errorf("PUT audioOutput alone = %d, want refused: a device's sound is asked of no one", changed.status)
	}
}

// A device's sound offset is its own, kept by the core: set on the device, it comes with every
// playback the device asks for, later or earlier, and within 2 s either way (the owner's TCL TV,
// 2026-10-24).
func TestPlayback_ADevicesSoundOffsetIsKeptAndComesWithItsPlaybacks(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	device := "/me/devices/" + strconv.FormatInt(harness.thisDeviceID(t, user), 10)
	offsetOf := func() int64 {
		t.Helper()
		answer := harness.do(t, http.MethodPost, "/playback", user, chrome(1))
		var answered struct {
			SoundOffsetMs int64 `json:"soundOffsetMs"`
		}
		if err := json.Unmarshal([]byte(answer.body), &answered); err != nil || answer.status != http.StatusOK {
			t.Fatalf("POST /playback = %d %s", answer.status, answer.body)
		}
		return answered.SoundOffsetMs
	}
	setOffset := func(ms int64) int {
		return harness.do(t, http.MethodPut, device, user, map[string]int64{"soundOffsetMs": ms}).status
	}
	if got := offsetOf(); got != 0 {
		t.Errorf("a device that set none: %d ms, want 0", got)
	}
	for _, kept := range []int64{600, -450} {
		if status := setOffset(kept); status != http.StatusNoContent {
			t.Fatalf("PUT soundOffsetMs %d: %d", kept, status)
		}
		if got := offsetOf(); got != kept {
			t.Errorf("after the device set %d ms: %d ms", kept, got)
		}
	}
	for _, refused := range []int64{-2050, 2050} {
		if status := setOffset(refused); status < 400 || status >= 500 {
			t.Errorf("an offset of %d ms: %d, want refused", refused, status)
		}
	}
}
