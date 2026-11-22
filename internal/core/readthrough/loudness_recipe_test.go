// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The loudness table's key holds the recipe a measurement was made with (D536 §3): a file's bytes
// measured only by another recipe have no loudness of this one, so the sweep finds the file, its
// read-through asks for every track's loudness, and what it measures is kept beside the other
// recipe's, which stays (D536 §5); measured, the file is found no more. Another recipe is this one's
// next here, which stands for any other: the schema holds no recipe below 1.
func TestLoudnessRecipe_AMeasurementOfAnotherRecipeIsNoneAndTheFileIsReadAgain(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	ctx, hash := t.Context(), []byte{0x04}
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	librarytest.File(t, database, librarytest.FileSpec{
		ID: 4, LibraryID: libraryID, NodeID: nodeID, Path: "/m/a.mkv", SizeBytes: 4000, PayloadHash: hash,
		Filing: "unidentified",
	})
	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: 4})
	librarytest.Stream(t, database, librarytest.StreamSpec{
		BytesFile: 4, StreamIndex: 1, Kind: "audio", Codec: "aac", ChannelCount: 2,
	})
	another := strconv.Itoa(nodeproto.LoudnessRecipe + 1)
	storetest.ExecRaw(t, database.Path(), `INSERT INTO loudness (payload_hash, stream_index, measured_mix, recipe,
		integrated_lufs, range_lu, true_peak_dbtp, measured_at)
		VALUES (X'04', 1, 'stereo', `+another+`, -30, 5, -2, 1)`)
	now := clocktest.NewFake(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	service := &Service{database: database, clock: now}
	file := sqlcgen.GetPlayableFileRow{ID: 4, NodeID: nodeID, PayloadHash: hash}
	probe := nodeproto.Probe{Streams: []nodeproto.ProbeStream{
		{StreamIndex: 1, Kind: "audio", Codec: "aac", Audio: nodeproto.ProbeAudio{ChannelCount: 2}},
	}}

	if swept, err := service.FilesToReadThrough(ctx); err != nil || !slices.Equal(swept, []int64{4}) {
		t.Fatalf("swept %v, %v; want file 4, whose bytes another recipe alone measured", swept, err)
	}
	command, err := service.whatIsLacking(ctx, file, probe)
	if err != nil {
		t.Fatal(err)
	}
	want := []nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}
	if !slices.Equal(command.LoudnessTracks, want) {
		t.Fatalf("the read-through asks the loudness of %+v; want %+v: another recipe's is none of this one's",
			command.LoudnessTracks, want)
	}
	answer := nodeproto.ReadThroughResult{Loudness: []nodeproto.TrackLoudness{
		{StreamIndex: 1, Stereo: nodeproto.Measurement{IntegratedLUFS: -20, RangeLU: 6}},
	}}
	if err := service.keep(ctx, hash, command, answer); err != nil {
		t.Fatal(err)
	}

	if swept, err := service.FilesToReadThrough(ctx); err != nil || len(swept) != 0 {
		t.Errorf("after the read, swept %v, %v; want none: this recipe has measured it", swept, err)
	}
	if command, err := service.whatIsLacking(ctx, file, probe); err != nil || len(command.LoudnessTracks) != 0 {
		t.Errorf("after the read, the read-through asks the loudness of %+v, %v; want none",
			command.LoudnessTracks, err)
	}
	kept := storetest.QueryRaw(t, database.Path(),
		"SELECT recipe || ':' || integrated_lufs FROM loudness WHERE payload_hash = X'04' ORDER BY recipe")
	wantKept := []string{strconv.Itoa(nodeproto.LoudnessRecipe) + ":-20.0", another + ":-30.0"}
	if !slices.Equal(kept, wantKept) {
		t.Errorf("the measurements kept are %v; want %v, this recipe's beside the other's", kept, wantKept)
	}
}
