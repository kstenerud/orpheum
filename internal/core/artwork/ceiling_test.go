// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// sourceSha256HexOf is a made-up image's source SHA-256, in hex.
func sourceSha256HexOf(letter string) string { return strings.Repeat(letter, 64) }

// show records that the bench's item shows an image, removed at removedAt (ms) when not zero.
func (bench *bench) show(t *testing.T, kind, sourceSha256Hex string, removedAt int64) {
	t.Helper()
	removed := "NULL"
	if removedAt != 0 {
		removed = fmt.Sprint(removedAt)
	}
	storetest.ExecRaw(t, bench.database.Path(), fmt.Sprintf(
		"INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, removed_at) "+
			"VALUES (%d, '%s', 'tmdb', '/%s.jpg', X'%s', %s)",
		bench.itemID, kind, sourceSha256Hex[:4], sourceSha256Hex, removed))
}

// hold makes the bench hold files of the sizes given, in fractions of a GiB.
func (bench *bench) hold(sourceSha256Hex string, sizesGiB map[string]float64) {
	for name, size := range sizesGiB {
		bench.service.holdings.add(sourceSha256Hex, name, gibToBytes(size))
		bench.art[sourceSha256Hex+"/"+name] = nil
	}
}

// gibToBytes is a fraction of a GiB in bytes, as hold counts it.
func gibToBytes(fraction float64) int64 { return int64(fraction * float64(diskspace.BytesPerGiB)) }

func (bench *bench) isHeld(sourceSha256Hex, name string) bool {
	_, found := bench.service.holdings.sizeBytesByNameBySourceSha256[sourceSha256Hex][name]
	return found
}

func TestSweep_EvictsInTheOrderOfD212DownToLowWaterAndNeverAWallSize(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	settingstest.Set(t, bench.settings, settings.ArtworkCacheCeilingGiB, "1")
	clock := bench.clock
	poster, backdrop := sourceSha256HexOf("a"), sourceSha256HexOf("b")
	removed, orphan := sourceSha256HexOf("c"), sourceSha256HexOf("d")
	bench.show(t, library.ArtPoster, poster, 0)
	bench.show(t, library.ArtBackdrop, backdrop, 0)
	bench.show(t, library.ArtPoster, removed, clock.Now().Add(-24*time.Hour).UnixMilli()) // within its grace
	bench.hold(poster, map[string]float64{
		"portrait-s.webp": 0.01, "portrait-m.webp": 0.1, "portrait-l.webp": 0.3, "portrait-m.jpeg": 0.2,
	})
	bench.hold(backdrop, map[string]float64{
		"landscape-s.webp": 0.01, "landscape-m.webp": 0.2, "landscape-l.webp": 0.3,
	})
	bench.hold(removed, map[string]float64{"portrait-m.webp": 0.2})
	bench.hold(orphan, map[string]float64{"portrait-s.webp": 0.05})

	pressure, err := bench.service.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// 1.37 GiB against 1: the removed item's and the orphan's images, then the on-request size, to 0.82.
	for _, gone := range [][2]string{
		{removed, "portrait-m.webp"}, {orphan, "portrait-s.webp"}, {poster, "portrait-l.webp"},
	} {
		if bench.isHeld(gone[0], gone[1]) {
			t.Errorf("%s %s is still held", gone[0][:4], gone[1])
		}
	}
	for _, kept := range [][2]string{
		{poster, "portrait-s.webp"},
		{poster, "portrait-m.webp"},
		{poster, "portrait-m.jpeg"},
		{backdrop, "landscape-s.webp"},
		{backdrop, "landscape-m.webp"},
		{backdrop, "landscape-l.webp"},
	} {
		if !bench.isHeld(kept[0], kept[1]) {
			t.Errorf("%s %s was evicted; the sweep should stop at 90%% of the ceiling", kept[0][:4], kept[1])
		}
	}
	required := gibToBytes(0.1) + 2*gibToBytes(0.01)
	if pressure.IsHighWater || pressure.RequiredBytes != required || pressure.TotalBytes > gibToBytes(0.9) {
		t.Errorf("pressure %+v; want %d required, under the low water, no high water", pressure, required)
	}
}

func TestSweep_UnderTheCeilingOnlyARemovedItemsImagesGoAndOnlyAfterTheirGrace(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	clock := bench.clock
	removed := sourceSha256HexOf("c")
	bench.show(t, library.ArtPoster, removed, clock.Now().UnixMilli())
	bench.hold(removed, map[string]float64{"portrait-m.webp": 0.001})
	bench.hold(sourceSha256HexOf("a"), map[string]float64{"portrait-l.webp": 0.001})
	if _, err := bench.service.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !bench.isHeld(removed, "portrait-m.webp") {
		t.Fatal("a removed item's image went within its grace, under the ceiling")
	}
	clock.Advance(31 * 24 * time.Hour)
	if _, err := bench.service.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if bench.isHeld(removed, "portrait-m.webp") {
		t.Error("a removed item's image stayed after its 30 days' grace")
	}
}

func TestSweep_HighWaterWhenWhatIsNeverEvictedExceedsTheCeiling(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	settingstest.Set(t, bench.settings, settings.ArtworkCacheCeilingGiB, "2")
	wall := sourceSha256HexOf("a")
	bench.show(t, library.ArtPoster, wall, 0)
	bench.hold(wall, map[string]float64{"portrait-m.webp": 2.5})
	pressure, err := bench.service.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	isThreeGiB := pressure.RecommendedBytes == 3*diskspace.BytesPerGiB
	if !pressure.IsHighWater || !isThreeGiB || !bench.isHeld(wall, "portrait-m.webp") {
		t.Errorf("pressure %+v; want high water, 3 GiB recommended (2.5 + 15%%), and the wall size kept", pressure)
	}
	if got := bench.service.Pressure(); !got.IsHighWater {
		t.Error("the last sweep's pressure is not kept for the admin to see")
	}
}

func TestFlush_WritesWhenImagesWereServedAndLoadHoldingsReadsItBack(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	sourceSha256Hex := sourceSha256HexOf("a")
	bench.art[sourceSha256Hex+"/portrait-s.webp"] = ourWebP
	if _, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitS, FormatWebP); err != nil {
		t.Fatal(err)
	}
	if err := bench.service.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := NewService(bench.service.parts)
	if err := fresh.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	today := bench.service.parts.Clock.Now().Unix() / 86400
	servedDay := fresh.holdings.lastServedDayBySourceSha256[sourceSha256Hex]
	if servedDay != today || fresh.holdings.totalBytes != int64(len(ourWebP)) {
		t.Errorf("after Load: served day %d (want %d), total %d", servedDay, today, fresh.holdings.totalBytes)
	}
}

func TestHandle_AnEvictedSizeIsCutFromTheBaseWithoutFetching(t *testing.T) {
	t.Parallel()
	source := []byte("backdrop")
	provider := &failingFetcher{err: ErrNoImage} // were it asked, it has nothing
	bench := newBench(t, fakeFetcher{"/back.jpg": source})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		var images []nodeproto.ArtImage
		for _, size := range command.SizeNames {
			images = append(images, nodeproto.ArtImage{SizeName: size, WebP: ourWebP})
		}
		return nodeproto.ArtworkResult{Images: images}
	}
	bench.handle(t, Ref{Kind: library.ArtBackdrop, SourceRef: "/back.jpg"})
	sourceSha256Hex := Received(source).sourceSha256Hex()
	if err := bench.service.evict(candidate{sourceSha256Hex: sourceSha256Hex, name: "landscape-m.webp"}); err != nil {
		t.Fatal(err)
	}
	served, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.LandscapeM, FormatWebP)
	if err != nil || served.RedirectName != "landscape-s.webp" || len(*queue) != 1 {
		t.Fatalf("an evicted size: %+v, %v, queued %+v", served, err, *queue)
	}
	bench.service.parts.Fetchers["tmdb"] = provider
	sent := len(bench.node.sent)
	job := jobs.Job{
		ID: 3, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: (*queue)[0].ArgsJSON,
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	cutFromBase := len(bench.node.sent) == sent+1 && string(bench.node.sent[sent].SourceImage) == string(ourWebP)
	if provider.callCount != 0 || !cutFromBase {
		t.Errorf("TMDB asked %d times; the node was sent %d; want the base cut by the node, nothing fetched",
			provider.callCount, len(bench.node.sent)-sent)
	}
	if !bench.isHeld(sourceSha256Hex, "landscape-m.webp") {
		t.Error("landscape-m was not made again")
	}
}
