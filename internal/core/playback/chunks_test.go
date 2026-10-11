// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"
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
