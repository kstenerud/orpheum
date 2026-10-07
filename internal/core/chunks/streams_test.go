// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestStreamNamed_ReadsEachStreamsNameBackAndRefusesOthers(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]Stream{
		"picture":            {Kind: nodeproto.StreamPicture},
		"q1200k480p":         {Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}},
		"q50000k2160p":       {Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 50_000, Height: 2160}},
		"sound0-96k":         {Kind: nodeproto.StreamSound, SoundKbps: 96},
		"sound12-boost-128k": {Kind: nodeproto.StreamSound, AudioTrackIndex: 12, IsBoosted: true, SoundKbps: 128},
		"sound0-96k-raw":     {Kind: nodeproto.StreamSound, SoundKbps: 96, IsUnlevelled: true},
		"sound1-boost-96k-raw": {
			Kind: nodeproto.StreamSound, AudioTrackIndex: 1, IsBoosted: true, SoundKbps: 96, IsUnlevelled: true,
		},
	} {
		got, isStream := StreamNamed(name)
		if !isStream || got != want || got.Name() != name {
			t.Errorf("StreamNamed(%q) = %+v, %t, named %q; want %+v, named as it was", name, got, isStream,
				got.Name(), want)
		}
	}
	for _, name := range []string{
		"", "Picture", "video", "q1200k", "q01200k480p", "q1200k480", "sound", "sound01", "sound0-boosted",
		"sound0/../picture", "audio0", "sound1000", "sound0", "sound0-boost", "sound0-96", "sound0-0k",
		"sound0-raw", "sound0-96k-raw-raw", "picture-raw",
	} {
		if got, isStream := StreamNamed(name); isStream {
			t.Errorf("StreamNamed(%q) = %+v; want no stream", name, got)
		}
	}
}

func TestGridID_IsTheHashOfItsStartsAndLengthAndNamesTheStore(t *testing.T) {
	t.Parallel()
	grid := Grid{StartsMs: []int64{0, 2000, 4000}, FilmDurationMs: 7000}
	// The first 16 hex characters of the SHA-256 of "durationMs=7000;startsMs=0,2000,4000".
	const want = "eedaa441ddeb8d18"
	if got := grid.ID(); got != want {
		t.Errorf("the grid's id = %q, want %q", got, want)
	}
	for what, other := range map[string]Grid{
		"another start":  {StartsMs: []int64{0, 2000, 4500}, FilmDurationMs: 7000},
		"another length": {StartsMs: []int64{0, 2000, 4000}, FilmDurationMs: 7001},
	} {
		if other.ID() == grid.ID() {
			t.Errorf("a grid with %s has the same id, %q", what, other.ID())
		}
	}
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 700, Height: 432}}
	if dir := StoreDir(3, quality, grid.ID()); dir != "3/chunks-q700k432p/"+want {
		t.Errorf("the store's directory = %q, want <file_id>/chunks-<stream>/<grid_id>", dir)
	}
}

func TestStoreFileNamed_IsTheInitOrAChunkByItsNumber(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]struct {
		chunk  int
		isInit bool
	}{
		"init.mp4": {isInit: true}, "00000.m4s": {}, "00042.m4s": {chunk: 42}, "123456.m4s": {chunk: 123_456},
	} {
		chunk, isInit, isStoreFile := storeFileNamed(name)
		if !isStoreFile || chunk != want.chunk || isInit != want.isInit {
			t.Errorf("storeFileNamed(%q) = %d, %t, %t; want %+v", name, chunk, isInit, isStoreFile, want)
		}
	}
	for _, name := range []string{"", "init.m4s", "42.m4s", "0042.m4s", "00042.mp4", "../00042.m4s", "store.json"} {
		if _, _, isStoreFile := storeFileNamed(name); isStoreFile {
			t.Errorf("storeFileNamed(%q) names a store's file; want none", name)
		}
	}
}
