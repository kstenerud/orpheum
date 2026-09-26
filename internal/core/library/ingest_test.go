// SPDX-License-Identifier: GPL-3.0-or-later

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

	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
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
	seq      int64
}

// newHarness is a database with a node and one library covering root, and the ingest.
func newHarness(t *testing.T) *harness {
	t.Helper()
	database := storetest.Open(t)
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	var nodeID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		nodeID, err = queries.InsertNode(t.Context(), sqlcgen.InsertNodeParams{Name: "local", EnrolledAt: 1})
		if err != nil {
			return err
		}
		libraryID, err := queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Films", Kind: "movies", MetadataSource: "nfo", ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return err
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: nodeID, Path: root,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{
		t: t, database: database, clock: now, nodeID: nodeID,
		ingester: NewIngester(database, now, settingstest.Seeded(t, database), slog.New(slog.DiscardHandler)),
	}
}

// hashOf is a stand-in payload hash for a file's content.
func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func observed(path, content string) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: path}, Size: int64(len(content)), MtimeMs: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file", Hash: hashOf(content), PayloadLength: 1,
		},
		Probe: nodeproto.Probe{Streams: []nodeproto.ProbeStream{
			{Index: 0, Kind: "video", Codec: "h264"},
			{
				Index: 1, Kind: "audio", Codec: "aac", Language: "ger", LanguageTag: "de", Title: "Deutsch",
				Disposition: []string{"default", "forced"}, Audio: nodeproto.ProbeAudio{Channels: 6},
			},
			{Index: 2, Kind: "data", Codec: "bin_data"},
		}},
	}}
}

func skipped(path string, rule nodeproto.SkipRule) nodeproto.Record {
	return nodeproto.Record{
		Type: nodeproto.RecordSkip, Skip: &nodeproto.Skip{Path: nodeproto.Path{Text: path}, Rule: rule},
	}
}

func failed(path string, scope nodeproto.ErrorScope) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordError, Error: &nodeproto.Error{
		Path: nodeproto.Path{Text: path}, Scope: scope, Code: nodeproto.CodePermissionDenied, RepeatCount: 1,
		Raw: nodeproto.ErrorRaw{Text: "permission denied", OwnerUID: 0, GroupGID: 0, Mode: "-rw-------"},
	}}
}

// scan sends one scan of root: begin, the records, and complete unless the scan is cut off.
func (world *harness) scan(records []nodeproto.Record, completes bool) {
	world.t.Helper()
	world.scanOn(nodeproto.ScanBegin{Root: root, Device: 7, MountPoint: true}, records, completes)
}

// scanOn is scan with the root's filesystem facts as the node reports them.
func (world *harness) scanOn(facts nodeproto.ScanBegin, records []nodeproto.Record, completes bool) {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	begin := nodeproto.Record{Type: nodeproto.RecordScanBegin, ScanBegin: &facts}
	all := append([]nodeproto.Record{begin}, records...)
	if completes {
		complete := &nodeproto.ScanComplete{Root: root}
		all = append(all, nodeproto.Record{Type: nodeproto.RecordScanComplete, ScanComplete: complete})
	}
	for index := range all {
		world.seq++
		all[index].Seq = world.seq
	}
	if err := world.ingester.Records(world.t.Context(), world.nodeID, all); err != nil {
		world.t.Fatal(err)
	}
}

// live is the live file rows by path.
func (world *harness) live() map[string]sqlcgen.ListLiveFilesByNodeRow {
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
	for path := range world.live() {
		paths = append(paths, strings.TrimPrefix(path, root+"/"))
	}
	slices.Sort(paths)
	return paths
}

func TestIngest_EveryEnumeratedPathGetsARowWithItsFilingAndReason(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.Record{
		observed("Wing (1979)/Wing.mkv", "alien"),
		skipped("Wing (1979)/notes.txt", nodeproto.SkipExtension),
		failed("Broken.mkv", nodeproto.ScopeEntry),
	}, true)
	live := world.live()
	alien, notes := live[root+"/Wing (1979)/Wing.mkv"], live[root+"/Wing (1979)/notes.txt"]
	broken := live[root+"/Broken.mkv"]
	if alien.Filing != filingUnidentified || *alien.FilingReason != notIdentifiedYet || alien.PayloadHash == nil {
		t.Errorf("the film = %+v, want unidentified with its payload hash", alien)
	}
	if notes.Filing != filingSkipped || *notes.FilingReason != "extension" || notes.PayloadHash != nil {
		t.Errorf("the notes = %+v, want skipped, rule extension (D163)", notes)
	}
	if broken.Filing != filingErrored || *broken.FilingReason != "permission_denied" ||
		broken.ErrorDetail == nil || !strings.Contains(*broken.ErrorDetail, `"mode":"-rw-------"`) {
		t.Errorf("the broken file = %+v, want errored with the owner, group and mode (D163)", broken)
	}
	world.checkStreams(alien.ID)
}

// checkStreams asserts the streams observed() describes: the video and the audio, which is
// default and forced, and no data stream.
func (world *harness) checkStreams(fileID int64) {
	world.t.Helper()
	streams, err := world.database.Read().ListStreamsOfFile(world.t.Context(), fileID)
	if err != nil {
		world.t.Fatal(err)
	}
	if len(streams) != 2 || streams[1].Kind != "audio" || *streams[1].LangRaw != "ger" || *streams[1].Lang != "de" ||
		streams[1].Disposition != 0x41 || *streams[1].Channels != 6 {
		world.t.Errorf("streams = %+v, want the video and the audio (default and forced), no data stream", streams)
	}
}

func TestIngest_D179MatchesAFileByItsPayloadWhereverItIs(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.Record{
		observed("Wing.mkv", "alien"), observed("Metropolis.mkv", "heat"), observed("Gone.mkv", "gone"),
		observed("Nosferatu.mkv", "ronin"),
	}, true)
	before := world.live()

	world.scan([]nodeproto.Record{
		observed("Wing.mkv", "alien"),                     // same path, same payload: the same file
		observed("Crime/Metropolis (1995).mkv", "heat"),          // a new path, a payload not seen yet: moved
		observed("Nosferatu.mkv", "ronin, the director's cut"), // same path, a new payload: replaced
		observed("Zarvo.mkv", "zulu"),                       // new
	}, true)
	after := world.live()

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
	world.scan([]nodeproto.Record{observed("a.mkv", "alien")}, true)
	original := world.live()[root+"/a.mkv"].ID
	// "a-copy.mkv" sorts before "a.mkv", so the copy arrives while the original is still unseen.
	world.scan([]nodeproto.Record{observed("a-copy.mkv", "alien"), observed("a.mkv", "alien")}, true)
	live := world.live()
	copied := live[root+"/a-copy.mkv"].ID
	if live[root+"/a.mkv"].ID != original || copied == 0 || copied == original {
		t.Errorf("rows %+v: want the original keeping id %d and the copy a row of its own (D179)", live, original)
	}
}

func TestIngest_OnlyACompletedScanRemovesAndNeverWhereItCouldNotRead(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	everything := []nodeproto.Record{
		observed("Locked/a.mkv", "a"), observed("Locked/b.mkv", "b"),
		observed("Open/c.mkv", "c"), observed("d.mkv", "d"),
	}
	world.scan(everything, true)

	world.scan([]nodeproto.Record{observed("Open/c.mkv", "c")}, false) // cut off: nothing may go
	if got := len(world.paths()); got != 4 {
		t.Fatalf("%d live files after a scan that never completed, want all 4 (D39)", got)
	}
	world.scan([]nodeproto.Record{failed("Locked", nodeproto.ScopeSubtree), observed("Open/c.mkv", "c")}, true)
	want := []string{"Locked", "Locked/a.mkv", "Locked/b.mkv", "Open/c.mkv"}
	if got := world.paths(); !slices.Equal(got, want) {
		t.Errorf("live %q, want %q: d.mkv gone, the unreadable Locked/ kept whole (D40)", got, want)
	}
	world.scan([]nodeproto.Record{failed(".", nodeproto.ScopeRoot)}, true)
	if got := world.paths(); !slices.Equal(got, want) {
		t.Errorf("live %q after the root could not be read, want nothing removed", got)
	}
}

func TestIngest_ARootNoLibraryCoversIsNotStored(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	records := []nodeproto.Record{
		{Seq: 1, Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{Root: "/media/elsewhere"}},
		observed("x.mkv", "x"),
		{Seq: 3, Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{Root: "/media/elsewhere"}},
	}
	if err := world.ingester.Records(t.Context(), world.nodeID, records); err != nil {
		t.Fatal(err)
	}
	if got := world.live(); len(got) != 0 {
		t.Errorf("stored %v for a root no library covers", got)
	}
}

func TestIngest_APathIsStoredInNFCBesideItsBytes(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	decomposed := "Fièvre.mkv" // as macOS writes it
	world.scan([]nodeproto.Record{observed(decomposed, "fievre")}, true)
	row, found := world.live()[root+"/Fièvre.mkv"]
	if !found || string(row.PathRaw) != root+"/"+decomposed {
		t.Errorf("rows %+v: want the NFC path, with the decomposed bytes beside it (D49)", world.live())
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
	world.scan([]nodeproto.Record{observed("Film.mkv", "same bytes")}, true)
	film := world.live()[root+"/Film.mkv"]

	// The shows root holds a file with the film's payload. The film was not seen by this scan
	// only because it is in another root, so it is not absent: this is a duplicate, not a move.
	world.clock.Advance(time.Minute)
	records := []nodeproto.Record{
		{Seq: 100, Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{Root: shows}},
		observed("Episode.mkv", "same bytes"),
		{Seq: 102, Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{Root: shows}},
	}
	if err := world.ingester.Records(t.Context(), world.nodeID, records); err != nil {
		t.Fatal(err)
	}
	live := world.live()
	if kept, found := live[root+"/Film.mkv"]; !found || kept.ID != film.ID || kept.LibraryID != film.LibraryID {
		t.Errorf("the film is %+v, want it left where it is, in its library", kept)
	}
	if episode, found := live[shows+"/Episode.mkv"]; !found || episode.ID == film.ID {
		t.Errorf("the episode is %+v, want a row of its own", episode)
	}
}

func rootStateParams(nodeID int64) sqlcgen.GetRootStateParams {
	return sqlcgen.GetRootStateParams{NodeID: nodeID, Path: root}
}

func sidecar(path string, kind nodeproto.SidecarKind, body string) nodeproto.Record {
	record := &nodeproto.Sidecar{Path: nodeproto.Path{Text: path}, Kind: kind, Size: int64(len(body)), MtimeMs: 1}
	if kind == nodeproto.SidecarNFO {
		record.Body = []byte(body)
	}
	return nodeproto.Record{Type: nodeproto.RecordSidecar, Sidecar: record}
}

func TestIngest_ASidecarIsAttachedToTheVideoItBelongsTo(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.Record{
		// Sidecars before their videos, as byte order often sends them.
		sidecar("Metropolis (1995)/Metropolis (1995) - 720p.en.srt", nodeproto.SidecarSubtitle, "1\n"),
		sidecar("Metropolis (1995)/Metropolis (1995).nfo", nodeproto.SidecarNFO, "<movie><title>Metropolis</title></movie>"),
		observed("Metropolis (1995)/Metropolis (1995) - 720p.mkv", "heat 720"),
		observed("Metropolis (1995)/Metropolis (1995).mkv", "heat 1080"),
		observed("Nosferatu (1998)/nosferatu.mkv", "ronin"),
		sidecar("Nosferatu (1998)/movie.nfo", nodeproto.SidecarNFO, "<movie/>"),
		sidecar("Show/tvshow.nfo", nodeproto.SidecarNFO, "<tvshow/>"),
	}, true)
	rows, err := world.database.Read().ListLiveFilesForAttaching(t.Context(), sqlcgen.ListLiveFilesForAttachingParams{
		NodeID: world.nodeID, LibraryID: world.live()[root+"/Metropolis (1995)/Metropolis (1995).mkv"].LibraryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListLiveFilesForAttachingRow{}
	for _, row := range rows {
		byPath[strings.TrimPrefix(row.Path, root+"/")] = row
	}
	owner := func(sidecarPath string) string {
		attached := byPath[sidecarPath].AttachedTo
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
	body, err := world.database.Read().GetSidecarBody(t.Context(), byPath["Metropolis (1995)/Metropolis (1995).nfo"].ID)
	if err != nil || string(body) != "<movie><title>Metropolis</title></movie>" {
		t.Errorf("the NFO's stored bytes are %q, %v; identification reads them (D182)", body, err)
	}
	for sidecarPath, video := range want {
		if byPath[sidecarPath].Filing != filingAttached {
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
	world.ingester.AfterScan(func(_ context.Context, libraryID int64) { handed = append(handed, libraryID) })
	world.scan([]nodeproto.Record{observed("a.mkv", "a")}, false)
	if len(handed) != 0 {
		t.Errorf("a scan that never ended handed on %v", handed)
	}
	world.scan([]nodeproto.Record{observed("a.mkv", "a")}, true)
	if len(handed) != 1 || handed[0] != world.live()[root+"/a.mkv"].LibraryID {
		t.Errorf("handed on %v, want the scanned library once", handed)
	}
}

// D211 §1: two names that fold to one text are two files.
func TestIngest_TwoNamesThatAreOneTextAreTwoFiles(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	invalid1, invalid2 := observed("a�.mkv", "first"), observed("a�.mkv", "second")
	invalid1.Observed.Path.Raw, invalid2.Observed.Path.Raw = []byte("a\xff.mkv"), []byte("a\xfe.mkv")
	// "é" as one code point and as e plus a combining accent: one NFC text, two files on ext4.
	composed, decomposed := observed("café.mkv", "third"), observed("café.mkv", "fourth")
	for range 2 {
		world.scan([]nodeproto.Record{invalid1, invalid2, composed, decomposed}, true)
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
	hostile := observed("evil\u202Evkm.mkv", "hostile")
	hostile.Observed.Probe.Streams[1].Title = "Commentary\u2066\u0007\u200B"
	world.scan([]nodeproto.Record{hostile}, true)
	rows, err := world.database.Read().ListLiveFilesByNode(t.Context(), world.nodeID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("files %+v, %v", rows, err)
	}
	file := rows[0]
	if file.Path != root+"/evil�vkm.mkv" || string(file.PathRaw) != root+"/evil\u202Evkm.mkv" {
		t.Errorf("path %q, raw %q; want the override marked in the text and kept in the bytes",
			file.Path, file.PathRaw)
	}
	streams, err := world.database.Read().ListStreamsOfFile(t.Context(), file.ID)
	if err != nil || len(streams) < 2 || streams[1].TitleRaw == nil || *streams[1].TitleRaw != "Commentary" {
		t.Errorf("streams %+v, %v; want the track title clean", streams, err)
	}
}
