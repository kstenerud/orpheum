// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"cmp"
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrATimeline checks the sweep's
// query: a live video whose bytes lack loudness though it has sound, or a timeline though it has a
// picture, or have only what the file's index said of it, its keyframes without its bytes in each
// second (D259 §2); never a cover alone, bytes with both, a skipped or removed file, one without a
// probe, or one no library's folder covers (D363 §10).
func TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrATimeline(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	// 1 lacks both; 2 has its loudness and lacks its timeline; 3 has its loudness and only a cover;
	// 4 has a picture alone; 5 is removed; 6 was skipped; 7 has both; 8 has a timeline and no sound; 9
	// has a timeline read from its index, and no sound; 10 lacks both, under no library's folder.
	storetest.ExecRaw(t, database.Path(),
		rowstest.Node(rowstest.NodeSpec{ID: 1, Name: "local"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1}),
		readThroughFile(readThrough{id: 1, path: "/m/1.mkv", hasSound: true}),
		readThroughFile(readThrough{id: 2, path: "/m/2.mkv", hasSound: true}),
		readThroughFile(readThrough{
			id: 3, path: "/m/3.mkv", videoCodec: "mjpeg", videoDisposition: 1024, hasSound: true,
		}),
		readThroughFile(readThrough{id: 4, path: "/m/4.mkv", videoDisposition: 1}),
		readThroughFile(readThrough{id: 5, path: "/m/5.mkv", removedAt: 9}),
		readThroughFile(readThrough{id: 6, path: "/m/6.mkv", filing: "skipped"}),
		readThroughFile(readThrough{id: 7, path: "/m/7.mkv", hasSound: true}),
		readThroughFile(readThrough{id: 8, path: "/m/8.mkv"}),
		readThroughFile(readThrough{id: 9, path: "/m/9.mkv"}),
		readThroughFile(readThrough{id: 10, path: "/x/10.mkv", hasSound: true}),
		`INSERT INTO loudness (payload_hash, stream_index, measured_mix, integrated_lufs, measured_at)
		 VALUES (X'02', 1, 'stereo', -20, 1), (X'03', 1, 'stereo', -20, 1), (X'07', 1, 'stereo', -20, 1)`,
		`INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json,
		 made_at) VALUES (X'07', 0, '[0]', '[1]', 1), (X'08', 0, '[0]', '[1]', 1), (X'09', 0, '[0]', '[]', 1)`)
	rows, err := database.Read().ListFilesToReadThrough(t.Context(),
		sqlcgen.ListFilesToReadThroughParams{AfterID: 0, ThroughID: 10})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if !slices.Equal(ids, []int64{1, 2, 4, 9}) {
		t.Errorf("listed files %v; want 1, lacking both, 2 and 4, lacking their pictures' timelines, and 9, "+
			"whose timeline is its index's alone", ids)
	}
	if limited, err := database.Read().ListFilesToReadThrough(t.Context(),
		sqlcgen.ListFilesToReadThroughParams{AfterID: 1, ThroughID: 2}); err != nil || len(limited) != 1 ||
		limited[0].ID != 2 {
		t.Errorf("the ids after 1 through 2: %+v, %v; want file 2 alone", limited, err)
	}
}

// readThrough is a probed video under library 1's folder, of ten bytes whose hash is its id, with a
// picture at stream 0 and, if it has sound, AAC at stream 1.
type readThrough struct {
	id               byte // its file's, and its payload hash's one byte
	path             string
	filing           string // its place's; "": read
	removedAt        int64  // 0: live
	videoCodec       string // "": H.264
	videoDisposition int64
	hasSound         bool
}

// readThroughFile inserts the file, its place, its probe and its streams.
func readThroughFile(file readThrough) string {
	id := int64(file.id)
	video := rowstest.Join(
		rowstest.File(rowstest.FileSpec{
			ID: id, SizeBytes: 10, PayloadHash: []byte{file.id}, NodeID: 1, Path: file.path,
			Filing: file.filing, RemovedAt: file.removedAt,
		}),
		rowstest.Probe(rowstest.ProbeSpec{FileID: id}),
		rowstest.Stream(rowstest.StreamSpec{
			BytesFile: id, Kind: "video", Codec: cmp.Or(file.videoCodec, "h264"),
			DispositionFlags: file.videoDisposition,
		}))
	if !file.hasSound {
		return video
	}
	return rowstest.Join(video,
		rowstest.Stream(rowstest.StreamSpec{BytesFile: id, StreamIndex: 1, Kind: "audio", Codec: "aac"}))
}

// TestVideoTimeline_IsKeptByItsBytesAndPictureAndAReadAgainReplacesIt checks video_timeline: a
// picture's timeline, or FFmpeg's word, by payload hash and stream, and a read again replaces it.
func TestVideoTimeline_IsKeptByItsBytesAndPictureAndAReadAgainReplacesIt(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	failure := "Invalid data found"
	for _, row := range []sqlcgen.UpsertVideoTimelineParams{
		{
			PayloadHash: []byte{0x0a}, VideoStreamIndex: 0, KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]",
			Failure: &failure, MadeAt: 1,
		},
		{
			PayloadHash: []byte{0x0a}, VideoStreamIndex: 0, KeyframesMsJSON: "[0,2000]",
			BytesInEachSecondJSON: "[5,6]", MadeAt: 2,
		},
		{
			PayloadHash: []byte{0x0a}, VideoStreamIndex: 3, KeyframesMsJSON: "[0]", BytesInEachSecondJSON: "[1]",
			MadeAt: 3,
		},
	} {
		if writeErr := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertVideoTimeline(t.Context(), row)
		}); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	kept, err := database.Read().GetVideoTimeline(t.Context(),
		sqlcgen.GetVideoTimelineParams{PayloadHash: []byte{0x0a}, VideoStreamIndex: 0})
	if err != nil || kept.KeyframesMsJSON != "[0,2000]" || kept.BytesInEachSecondJSON != "[5,6]" ||
		kept.Failure != nil || kept.MadeAt != 2 {
		t.Errorf("stream 0's timeline = %+v (%v); want the second read's, without the first's failure", kept, err)
	}
	other, err := database.Read().GetVideoTimeline(t.Context(),
		sqlcgen.GetVideoTimelineParams{PayloadHash: []byte{0x0a}, VideoStreamIndex: 3})
	if err != nil || other.KeyframesMsJSON != "[0]" {
		t.Errorf("stream 3's timeline = %+v (%v); want its own, beside stream 0's", other, err)
	}
	_, err = store.OrNotFound(database.Read().GetVideoTimeline(t.Context(),
		sqlcgen.GetVideoTimelineParams{PayloadHash: []byte{0x0b}, VideoStreamIndex: 0}))
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a timeline never read: %v; want ErrNotFound", err)
	}
}

// A timeline's keyframes, once known, are kept: a read through after the file's index was read adds
// its bytes in each second and leaves the keyframes, so a playback's grid does not move, and a failed
// read changes nothing of it (D259 §2).
// A read through that fails after the index gave the keyframes keeps them and its failure as
// read_failure, and the sweep reads the file no more; a read through that succeeds clears it
// (D270 section 1).
func TestVideoTimeline_AFailedReadAfterTheIndexIsKeptAsItsReadFailure(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	storetest.ExecRaw(t, database.Path(),
		rowstest.Node(rowstest.NodeSpec{ID: 1, Name: "local"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1}),
		rowstest.File(rowstest.FileSpec{ID: 1, SizeBytes: 10, PayloadHash: []byte{0x0c}, NodeID: 1, Path: "/m/1.mkv"}),
		rowstest.Probe(rowstest.ProbeSpec{FileID: 1}),
		rowstest.Stream(rowstest.StreamSpec{BytesFile: 1, Kind: "video", Codec: "h264"}))
	failure := "Invalid data found"
	upsert := func(row sqlcgen.UpsertVideoTimelineParams) sqlcgen.VideoTimeline {
		t.Helper()
		row.PayloadHash, row.VideoStreamIndex = []byte{0x0c}, 0
		if err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertVideoTimeline(t.Context(), row)
		}); err != nil {
			t.Fatal(err)
		}
		kept, err := database.Read().GetVideoTimeline(t.Context(),
			sqlcgen.GetVideoTimelineParams{PayloadHash: []byte{0x0c}, VideoStreamIndex: 0})
		if err != nil {
			t.Fatal(err)
		}
		return kept
	}
	swept := func() int {
		t.Helper()
		rows, err := database.Read().ListFilesToReadThrough(t.Context(),
			sqlcgen.ListFilesToReadThroughParams{AfterID: 0, ThroughID: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}
	upsert(sqlcgen.UpsertVideoTimelineParams{KeyframesMsJSON: "[0,2000]", BytesInEachSecondJSON: "[]", MadeAt: 1})
	if swept() != 1 {
		t.Fatal("a film with its index's keyframes alone is not swept; want it read through")
	}
	failed := upsert(sqlcgen.UpsertVideoTimelineParams{
		KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]", Failure: &failure, MadeAt: 2,
	})
	if failed.KeyframesMsJSON != "[0,2000]" || failed.Failure != nil || failed.ReadFailure == nil ||
		*failed.ReadFailure != failure {
		t.Errorf("after its read through failed, the timeline = %+v; want its keyframes and the read's failure", failed)
	}
	if swept() != 0 {
		t.Error("a film whose read through failed is swept again; want it left")
	}
	if read := upsert(sqlcgen.UpsertVideoTimelineParams{
		KeyframesMsJSON: "[0,2000]", BytesInEachSecondJSON: "[5,6]", MadeAt: 3,
	}); read.ReadFailure != nil {
		t.Errorf("after a read through that succeeded, the timeline = %+v; want no read failure", read)
	}
}

func TestVideoTimeline_KeepsItsKeyframesAndALaterReadAddsItsBytes(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	failure := "Invalid data found"
	upsert := func(row sqlcgen.UpsertVideoTimelineParams) sqlcgen.VideoTimeline {
		t.Helper()
		row.PayloadHash, row.VideoStreamIndex = []byte{0x0c}, 0
		if err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertVideoTimeline(t.Context(), row)
		}); err != nil {
			t.Fatal(err)
		}
		kept, err := database.Read().GetVideoTimeline(t.Context(),
			sqlcgen.GetVideoTimelineParams{PayloadHash: []byte{0x0c}, VideoStreamIndex: 0})
		if err != nil {
			t.Fatal(err)
		}
		return kept
	}
	upsert(sqlcgen.UpsertVideoTimelineParams{KeyframesMsJSON: "[0,2000]", BytesInEachSecondJSON: "[]", MadeAt: 1})
	if read := upsert(sqlcgen.UpsertVideoTimelineParams{
		KeyframesMsJSON: "[0,1500,2000]", BytesInEachSecondJSON: "[5,6]", MadeAt: 2,
	}); read.KeyframesMsJSON != "[0,2000]" || read.BytesInEachSecondJSON != "[5,6]" {
		t.Errorf("after a read through, the timeline = %+v; want the index's keyframes and the read's bytes", read)
	}
	if failed := upsert(sqlcgen.UpsertVideoTimelineParams{
		KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]", Failure: &failure, MadeAt: 3,
	}); failed.KeyframesMsJSON != "[0,2000]" || failed.BytesInEachSecondJSON != "[5,6]" || failed.Failure != nil {
		t.Errorf("after a failed read, the timeline = %+v; want it as it was", failed)
	}
}
