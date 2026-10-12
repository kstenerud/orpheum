// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
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

// Over the films' share of the server's upload, a playback is lowered, and every chunk's answer
// tells its player the ceiling; with the setting unanswered there is none (D286 §5).
func TestUpload_APlaybackOverTheShareIsToldItsCeilingWithEachChunk(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	if master := harness.getBytePath(t, base+"master.m3u8", user, true, ""); master.status != http.StatusOK {
		t.Fatalf("the master playlist: %d %s", master.status, master.body)
	}
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store := nodeproto.PathBytesChunks + "1/chunks-q2000k540p/" + grid.ID() + "/"
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files[store+"00001.m4s"] = "chunk one"
	harness.nodeBytes.files[store+"00002.m4s"] = "chunk two"
	harness.nodeBytes.mutex.Unlock()
	if first := harness.getBytePath(t, base+"q2000k540p/00001.m4s", user, true, ""); first.status != http.StatusOK ||
		first.header.Get(CeilingHeader) != "" {
		t.Fatalf("a chunk, the upload unanswered: %d, ceiling %q; want none", first.status,
			first.header.Get(CeilingHeader))
	}
	harness.setSetting(t, "server_upload_mbps", "1") // a share of 850 kbit/s
	ctx, cancel := context.WithCancel(t.Context())
	tick, done := make(chan time.Time), make(chan struct{})
	logger := slog.New(slog.NewJSONHandler(harness.logs, nil))
	go func() {
		harness.server.dependencies.Load().Playback.KeepUnderShare(ctx, tick, logger)
		close(done)
	}()
	tick <- harness.clock.Now()
	harness.logs.AwaitLine(t, `"msg":"a playback's quality was kept within the server's upload"`,
		`"toCeilingKbps":796`, `"shareKbps":850`)
	cancel()
	testutil.Receive(t, done, "the upload's keeping to stop")
	next := harness.getBytePath(t, base+"q2000k540p/00002.m4s", user, true, "")
	if ceiling := next.header.Get(CeilingHeader); ceiling != "796" {
		t.Errorf("the next chunk's ceiling = %q; want 796, the highest quality under the share, 700k "+
			"and its sound", ceiling)
	}
}

// A playback starting on its ladder takes what is left of the films' share, counting the playbacks
// started before it from their start: of two started a moment apart under 2550 kbit/s, the first
// takes the film's own picture, 2096 with its sound, the second what is left, the lowest (D286 §5).
func TestUpload_EachStartTakesWhatIsLeftOfTheShare(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	harness.setSetting(t, "server_upload_mbps", "3")
	first, second := harness.play(t, user, chrome(1)), harness.play(t, user, chrome(1))
	if first.OnLadder == nil || second.OnLadder == nil {
		t.Fatalf("the starts = %+v, %+v; want both on the ladder", first, second)
	}
	if first.OnLadder.QualityKbps != 2400 || second.OnLadder.QualityKbps != 400 {
		t.Errorf("the starts' qualities = %d, %d; want the film's own, 2400, and 400", first.OnLadder.QualityKbps,
			second.OnLadder.QualityKbps)
	}
	chunk := Prefix + "/bytes/media/chunks/" + second.MediaToken + "/q400k360p/00001.m4s"
	if ceiling := harness.getBytePath(t, chunk, user, true, "").header.Get(CeilingHeader); ceiling != "496" {
		t.Errorf("the second's first chunk says a ceiling of %q; want 496", ceiling)
	}
}
