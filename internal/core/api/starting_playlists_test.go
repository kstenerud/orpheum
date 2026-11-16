// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// A playback started on its film's ladder is answered with the playlists its player loads first, the
// master and the playlists of the quality and the sound it starts on, each as the path it names
// answers it, so its player asks for none of them (D502 §3); while its film has no grid, it is
// answered with none, and its player asks for the master, which says when to ask again.
func TestPlayback_AStartOnTheLadderCarriesItsFirstPlaylists(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	if early := harness.play(t, user, chrome(1)); early.OnLadder != nil && early.OnLadder.Playlists != nil {
		t.Errorf("a start before film 1 has a grid was offered %+v; want no playlists", early.OnLadder.Playlists)
	}
	harness.keepFilmOnesTimeline(t)
	started := harness.play(t, user, chrome(1))
	if started.OnLadder == nil || started.OnLadder.Playlists == nil || started.OnLadder.Playlists.Sound == nil {
		t.Fatalf("a start on film 1's ladder was offered %+v; want its master, picture and sound playlists",
			started.OnLadder)
	}
	playlists := started.OnLadder.Playlists
	base := api.Prefix + "/bytes/media/chunks/" + started.MediaToken + "/"
	soundName := strings.TrimSuffix(strings.TrimPrefix(playlists.Sound.URL, base), "/stream.m3u8")
	if !strings.Contains(playlists.Master.Text, `URI="`+soundName+`/stream.m3u8"`) {
		t.Errorf("the start's sound, %s, is not among its master's:\n%s", playlists.Sound.URL, playlists.Master.Text)
	}
	for _, each := range []struct {
		carried api.PlaylistBody
		url     string
	}{
		{playlists.Master, base + "master.m3u8"},
		{playlists.Picture, base + "picture/stream.m3u8"},
		{*playlists.Sound, base + soundName + "/stream.m3u8"},
	} {
		asked := harness.getBytePath(t, each.url, user, true, "")
		if each.carried.URL != each.url || asked.status != http.StatusOK || each.carried.Text != asked.body {
			t.Errorf("the start carried %s as\n%s\nwant %s as it answers (%d)\n%s",
				each.carried.URL, each.carried.Text, each.url, asked.status, asked.body)
		}
	}
}
