// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// stillTaken records a frame still of fileID as taken for itemID, its image under sourceSha256Hex.
func (world *frameWorld) stillTaken(t *testing.T, itemID, fileID int64, sourceSha256Hex string) {
	t.Helper()
	sourceSha256, err := hex.DecodeString(sourceSha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	world.store(t, itemID, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: new(fmt.Sprintf("file:%d:5000:7", fileID)),
		SourceSha256: sourceSha256,
	})
}

// isStillLive is whether item's frame still is still recorded.
func (world *frameWorld) isStillLive(t *testing.T, itemID int64) bool {
	t.Helper()
	rows, err := world.bench.database.Read().ListLiveArtwork(t.Context(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool { return row.Source == SourceFrame })
}

// filesOf are the names the bench's art directory holds of an image.
func (world *frameWorld) filesOf(sourceSha256Hex string) []string {
	var names []string
	for key := range world.bench.art {
		if name, isOf := strings.CutPrefix(key, sourceSha256Hex+"/"); isOf {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// runSizesJob runs the job that makes sizes of an item's frame still, as a look at a size not held
// queues it (serveNearest).
func (world *frameWorld) runSizesJob(
	t *testing.T, itemID, fileID int64, sourceSha256Hex string, sizes ...nodeproto.ArtSizeName,
) {
	t.Helper()
	world.runImageSizesJob(t, itemID, SourceFrame,
		Ref{Kind: library.ArtStill, SourceRef: fmt.Sprintf("file:%d:5000:7", fileID)}, sourceSha256Hex, sizes...)
}

// runImageSizesJob runs the job that makes sizes of an item's image from a source.
func (world *frameWorld) runImageSizesJob(
	t *testing.T, itemID int64, source string, ref Ref, sourceSha256Hex string, sizes ...nodeproto.ArtSizeName,
) {
	t.Helper()
	world.bench.itemID = itemID
	args, err := json.Marshal(artworkJobArgs{
		ArtworkSource: source, SourceSha256: sourceSha256Hex, SizeNames: sizes, Refs: []Ref{ref},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{
		ID: itemID, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: itemID},
		ArgsJSON: string(args),
	}
	if err := world.bench.run(t, job); err != nil {
		t.Fatal(err)
	}
}

// A frame still recorded as taken whose image the art directory does not hold at start, as after a
// restore from a backup that carries no cache, is not taken yet: its row is retired, the .none note
// left of it goes, and its still is planned again at once; a still whose image is held stays (D507 §3).
func TestLoadHoldings_AFrameStillRecordedAsTakenWithNoImageHeldIsTakenAgain(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	lostID, lostFile := world.video(t, otherVideo)
	heldID, heldFile := world.video(t, otherVideo)
	lost, held := sourceSha256HexOf("a"), sourceSha256HexOf("b")
	world.stillTaken(t, lostID, lostFile, lost)
	world.stillTaken(t, heldID, heldFile, held)
	world.bench.art[lost+"/landscape-s.none"] = nil
	world.bench.art[held+"/landscape-s.webp"] = ourWebP
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if world.isStillLive(t, lostID) || !world.isStillLive(t, heldID) {
		t.Errorf("live stills: lost %v, held %v; want the lost one retired and the held one kept",
			world.isStillLive(t, lostID), world.isStillLive(t, heldID))
	}
	if planned := world.stillsPlanned(t); !slices.Equal(planned, []int64{lostID}) {
		t.Errorf("stills planned for %v; want the lost one's item %d alone, at once", planned, lostID)
	}
	if names := world.filesOf(lost); len(names) != 0 || world.bench.isHeld(lost, "landscape-s.none") {
		t.Errorf("the lost still's files %v; want its note gone, from the directory and its holdings", names)
	}
}

// A frame still whose image goes while the core runs is not taken yet when a size of it is asked
// for: whether nothing of it is held, or the base held is not on disk, its row is retired and its
// still planned again, and no .none note says it cannot be made (D507 §3).
func TestHandle_AFrameStillWhoseImageWentWhileRunningIsTakenAgainAndNotNoted(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	vanishedID, vanishedFile := world.video(t, otherVideo)
	vanished := sourceSha256HexOf("a")
	world.stillTaken(t, vanishedID, vanishedFile, vanished)
	world.bench.art[vanished+"/landscape-s.webp"] = ourWebP
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	delete(world.bench.art, vanished+"/landscape-s.webp") // gone from the disk; the holdings still list it
	neverHeldID, neverHeldFile := world.video(t, otherVideo)
	neverHeld := sourceSha256HexOf("b")
	world.stillTaken(t, neverHeldID, neverHeldFile, neverHeld)
	*world.queue = nil
	world.runSizesJob(t, vanishedID, vanishedFile, vanished, nodeproto.LandscapeS)
	world.runSizesJob(t, neverHeldID, neverHeldFile, neverHeld, nodeproto.LandscapeM)
	for _, still := range []struct {
		itemID          int64
		sourceSha256Hex string
	}{{vanishedID, vanished}, {neverHeldID, neverHeld}} {
		if world.isStillLive(t, still.itemID) {
			t.Errorf("item %d's still is still recorded as taken; want it retired", still.itemID)
		}
		if names := world.filesOf(still.sourceSha256Hex); len(names) != 0 {
			t.Errorf("item %d's still's files %v; want none, no .none note among them", still.itemID, names)
		}
	}
	if planned := world.stillsPlanned(t); !slices.Contains(planned, vanishedID) ||
		!slices.Contains(planned, neverHeldID) {
		t.Errorf("stills planned for %v; want %d and %d taken again", planned, vanishedID, neverHeldID)
	}
	if len(world.bench.node.sent) != 0 {
		t.Errorf("the node was sent %+v; want nothing until the still is taken again", world.bench.node.sent)
	}
}

// A frame still whose base is held but too small for a larger size keeps D362 §1's note: that is a
// fact of the still, not of the cache, and the still is neither retired nor taken again.
func TestHandle_AFrameStillTooSmallForALargerSizeIsNotedSo(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	itemID, fileID := world.video(t, otherVideo)
	small := sourceSha256HexOf("a")
	world.stillTaken(t, itemID, fileID, small)
	world.bench.art[small+"/landscape-s.webp"] = ourWebP
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	*world.queue = nil
	world.runSizesJob(t, itemID, fileID, small, nodeproto.LandscapeL)
	if names := world.filesOf(small); !slices.Equal(names, []string{"landscape-l.none", "landscape-s.webp"}) {
		t.Errorf("the still's files %v; want landscape-l noted as one it cannot be made at", names)
	}
	if !world.isStillLive(t, itemID) || len(world.stillsPlanned(t)) != 0 {
		t.Errorf("the still live %v, planned %v; want it kept and not taken again", world.isStillLive(t, itemID),
			world.stillsPlanned(t))
	}
}

// posterStored records an item's poster from a source as stored, its image under sourceSha256Hex.
func (world *frameWorld) posterStored(t *testing.T, itemID int64, source, ref, sourceSha256Hex string) {
	t.Helper()
	sourceSha256, err := hex.DecodeString(sourceSha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	world.store(t, itemID, sqlcgen.InsertArtworkParams{
		Kind: library.ArtPoster, Source: source, SourceRef: new(ref), SourceSha256: sourceSha256,
	})
}

// livePosters are an item's posters still recorded, from any source.
func (world *frameWorld) livePosters(t *testing.T, itemID int64) []sqlcgen.ListLiveArtworkRow {
	t.Helper()
	rows, err := world.bench.database.Read().ListLiveArtwork(t.Context(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool { return row.Kind != library.ArtPoster })
}

// imagesQueued are the items a provider's or the media's image was queued to be made for.
func (world *frameWorld) imagesQueued(t *testing.T) []int64 {
	t.Helper()
	var queued []int64
	for _, request := range *world.queue {
		var args artworkJobArgs
		if err := json.Unmarshal([]byte(request.ArgsJSON), &args); err != nil {
			t.Fatal(err)
		}
		if args.ArtworkSource != SourceFrame {
			queued = append(queued, request.ItemID)
		}
	}
	return queued
}

// passNaming has the world's library's pass name each item's TMDB poster, at "/<item>.jpg".
func (world *frameWorld) passNaming(itemIDs ...int64) {
	world.bench.pass = func(ctx context.Context, _ int64) {
		var named []ItemImages
		for _, itemID := range itemIDs {
			named = append(named, ItemImages{
				ItemID: itemID, Source: "tmdb",
				Refs: []Ref{{Kind: library.ArtPoster, SourceRef: fmt.Sprintf("/%d.jpg", itemID)}},
			})
		}
		world.bench.service.Take(ctx, Found{Named: named})
	}
}

// An image of any source recorded as stored whose image the art directory does not hold at start,
// as after a restore that carries no cache, is not stored yet: its row is retired, a .none note left
// of it goes, and its library owes a pass, whose ordinary planning fetches it again; an image held
// stays (D510 §1, §2, §4).
func TestLoadHoldings_AnImageOfAnySourceRecordedAsStoredWithNoImageHeldIsMadeAgain(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	providerID, _ := world.video(t, otherVideo)
	localID, localFile := world.video(t, otherVideo)
	heldID, _ := world.video(t, otherVideo)
	provider, local, held := sourceSha256HexOf("a"), sourceSha256HexOf("b"), sourceSha256HexOf("c")
	world.posterStored(t, providerID, "tmdb", fmt.Sprintf("/%d.jpg", providerID), provider)
	world.posterStored(t, localID, SourceLocal, fmt.Sprintf("file:%d", localFile), local)
	world.posterStored(t, heldID, "tmdb", fmt.Sprintf("/%d.jpg", heldID), held)
	world.bench.art[provider+"/portrait-l.none"] = nil
	world.bench.art[held+"/portrait-s.webp"] = ourWebP
	world.passNaming(providerID, heldID)
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(world.livePosters(t, providerID)) != 0 || len(world.livePosters(t, localID)) != 0 ||
		len(world.livePosters(t, heldID)) != 1 {
		t.Errorf("live posters: provider's %v, local %v, held %v; want the lost two retired and the held one kept",
			world.livePosters(t, providerID), world.livePosters(t, localID), world.livePosters(t, heldID))
	}
	if !slices.Equal(world.bench.owedPasses, []int64{world.libraryID}) {
		t.Errorf("passes owed %v; want library %d's, once", world.bench.owedPasses, world.libraryID)
	}
	if queued := world.imagesQueued(t); !slices.Equal(queued, []int64{providerID}) {
		t.Errorf("images queued for %v; want the provider's lost poster of item %d alone", queued, providerID)
	}
	if names := world.filesOf(provider); len(names) != 0 || world.bench.isHeld(provider, "portrait-l.none") {
		t.Errorf("the lost poster's files %v; want its note gone, from the directory and its holdings", names)
	}
}

// A provider's image whose image goes while the core runs is not stored yet when a size of it is
// asked for: whether nothing of it is held, or the base held is not on disk, its row is retired, its
// library owes a pass, and nothing is fetched nor noted as one that cannot be made (D510 §1).
func TestHandle_AProviderImageWhoseImageWentWhileRunningIsFetchedAgainAndNotNoted(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	vanishedID, _ := world.video(t, otherVideo)
	vanished := sourceSha256HexOf("a")
	world.posterStored(t, vanishedID, "tmdb", "/vanished.jpg", vanished)
	world.bench.art[vanished+"/portrait-m.webp"] = ourWebP
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	delete(world.bench.art, vanished+"/portrait-m.webp") // gone from the disk; the holdings still list it
	neverHeldID, _ := world.video(t, otherVideo)
	neverHeld := sourceSha256HexOf("b")
	world.posterStored(t, neverHeldID, "tmdb", "/never-held.jpg", neverHeld)
	world.runImageSizesJob(t, vanishedID, "tmdb", Ref{Kind: library.ArtPoster, SourceRef: "/vanished.jpg"},
		vanished, nodeproto.PortraitS)
	world.runImageSizesJob(t, neverHeldID, "tmdb", Ref{Kind: library.ArtPoster, SourceRef: "/never-held.jpg"},
		neverHeld, nodeproto.PortraitL)
	for _, poster := range []struct {
		itemID          int64
		sourceSha256Hex string
	}{{vanishedID, vanished}, {neverHeldID, neverHeld}} {
		if rows := world.livePosters(t, poster.itemID); len(rows) != 0 {
			t.Errorf("item %d's posters %+v; want its lost one retired", poster.itemID, rows)
		}
		if names := world.filesOf(poster.sourceSha256Hex); len(names) != 0 {
			t.Errorf("item %d's poster's files %v; want none, no .none note among them", poster.itemID, names)
		}
	}
	if !slices.Equal(world.bench.owedPasses, []int64{world.libraryID, world.libraryID}) {
		t.Errorf("passes owed %v; want library %d's for each", world.bench.owedPasses, world.libraryID)
	}
	if len(world.bench.node.sent) != 0 {
		t.Errorf("the node was sent %+v; want nothing until the pass fetches the image again", world.bench.node.sent)
	}
}

// A look at an image the art directory holds nothing of writes nothing, as a reading request never
// does (D303), and queues no sizes under a record of an image no longer there: it asks for the
// sweep, which retires the row and owes its library a pass (D510 §1).
func TestServe_AnImageWithNothingHeldAsksForTheSweepThatRetiresIt(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	itemID, _ := world.video(t, otherVideo)
	lost := sourceSha256HexOf("a")
	world.posterStored(t, itemID, "tmdb", "/lost.jpg", lost)
	*world.queue = nil
	_, err := world.bench.service.Serve(t.Context(), lost, nodeproto.PortraitM, FormatWebP)
	if !errors.Is(err, ErrNoArt) {
		t.Fatalf("Serve = %v, want ErrNoArt", err)
	}
	if world.bench.lostSweepsAsked != 1 || len(world.livePosters(t, itemID)) != 1 || len(world.imagesQueued(t)) != 0 {
		t.Fatalf("sweeps asked %d, posters live %d, queued %v; want the sweep asked, the row untouched "+
			"and nothing queued", world.bench.lostSweepsAsked, len(world.livePosters(t, itemID)), world.imagesQueued(t))
	}
	if _, err := world.bench.service.SweepLost(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rows := world.livePosters(t, itemID); len(rows) != 0 {
		t.Errorf("the item's posters %+v; want the lost one retired", rows)
	}
	if queued := world.imagesQueued(t); !slices.Equal(world.bench.owedPasses, []int64{world.libraryID}) ||
		len(queued) != 0 {
		t.Errorf("passes owed %v, posters queued for %v; want library %d's pass owed and no sizes queued under "+
			"the lost record", world.bench.owedPasses, queued, world.libraryID)
	}
}

// The sweep finds an image deleted from the art directory while the core runs, which no lookup has
// asked for yet: it lists the directory afresh, retires the image's row, forgets what the holdings
// listed of it, and owes its library a pass; an image still on disk stays (D510 §2).
func TestSweepLost_AnImageDeletedFromTheArtDirectoryWhileRunningIsRetired(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	lostID, _ := world.video(t, otherVideo)
	keptID, _ := world.video(t, otherVideo)
	lost, kept := sourceSha256HexOf("a"), sourceSha256HexOf("b")
	world.posterStored(t, lostID, "tmdb", "/lost.jpg", lost)
	world.posterStored(t, keptID, "tmdb", "/kept.jpg", kept)
	world.bench.art[lost+"/portrait-s.webp"] = ourWebP
	world.bench.art[lost+"/portrait-m.webp"] = ourWebP
	world.bench.art[kept+"/portrait-s.webp"] = ourWebP
	if err := world.bench.service.LoadHoldings(t.Context()); err != nil {
		t.Fatal(err)
	}
	delete(world.bench.art, lost+"/portrait-s.webp")
	delete(world.bench.art, lost+"/portrait-m.webp")
	if _, err := world.bench.service.SweepLost(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(world.livePosters(t, lostID)) != 0 || len(world.livePosters(t, keptID)) != 1 {
		t.Errorf("live posters: lost %v, kept %v; want the lost one retired and the kept one kept",
			world.livePosters(t, lostID), world.livePosters(t, keptID))
	}
	if world.bench.isHeld(lost, "portrait-s.webp") || world.bench.isHeld(lost, "portrait-m.webp") ||
		!world.bench.isHeld(kept, "portrait-s.webp") {
		t.Error("the holdings still list the lost image's sizes, or forgot the kept one's")
	}
	if !slices.Equal(world.bench.owedPasses, []int64{world.libraryID}) {
		t.Errorf("passes owed %v; want library %d's, once", world.bench.owedPasses, world.libraryID)
	}
}

// A person's choice whose image the art directory lost is chosen again once the image is stored
// again from the same source and place; a choice of another place, also lost, is not moved to it
// (D510 §3).
func TestRecord_APersonsChoiceTheCacheLostIsChosenAgainWhenStoredAgain(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	itemID, _ := world.video(t, otherVideo)
	otherID, _ := world.video(t, otherVideo)
	chosen, elsewhere := sourceSha256HexOf("a"), sourceSha256HexOf("b")
	world.posterStored(t, itemID, "tmdb", "/chosen.jpg", chosen)
	world.posterStored(t, otherID, "tmdb", "/elsewhere.jpg", elsewhere)
	storetest.ExecRaw(t, world.bench.database.Path(), "UPDATE artwork SET is_chosen = 1")
	if _, err := world.bench.service.SweepLost(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rows := world.livePosters(t, itemID); len(rows) != 0 {
		t.Fatalf("the item's posters %+v; want its lost choice retired", rows)
	}
	again, err := hex.DecodeString(sourceSha256HexOf("c"))
	if err != nil {
		t.Fatal(err)
	}
	for _, stored := range []struct {
		itemID int64
		ref    string
	}{{itemID, "/chosen.jpg"}, {otherID, "/another.jpg"}} {
		want := wantedImage{
			itemID: stored.itemID, source: "tmdb", ref: Ref{Kind: library.ArtPoster, SourceRef: stored.ref},
		}
		if err := world.bench.service.record(t.Context(), want, again, nil); err != nil {
			t.Fatal(err)
		}
	}
	if rows := world.livePosters(t, itemID); len(rows) != 1 || rows[0].IsChosen != 1 {
		t.Errorf("the item's posters %+v; want the one stored again chosen, as the lost one was", rows)
	}
	if rows := world.livePosters(t, otherID); len(rows) != 1 || rows[0].IsChosen != 0 {
		t.Errorf("the other item's posters %+v; want the one of another place not chosen", rows)
	}
}
