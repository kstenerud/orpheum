// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
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

// examined names each file taken to be read, as "kind path".
func examined(taken TakenListing) []string {
	var named []string
	for _, file := range taken.ToExamine {
		named = append(named, string(file.Kind)+" "+file.Path.Text)
	}
	return named
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
		listedDirectory("Wing (1979)"),
		listedFile("Wing (1979)/Wing (1979).en.srt", 2, 1),
		listedFile("Wing (1979)/Wing (1979).mkv", 5, 1),
		listedFile("Wing (1979)/Wing (1979).nfo", 8, 1),
		listedFile("Wing (1979)/sample.mkv", 6, 1),
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
		"subtitle Wing (1979)/Wing (1979).en.srt", "video Wing (1979)/Wing (1979).mkv",
		"nfo Wing (1979)/Wing (1979).nfo", "video Metropolis (1995)/Metropolis.mkv", "video Shows/.trash/kept.mkv",
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
		".trash": "trash", "Wing (1979)/sample.mkv": "sample", "Metropolis (1995)/._Metropolis.mkv": "appledouble",
		"Metropolis (1995)/Sample": "sample", "Metropolis (1995)/metropolis-SAMPLE.mp4": "sample", "Metropolis (1995)/notes.txt": "extension",
		"pipe.mkv": "not_regular",
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
	subtitle := sidecarRecord("Wing/Wing.en.srt", nodeproto.SidecarSubtitle, "1\n")
	subtitle.Sidecar.Identity = &nodeproto.Identity{
		Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf("1\n"), PayloadBytes: 2,
	}
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Wing/Wing.mkv", "alien"), observedRecord("Metropolis.mkv", "heat"),
		sidecarRecord("Wing/Wing.nfo", nodeproto.SidecarNFO, "<movie/>"), subtitle,
		errorRecord("Broken.mkv", nodeproto.ScopeEntry),
	}, true)
	before := world.liveFilesByPath()
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedFile("Wing/Wing.en.srt", 2, 1),
		listedFile("Wing/Wing.mkv", int64(len("alien")), 1),
		listedFile("Wing/Wing.nfo", int64(len("<movie/>")), 1),
		listedFile("Broken.mkv", 6, 1),
		listedFile("Metropolis.mkv", int64(len("heat")), 2),
	}, false)
	if got, want := examined(taken), []string{"video Broken.mkv", "video Metropolis.mkv"}; !slices.Equal(got, want) {
		t.Errorf("to read %v, want %v: only what changed, or could not be read", got, want)
	}
	after := world.liveFilesByPath()
	for _, name := range []string{"Wing/Wing.mkv", "Wing/Wing.nfo", "Wing/Wing.en.srt"} {
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
	world.scan([]nodeproto.ScanRecord{observedRecord("Wing.mkv", "alien")}, true)
	taken := world.takeListing(nil, true)
	if !taken.Complete.IsStoppedByErrors || taken.Complete.RootPath != root {
		t.Errorf("a stopped listing ends %+v, want stopped", taken.Complete)
	}
}
