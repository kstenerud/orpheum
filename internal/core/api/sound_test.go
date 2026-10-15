// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveSurroundFilm adds file 4 to shelveFilms' library: an MP4 whose sound is 5.1 AAC, which a
// browser listing aac-multi plays as it is.
func (harness testHarness) shelveSurroundFilm(t *testing.T) {
	t.Helper()
	probe := `{"format":{"formatNames":["mov","mp4","m4a"]},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"aac","codecProfile":"LC","audio":{"channelCount":6}}]}`
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
		 first_seen_at, last_seen_at) VALUES
		 (4, 1, 1, '/media/Loud (2008).mp4', CAST('/media/Loud (2008).mp4' AS BLOB), 10, X'1a1b', 'listed', 1, 1)`,
		`INSERT INTO file_probe (file_id, probe_json) VALUES (4, '`+probe+`')`)
}

// thisDeviceID is the device a session is on, from its list of devices.
func (harness testHarness) thisDeviceID(t *testing.T, token string) int64 {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/me/devices", token, nil)
	var listed struct {
		Devices []struct {
			ID           int64  `json:"id"`
			AudioOutput  string `json:"audioOutput"`
			IsThisDevice bool   `json:"isThisDevice"`
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

// soundDecision is a playback's answer with how the device makes sound.
type soundDecision struct {
	decision

	AudioOutput string `json:"audioOutput"`
}

func (harness testHarness) playWithSound(t *testing.T, token string, body map[string]any) soundDecision {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/playback", token, body)
	var answered soundDecision
	if err := json.Unmarshal([]byte(answer.body), &answered); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /playback = %d %s", answer.status, answer.body)
	}
	return answered
}

func TestPlayback_OnStereoOutputASurroundSoundIsAlwaysPreparedSoTheBoostCanAct(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSurroundFilm(t)
	device := "/me/devices/" + strconv.FormatInt(harness.thisDeviceID(t, user), 10)
	answer := func(output string) int {
		return harness.do(t, http.MethodPut, device, user, map[string]string{"audioOutput": output}).status
	}
	first := harness.playWithSound(t, user, surroundFormats())
	if first.Delivery != "direct" || first.AudioOutput != "" {
		t.Errorf("before the device is asked: %s %q, want direct and no answer", first.Delivery, first.AudioOutput)
	}
	for _, output := range []string{"built_in_speakers", "headphones"} {
		if status := answer(output); status != http.StatusNoContent {
			t.Fatalf("PUT audioOutput %s: %d", output, status)
		}
		got := harness.playWithSound(t, user, surroundFormats())
		if got.Delivery != "hls" || !slices.Equal(got.NotDirectReasons, []string{"dialogue_boost"}) ||
			got.AudioOutput != output {
			t.Errorf("on %s: %s %v %q, want prepared for dialogue_boost (D222)",
				output, got.Delivery, got.NotDirectReasons,
				got.AudioOutput)
		}
		if stereo := harness.playWithSound(t, user, chrome(1)); stereo.Delivery != "direct" {
			t.Errorf("a stereo film on %s: %s, want direct: nothing to fold down", output, stereo.Delivery)
		}
	}
	if status := answer("receiver"); status != http.StatusNoContent {
		t.Fatalf("PUT audioOutput receiver: %d", status)
	}
	got := harness.playWithSound(t, user, surroundFormats())
	if got.Delivery != "direct" || got.AudioOutput != "receiver" {
		t.Errorf("on a receiver: %s %q, want direct: it takes the sound as it is", got.Delivery, got.AudioOutput)
	}
	if status := answer("radio"); status < 400 || status >= 500 {
		t.Errorf("an answer not among the three: %d, want refused", status)
	}
	if status := harness.do(t, http.MethodPut, device, user, map[string]string{}).status; status != 400 {
		t.Errorf("a change naming nothing: %d, want 400", status)
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

// The answers to how a device makes sound are auth's one list, wherever the API names them, and a
// name not on it, as the old tv_speakers, is refused (D222).
func TestDevices_TheDocumentNamesEveryAudioOutputAndAnOldNameIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	for _, schema := range []string{"DeviceBody", "StartPlaybackOutputBody", "ChangeDeviceInputBody"} {
		documented := document.Components.Schemas[schema].Properties["audioOutput"].Enum
		if !slices.Equal(documented, auth.AudioOutputs()) {
			t.Errorf("%s names the answers %v, want %v", schema, documented, auth.AudioOutputs())
		}
	}
	device := "/me/devices/" + strconv.FormatInt(harness.thisDeviceID(t, user), 10)
	old := harness.do(t, http.MethodPut, device, user, map[string]string{"audioOutput": "tv_speakers"})
	if old.status < 400 {
		t.Errorf("PUT audioOutput tv_speakers = %d, want refused: the answer is built_in_speakers now", old.status)
	}
}
