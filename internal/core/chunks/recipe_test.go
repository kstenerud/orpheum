// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Each stream's store is named by the recipe of how the node makes it too: at its first version, as
// it was named before recipes were counted, so the deploy that counted them made no store again; at
// a raised one, by another key of the same length on the same grid, so a store the old recipe made is
// never served or continued as the new's (D536).
func TestStoreKey_EachStreamsStoreIsNamedByItsRecipeItsFirstVersionAsBefore(t *testing.T) {
	t.Parallel()
	probe := nodeproto.Probe{
		Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}},
		Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "video", Codec: "h264", Video: nodeproto.ProbeVideo{Width: 1920, Height: 1080}},
			{StreamIndex: 1, Kind: "audio", Codec: "ac3", Audio: nodeproto.ProbeAudio{ChannelCount: 6}},
		},
	}
	const gridID = "0123456789abcdef"
	picture, sound := library.PictureChunksFactsKey(probe), library.SoundChunksFactsKey(probe, 0)
	encoded := sha256.Sum256([]byte(picture + ";encode=sourceTimeBase"))
	for _, each := range []struct {
		stream Stream
		recipe int
		before string
	}{
		{Stream{Kind: nodeproto.StreamPicture}, nodeproto.PictureRecipe, gridID + picture},
		{
			Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}},
			nodeproto.QualityRecipe, gridID + hex.EncodeToString(encoded[:])[:gridIDHexLength],
		},
		{Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}, nodeproto.SoundRecipe, gridID + sound},
		{Stream{Kind: nodeproto.StreamSoundCopied}, nodeproto.SoundCopiedRecipe, gridID + sound},
	} {
		name := each.stream.Name()
		if key := storeKeyAt(gridID, probe, each.stream, 1); key != each.before {
			t.Errorf("%s's store at its first recipe is %s; want it as before, %s", name, key, each.before)
		}
		if key, want := StoreKey(gridID, probe, each.stream), storeKeyAt(gridID, probe, each.stream,
			each.recipe); key != want {
			t.Errorf("%s's store is %s; want it at its recipe's version, %s", name, key, want)
		}
		raised := storeKeyAt(gridID, probe, each.stream, 2)
		if raised == each.before || len(raised) != len(each.before) || !strings.HasPrefix(raised, gridID) {
			t.Errorf("%s's store at a raised recipe is %s; want another on its grid, as long as %s",
				name, raised, each.before)
		}
	}
}
