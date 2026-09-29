// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// hashOf is a made-up image hash.
func hashOf(letter string) string { return strings.Repeat(letter, 64) }

// show records that the bench's item shows an image, removed at removedAt (ms) when not zero.
func (bench *bench) show(t *testing.T, kind, hash string, removedAt int64) {
	t.Helper()
	removed := "NULL"
	if removedAt != 0 {
		removed = fmt.Sprint(removedAt)
	}
	storetest.ExecRaw(t, bench.database.Path(), fmt.Sprintf(
		"INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, removed_at) "+
			"VALUES (%d, '%s', 'tmdb', '/%s.jpg', X'%s', %s)",
		bench.itemID, kind, hash[:4], hash, removed))
}

// hold makes the bench hold files of the sizes given, in fractions of a GiB.
func (bench *bench) hold(hash string, files map[string]float64) {
	for name, size := range files {
		bench.service.holdings.add(hash, name, gibs(size))
		bench.art[hash+"/"+name] = nil
	}
}

// gibs is a fraction of a GiB in bytes, as hold counts it.
func gibs(fraction float64) int64 { return int64(fraction * float64(gib)) }

func (bench *bench) holds(hash, name string) bool {
	_, found := bench.service.holdings.files[hash][name]
	return found
}

func TestSweep_EvictsInTheOrderOfD212DownToLowWaterAndNeverAWallSize(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{})
	settingstest.Set(t, bench.config, settings.ArtworkCacheCeilingGiB, "1")
	clock := bench.clock
	poster, backdrop, removed, orphan := hashOf("a"), hashOf("b"), hashOf("c"), hashOf("d")
	bench.show(t, identify.ArtPoster, poster, 0)
	bench.show(t, identify.ArtBackdrop, backdrop, 0)
	bench.show(t, identify.ArtPoster, removed, clock.Now().Add(-24*time.Hour).UnixMilli()) // within its grace
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
		if bench.holds(gone[0], gone[1]) {
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
		if !bench.holds(kept[0], kept[1]) {
			t.Errorf("%s %s was evicted; the sweep should stop at 90%% of the ceiling", kept[0][:4], kept[1])
		}
	}
	required := gibs(0.1) + 2*gibs(0.01)
	if pressure.HighWater || pressure.RequiredBytes != required || pressure.TotalBytes > gibs(0.9) {
		t.Errorf("pressure %+v; want %d required, under the low water, no high water", pressure, required)
	}
}

func TestSweep_UnderTheCeilingOnlyARemovedItemsImagesGoAndOnlyAfterTheirGrace(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{})
	clock := bench.clock
	removed := hashOf("c")
	bench.show(t, identify.ArtPoster, removed, clock.Now().UnixMilli())
	bench.hold(removed, map[string]float64{"portrait-m.webp": 0.001})
	bench.hold(hashOf("a"), map[string]float64{"portrait-l.webp": 0.001})
	if _, err := bench.service.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !bench.holds(removed, "portrait-m.webp") {
		t.Fatal("a removed item's image went within its grace, under the ceiling")
	}
	clock.Advance(31 * 24 * time.Hour)
	if _, err := bench.service.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if bench.holds(removed, "portrait-m.webp") {
		t.Error("a removed item's image stayed after its 30 days' grace")
	}
}

func TestSweep_HighWaterWhenWhatIsNeverEvictedExceedsTheCeiling(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{})
	settingstest.Set(t, bench.config, settings.ArtworkCacheCeilingGiB, "2")
	wall := hashOf("a")
	bench.show(t, identify.ArtPoster, wall, 0)
	bench.hold(wall, map[string]float64{"portrait-m.webp": 2.5})
	pressure, err := bench.service.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !pressure.HighWater || pressure.RecommendedBytes != 3*gib || !bench.holds(wall, "portrait-m.webp") {
		t.Errorf("pressure %+v; want high water, 3 GiB recommended (2.5 + 15%%), and the wall size kept", pressure)
	}
	if got := bench.service.Pressure(); !got.HighWater {
		t.Error("the last sweep's pressure is not kept for the admin to see")
	}
}

func TestFlush_WritesWhenImagesWereServedAndLoadReadsItBack(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{})
	hash := hashOf("a")
	bench.art[hash+"/portrait-s.webp"] = ourWebP
	if _, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitS, FormatWebP); err != nil {
		t.Fatal(err)
	}
	if err := bench.service.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := NewService(bench.service.parts)
	if err := fresh.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	today := bench.service.parts.Clock.Now().Unix() / 86400
	if fresh.holdings.lastServedDayByHash[hash] != today || fresh.holdings.total != int64(len(ourWebP)) {
		t.Errorf("after Load: served day %d (want %d), total %d", fresh.holdings.lastServedDayByHash[hash], today,
			fresh.holdings.total)
	}
}

func TestHandle_AnEvictedSizeIsCutFromTheBaseWithoutFetching(t *testing.T) {
	t.Parallel()
	source := []byte("backdrop")
	provider := &failing{err: ErrNoImage} // were it asked, it has nothing
	bench := newBench(t, fetched{"/back.jpg": source})
	queue := &queued{}
	bench.service.parts.Queue = queue
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		var images []nodeproto.ArtImage
		for _, size := range command.SizeNames {
			images = append(images, nodeproto.ArtImage{SizeName: size, WebP: ourWebP})
		}
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: images}
	}
	bench.handle(t, owedRef{Kind: identify.ArtBackdrop, SourceRef: "/back.jpg"})
	hash := Received(source).hash()
	if err := bench.service.evict(candidate{hash: hash, name: "landscape-m.webp"}); err != nil {
		t.Fatal(err)
	}
	served, err := bench.service.Serve(t.Context(), hash, nodeproto.LandscapeM, FormatWebP)
	if err != nil || served.Redirect != "landscape-s.webp" || len(*queue) != 1 {
		t.Fatalf("an evicted size: %+v, %v, queued %+v", served, err, *queue)
	}
	bench.service.parts.Fetchers["tmdb"] = provider
	sent := len(bench.node.sent)
	job := jobs.Job{ID: 3, Kind: jobs.KindArtwork, Target: jobs.Target{ItemID: bench.itemID}, Args: (*queue)[0].Args}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	cutFromBase := len(bench.node.sent) == sent+1 && string(bench.node.sent[sent].SourceImage) == string(ourWebP)
	if provider.calls != 0 || !cutFromBase {
		t.Errorf("TMDB asked %d times; the node was sent %d; want the base cut by the node, nothing fetched",
			provider.calls, len(bench.node.sent)-sent)
	}
	if !bench.holds(hash, "landscape-m.webp") {
		t.Error("landscape-m was not made again")
	}
}
