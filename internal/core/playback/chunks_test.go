// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
)

// A playback's film, as its chunks are asked for, carries when its person pressed play: what failed
// before then is tried again for them (D248 §4).
func TestChunksPlaybackOf_CarriesWhenItsPersonPressedPlay(t *testing.T) {
	t.Parallel()
	pressed := time.Date(2026, 10, 6, 6, 44, 8, 0, time.UTC)
	chunked := chunksPlaybackOf(tokenPlayback{fileID: 226, positionMs: 370_834, startedAt: pressed})
	if !chunked.StartedAt.Equal(pressed) || chunked.FileID != 226 || chunked.PositionMs != 370_834 {
		t.Errorf("the chunks' playback = %+v; want file 226 at 370834 ms, started at %v", chunked, pressed)
	}
}

// A device under a data cap asks for its film's ladder (D488 §4), which converts it for that wish
// alone and copies its picture: the ladder it is offered keeps the film's own picture at its top, one
// more rung the cap may play (D491). One whose device cannot decode the picture has none to keep.
func TestChunksPlaybackOf_ACappedDevicesLadderKeepsTheFilmsOwnPicture(t *testing.T) {
	t.Parallel()
	capped := tokenPlayback{decision: decide.Decision{
		Delivery: decide.HLS, Profile: decide.Remux, NotDirectReasons: []decide.Reason{decide.ReasonUserChoice},
	}}
	if !chunksPlaybackOf(capped).IsSourcePictureDecodable {
		t.Error("a capped device's ladder leaves the film's own picture out; want it kept as a rung")
	}
	reencoded := tokenPlayback{decision: decide.Decision{
		Delivery: decide.HLS, Profile: decide.H264,
		NotDirectReasons: []decide.Reason{decide.ReasonVideoCodec, decide.ReasonUserChoice},
	}}
	if chunksPlaybackOf(reencoded).IsSourcePictureDecodable {
		t.Error("a ladder re-encoding the picture offers the film's own; want it left out")
	}
}
