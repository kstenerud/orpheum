// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A film its device cannot play as it is that cannot start on its ladder either is refused as one
// that cannot be converted, and starts no work: one with no picture, or with neither a known length
// nor keyframes to cut it by (D344 §5).
func TestStartPlayback_AFilmThatCannotBeConvertedIsRefusedAndStartsNoWork(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	// Film 2 with no picture: its sound and subtitles alone.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_remove(probe_json, '$.streams[0]') WHERE file_id = 2`)
	wantError(t, harness.do(t, http.MethodPost, "/playback", admin, chrome(2)), http.StatusUnprocessableEntity,
		CodeCannotConvert)
	if listed := harness.listedJobs(t, admin); len(listed) != 0 {
		t.Errorf("a play of a film with no picture queued %+v; want nothing", listed)
	}
	// Film 2 with a picture but neither a length nor keyframes known.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_remove(json_insert(probe_json, '$.streams[0]', `+
			`json('{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}}')), `+
			`'$.format.durationMs'), duration_ms = 0 WHERE file_id = 2`)
	wantError(t, harness.do(t, http.MethodPost, "/playback", admin, chrome(2)), http.StatusUnprocessableEntity,
		CodeCannotConvert)
}

// The whole-film prepare's byte paths are gone (D344 §1): a converted film's bytes are its chunks'.
func TestBytes_TheRetiredPreparedCopysPathsAreNotFound(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, admin, chrome(2)).MediaToken
	for _, path := range []string{"/master.m3u8", "/audio0/stream.m3u8", "/video/seg00001.m4s"} {
		url := Prefix + "/bytes/media/hls/" + token + path
		if answer := harness.getBytePath(t, url, admin, true, ""); answer.status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", url, answer.status)
		}
	}
}
