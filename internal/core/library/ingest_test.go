// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const root = "/media/films"

type harness struct {
	t        *testing.T
	database *store.Database
	ingester *Ingester
	clock    *clocktest.Fake
	nodeID   int64
	rootPath string // the library's one root, as its node names it
	settings *settings.Service
}

// newHarness is a database with a node and one library covering root, and the ingest.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessAt(t, root)
}

// newHarnessAt is newHarness with the library's root at rootPath.
func newHarnessAt(t *testing.T, rootPath string) *harness {
	t.Helper()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	nodeID, _ := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: rootPath})
	seeded := settingstest.Seeded(t, database)
	return &harness{
		t: t, database: database, clock: fakeClock, nodeID: nodeID, rootPath: rootPath, settings: seeded,
		ingester: NewIngester(database, fakeClock, seeded, slog.New(slog.DiscardHandler)),
	}
}

// hashOf is a stand-in payload hash for a file's content.
func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func observedRecord(path, content string) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: path}, SizeBytes: int64(len(content)), ModifiedAt: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf(content), PayloadBytes: 1,
		},
		Probe: nodeproto.Probe{Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "video", Codec: "h264"},
			{
				StreamIndex: 1, Kind: "audio", Codec: "aac", ContainerLanguage: "ger", Language: "de",
				ContainerTitle: "Deutsch",
				Disposition:    []string{"default", "forced"}, Audio: nodeproto.ProbeAudio{ChannelCount: 6},
			},
			{StreamIndex: 2, Kind: "data", Codec: "bin_data"},
		}},
	}}
}

func skipRecord(path string, rule nodeproto.SkipRule) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{
		Type: nodeproto.RecordSkip, Skip: &nodeproto.Skip{Path: nodeproto.Path{Text: path}, Rule: rule},
	}
}

func errorRecord(path string, scope nodeproto.ErrorScope) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordError, Error: &nodeproto.ScanError{
		Path: nodeproto.Path{Text: path}, Scope: scope, Code: nodeproto.CodePermissionDenied, OccurrenceCount: 1,
		PlatformError: nodeproto.PlatformError{
			Text: "permission denied", OwnerUID: 0, GroupGID: 0,
			ModeText: "-rw-------",
		},
	}}
}

// scan sends one scan of the root: begin, the records, and complete unless the scan is cut off.
func (world *harness) scan(records []nodeproto.ScanRecord, isComplete bool) {
	world.t.Helper()
	facts := nodeproto.ScanBegin{RootPath: world.rootPath, FilesystemDevice: 7, IsMountPoint: true}
	world.scanOn(facts, records, isComplete)
}

// scanOn is scan with the root's filesystem facts as the node reports them.
func (world *harness) scanOn(facts nodeproto.ScanBegin, records []nodeproto.ScanRecord, isComplete bool) {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	all := librarytest.Scan(facts, records, isComplete)
	if err := world.ingester.IngestScan(world.t.Context(), world.nodeID, facts.RootPath, all); err != nil {
		world.t.Fatal(err)
	}
}

// liveFilesByPath is the live file rows by path.
func (world *harness) liveFilesByPath() map[string]sqlcgen.ListLiveFilesByNodeRow {
	world.t.Helper()
	rows, err := world.database.Read().ListLiveFilesByNode(world.t.Context(), world.nodeID)
	if err != nil {
		world.t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListLiveFilesByNodeRow{}
	for _, row := range rows {
		byPath[row.Path] = row
	}
	return byPath
}

func (world *harness) paths() []string {
	world.t.Helper()
	var paths []string
	for path := range world.liveFilesByPath() {
		paths = append(paths, strings.TrimPrefix(path, root+"/"))
	}
	slices.Sort(paths)
	return paths
}

func TestIngest_EveryEnumeratedPathGetsARowWithItsFilingAndReason(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Wing (1979)/Wing.mkv", "alien"),
		skipRecord("Wing (1979)/notes.txt", nodeproto.SkipExtension),
		errorRecord("Broken.mkv", nodeproto.ScopeEntry),
	}, true)
	live := world.liveFilesByPath()
	alien, notes := live[root+"/Wing (1979)/Wing.mkv"], live[root+"/Wing (1979)/notes.txt"]
	broken := live[root+"/Broken.mkv"]
	if alien.Filing != FilingUnidentified || *alien.FilingReason != notIdentifiedYet || alien.PayloadHash == nil {
		t.Errorf("the film = %+v, want unidentified with its payload hash", alien)
	}
	if notes.Filing != FilingSkipped || *notes.FilingReason != "extension" || notes.PayloadHash != nil {
		t.Errorf("the notes = %+v, want skipped, rule extension (D163)", notes)
	}
	if broken.Filing != FilingErrored || *broken.FilingReason != "permission_denied" ||
		broken.ErrorDetailJSON == nil || !strings.Contains(*broken.ErrorDetailJSON, `"modeText":"-rw-------"`) {
		t.Errorf("the broken file = %+v, want errored with the owner, group and mode (D163)", broken)
	}
	world.checkStreams(alien.ID)
}

// checkStreams asserts the streams observed() describes: the video and the audio, which is
// default and forced, and no data stream.
func (world *harness) checkStreams(fileID int64) {
	world.t.Helper()
	streams, err := world.database.Read().ListStreamsOfBytesFile(world.t.Context(), fileID)
	if err != nil {
		world.t.Fatal(err)
	}
	if len(streams) != 2 || streams[1].Kind != "audio" || *streams[1].ContainerLanguage != "ger" ||
		*streams[1].Language != "de" ||
		streams[1].DispositionFlags != 0x41 || *streams[1].ChannelCount != 6 {
		world.t.Errorf("streams = %+v, want the video and the audio (default and forced), no data stream", streams)
	}
}

func TestIngest_D179MatchesAFileByItsPayloadWhereverItIs(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Wing.mkv", "alien"), observedRecord("Metropolis.mkv", "heat"), observedRecord("Gone.mkv", "gone"),
		observedRecord("Nosferatu.mkv", "ronin"),
	}, true)
	before := world.liveFilesByPath()

	world.scan([]nodeproto.ScanRecord{
		observedRecord("Wing.mkv", "alien"),                     // same path, same payload: the same file
		observedRecord("Crime/Metropolis (1995).mkv", "heat"),          // a new path, a payload not seen yet: moved
		observedRecord("Nosferatu.mkv", "ronin, the director's cut"), // same path, a new payload: replaced
		observedRecord("Zarvo.mkv", "zulu"),                       // new
	}, true)
	after := world.liveFilesByPath()

	if after[root+"/Wing.mkv"].ID != before[root+"/Wing.mkv"].ID {
		t.Error("a file seen again at its path got a new row")
	}
	if moved := after[root+"/Crime/Metropolis (1995).mkv"]; moved.ID != before[root+"/Metropolis.mkv"].ID {
		t.Errorf("the moved file has id %d, want %d: its history follows it (D179)",
			moved.ID, before[root+"/Metropolis.mkv"].ID)
	}
	if replaced := after[root+"/Nosferatu.mkv"]; replaced.ID == before[root+"/Nosferatu.mkv"].ID {
		t.Error("a different payload at the same path kept the old row; D173 says a new file, the old tombstoned")
	}
	want := []string{"Wing.mkv", "Crime/Metropolis (1995).mkv", "Nosferatu.mkv", "Zarvo.mkv"}
	if got := world.paths(); !slices.Equal(got, want) {
		t.Errorf("live files %q, want %q: Gone.mkv tombstoned, Metropolis.mkv moved", got, want)
	}
}

func TestIngest_ACopyIsADuplicateAndTheRecordedPathKeepsTheIdentity(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "alien")}, true)
	original := world.liveFilesByPath()[root+"/a.mkv"].ID
	// "a-copy.mkv" sorts before "a.mkv", so the copy arrives while the original is still unseen.
	world.scan([]nodeproto.ScanRecord{observedRecord("a-copy.mkv", "alien"), observedRecord("a.mkv", "alien")}, true)
	live := world.liveFilesByPath()
	copied := live[root+"/a-copy.mkv"].ID
	if live[root+"/a.mkv"].ID != original || copied == 0 || copied == original {
		t.Errorf("rows %+v: want the original keeping id %d and the copy a row of its own (D179)", live, original)
	}
}

func TestIngest_OnlyACompletedScanRemovesAndNeverWhereItCouldNotRead(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	everything := []nodeproto.ScanRecord{
		observedRecord("Locked/a.mkv", "a"), observedRecord("Locked/b.mkv", "b"),
		observedRecord("Open/c.mkv", "c"), observedRecord("d.mkv", "d"),
	}
	world.scan(everything, true)

	world.scan([]nodeproto.ScanRecord{observedRecord("Open/c.mkv", "c")}, false) // cut off: nothing may go
	if got := len(world.paths()); got != 4 {
		t.Fatalf("%d live files after a scan that never completed, want all 4 (D39)", got)
	}
	world.scan([]nodeproto.ScanRecord{
		errorRecord("Locked", nodeproto.ScopeSubtree), observedRecord("Open/c.mkv", "c"),
	}, true)
	want := []string{"Locked", "Locked/a.mkv", "Locked/b.mkv", "Open/c.mkv"}
	if got := world.paths(); !slices.Equal(got, want) {
		t.Errorf("live %q, want %q: d.mkv gone, the unreadable Locked/ kept whole (D40)", got, want)
	}
	world.scan([]nodeproto.ScanRecord{errorRecord(".", nodeproto.ScopeRoot)}, true)
	if got := world.paths(); !slices.Equal(got, want) {
		t.Errorf("live %q after the root could not be read, want nothing removed", got)
	}
}

func TestIngest_ARootNoLibraryCoversIsNotStored(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scanOn(nodeproto.ScanBegin{RootPath: "/media/elsewhere"},
		[]nodeproto.ScanRecord{observedRecord("x.mkv", "x")},
		true)
	if got := world.liveFilesByPath(); len(got) != 0 {
		t.Errorf("stored %v for a root no library covers", got)
	}
}

func TestIngest_APathIsStoredInNFCBesideItsBytes(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	decomposed := "Fièvre.mkv" // as macOS writes it
	world.scan([]nodeproto.ScanRecord{observedRecord(decomposed, "fievre")}, true)
	row, found := world.liveFilesByPath()[root+"/Fièvre.mkv"]
	if !found || string(row.ReportedPathRaw) != root+"/"+decomposed {
		t.Errorf("rows %+v: want the NFC path, with the decomposed bytes beside it (D49)", world.liveFilesByPath())
	}
}

func TestIngest_AFileInAnotherRootIsNeverTakenForAMove(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	const shows = "/media/shows"
	err := world.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		libraryID, err := queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Shows", Kind: "shows", MetadataSource: "nfo", ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return err
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: world.nodeID, Path: shows,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("Film.mkv", "same bytes")}, true)
	film := world.liveFilesByPath()[root+"/Film.mkv"]

	// The shows root holds a file with the film's payload. The film was not seen by this scan
	// only because it is in another root, so it is not absent: this is a duplicate, not a move.
	world.scanOn(nodeproto.ScanBegin{RootPath: shows},
		[]nodeproto.ScanRecord{observedRecord("Episode.mkv", "same bytes")},
		true)
	live := world.liveFilesByPath()
	if kept, found := live[root+"/Film.mkv"]; !found || kept.ID != film.ID || kept.LibraryID != film.LibraryID {
		t.Errorf("the film is %+v, want it left where it is, in its library", kept)
	}
	if episode, found := live[shows+"/Episode.mkv"]; !found || episode.ID == film.ID {
		t.Errorf("the episode is %+v, want a row of its own", episode)
	}
}

func rootStateParams(nodeID int64) sqlcgen.GetRootStateParams {
	return sqlcgen.GetRootStateParams{NodeID: nodeID, RootPath: root}
}

func sidecarRecord(path string, kind nodeproto.SidecarKind, body string) nodeproto.ScanRecord {
	record := &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: kind, SizeBytes: int64(len(body)),
		ModifiedAt: 1,
	}
	if kind == nodeproto.SidecarNFO {
		record.NFOBody = []byte(body)
	}
	return nodeproto.ScanRecord{Type: nodeproto.RecordSidecar, Sidecar: record}
}

func TestIngest_ASidecarIsAttachedToTheVideoItBelongsTo(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		// Sidecars before their videos, as byte order often sends them.
		sidecarRecord("Metropolis (1995)/Metropolis (1995) - 720p.en.srt", nodeproto.SidecarSubtitle, "1\n"),
		sidecarRecord("Metropolis (1995)/Metropolis (1995).nfo", nodeproto.SidecarNFO, "<movie><title>Metropolis</title></movie>"),
		observedRecord("Metropolis (1995)/Metropolis (1995) - 720p.mkv", "heat 720"),
		observedRecord("Metropolis (1995)/Metropolis (1995).mkv", "heat 1080"),
		observedRecord("Nosferatu (1998)/nosferatu.mkv", "ronin"),
		sidecarRecord("Nosferatu (1998)/movie.nfo", nodeproto.SidecarNFO, "<movie/>"),
		sidecarRecord("Show/tvshow.nfo", nodeproto.SidecarNFO, "<tvshow/>"),
	}, true)
	rows, err := world.database.Read().ListLiveFilesForAttaching(t.Context(), sqlcgen.ListLiveFilesForAttachingParams{
		NodeID: world.nodeID, LibraryID: world.liveFilesByPath()[root+"/Metropolis (1995)/Metropolis (1995).mkv"].LibraryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListLiveFilesForAttachingRow{}
	for _, row := range rows {
		byPath[strings.TrimPrefix(row.Path, root+"/")] = row
	}
	owner := func(sidecarPath string) string {
		attached := byPath[sidecarPath].AttachedToFile
		if attached == nil {
			return ""
		}
		for name, row := range byPath {
			if row.ID == *attached {
				return name
			}
		}
		return "?"
	}
	want := map[string]string{
		"Metropolis (1995)/Metropolis (1995) - 720p.en.srt": "Metropolis (1995)/Metropolis (1995) - 720p.mkv",
		"Metropolis (1995)/Metropolis (1995).nfo":           "Metropolis (1995)/Metropolis (1995).mkv",
		"Nosferatu (1998)/movie.nfo":                "Nosferatu (1998)/nosferatu.mkv",
		"Show/tvshow.nfo":                       "",
	}
	body, err := world.database.Read().GetNFOBody(t.Context(), byPath["Metropolis (1995)/Metropolis (1995).nfo"].ID)
	if err != nil || string(body) != "<movie><title>Metropolis</title></movie>" {
		t.Errorf("the NFO's stored bytes are %q, %v; identification reads them (D182)", body, err)
	}
	for sidecarPath, video := range want {
		if byPath[sidecarPath].Filing != FilingAttached {
			t.Errorf("%s is filed %q, want attached", sidecarPath, byPath[sidecarPath].Filing)
		}
		if got := owner(sidecarPath); got != video {
			t.Errorf("%s is attached to %q, want %q", sidecarPath, got, video)
		}
	}
}

func TestIngest_AfterAScanEndsItsLibraryIsHandedOn(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	var handed []int64
	world.ingester.SetOnScanEnded(func(_ context.Context, libraryID int64) { handed = append(handed, libraryID) })
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a")}, false)
	if len(handed) != 0 {
		t.Errorf("a scan that never ended handed on %v", handed)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a")}, true)
	if len(handed) != 1 || handed[0] != world.liveFilesByPath()[root+"/a.mkv"].LibraryID {
		t.Errorf("handed on %v, want the scanned library once", handed)
	}
}

func TestIngest_ABatchThatAddsAFileAsksForAPassBeforeTheScanEnds(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	asked := 0
	world.ingester.SetOnNewFiles(func(context.Context) { asked++ })
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a")}, false)
	if asked != 1 {
		t.Errorf("a scan still going that found a new film asked for %d passes, want 1", asked)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a")}, false)
	if asked != 1 {
		t.Errorf("the same film read again unchanged asked for a pass (%d asked), want none", asked)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a"), observedRecord("b.mkv", "b")}, true)
	if asked != 2 {
		t.Errorf("a second film asked for %d passes in all, want 2", asked)
	}
}

// D211 §1: two names that fold to one text are two files.
func TestIngest_TwoNamesThatAreOneTextAreTwoFiles(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	invalid1, invalid2 := observedRecord("a�.mkv", "first"), observedRecord("a�.mkv", "second")
	invalid1.Observed.Path.Raw, invalid2.Observed.Path.Raw = []byte("a\xff.mkv"), []byte("a\xfe.mkv")
	// "é" as one code point and as e plus a combining accent: one NFC text, two files on ext4.
	composed, decomposed := observedRecord("café.mkv", "third"), observedRecord("café.mkv", "fourth")
	for range 2 {
		world.scan([]nodeproto.ScanRecord{invalid1, invalid2, composed, decomposed}, true)
		rows, err := world.database.Read().ListLiveFilesByNode(t.Context(), world.nodeID)
		if err != nil || len(rows) != 4 {
			t.Fatalf("%d live files (%v), want 4: each keeps its own row across scans", len(rows), err)
		}
	}
}

// Principles §22, D211: what a node reports is clean where it enters; the bytes stay bytes.
func TestIngest_WhatANodeReportsIsCleanWhereItEnters(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	hostile := observedRecord("evil\u202Evkm.mkv", "hostile")
	hostile.Observed.Probe.Streams[1].ContainerTitle = "Commentary\u2066\u0007\u200B"
	world.scan([]nodeproto.ScanRecord{hostile}, true)
	rows, err := world.database.Read().ListLiveFilesByNode(t.Context(), world.nodeID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("files %+v, %v", rows, err)
	}
	file := rows[0]
	if file.Path != root+"/evil�vkm.mkv" || string(file.ReportedPathRaw) != root+"/evil\u202Evkm.mkv" {
		t.Errorf("path %q, raw %q; want the override marked in the text and kept in the bytes",
			file.Path, file.ReportedPathRaw)
	}
	streams, err := world.database.Read().ListStreamsOfBytesFile(t.Context(), file.ID)
	if err != nil || len(streams) < 2 || streams[1].ContainerTitle == nil ||
		*streams[1].ContainerTitle != "Commentary" {
		t.Errorf("streams %+v, %v; want the track title clean", streams, err)
	}
}

// D224 §3: a video's OpenSubtitles hash is kept, and only in the shape the provider gives it.
func TestIngest_KeepsAVideosOpenSubtitlesHashOnlyInItsOwnShape(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	hashed := observedRecord("hashed.mkv", "hashed")
	hashed.Observed.OpensubtitlesHash = "8e245d9679d31e12"
	hostile := observedRecord("hostile.mkv", "hostile")
	hostile.Observed.OpensubtitlesHash = "8e245d9679d31e12&moviehash_match=only"
	world.scan([]nodeproto.ScanRecord{hashed, hostile}, true)
	rows, err := world.database.Read().ListLiveFilesByNode(t.Context(), world.nodeID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("files %+v, %v", rows, err)
	}
	for _, row := range rows {
		file, err := world.database.Read().GetFileForSubtitleSearch(t.Context(), row.ID)
		if err != nil {
			t.Fatal(err)
		}
		got := ptr.ValueOrZero(file.OpensubtitlesHash)
		want := ""
		if strings.HasSuffix(row.Path, "/hashed.mkv") {
			want = "8e245d9679d31e12"
		}
		if got != want {
			t.Errorf("%s keeps the hash %q, want %q", row.Path, got, want)
		}
	}
}
