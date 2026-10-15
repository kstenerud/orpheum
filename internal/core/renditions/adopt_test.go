// SPDX-License-Identifier: AGPL-3.0-or-later

package renditions

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// manifestOf is the manifest of the making run, from a source whose payload hash is hash.
func manifestOf(run renditionRun, hash string) nodeproto.RenditionManifest {
	return nodeproto.RenditionManifest{
		Prepare: nodeproto.Prepare{
			Video: nodeproto.BytesFile{PayloadHash: hash}, Profile: nodeproto.PrepareProfile(run.profile),
			RenditionDir: run.dir(), SegmentLengthSeconds: 4,
			AudioTracks: []nodeproto.PrepareAudio{{Language: "fr", CarriedGainDB: -3.5}},
		},
		MadeAtMs: 1000, SizeBytes: 77, VideoEncoder: "copy",
	}
}

// The index is reconciled with the renditions a node's hello lists (D238 §2): one it does not know, as
// after a cache.db discard, is adopted complete when its source is still that file on that node;
// one whose making it names is kept, complete though the node's word on its end was lost; one it
// knows under another making, or whose source changed or is gone, is not kept, and the node evicts
// it.
func TestAdoptRenditions_RebuildsTheIndexFromTheNodesManifestsAndKeepsOnlyWhatItNames(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	for _, fileID := range []int64{3, 4, 5, 6} {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf("INSERT INTO file (id, library_id, node_id, path, "+
			"payload_hash, filing, first_seen_at, last_seen_at) "+
			"VALUES (%d, %d, %d, '/m/f.mkv', X'0a0b', 'unidentified', 1, 1)", fileID, libraryID, nodeID),
			fmt.Sprintf("INSERT INTO file_probe (file_id, probe_json) VALUES (%d, '{}')", fileID))
	}
	cache := storetest.OpenCache(t)
	clockSource := clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	service := &Service{
		database: database, cache: cache, clock: clockSource, logger: slog.New(slog.DiscardHandler),
	}
	unknown := renditionRun{fileID: 3, profile: "remux", runID: "aaaaaaaaaaaaaaaa"}
	unheard := renditionRun{fileID: 4, profile: "remux", runID: "bbbbbbbbbbbbbbbb"}
	remade := renditionRun{fileID: 5, profile: "remux", runID: "cccccccccccccccc"}
	changed := renditionRun{fileID: 6, profile: "remux", runID: "dddddddddddddddd"}
	gone := renditionRun{fileID: 9, profile: "remux", runID: "eeeeeeeeeeeeeeee"}
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		for _, known := range []renditionRun{unheard, {fileID: 5, profile: "remux", runID: "ffffffffffffffff"}} {
			if _, err := queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
				SourceFile: known.fileID, SourcePayloadHash: []byte{0x0a, 0x0b}, Profile: known.profile, NodeID: nodeID,
				SegmentLengthSeconds: 4, RunID: known.runID, CreatedAt: 1,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := service.AdoptRenditions(t.Context(), nodeID, []nodeproto.RenditionManifest{
		manifestOf(unknown, "0a0b"), manifestOf(unheard, "0a0b"), manifestOf(remade, "0a0b"),
		manifestOf(changed, "0c0d"), manifestOf(gone, "0a0b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{unknown.dir(), unheard.dir()}; !slices.Equal(kept, want) {
		t.Errorf("kept %v, want %v", kept, want)
	}
	checkAdopted(t, cache, 3, nodeID, unknown.runID)
	if heard, isFound := remuxOf(t, cache, 4); !isFound || heard.State != renditionComplete || *heard.SizeBytes != 77 {
		t.Errorf("the rendition whose end was not heard is %+v, want it complete", heard)
	}
	for _, notAdopted := range []int64{6, 9} {
		if _, isFound := remuxOf(t, cache, notAdopted); isFound {
			t.Errorf("file %d's rendition, whose source changed or is gone, was adopted", notAdopted)
		}
	}
}

// checkAdopted fails the test unless the index holds file fileID's remux adopted complete, as
// manifestOf's says, from node nodeID's making runID.
func checkAdopted(t *testing.T, cache *store.Cache, fileID, nodeID int64, runID string) {
	t.Helper()
	adopted, isFound := remuxOf(t, cache, fileID)
	if !isFound {
		t.Fatalf("file %d's remux was not adopted", fileID)
	}
	got := []any{
		adopted.State, adopted.RunID, adopted.NodeID, adopted.SegmentLengthSeconds, *adopted.SizeBytes,
		adopted.CarriedGainsDBJSON, adopted.AudioLanguagesJSON, adopted.CreatedAt,
	}
	want := []any{renditionComplete, runID, nodeID, int64(4), int64(77), "[-3.5]", `["fr"]`, int64(1000)}
	if !slices.Equal(got, want) {
		t.Errorf("adopted %v, want it complete as its manifest says, %v", got, want)
	}
}

// remuxOf is the index's remux of a file, and whether it has one.
func remuxOf(t *testing.T, cache *store.Cache, fileID int64) (cachegen.PreparedRendition, bool) {
	t.Helper()
	rendition, err := store.OrNotFound(cache.Read().GetRenditionOfFileAndProfile(t.Context(),
		cachegen.GetRenditionOfFileAndProfileParams{SourceFile: fileID, Profile: "remux"}))
	if errors.Is(err, store.ErrNotFound) {
		return rendition, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return rendition, true
}
