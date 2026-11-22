// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
)

// A film whose sound's first packet alone lasts seconds is not played directly, for its sound's
// timing, a reason the schema names: a player taking the file's times would play its sound that much
// late, so the server makes it, starting with the picture (D534 §2).
func TestPlayback_ASoundWhoseFirstPacketIsStretchedIsNotPlayedDirectlyForItsTiming(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	probe := `{"format":{"formatNames":["mov","mp4","m4a"],"durationMs":1200000},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		`{"streamIndex":1,"kind":"audio","codec":"aac","codecProfile":"LC",` +
		`"audio":{"channelCount":2,"firstPacketStretchMs":31902}}]}`
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 4, LibraryID: 1, NodeID: 1, Path: "/media/a.mp4", SizeBytes: 10,
		PayloadHash: []byte{0x2a, 0x2b}, Filing: "listed",
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 4, ProbeJSON: probe, DurationMs: 1200000})
	played := harness.play(t, user, chrome(4))
	if played.Delivery == "direct" || !slices.Equal(played.NotDirectReasons, []string{"audioTiming"}) {
		t.Errorf("the film plays %s, for %v; want not directly, for its sound's timing (D534 §2)",
			played.Delivery, played.NotDirectReasons)
	}
	body, found := harness.server.OpenAPI().Components.Schemas.Map()["StartPlaybackOutputBody"]
	if !found {
		t.Fatal("the schema has no StartPlaybackOutputBody")
	}
	var named []string
	for _, value := range body.Properties["notDirectReasons"].Items.Enum {
		named = append(named, fmt.Sprint(value))
	}
	if !slices.Contains(named, "audioTiming") {
		t.Errorf("the schema's reasons not to play directly %v lack audioTiming", named)
	}
}
