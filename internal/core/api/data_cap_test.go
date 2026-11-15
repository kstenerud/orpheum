// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// setDataCaps keeps a device's data caps on the core, as its browser does when its person chooses one.
func (harness testHarness) setDataCaps(t *testing.T, token string, deviceID int64, caps map[string]any) {
	t.Helper()
	answer := harness.do(t, http.MethodPut, "/me/devices/"+strconv.FormatInt(deviceID, 10), token,
		map[string]any{"dataCaps": caps})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT the device's data caps = %d %s", answer.status, answer.body)
	}
}

// meDataCaps are the data caps GET /me says for the session's device.
func (harness testHarness) meDataCaps(t *testing.T, token string) api.DataCapsBody {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/me", token, nil)
	var signedIn struct {
		Device *struct {
			DataCaps api.DataCapsBody `json:"dataCaps"`
		} `json:"device"`
	}
	if err := json.Unmarshal([]byte(answer.body), &signedIn); err != nil || signedIn.Device == nil {
		t.Fatalf("GET /me = %d %s", answer.status, answer.body)
	}
	return signedIn.Device.DataCaps
}

// A device's data caps, one for each kind of network, are kept by the core, and told with who is
// signed in, where its browser refreshes its copy, and in the list of devices; a kind left out is
// No limit (D493).
func TestDevices_ADevicesDataCapsAreKeptByTheCoreAndToldWithWhoIsSignedIn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	deviceID := harness.thisDeviceID(t, user)
	if got := harness.meDataCaps(t, user); got != (api.DataCapsBody{}) {
		t.Errorf("a new device's caps are %+v; want No limit on every network", got)
	}
	harness.setDataCaps(t, user, deviceID, map[string]any{"mobileKbps": 400, "wifiKbps": 5000})
	want := api.DataCapsBody{WifiKbps: 5000, MobileKbps: 400}
	if got := harness.meDataCaps(t, user); got != want {
		t.Errorf("GET /me says the caps %+v; want %+v", got, want)
	}
	listed := harness.do(t, http.MethodGet, "/me/devices", user, nil)
	if !strings.Contains(listed.body, `"dataCaps":{"wifiKbps":5000,"mobileKbps":400}`) {
		t.Errorf("the list of devices does not say the caps: %s", listed.body)
	}
	harness.setDataCaps(t, user, deviceID, map[string]any{})
	if got := harness.meDataCaps(t, user); got != (api.DataCapsBody{}) {
		t.Errorf("after every cap was set to No limit, GET /me says %+v", got)
	}
	harness.logs.AwaitEvent(t, "dataCapsSet", `"mobileKbps":400`)
}

// A capped device's start says the kind of network it is on, and the core applies that kind's cap
// itself: the film plays from its ladder, starting on the highest rung under the cap, where it began
// at the top and made the own picture's chunks while the player dropped (staging, 2026-11-15); on a
// network with no cap, or a start that says none, as a cast's, it plays as before (D493, D489).
func TestPlayback_ACappedDeviceStartsOnItsLadderUnderItsCap(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	harness.setDataCaps(t, user, harness.thisDeviceID(t, user), map[string]any{"mobileKbps": 1000})
	onMobile := chrome(1)
	onMobile["networkKind"] = "mobile"
	capped := harness.playFor(t, user, onMobile)
	if capped.Delivery != "hls" || !slices.Equal(capped.NotDirectReasons, []string{"userChoice"}) ||
		capped.OnLadder == nil || capped.OnLadder.QualityKbps > 1000 {
		t.Errorf("on mobile data under a 1000 kbit/s cap, film 1 started %+v (on its ladder: %+v); want its "+
			"ladder, at a rung under the cap", capped, capped.OnLadder)
	}
	harness.logs.AwaitEvent(t, "playbackStarted", `"networkKind":"mobile"`, `"dataCapKbps":1000`,
		`"ladderQuality":"q700k432p"`)
	for _, kind := range []string{"wifi", ""} {
		body := chrome(1)
		if kind != "" {
			body["networkKind"] = kind
		}
		if uncapped := harness.playFor(t, user, body); uncapped.Delivery != "direct" {
			t.Errorf("on %q, with no cap, film 1 started %+v; want it played as it is", kind, uncapped)
		}
	}
	// The film's own file, chosen, plays from the ladder still under a cap below its rate (D491 §4).
	original := chrome(1)
	original["networkKind"], original["preferredDelivery"] = "mobile", "direct"
	if chosen := harness.playFor(t, user, original); chosen.Delivery != "hls" {
		t.Errorf("the original chosen under a cap below its rate started %+v; want the ladder", chosen)
	}
}

// The films whose own picture a device gave up are kept in the store, so a core that never heard
// the report, one restarted since, does not offer the device the picture; another device still is
// offered it (D266, D493).
func TestChunks_AnOwnPictureGivenUpOutlivesTheCoreThatHeardIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	deviceID := harness.thisDeviceID(t, user)
	// As the core before the restart kept it.
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO own_picture_given_up (device_id, file_id, given_up_at) `+
		`VALUES (`+strconv.FormatInt(deviceID, 10)+`, 1, 0)`)
	next := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/master.m3u8"
	if master := harness.getBytePath(t, next, user, true, ""); master.status != http.StatusOK ||
		strings.Contains(master.body, "picture/stream.m3u8") {
		t.Errorf("the device's playback after a restart offers the picture it gave up:\n%s", master.body)
	}
	other := harness.signIn(t, "owner", "owner-password")
	elsewhere := api.Prefix + "/bytes/media/chunks/" + harness.play(t, other, chrome(1)).MediaToken + "/master.m3u8"
	if master := harness.getBytePath(t, elsewhere, other, true, ""); !strings.Contains(master.body,
		"picture/stream.m3u8") {
		t.Errorf("another device's master playlist lost the picture:\n%s", master.body)
	}
}
