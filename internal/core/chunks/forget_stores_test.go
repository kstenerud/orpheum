// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// retimedFilmOne keeps film 1's probe as the fixture's, its picture marked for re-timing (D392 §3;
// file 5667).
func retimedFilmOne(t *testing.T, service *Service) {
	t.Helper()
	durationMs := int64(filmMinutes * 60_000)
	librarytest.Probe(t, service.database, librarytest.ProbeSpec{
		FileID: 1, DurationMs: durationMs,
		ProbeJSON: fmt.Sprintf(`{"format":{"durationMs":%d,"bitRateBps":4000000},"streams":[`+
			`{"kind":"video","codec":"h264","video":{"width":1920,"height":1080,"needsRetiming":true}},`+
			`{"streamIndex":1,"kind":"audio","codec":"ac3","language":"en","audio":{"channelCount":6}},`+
			`{"streamIndex":2,"kind":"audio","codec":"aac","audio":{"channelCount":2}}]}`, durationMs),
	})
}

// A film whose picture's facts changed has its picture's chunks made into new stores, on the same
// grid, and its sounds' kept; a run of a store made before ends for good, never adding a chunk made
// from the old facts to it (D392 §3).
func TestStoreKey_AFilmWhosePicturesFactsChangedHasItsPictureMadeIntoNewStores(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	gridID := mustGrid(t, service).ID()
	picture := Stream{Kind: nodeproto.StreamPicture}
	sound := Stream{Kind: nodeproto.StreamSound, AudioTrackIndex: 1, SoundKbps: 96}
	before, soundBefore := testStoreKey(t, service, picture, gridID), testStoreKey(t, service, sound, gridID)
	staleID, err := service.storeIDOf(t.Context(), 1, picture, before)
	if err != nil {
		t.Fatal(err)
	}
	retimedFilmOne(t, service)
	if after := testStoreKey(t, service, picture, mustGrid(t, service).ID()); after == before {
		t.Errorf("the picture's store is %s after its picture was marked for re-timing; want another", after)
	}
	if after := testStoreKey(t, service, sound, gridID); after != soundBefore {
		t.Errorf("a sound's store is %s after the picture changed; want it kept, %s", after, soundBefore)
	}
	ended := fixture.resolver.Start(t, service.HandleMakeChunks, jobs.Job{
		ID: 7, Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: staleID},
		ArgsJSON: `{"firstChunk":0,"lastChunk":1}`, Priority: jobs.PriorityPlayNow,
	})
	if err := testutil.Receive(t, ended, "the stale store's run to end"); !errors.Is(err, ErrGridChanged) {
		t.Errorf("a run of the store made before the picture changed ended %v; want ErrGridChanged", err)
	}
}

// A quality's store is named by its encode's recipe too, so every quality's store made before each
// encode kept its source's time base, named by the grid and the picture's facts alone, is made again,
// and a run of one ends for good rather than write an init at another timescale over its chunks; the
// copied picture's stores are kept, and a quality's still follow the picture's facts (D392 §3).
func TestStoreKey_AQualitysStoreIsNamedByItsEncodesRecipeToo(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	service := fixture.service
	gridID := mustGrid(t, service).ID()
	file, err := library.PlayableFile(t.Context(), service.database.Read(), 1)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		t.Fatal(err)
	}
	madeBefore := gridID + library.PictureChunksFactsKey(probe)
	quality := Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}
	if key := testStoreKey(t, service, Stream{Kind: nodeproto.StreamPicture}, gridID); key != madeBefore {
		t.Errorf("the copied picture's store is %s; want it kept, %s", key, madeBefore)
	}
	key := testStoreKey(t, service, quality, gridID)
	if key == madeBefore || len(key) != len(madeBefore) {
		t.Fatalf("the quality's store is %s; want another of the same length as the one made before, %s",
			key, madeBefore)
	}
	staleID, err := service.storeIDOf(t.Context(), 1, quality, madeBefore)
	if err != nil {
		t.Fatal(err)
	}
	ended := fixture.resolver.Start(t, service.HandleMakeChunks, jobs.Job{
		ID: 7, Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: staleID},
		ArgsJSON: `{"firstChunk":0,"lastChunk":1}`, Priority: jobs.PriorityPlayNow,
	})
	if err := testutil.Receive(t, ended, "the old store's run to end"); !errors.Is(err, ErrGridChanged) {
		t.Errorf("a run of the quality's store made before ended %v; want ErrGridChanged", err)
	}
	retimedFilmOne(t, service)
	if after := testStoreKey(t, service, quality, mustGrid(t, service).ID()); after == key {
		t.Errorf("the quality's store is %s after its picture was marked for re-timing; want another", after)
	}
}

// A film's picture's stores, copied and at each quality, are thrown out once their facts changed, its
// sounds' kept; a track's sound stores, every mix and its copy, and no other track's; what the
// tracker held of them with them. A second time finds nothing (D392 §3).
func TestForgetStores_ThrowsOutTheStoresOfTheChangedStreamAlone(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	ctx, gridID := t.Context(), mustGrid(t, service).ID()
	streams := []Stream{
		{Kind: nodeproto.StreamPicture},
		{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}},
		{Kind: nodeproto.StreamSound, AudioTrackIndex: 0, SoundKbps: 96},
		{Kind: nodeproto.StreamSoundCopied, AudioTrackIndex: 0},
		{Kind: nodeproto.StreamSound, AudioTrackIndex: 1, SoundKbps: 96, IsBoosted: true},
	}
	for _, stream := range streams {
		key := testStoreKey(t, service, stream, gridID)
		if _, err := service.storeIDOf(ctx, 1, stream, key); err != nil {
			t.Fatal(err)
		}
		service.runs.served(StoreDir(1, stream, key), 0)
	}
	kept := func() []string {
		t.Helper()
		ids, err := service.cache.Read().ListChunkStoresOfFile(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, id := range ids {
			row, err := service.cache.Read().GetChunkStore(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, row.Stream)
		}
		slices.Sort(names)
		return names
	}
	if isDropped, err := service.ForgetPictureStores(ctx, library.DerivedOf{FileID: 1}); err != nil || !isDropped {
		t.Fatalf("ForgetPictureStores = %v, %v; want the picture's stores thrown out", isDropped, err)
	}
	if got, want := kept(), []string{"sound0-96k", "sound0-own", "sound1-boost-96k"}; !slices.Equal(got, want) {
		t.Errorf("after the picture's, the stores kept are %v; want %v", got, want)
	}
	if service.runs.isMadeFromThrough(StoreDir(1, streams[0], testStoreKey(t, service, streams[0], gridID)), 0, 0) {
		t.Error("the tracker still holds the picture's chunk made")
	}
	trackZero := library.DerivedOf{FileID: 1, StreamIndex: 1, AudioTrack: 0}
	if isDropped, err := service.ForgetSoundStores(ctx, trackZero); err != nil || !isDropped {
		t.Fatalf("ForgetSoundStores = %v, %v; want track 0's stores thrown out", isDropped, err)
	}
	if got, want := kept(), []string{"sound1-boost-96k"}; !slices.Equal(got, want) {
		t.Errorf("after track 0's, the stores kept are %v; want %v", got, want)
	}
	if isDropped, err := service.ForgetSoundStores(ctx, trackZero); err != nil || isDropped {
		t.Errorf("a second ForgetSoundStores = %v, %v; want nothing left to throw out", isDropped, err)
	}
}
