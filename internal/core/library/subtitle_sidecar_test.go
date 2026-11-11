// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

func TestSidecarTrackOf_ReadsTheLanguageAndFlagsAfterTheVideosName(t *testing.T) {
	t.Parallel()
	forced, deaf, isDefault := DispositionBit("forced"), DispositionBit("hearing_impaired"), DispositionBit("default")
	const video = "/media/Copper Kettle (2015)/Copper Kettle (2015).mkv"
	for name, want := range map[string]sidecarTrack{
		"Copper Kettle (2015).srt":    {codec: "subrip"},
		"Copper Kettle (2015).en.srt": {codec: "subrip", containerLanguage: "en", languageTag: "en"},
		"Copper Kettle (2015).ger.forced.srt": {
			codec: "subrip", containerLanguage: "ger", languageTag: "de", disposition: forced,
		},
		"Copper Kettle (2015).forced.pt-BR.ass": {
			codec: "ass", containerLanguage: "pt-BR", languageTag: "pt-BR", disposition: forced,
		},
		"Copper Kettle (2015).en.SDH.vtt": {
			codec: "webvtt", containerLanguage: "en", languageTag: "en", disposition: deaf,
		},
		"Copper Kettle (2015).es.hi.default.srt": {
			codec: "subrip", containerLanguage: "es", languageTag: "es", disposition: deaf | isDefault,
		},
		"Copper Kettle (2015).commentary.en.srt": {codec: "subrip", containerLanguage: "en", languageTag: "en"},
		"Copper Kettle (2015).fr.cc.ssa": {
			codec: "ssa", containerLanguage: "fr", languageTag: "fr", disposition: deaf,
		},
		"Copper Kettle (2015).en.fr.srt": {codec: "subrip", containerLanguage: "en", languageTag: "en"},
		"Something else entirely.en.srt": {codec: "subrip"},
	} {
		if got := sidecarTrackOf("/media/Copper Kettle (2015)/"+name, video); got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
}

// subtitleSidecarRecord is a subtitle file's record, with the payload hash a scan gives it.
func subtitleSidecarRecord(path, content string) nodeproto.ScanRecord {
	record := sidecarRecord(path, nodeproto.SidecarSubtitle, content)
	record.Sidecar.Identity = &nodeproto.Identity{
		Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf(content), PayloadBytes: int64(len(content)),
	}
	return record
}

func TestIngest_ASubtitleFileIsATrackOfItsVideoUntilItsVideoIsGone(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	// A subtitle file that belongs to no video is recorded, with no track, and the scan goes on.
	world.scan([]nodeproto.ScanRecord{
		subtitleSidecarRecord("Metropolis (1927)/Metropolis (1927).en.forced.srt", "1\n"),
	}, true)
	records := []nodeproto.ScanRecord{
		subtitleSidecarRecord("Metropolis (1927)/Metropolis (1927).en.forced.srt", "1\n"),
		observedRecord("Metropolis (1927)/Metropolis (1927).mkv", "metropolis"),
	}
	world.scan(records, true)
	files := world.liveFilesByPath()
	sidecarFile := files[root+"/Metropolis (1927)/Metropolis (1927).en.forced.srt"]
	video := files[root+"/Metropolis (1927)/Metropolis (1927).mkv"]
	if hex.EncodeToString(sidecarFile.PayloadHash) != hashOf("1\n") {
		t.Errorf("the subtitle file's payload hash is %x, want its bytes' (D179)", sidecarFile.PayloadHash)
	}
	track := func() (sqlcgen.GetSidecarStreamRow, error) {
		return store.OrNotFound(world.database.Read().GetSidecarStream(t.Context(), sidecarFile.ID))
	}
	first, err := track()
	if err != nil {
		t.Fatalf("the subtitle file has no track: %v", err)
	}
	if first.VideoFile != video.ID || first.Codec == nil || *first.Codec != "subrip" || first.Language == nil ||
		*first.Language != "en" || first.DispositionFlags != DispositionBit("forced") {
		t.Errorf("the track = %+v, want an English forced subrip track of the video", first)
	}
	world.scan(records, true)
	if again, err := track(); err != nil || again.ID != first.ID {
		t.Errorf("after a rescan the track is %+v, %v; want the same row, whose id a job names", again, err)
	}
	world.scan(records[:1], true)
	if _, err := track(); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("with its video gone the subtitle file still has a track: %v", err)
	}
}

// D363 §3: a sidecar beside a copy, attached through the ingest path that reads it, is a track of
// the file the copy is of, not of the copy's own file id: a play of the file is offered it however
// it is reached.
func TestIngest_ASubtitleBesideACopyIsATrackOfTheFileItIsACopyOf(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("Metropolis (1927)/Metropolis (1927).mkv", "metropolis")}, true)
	fileID := world.liveFilesByPath()[root+"/Metropolis (1927)/Metropolis (1927).mkv"].ID
	backupID := librarytest.LibraryOn(t, world.database, world.nodeID,
		librarytest.Library{Name: "Backup", Kind: "movie", MetadataSource: "nfo", Root: "/backup"})
	payloadHash, decodeErr := hex.DecodeString(hashOf("metropolis"))
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	copyID := librarytest.File(t, world.database, librarytest.FileSpec{
		LibraryID: backupID, NodeID: world.nodeID, Path: "/backup/Metropolis (1927).mkv", Filing: "copy",
		CopyOf: fileID, PayloadHash: payloadHash, FirstSeenAt: 3,
	})
	// A scan of Backup that still sees its copy (as its own, unrelated scan would) alongside the
	// subtitle beside it: attachSidecars runs over the two together, as it does in production.
	backupFacts := nodeproto.ScanBegin{RootPath: "/backup", FilesystemDevice: 7, IsMountPoint: true}
	backupRecords := []nodeproto.ScanRecord{
		observedRecord("Metropolis (1927).mkv", "metropolis"),
		sidecarRecord("Metropolis (1927).en.srt", nodeproto.SidecarSubtitle, "1\n"),
	}
	world.scanOn(backupFacts, backupRecords, true)
	subtitleID := world.liveFilesByPath()["/backup/Metropolis (1927).en.srt"].ID
	track, err := store.OrNotFound(world.database.Read().GetSidecarStream(t.Context(), subtitleID))
	if err != nil {
		t.Fatalf("the subtitle beside the copy has no track: %v", err)
	}
	if track.VideoFile != fileID {
		t.Errorf("the subtitle's track names video file %d, the copy's own; want %d, the file it is a copy of "+
			"(copy id %d)", track.VideoFile, fileID, copyID)
	}
}
