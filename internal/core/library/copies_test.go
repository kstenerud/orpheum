// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// copiesBench is a film found at two places of one library, its first place (file first) found before
// its second (file second), with the copies' confirmation over them and what it asks of others.
type copiesBench struct {
	*harness
	copies   *Copies
	jobs     *queuedJobs
	sent     *sentWork
	work     *fakeWork
	logs     *bytes.Buffer
	folds    [][2]int64 // the copy and its file, for each move another package was asked for
	retired  [][2]int64 // the version and the title it was retired into
	passes   []int64    // the libraries passed again
	first    int64
	second   int64
	subtitle int64 // the subtitle beside the second place
}

const (
	firstPlace  = root + "/Metropolis (1927)/Metropolis (1927).mkv"
	secondPlace = root + "/Backup/Metropolis (1927).mkv"
)

func newCopiesBench(t *testing.T) *copiesBench {
	t.Helper()
	bench := &copiesBench{
		harness: newHarness(t), jobs: &queuedJobs{}, sent: &sentWork{}, work: &fakeWork{}, logs: &bytes.Buffer{},
	}
	database := bench.database
	librarytest.Title(t, database, librarytest.TitleSpec{ID: 10, Library: bench.libraryID})
	librarytest.Title(t, database, librarytest.TitleSpec{ID: 11, Library: bench.libraryID})
	librarytest.Version(t, database, librarytest.VersionSpec{ID: 1, TitleID: 10})
	librarytest.Version(t, database, librarytest.VersionSpec{ID: 2, TitleID: 11})
	payload := []byte{0x0e}
	bench.first = librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: bench.libraryID, NodeID: bench.nodeID, Path: firstPlace, SizeBytes: 100, ModifiedAt: 5,
		PayloadHash: payload, VersionID: 1, FirstSeenAt: 1,
	})
	bench.second = librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: bench.libraryID, NodeID: bench.nodeID, Path: secondPlace, SizeBytes: 100, ModifiedAt: 7,
		PayloadHash: payload, VersionID: 2, FirstSeenAt: 2,
	})
	bench.subtitle = librarytest.File(t, database, librarytest.FileSpec{
		NodeID: bench.nodeID, Path: root + "/Backup/Metropolis (1927).en.srt", SidecarKind: "subtitle",
		AttachedToFile: bench.second, FirstSeenAt: 2,
	})
	for _, fileID := range []int64{bench.first, bench.second} {
		librarytest.Stream(t, database, librarytest.StreamSpec{BytesFile: fileID, StreamIndex: 0, Kind: "video"})
		librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: fileID})
	}
	librarytest.Stream(t, database, librarytest.StreamSpec{
		BytesFile: bench.subtitle, VideoFile: bench.second, Kind: "subtitle", Codec: "subrip", Language: "en",
	})
	bench.copies = NewCopies(CopiesParts{
		Database: database, Jobs: bench.jobs, Assigner: bench.sent, Nodes: &fakeNodes{id: bench.nodeID},
		Work: bench.work,
		RetireVersion: func(_ context.Context, _ *sqlcgen.Queries, versionID, intoTitleID, _ int64) error {
			bench.retired = append(bench.retired, [2]int64{versionID, intoTitleID})
			return nil
		},
		Folds: []CopyFold{func(_ context.Context, _ *sqlcgen.Queries, fromFileID, toFileID int64) error {
			bench.folds = append(bench.folds, [2]int64{fromFileID, toFileID})
			return nil
		}},
		Passes: func(_ context.Context, libraryID int64) { bench.passes = append(bench.passes, libraryID) },
		Clock:  bench.clock, Logger: slog.New(slog.NewTextHandler(bench.logs, nil)),
	})
	return bench
}

// sweep runs the copies' sweep and returns the files whose whole reads it queued.
func (bench *copiesBench) sweep() []int64 {
	bench.t.Helper()
	before := len(bench.jobs.requests)
	if _, err := bench.copies.QueueCopyChecks(bench.t.Context()); err != nil {
		bench.t.Fatal(err)
	}
	var fileIDs []int64
	for _, request := range bench.jobs.requests[before:] {
		if request.Kind == jobs.KindHashWholeFile {
			fileIDs = append(fileIDs, request.FileID)
		}
	}
	return fileIDs
}

// read runs fileID's whole read: the node is handed the file at its place, and answers sha256 at the
// size and modified time it found.
func (bench *copiesBench) read(fileID int64, sha256 string, modifiedAt int64) {
	bench.t.Helper()
	job := jobs.Job{Kind: jobs.KindHashWholeFile, Target: jobs.Target{FileID: fileID}}
	if err := bench.copies.HandleHashWhole(bench.t.Context(), job); err != nil {
		bench.t.Fatal(err)
	}
	sent := bench.sent.commands[len(bench.sent.commands)-1]
	if sent.HashWhole == nil {
		bench.t.Fatalf("file %d's read sent %+v; want a whole-file read", fileID, sent)
	}
	answer := nodeproto.Result{WholeHash: &nodeproto.WholeHash{SHA256: sha256, SizeBytes: 100, ModifiedAt: modifiedAt}}
	err := bench.copies.Assignee(nil).Answer(bench.t.Context(), job, nodes.Attempt{NodeID: bench.nodeID}, answer)
	if err != nil {
		bench.t.Fatal(err)
	}
}

// row is a file's own filing and the file it is a copy of, as its row has them.
func (bench *copiesBench) row(fileID int64) string {
	bench.t.Helper()
	return strings.Join(storetest.QueryRaw(bench.t, bench.database.Path(),
		"SELECT "+filingWord+", CASE WHEN place.id IS NOT place.file_id THEN place.file_id ELSE 0 END, "+
			"place.removed_at IS NOT NULL FROM place JOIN file ON file.id = place.file_id WHERE place.id = "+
			itoa(fileID)), "; ")
}

// filingWord is how a place is filed, in the words the API answers with: skipped or errored as it was
// stored, else attached for a sidecar, copy for a place holding a file found first elsewhere, and
// video (D426 section 4).
const filingWord = "CASE WHEN place.filing <> 'read' THEN place.filing " +
	"WHEN file.sidecar_kind IS NOT NULL THEN 'attached' " +
	"WHEN place.id IS NOT place.file_id THEN 'copy' ELSE 'video' END"

// itoa is an id as SQL and a row read back write it.
func itoa(value int64) string { return strconv.FormatInt(value, 10) }

// D363 §2: a video found at a second place whose payload a live video found before it has is a
// candidate; both are read whole (D116), and the same bytes make the second the first's place from
// then on: one file, its tracks and probe the first's, the subtitle beside the second place the file's
// track, what each kind of library filed it as and what other packages held of it moved onto the file,
// its work cancelled and its library passed again. The merge is logged.
func TestCopies_AFileFoundAtASecondPlaceWithTheSameBytesIsOneFileAtTwoPlaces(t *testing.T) {
	t.Parallel()
	bench := newCopiesBench(t)
	if owed := bench.sweep(); !slices.Equal(owed, []int64{bench.first, bench.second}) {
		t.Fatalf("the sweep queued the whole reads of %v; want both places' files", owed)
	}
	same := strings.Repeat("ab", 32)
	bench.read(bench.first, same, 5)
	if got := bench.row(bench.second); got != "video|0|0" {
		t.Fatalf("with one half read, the second place's row = %s; want it a file still", got)
	}
	bench.read(bench.second, same, 7)
	if got := bench.row(bench.second); got != "copy|"+itoa(bench.first)+"|0" {
		t.Errorf("the second place's row = %s; want a copy of the first's file", got)
	}
	tracks := storetest.QueryRaw(t, bench.database.Path(),
		"SELECT bytes_file, video_file, kind FROM stream ORDER BY id")
	wantTracks := []string{
		itoa(bench.first) + "|" + itoa(bench.first) + "|video",
		itoa(bench.subtitle) + "|" + itoa(bench.first) + "|subtitle",
	}
	if !slices.Equal(tracks, wantTracks) {
		t.Errorf("the tracks = %v; want the file's own, and the subtitle beside the copy now the file's: %v",
			tracks, wantTracks)
	}
	if probes := storetest.QueryRaw(t, bench.database.Path(), "SELECT file_id FROM file_probe"); !slices.Equal(
		probes, []string{itoa(bench.first)}) {
		t.Errorf("the probes are of %v; want the file's alone", probes)
	}
	filings := storetest.QueryRaw(t, bench.database.Path(),
		"SELECT file_id, version_id FROM file_filing ORDER BY file_id")
	if !slices.Equal(filings, []string{itoa(bench.first) + "|1"}) {
		t.Errorf("the filings = %v; want the file's alone, in its version", filings)
	}
	if want := [][2]int64{{2, 10}}; !slices.Equal(bench.retired, want) {
		t.Errorf("versions retired %v; want the copy's version into the file's title %v", bench.retired, want)
	}
	if want := [][2]int64{{bench.second, bench.first}}; !slices.Equal(bench.folds, want) {
		t.Errorf("moves asked of other packages %v; want %v", bench.folds, want)
	}
	if !slices.Equal(bench.passes, []int64{bench.libraryID}) {
		t.Errorf("libraries passed again %v; want the one covering the file", bench.passes)
	}
	if len(bench.work.cancelled) != 1 || !slices.Equal(bench.work.cancelled[0].FileIDs, []int64{bench.second}) {
		t.Errorf("work cancelled %+v; want the copy's", bench.work.cancelled)
	}
	if len(testutil.LinesWith(bench.logs.String(), "copiesMerged")) == 0 {
		t.Errorf("the log does not say the copy was merged:\n%s", bench.logs.String())
	}
	if owed := bench.sweep(); len(owed) != 0 {
		t.Errorf("the next sweep queued %v; want nothing, the copy settled", owed)
	}
}

// D363 §2: the same payload with other bytes leaves two files, which is logged, and the next sweep
// asks nothing more of them until one changes.
func TestCopies_AFileWithTheSamePayloadAndOtherBytesStaysAFileOfItsOwn(t *testing.T) {
	t.Parallel()
	bench := newCopiesBench(t)
	bench.sweep()
	bench.read(bench.first, strings.Repeat("ab", 32), 5)
	bench.read(bench.second, strings.Repeat("cd", 32), 7)
	if got := bench.row(bench.second); got != "video|0|0" {
		t.Errorf("the second place's row = %s; want a file of its own", got)
	}
	if len(bench.folds) != 0 || len(bench.retired) != 0 || len(bench.work.cancelled) != 0 {
		t.Errorf("a difference moved %v, retired %v and cancelled %+v; want nothing", bench.folds, bench.retired,
			bench.work.cancelled)
	}
	if len(testutil.LinesWith(bench.logs.String(), "copiesKeptApart")) == 0 {
		t.Errorf("the log does not say the two differ:\n%s", bench.logs.String())
	}
	if owed := bench.sweep(); len(owed) != 0 {
		t.Errorf("the next sweep queued %v; want nothing, the pair settled as two", owed)
	}
	storetest.ExecRaw(t, bench.database.Path(), "UPDATE place SET modified_at = 9 WHERE id = "+itoa(bench.second))
	if owed := bench.sweep(); !slices.Equal(owed, []int64{bench.second}) {
		t.Errorf("after the second changed, the sweep queued %v; want its read again", owed)
	}
}

// D363 §2: when a file's first place goes and a copy of it lives, the file moves to the copy's place,
// keeping its id and so all that is known and watched of it, and the subtitle beside the copy is the
// file's; a copy's own place going takes only that place.
func TestRemovePlace_AFilesFirstPlaceGoingLeavesItAtItsCopysPlace(t *testing.T) {
	t.Parallel()
	bench := newCopiesBench(t)
	bench.sweep()
	same := strings.Repeat("ab", 32)
	bench.read(bench.first, same, 5)
	bench.read(bench.second, same, 7)
	var isGone bool
	err := bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		isGone, err = removePlace(t.Context(), queries, slog.New(slog.NewTextHandler(bench.logs, nil)), bench.first, 9)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if isGone {
		t.Error("the file is gone; want it at its copy's place")
	}
	got := storetest.QueryRaw(t, bench.database.Path(),
		"SELECT place.id, place.path, "+filingWord+", "+
			"place.removed_at IS NOT NULL FROM place JOIN file ON file.id = place.file_id "+
			"WHERE place.id IN ("+itoa(bench.first)+", "+
			itoa(bench.second)+") ORDER BY place.id")
	want := []string{
		itoa(bench.first) + "|" + secondPlace + "|video|0",
		itoa(bench.second) + "|" + secondPlace + "|copy|1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the rows = %v; want the file at the copy's place, the copy's row gone: %v", got, want)
	}
	attached := storetest.QueryRaw(t, bench.database.Path(),
		"SELECT attached_to_place FROM place WHERE id = "+itoa(bench.subtitle))
	if !slices.Equal(attached, []string{itoa(bench.first)}) {
		t.Errorf("the subtitle beside the copy is attached to %v; want the file", attached)
	}
	if len(testutil.LinesWith(bench.logs.String(), "placeMovedToCopy")) == 0 {
		t.Errorf("the log does not say the file moved:\n%s", bench.logs.String())
	}
}

// D363 §2: a scan that no longer sees a file's first place nor its copy removes the file; one that
// sees the copy alone leaves the file at the copy's place.
func TestRemovePlaces_AFileWhosePlacesAllWentIsGoneAndOneWithACopyLeftIsThere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		isCopyGone bool
		wantFirst  string // the file's path and whether it is gone
	}{
		{"both places", true, firstPlace + "|1"},
		{"the first place", false, secondPlace + "|0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bench := newCopiesBench(t)
			bench.sweep()
			same := strings.Repeat("ab", 32)
			bench.read(bench.first, same, 5)
			bench.read(bench.second, same, 7)
			gone := []int64{bench.first}
			if tc.isCopyGone {
				gone = append(gone, bench.second)
			}
			err := bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
				return removePlaces(t.Context(), queries, slog.New(slog.DiscardHandler), gone, 9)
			})
			if err != nil {
				t.Fatal(err)
			}
			got := storetest.QueryRaw(t, bench.database.Path(),
				"SELECT id, path, removed_at IS NOT NULL FROM place WHERE id IN ("+itoa(bench.first)+", "+
					itoa(bench.second)+") ORDER BY place.id")
			want := []string{itoa(bench.first) + "|" + tc.wantFirst, itoa(bench.second) + "|" + secondPlace + "|1"}
			if !slices.Equal(got, want) {
				t.Errorf("with %s gone the rows are %v; want %v", tc.name, got, want)
			}
		})
	}
}
