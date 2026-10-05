// SPDX-License-Identifier: GPL-3.0-or-later

package store_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrHaveAPicture checks the sweep's
// query: a live video whose bytes lack loudness though it has sound, or that has a picture, which
// cache.db's timeline may lack, with whether it lacks loudness and has a picture; never a cover
// alone, a skipped or removed file, or one without a probe.
func TestListFilesToReadThrough_ListsTheLiveVideosThatLackLoudnessOrHaveAPicture(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	// 1 has sound and a picture; 2 has sound already measured and a picture; 3 has sound measured and
	// only a cover; 4 has a picture alone; 5 is removed; 6 was skipped.
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
		 (6, 1, 1, '/m/6.mkv', 10, X'06', '{}', 'skipped', NULL, 1, 1)`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, disposition_flags) VALUES
		 (1, 1, 0, 'video', 'h264', 0), (1, 1, 1, 'audio', 'aac', 0),
		 (2, 2, 0, 'video', 'h264', 0), (2, 2, 1, 'audio', 'aac', 0),
		 (3, 3, 0, 'video', 'mjpeg', 1024), (3, 3, 1, 'audio', 'aac', 0),
		 (4, 4, 0, 'video', 'h264', 1),
		 (5, 5, 0, 'video', 'h264', 0), (6, 6, 0, 'video', 'h264', 0)`,
		`INSERT INTO loudness (payload_hash, stream_index, measured_mix, integrated_lufs, measured_at)
		 VALUES (X'02', 1, 'stereo', -20, 1), (X'03', 1, 'stereo', -20, 1)`)
	rows, err := database.Read().ListFilesToReadThrough(t.Context(),
		sqlcgen.ListFilesToReadThroughParams{AfterID: 0, ThroughID: 10})
	if err != nil {
		t.Fatal(err)
	}
	type listed struct{ id, lacksLoudness, hasPicture int64 }
	var got []listed
	for _, row := range rows {
		got = append(got, listed{row.ID, row.LacksLoudness, row.HasPicture})
	}
	if want := []listed{{1, 1, 1}, {2, 0, 1}, {4, 0, 1}}; !slices.Equal(got, want) {
		t.Errorf("listed %+v; want 1 lacking loudness with a picture, and 2 and 4 for their pictures alone", got)
	}
	if limited, err := database.Read().ListFilesToReadThrough(t.Context(),
		sqlcgen.ListFilesToReadThroughParams{AfterID: 1, ThroughID: 2}); err != nil || len(limited) != 1 ||
		limited[0].ID != 2 {
		t.Errorf("the ids after 1 through 2: %+v, %v; want file 2 alone", limited, err)
	}
}

// TestVideoTimeline_IsKeptByItsBytesAndPictureAndAReadAgainReplacesIt checks cache.db's
// video_timeline: a picture's timeline, or FFmpeg's word, by payload hash and stream; a read again
// replaces it; and how many a payload's pictures have.
func TestVideoTimeline_IsKeptByItsBytesAndPictureAndAReadAgainReplacesIt(t *testing.T) {
	t.Parallel()
	cache, err := store.OpenCache(t.Context(), filepath.Join(t.TempDir(), "cache.db"), storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() }) //nolint:errcheck // a test's temporary file
	failure := "Invalid data found"
	upsert := func(row cachegen.UpsertVideoTimelineParams) {
		t.Helper()
		if writeErr := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
			return queries.UpsertVideoTimeline(t.Context(), row)
		}); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	upsert(cachegen.UpsertVideoTimelineParams{
		PayloadHash: []byte{0x0a}, VideoStreamIndex: 0, KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]",
		Failure: &failure, MadeAt: 1,
	})
	upsert(cachegen.UpsertVideoTimelineParams{
		PayloadHash: []byte{0x0a}, VideoStreamIndex: 0, KeyframesMsJSON: "[0,2000]", BytesInEachSecondJSON: "[5,6]",
		MadeAt: 2,
	})
	upsert(cachegen.UpsertVideoTimelineParams{
		PayloadHash: []byte{0x0a}, VideoStreamIndex: 3, KeyframesMsJSON: "[0]", BytesInEachSecondJSON: "[1]", MadeAt: 3,
	})
	kept, err := cache.Read().GetVideoTimeline(t.Context(),
		cachegen.GetVideoTimelineParams{PayloadHash: []byte{0x0a}, VideoStreamIndex: 0})
	if err != nil || kept.KeyframesMsJSON != "[0,2000]" || kept.BytesInEachSecondJSON != "[5,6]" ||
		kept.Failure != nil || kept.MadeAt != 2 {
		t.Errorf("stream 0's timeline = %+v (%v); want the second read's, without the first's failure", kept, err)
	}
	if count, countErr := cache.Read().CountVideoTimelinesOfPayload(t.Context(), []byte{0x0a}); countErr != nil ||
		count != 2 {
		t.Errorf("the payload's timelines: %d (%v); want its two pictures'", count, countErr)
	}
	if count, countErr := cache.Read().CountVideoTimelinesOfPayload(t.Context(), []byte{0x0b}); countErr != nil ||
		count != 0 {
		t.Errorf("another payload's timelines: %d (%v); want none", count, countErr)
	}
	_, err = store.OrNotFound(cache.Read().GetVideoTimeline(t.Context(),
		cachegen.GetVideoTimelineParams{PayloadHash: []byte{0x0b}, VideoStreamIndex: 0}))
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a timeline never read: %v; want ErrNotFound", err)
	}
	err = cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		_, insertErr := queries.InsertJob(t.Context(), cachegen.InsertJobParams{
			Kind: "read_through", FileID: new(int64(1)), ArgsJSON: "{}", RunnableAt: 1, CreatedAt: 1,
		})
		return insertErr //nolint:wrapcheck // the test reads it as is
	})
	if err != nil {
		t.Errorf("a read_through job: %v; want the kind taken", err)
	}
}
