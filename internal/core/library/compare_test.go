// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// listedFile and listedDirectory are a listing's entries, relative to the root.
func listedFile(path string, sizeBytes, modifiedAt int64) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordListed, Listed: &nodeproto.Listed{
		Path: nodeproto.Path{Text: path}, SizeBytes: sizeBytes, ModifiedAt: modifiedAt, FilesystemDevice: 7,
	}}
}

func listedDirectory(path string) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordListed, Listed: &nodeproto.Listed{
		Path: nodeproto.Path{Text: path}, IsDirectory: true, FilesystemDevice: 7,
	}}
}

// testRules read mkv and mp4 as videos and srt as subtitles.
var testRules = ScanRules{
	VideoExtensions: map[string]bool{"mkv": true, "mp4": true}, SubtitleExtensions: map[string]bool{"srt": true},
}

// takeListing has world's root listed with records, framed as a listing is, complete unless isStopped.
func (world *harness) takeListing(records []nodeproto.ScanRecord, isStopped bool) TakenListing {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	all := append([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: world.rootPath, FilesystemDevice: 7, IsMountPoint: true,
	}}}, records...)
	if !isStopped {
		all = append(all, nodeproto.ScanRecord{
			Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: world.rootPath},
		})
	}
	taken, err := world.ingester.TakeListing(world.t.Context(), world.nodeID, world.rootPath, wholeRoot, all, testRules)
	if err != nil {
		world.t.Fatal(err)
	}
	return taken
}

// observedAt is observedRecord read on device 7 at an inode.
func observedAt(path, content string, inode uint64) nodeproto.ScanRecord {
	record := observedRecord(path, content)
	record.Observed.FilesystemDevice, record.Observed.Inode = 7, inode
	return record
}

// listedAt is listedFile, modified at 1 as observedRecord's files are, at an inode on device 7.
func listedAt(path string, sizeBytes int64, inode uint64) nodeproto.ScanRecord {
	record := listedFile(path, sizeBytes, 1)
	record.Listed.Inode = inode
	return record
}

// examined names each file taken to be read, as "kind path".
func examined(taken TakenListing) []string {
	var named []string
	for _, file := range taken.ToExamine {
		named = append(named, string(file.Kind)+" "+file.Path.Text)
	}
	return named
}

// The folders a NAS or an operating system keeps for itself are skipped, at any depth, each filed
// with its rule: Synology's recycle bin was read as a film's folder (library-scanner-practice-
// 2026-10-16.md §5.3).
func TestTakeListing_SkipsTheFoldersANASOrAnOperatingSystemKeepsForItself(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory("#recycle"),
		listedFile("#recycle/Old Film (1950)/Old Film (1950).mkv", 3, 1),
		listedDirectory(".snapshot"),
		listedFile(".snapshot/daily.0/Metropolis (1995)/Metropolis.mkv", 3, 1),
		listedDirectory("Metropolis (1995)"),
		listedFile("Metropolis (1995)/Metropolis.mkv", 4, 1),
		listedDirectory("Metropolis (1995)/@eaDir"),
		listedFile("Metropolis (1995)/@eaDir/Metropolis.mkv", 4, 1),
	}, false)
	if got, want := examined(taken), []string{"video Metropolis (1995)/Metropolis.mkv"}; !slices.Equal(got, want) {
		t.Errorf("to read %v, want %v", got, want)
	}
	skipped := map[string]string{}
	for path, file := range world.liveFilesByPath() {
		if file.Filing == FilingSkipped {
			skipped[strings.TrimPrefix(path, root+"/")] = *file.FilingReason
		}
	}
	want := map[string]string{"#recycle": "system", ".snapshot": "system", "Metropolis (1995)/@eaDir": "system"}
	if !mapsEqual(skipped, want) {
		t.Errorf("skipped %v, want %v", skipped, want)
	}
}

// A disc's image is read as a video, its node serving its main title, though the video extensions
// do not name it (D350); a disc's folder is skipped, filed with its rule, so a film the beta does not
// read is not missing in silence, and the videos inside it are never read as films of their own
// (D306 §5).
func TestTakeListing_ReadsADiscsImageAndSkipsADiscsFolder(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedFile("Metropolis (1995).iso", 3, 1),
		listedDirectory("Arkon (1979)"),
		listedDirectory("Arkon (1979)/VIDEO_TS"),
		listedFile("Arkon (1979)/VIDEO_TS/VTS_01_1.mkv", 3, 1),
		listedDirectory("Nosferatu (1998)"),
		listedDirectory("Nosferatu (1998)/BDMV"),
		listedFile("Nosferatu (1998)/BDMV/00001.mkv", 3, 1),
		listedFile("Nosferatu (1998)/Nosferatu (1998).mkv", 4, 1),
	}, false)
	skipped := map[string]string{}
	for path, file := range world.liveFilesByPath() {
		if file.Filing == FilingSkipped {
			skipped[strings.TrimPrefix(path, root+"/")] = *file.FilingReason
		}
	}
	const rule = "discFolderNotSupportedYet"
	want := map[string]string{"Arkon (1979)/VIDEO_TS": rule, "Nosferatu (1998)/BDMV": rule}
	if !mapsEqual(skipped, want) {
		t.Errorf("skipped %v, want %v", skipped, want)
	}
	wantExamined := []string{"video Metropolis (1995).iso", "video Nosferatu (1998)/Nosferatu (1998).mkv"}
	if got := examined(taken); !slices.Equal(got, wantExamined) {
		t.Errorf("taken to be read: %q; want %q, the disc's image a video", got, wantExamined)
	}
}

// A video moved on its disk, listed at a new path on the same device and inode with the size and
// modified time stored, keeps its row and its history and is not read again; a new file elsewhere
// is read, and so is one whose inode matches but whose size does not (D304 §5, D240 §3).
func TestTakeListing_AVideoMovedOnItsDiskKeepsItsRowWithoutBeingReadAgain(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedAt("Metropolis.mkv", "heat", 41), observedAt("Arkon.mkv", "arkon", 42),
	}, true)
	heat := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory("Metropolis (1995)"),
		listedAt("Metropolis (1995)/Metropolis (1995).mkv", int64(len("heat")), 41),
		listedAt("Arkon (1979).mkv", int64(len("arkon"))+1, 42), // rewritten, not moved
		listedAt("Nosferatu.mkv", 5, 43),
	}, false)
	if got, want := examined(taken), []string{"video Arkon (1979).mkv", "video Nosferatu.mkv"}; !slices.Equal(got, want) {
		t.Errorf("to read %v, want %v: the moved film needs no read", got, want)
	}
	if moved := world.liveFilesByPath()[root+"/Metropolis (1995)/Metropolis (1995).mkv"]; moved.ID != heat {
		t.Errorf("the moved film is file %d, want %d with its history", moved.ID, heat)
	}
}

// readThen ingests what the node read of the listing's files, then the listing's end, as the scan's
// batches do once read.
func (world *harness) readThen(records ...nodeproto.ScanRecord) {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	all := append(slices.Clone(records), nodeproto.ScanRecord{
		Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: world.rootPath},
	})
	if err := world.ingester.IngestScan(world.t.Context(), world.nodeID, world.rootPath, all); err != nil {
		world.t.Fatal(err)
	}
}

// A new video is stored at its listing, unread, so the pass files it from its path before it is read;
// its read fills the same row in (D304 §1, §4).
func TestTakeListing_ANewVideoIsStoredUnreadAndItsReadFillsItsRow(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	const heat = "Metropolis (1995)/Metropolis (1995).mkv"
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory("Metropolis (1995)"), listedAt(heat, int64(len("heat")), 41),
	}, false)
	if got, want := examined(taken), []string{"video " + heat}; !slices.Equal(got, want) {
		t.Fatalf("to read %v, want %v", got, want)
	}
	unread, stored := world.liveFilesByPath()[root+"/"+heat]
	if !stored || unread.PayloadHash != nil {
		t.Fatalf("after its listing the film is %+v, stored %v; want it stored unread", unread, stored)
	}
	world.readThen(observedAt(heat, "heat", 41))
	if read := world.liveFilesByPath()[root+"/"+heat]; read.ID != unread.ID || read.PayloadHash == nil {
		t.Errorf("after its read the film is file %d, hash %x; want file %d, read", read.ID, read.PayloadHash,
			unread.ID)
	}
}

// A video moved to another disk, which no inode tells, is stored unread at its new path; its read
// finds the payload of the file stored before, which keeps its row and history, and the unread row
// is retired (D304 §5, D179).
func TestTakeListing_AVideoMovedToAnotherDiskReadAtItsNewPathKeepsItsRow(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedAt("Metropolis.mkv", "heat", 41)}, true)
	heat := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	const moved = "Disk 2/Metropolis (1995).mkv"
	world.takeListing([]nodeproto.ScanRecord{listedDirectory("Disk 2"), listedAt(moved, 4, 99)}, false)
	world.readThen(observedAt(moved, "heat", 99))
	live := world.liveFilesByPath()
	if got := live[root+"/"+moved]; got.ID != heat {
		t.Errorf("the moved film is file %d, want %d with its history", got.ID, heat)
	}
	if len(live) != 1 {
		t.Errorf("%d live files, %v; want the one, the unread row retired", len(live), slices.Collect(maps.Keys(live)))
	}
}

// The library's rules are the core's (D240 §3): a listing's trash, samples, AppleDouble files and
// files of no kind it reads are filed skipped, and what it reads is left to be read, in the listing's
// order.
func TestTakeListing_FilesEachEntryByTheLibrarysRules(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory(".trash"),
		listedFile(".trash/2026-09-01/old.mkv", 3, 1),
		listedDirectory("Arkon (1979)"),
		listedFile("Arkon (1979)/Arkon (1979).en.srt", 2, 1),
		listedFile("Arkon (1979)/Arkon (1979).mkv", 5, 1),
		listedFile("Arkon (1979)/Arkon (1979).nfo", 8, 1),
		listedFile("Arkon (1979)/sample.mkv", 6, 1),
		listedDirectory("Metropolis (1995)"),
		listedFile("Metropolis (1995)/._Metropolis.mkv", 4, 1),
		listedFile("Metropolis (1995)/Metropolis.mkv", 4, 1),
		listedDirectory("Metropolis (1995)/Sample"),
		listedFile("Metropolis (1995)/Sample/clip.mkv", 6, 1),
		listedFile("Metropolis (1995)/metropolis-SAMPLE.mp4", 6, 1),
		listedFile("Metropolis (1995)/notes.txt", 6, 1),
		listedDirectory("Shows/.trash"),
		listedFile("Shows/.trash/kept.mkv", 6, 1),
		{Type: nodeproto.RecordSkip, Skip: &nodeproto.Skip{
			Path: nodeproto.Path{Text: "pipe.mkv"}, Rule: nodeproto.SkipNotRegular,
		}},
	}, false)
	want := []string{
		"subtitle Arkon (1979)/Arkon (1979).en.srt", "video Arkon (1979)/Arkon (1979).mkv",
		"nfo Arkon (1979)/Arkon (1979).nfo", "video Metropolis (1995)/Metropolis.mkv", "video Shows/.trash/kept.mkv",
	}
	if got := examined(taken); !slices.Equal(got, want) {
		t.Errorf("to read:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	skipped := map[string]string{}
	for path, file := range world.liveFilesByPath() {
		if file.Filing == FilingSkipped {
			skipped[strings.TrimPrefix(path, root+"/")] = *file.FilingReason
		}
	}
	wantSkipped := map[string]string{
		".trash": "trash", "Arkon (1979)/sample.mkv": "sample", "Metropolis (1995)/._Metropolis.mkv": "appledouble",
		"Metropolis (1995)/Sample": "sample", "Metropolis (1995)/metropolis-SAMPLE.mp4": "sample", "Metropolis (1995)/notes.txt": "extension",
		"pipe.mkv": "notRegular",
	}
	if !mapsEqual(skipped, wantSkipped) {
		t.Errorf("skipped %v, want %v", skipped, wantSkipped)
	}
	if taken.ListedCount != 9 || taken.Complete.IsStoppedByErrors {
		t.Errorf("taken %d listed, its end %+v; want the 9 files outside the skipped directories and a "+
			"complete listing", taken.ListedCount,
			taken.Complete)
	}
}

// mapsEqual is whether two maps of strings hold the same.
func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

// An image beside the media is recorded as it is listed: its bytes are read only when its sizes are
// made (D215).
func TestTakeListing_RecordsAnImageBesideTheMediaWithoutReadingIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedFile("Film (1999)/Film (1999)-fanart.PNG", 3, 1),
		listedFile("Film (1999)/notes.gif", 3, 1),
		listedFile("Film (1999)/poster.jpg", 4, 1),
	}, false)
	if len(taken.ToExamine) != 0 {
		t.Errorf("to read %v, want nothing: an image is not read by a scan", examined(taken))
	}
	live := world.liveFilesByPath()
	for _, name := range []string{"Film (1999)-fanart.PNG", "poster.jpg"} {
		if image := live[root+"/Film (1999)/"+name]; image.Filing != FilingAttached || image.PayloadHash != nil ||
			image.SizeBytes == nil || *image.SizeBytes == 0 {
			t.Errorf("%s = %+v, want an attached image with its size and no hash", name, image)
		}
	}
	if gif := live[root+"/Film (1999)/notes.gif"]; gif.Filing != FilingSkipped {
		t.Errorf("a GIF is %+v, want skipped: it is not artwork anyone saves", gif)
	}
}

// A file whose size and modified time are as stored, and which was read whole, was seen and is not
// read again; one changed, or one a scan could not read, is (D240 §3).
func TestTakeListing_AFileAsStoredIsSeenAndNotReadAgain(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	subtitle := sidecarRecord("Arkon/Arkon.en.srt", nodeproto.SidecarSubtitle, "1\n")
	subtitle.Sidecar.Identity = &nodeproto.Identity{
		Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf("1\n"), PayloadBytes: 2,
	}
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Arkon/Arkon.mkv", "arkon"), observedRecord("Metropolis.mkv", "heat"),
		sidecarRecord("Arkon/Arkon.nfo", nodeproto.SidecarNFO, "<movie/>"), subtitle,
		errorRecord("Broken.mkv", nodeproto.ScopeEntry),
	}, true)
	before := world.liveFilesByPath()
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedFile("Arkon/Arkon.en.srt", 2, 1),
		listedFile("Arkon/Arkon.mkv", int64(len("arkon")), 1),
		listedFile("Arkon/Arkon.nfo", int64(len("<movie/>")), 1),
		listedFile("Broken.mkv", 6, 1),
		listedFile("Metropolis.mkv", int64(len("heat")), 2),
	}, false)
	if got, want := examined(taken), []string{"video Broken.mkv", "video Metropolis.mkv"}; !slices.Equal(got, want) {
		t.Errorf("to read %v, want %v: only what changed, or could not be read", got, want)
	}
	after := world.liveFilesByPath()
	for _, name := range []string{"Arkon/Arkon.mkv", "Arkon/Arkon.nfo", "Arkon/Arkon.en.srt"} {
		if seen := after[root+"/"+name]; seen.LastSeenAt <= before[root+"/"+name].LastSeenAt ||
			seen.Filing != before[root+"/"+name].Filing {
			t.Errorf("%s = %+v, want it seen again as it was", name, seen)
		}
	}
	if heat := after[root+"/Metropolis.mkv"]; heat.LastSeenAt != before[root+"/Metropolis.mkv"].LastSeenAt {
		t.Errorf("Metropolis.mkv, changed, was marked seen before it was read: %+v", heat)
	}
}

// A listing the node stopped, which never said it was complete, ends as a scan that could not read
// its root: it removes nothing (D39).
// A listing of files all as stored runs the same statements at twice their number: the files it
// finds seen are marked in one statement, not one each (D303, DL7).
func TestTakeListing_OfFilesAllAsStoredRunsTheSameStatementsAtTwiceTheirNumber(t *testing.T) {
	t.Parallel()
	var statements []int
	var tallies []string
	for _, size := range []int{3, 6} {
		world := newHarness(t)
		var observed, listed []nodeproto.ScanRecord
		for number := range size {
			name := fmt.Sprintf("Film %d.mkv", number)
			observed = append(observed, observedRecord(name, name))
			listed = append(listed, listedFile(name, int64(len(name)), 1))
		}
		world.scan(observed, true)
		world.clock.Advance(time.Minute)
		all := slices.Concat([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
			RootPath: world.rootPath, FilesystemDevice: 7, IsMountPoint: true,
		}}}, listed)
		ctx, tally := querytally.New(t.Context())
		taken, err := world.ingester.TakeListing(ctx, world.nodeID, world.rootPath, wholeRoot, all, testRules)
		if err != nil || len(taken.ToExamine) != 0 {
			t.Fatalf("listing %d files as stored: %v to read, %v", size, examined(taken), err)
		}
		for path, file := range world.liveFilesByPath() {
			if file.LastSeenAt != world.clock.Now().UnixMilli() {
				t.Errorf("%s was last seen at %d, want now: the listing saw it", path, file.LastSeenAt)
			}
		}
		statements, tallies = append(statements, tally.Statements()), append(tallies, tally.String())
	}
	if statements[0] != statements[1] {
		t.Errorf("a listing of 3 files as stored ran %d statements, of 6 ran %d; want the same:\n%s\n%s",
			statements[0], statements[1], tallies[0], tallies[1])
	}
}

func TestTakeListing_AListingThatWasStoppedEndsUnreadable(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon.mkv", "arkon")}, true)
	taken := world.takeListing(nil, true)
	if !taken.Complete.IsStoppedByErrors || taken.Complete.RootPath != root {
		t.Errorf("a stopped listing ends %+v, want stopped", taken.Complete)
	}
}
