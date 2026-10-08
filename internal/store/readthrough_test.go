// SPDX-License-Identifier: GPL-3.0-or-later

package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrATimeline checks the sweep's
// query: a live video whose bytes lack loudness though it has sound, or a timeline though it has a
// picture, or have only what the file's index said of it, its keyframes without its bytes in each
// second (D259 §2); never a cover alone, bytes with both, a skipped or removed file, or one without a
// probe.
func TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrATimeline(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	// 1 lacks both; 2 has its loudness and lacks its timeline; 3 has its loudness and only a cover;
	// 4 has a picture alone; 5 is removed; 6 was skipped; 7 has both; 8 has a timeline and no sound; 9
	// has a timeline read from its index, and no sound.
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, probe_json, filing, removed_at,
		 first_seen_at, last_seen_at) VALUES
		 (1, 1, 1, '/m/1.mkv', 10, X'01', '{}', 'listed', NULL, 1, 1),
		 (2, 1, 1, '/m/2.mkv', 10, X'02', '{}', 'extra', NULL, 1, 1),
		 (3, 1, 1, '/m/3.mkv', 10, X'03', '{}', 'unidentified', NULL, 1, 1),
		 (4, 1, 1, '/m/4.mkv', 10, X'04', '{}', 'listed', NULL, 1, 1),
		 (5, 1, 1, '/m/5.mkv', 10, X'05', '{}', 'listed', 9, 1, 1),
		 (6, 1, 1, '/m/6.mkv', 10, X'06', '{}', 'skipped', NULL, 1, 1),
		 (7, 1, 1, '/m/7.mkv', 10, X'07', '{}', 'listed', NULL, 1, 1),
		 (8, 1, 1, '/m/8.mkv', 10, X'08', '{}', 'listed', NULL, 1, 1),
		 (9, 1, 1, '/m/9.mkv', 10, X'09', '{}', 'listed', NULL, 1, 1)`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, disposition_flags) VALUES
		 (1, 1, 0, 'video', 'h264', 0), (1, 1, 1, 'audio', 'aac', 0),
		 (2, 2, 0, 'video', 'h264', 0), (2, 2, 1, 'audio', 'aac', 0),
		 (3, 3, 0, 'video', 'mjpeg', 1024), (3, 3, 1, 'audio', 'aac', 0),
		 (4, 4, 0, 'video', 'h264', 1),
		 (5, 5, 0, 'video', 'h264', 0), (6, 6, 0, 'video', 'h264', 0),
		 (7, 7, 0, 'video', 'h264', 0), (7, 7, 1, 'audio', 'aac', 0), (8, 8, 0, 'video', 'h264', 0),
		 (9, 9, 0, 'video', 'h264', 0)`,
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
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, probe_json, filing,
		 first_seen_at, last_seen_at) VALUES (1, 1, 1, '/m/1.mkv', 10, X'0c', '{}', 'listed', 1, 1)`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, disposition_flags)
		 VALUES (1, 1, 0, 'video', 'h264', 0)`)
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
