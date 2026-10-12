// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A playback's need of the server's upload is the rate of the quality it asks for, and every byte
// of a film the server sends it is counted to it (D286 §5).
func TestUpload_APlaybacksNeedAndTheBytesSentItAreCounted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store := nodeproto.PathBytesChunks + "1/chunks-q700k432p/" + grid.ID() + "/"
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files[store+"00001.m4s"] = strings.Repeat("x", 25_000)
	harness.nodeBytes.mutex.Unlock()
	if chunk := harness.getBytePath(t, base+"q700k432p/00001.m4s", user, true, ""); chunk.status != http.StatusOK {
		t.Fatalf("the chunk: %d %s", chunk.status, chunk.body)
	}
	upload := harness.server.dependencies.Load().Playback.TakeUpload(time.Second)
	if want := (playback.Upload{Playbacks: 1, NeedKbps: 700, SentKbps: 200}); upload != want {
		t.Errorf("the upload after a chunk of 25 kB of a 700k quality, over a second = %+v; want %+v", upload, want)
	}
	if again := harness.server.dependencies.Load().Playback.TakeUpload(time.Second); again.SentKbps != 0 {
		t.Errorf("the bytes sent, taken again = %d kbit/s; want them counted afresh", again.SentKbps)
	}
}
