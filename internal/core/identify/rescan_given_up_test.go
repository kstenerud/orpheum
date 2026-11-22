// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/repeats"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// givenUpForgotten records what a rescan asked the other packages to forget of the attempts given up
// on: the files whose subtitle texts, and the subjects whose failures of unknown cause.
type givenUpForgotten struct {
	subtitleFiles, subjects []string
}

func (forgotten *givenUpForgotten) forgetters() identify.GivenUpForgetters {
	return identify.GivenUpForgetters{
		OfFiles: []identify.FileFailuresForgetter{
			readthrough.ForgetFailuresOfFiles, repeats.ForgetFailuresOfFiles, library.ForgetHashFailuresOfFiles,
		},
		SubtitleText: func(_ context.Context, fileIDs []int64) error {
			for _, id := range fileIDs {
				forgotten.subtitleFiles = append(forgotten.subtitleFiles, strconv.FormatInt(id, 10))
			}
			return nil
		},
		UnknownFailures: func(_ context.Context, subjects []string) (int64, error) {
			forgotten.subjects = append(forgotten.subjects, subjects...)
			return int64(len(subjects)), nil
		},
	}
}

// givenUpWorld is the chosen film's library with, for each of its two files, every attempt to
// gather information given up on and, beside each, one that succeeded.
type givenUpWorld struct {
	chosenAndWatched
	other, otherWork int64
	forgotten        *givenUpForgotten
}

func newGivenUpWorld(t *testing.T) givenUpWorld {
	t.Helper()
	world := givenUpWorld{chosenAndWatched: newChosenAndWatched(t), forgotten: &givenUpForgotten{}}
	crowd := world.rescan.lib.titlesByPath()[crowdFile]
	world.other, world.otherWork = crowd.ID, crowd.WorkID
	world.rescan.rescans.SetGivenUpForgetters(world.forgotten.forgetters())
	path := world.rescan.lib.database.Path()
	for _, file := range []int64{world.file, world.other} {
		id := strconv.FormatInt(file, 10)
		payload := `(SELECT payload_hash FROM file WHERE id = ` + id + `)`
		storetest.ExecRaw(t, path, `INSERT INTO loudness (payload_hash, stream_index, measured_mix, failure,
			measured_at) VALUES (`+payload+`, 1, 'stereo', 'ffmpeg exited 1', 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO loudness (payload_hash, stream_index, measured_mix,
			integrated_lufs, range_lu, true_peak_dbtp, measured_at) VALUES (`+payload+`, 2, 'stereo', -23, 7, -1, 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO video_timeline (payload_hash, video_stream_index, failure,
			made_at) VALUES (`+payload+`, 0, 'ffmpeg exited 1', 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO video_timeline (payload_hash, video_stream_index,
			keyframes_ms_json, made_at, read_failure) VALUES (`+payload+`, 1, '[0,2000]', 1, 'the read stopped')`)
		storetest.ExecRaw(t, path, `INSERT INTO video_timeline (payload_hash, video_stream_index,
			keyframes_ms_json, bytes_in_each_second_json, made_at) VALUES (`+payload+`, 2, '[0,2000]', '[10,10]', 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO repeat_measurement (payload_hash, version, outcome, failure,
			measured_at) VALUES (`+payload+`, 1, 'failed', 'ffmpeg exited 1', 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO repeat_measurement (payload_hash, version, outcome, measured_at)
			VALUES (`+payload+`, 2, 'none', 1)`)
		storetest.ExecRaw(t, path, `INSERT INTO whole_file_hash (file_id, failure, hashed_at)
			VALUES (`+id+`, 'the read stopped', 1)`)
	}
	storetest.ExecRaw(t, path, `INSERT INTO fetch_failure (item_id, failure, failed_at)
		VALUES (`+strconv.FormatInt(world.otherWork, 10)+`, 'the provider said no', 1)`)
	return world
}

// givenUpKept names the attempts given up on still kept for a file and its film: each kind's, by
// what it was of.
func (world givenUpWorld) givenUpKept(t *testing.T, file, work int64) []string {
	t.Helper()
	id := strconv.FormatInt(file, 10)
	payload := `(SELECT payload_hash FROM file WHERE id = ` + id + `)`
	return storetest.QueryRaw(t, world.rescan.lib.database.Path(), `
		SELECT 'fetch' FROM fetch_failure WHERE item_id = `+strconv.FormatInt(work, 10)+`
		UNION ALL SELECT 'loudness' FROM loudness WHERE failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'timeline' FROM video_timeline WHERE failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'timeline read' FROM video_timeline
			WHERE read_failure IS NOT NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'repeats' FROM repeat_measurement WHERE outcome = 'failed' AND payload_hash = `+payload+`
		UNION ALL SELECT 'whole hash' FROM whole_file_hash WHERE failure IS NOT NULL AND file_id = `+id)
}

var everyGivenUp = []string{"fetch", "loudness", "timeline", "timeline read", "repeats", "whole hash"}

// wantMadeKept fails unless what succeeded of a file is all kept: a measured loudness, a timeline
// read, the keyframes of a timeline whose read through failed and a measurement that found no
// repeats. The subtitles' texts are the subtitles package's to forget (ForgetTextFailures).
func (world givenUpWorld) wantMadeKept(t *testing.T, file int64) {
	t.Helper()
	payload := `(SELECT payload_hash FROM file WHERE id = ` + strconv.FormatInt(file, 10) + `)`
	kept := storetest.QueryRaw(t, world.rescan.lib.database.Path(), `
		SELECT 'loudness' FROM loudness WHERE failure IS NULL AND payload_hash = `+payload+`
		UNION ALL SELECT 'timeline ' || video_stream_index FROM video_timeline
			WHERE failure IS NULL AND keyframes_ms_json = '[0,2000]' AND payload_hash = `+payload+`
		UNION ALL SELECT 'repeats' FROM repeat_measurement WHERE outcome = 'none' AND payload_hash = `+payload)
	slices.Sort(kept)
	if want := []string{"loudness", "repeats", "timeline 1", "timeline 2"}; !slices.Equal(kept, want) {
		t.Errorf("what succeeded of file %d kept %v; want %v", file, kept, want)
	}
}

// D535 §8: a rescan that keeps what was made still forgets every attempt given up on in its
// library, of its films and its files, and asks for the subtitle texts and the failures of unknown
// cause of them to be forgotten; what succeeded stays.
func TestRescan_ALibrarysRescanForgetsEveryAttemptGivenUpOnAndKeepsWhatSucceeded(t *testing.T) {
	t.Parallel()
	world := newGivenUpWorld(t)
	if _, err := world.rescan.rescans.RescanLibrary(t.Context(), world.rescan.lib.libraryID, false); err != nil {
		t.Fatal(err)
	}
	for _, file := range [][2]int64{{world.file, world.chosen}, {world.other, world.otherWork}} {
		if kept := world.givenUpKept(t, file[0], file[1]); len(kept) != 0 {
			t.Errorf("the rescan kept, of file %d, the attempts given up on %v; want none", file[0], kept)
		}
		world.wantMadeKept(t, file[0])
	}
	file, other := strconv.FormatInt(world.file, 10), strconv.FormatInt(world.other, 10)
	if !slices.Contains(world.forgotten.subtitleFiles, file) || !slices.Contains(world.forgotten.subtitleFiles, other) {
		t.Errorf("the rescan asked to forget the subtitle texts of files %v; want %s and %s among them",
			world.forgotten.subtitleFiles, file, other)
	}
	for _, subject := range []string{"item " + strconv.FormatInt(world.chosen, 10), "file " + file, "file " + other} {
		if !slices.Contains(world.forgotten.subjects, subject) {
			t.Errorf("the rescan asked to forget the unknown failures of %v; want %s among them",
				world.forgotten.subjects, subject)
		}
	}
}

// D535 §8: a video page's rescan forgets the attempts given up on of its film and its file, and
// leaves another film's and file's in the library as they are.
func TestRescan_AVideoPagesRescanForgetsOnlyItsOwnAttemptsGivenUpOn(t *testing.T) {
	t.Parallel()
	world := newGivenUpWorld(t)
	isStarted, err := world.rescan.rescans.RescanItem(t.Context(), world.chosen, false)
	if err != nil || !isStarted {
		t.Fatalf("the rescan = %v, %v; want it started", isStarted, err)
	}
	if kept := world.givenUpKept(t, world.file, world.chosen); len(kept) != 0 {
		t.Errorf("the rescan kept, of its own file, the attempts given up on %v; want none", kept)
	}
	world.wantMadeKept(t, world.file)
	if kept := world.givenUpKept(t, world.other, world.otherWork); !slices.Equal(kept, everyGivenUp) {
		t.Errorf("of another file, the rescan kept the attempts given up on %v; want every one, %v", kept,
			everyGivenUp)
	}
	file := strconv.FormatInt(world.file, 10)
	if !slices.Equal(world.forgotten.subtitleFiles, []string{file}) {
		t.Errorf("the rescan asked to forget the subtitle texts of files %v; want only its own, %s",
			world.forgotten.subtitleFiles, file)
	}
	if want := []string{"item " + strconv.FormatInt(world.chosen, 10), "file " + file}; !slices.Equal(
		world.forgotten.subjects, want) {
		t.Errorf("the rescan asked to forget the unknown failures of %v; want %v", world.forgotten.subjects, want)
	}
}

// D535 §8: a scan, asked for or automatic, forgets none of the attempts given up on; only a rescan
// does.
func TestScan_ForgetsNoAttemptGivenUpOn(t *testing.T) {
	t.Parallel()
	world := newGivenUpWorld(t)
	world.rescan.lib.scan(videoRecord(crowdFile), videoRecord("Metropolis (1927).mkv"))
	for _, file := range [][2]int64{{world.file, world.chosen}, {world.other, world.otherWork}} {
		if kept := world.givenUpKept(t, file[0], file[1]); !slices.Equal(kept, everyGivenUp) {
			t.Errorf("a scan kept, of file %d, the attempts given up on %v; want every one, %v", file[0], kept,
				everyGivenUp)
		}
	}
	if len(world.forgotten.subtitleFiles) != 0 || len(world.forgotten.subjects) != 0 {
		t.Errorf("a scan asked to forget subtitle texts of %v and unknown failures of %v; want neither",
			world.forgotten.subtitleFiles, world.forgotten.subjects)
	}
}
