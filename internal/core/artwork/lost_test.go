// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
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
	world.bench.itemID = itemID
	args, err := json.Marshal(artworkJobArgs{
		ArtworkSource: SourceFrame, SourceSha256: sourceSha256Hex, SizeNames: sizes,
		Refs: []Ref{{Kind: library.ArtStill, SourceRef: fmt.Sprintf("file:%d:5000:7", fileID)}},
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
