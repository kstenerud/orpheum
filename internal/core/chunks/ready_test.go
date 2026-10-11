// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A stall's offer is ready once its quality and the starting sound are both made from where the
// player is for 10 s, as far as a stalled player loads before it plays again (D253 §1). Film 1's
// chunks are 10 s long: from 95 s, that is chunks 9 and 10.
func TestIsReadyAt_AnOfferIsReadyOnceItsPictureAndSoundAreMadeTenSecondsOnFromThePlayer(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	offer := stalledAt(t, service, 2000)
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true}
	gridID := mustGrid(t, service).ID()
	picture := StoreDir(1, quality(1200, 480), gridID)
	sound := StoreDir(1, Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}, gridID)
	isReady := func() bool {
		t.Helper()
		ready, err := service.IsReadyAt(t.Context(), playback, offer, 95_000)
		if err != nil {
			t.Fatal(err)
		}
		return ready
	}
	if isReady() {
		t.Error("an offer with nothing made yet is ready; want it not")
	}
	for _, step := range []struct {
		dir     string
		chunk   int
		isReady bool
		what    string
	}{
		{picture, 9, false, "the picture's chunk at the player alone"},
		{picture, 10, false, "the picture's 10 s, without the sound"},
		{sound, 9, false, "the picture's 10 s and the sound's first chunk"},
		{sound, 10, true, "both made for 10 s on"},
	} {
		service.runs.served(step.dir, step.chunk)
		if got := isReady(); got != step.isReady {
			t.Errorf("with %s, ready = %t; want %t", step.what, got, step.isReady)
		}
	}
}
