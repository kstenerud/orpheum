// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A rescan that discards what was made throws out everything made of a file it reads again, though
// its probe is the one kept: every picture's and sound's chunks, each sound's loudness, the picture's
// timeline, its frame stills and each subtitle's text, each by the one way a difference throws it
// out (D535 §6); the same file read again without discarding throws out nothing
// (TestScans_ASecondFullScanOfAnUnchangedLibraryChangesNothing).
func TestScans_ADiscardingRescanThrowsOutEverythingMadeOfAnUnchangedFile(t *testing.T) {
	t.Parallel()
	bench, dropped, logged := fullScanBench(t)
	bench.scan([]nodeproto.ScanRecord{probedRecord("film.mp4", "film", filmProbe())}, true)
	logged.Reset()
	err := bench.scans.RequestDiscarding(bench.t.Context(), bench.nodeID, root, wholeRoot, readEverythingAgain(),
		jobs.PriorityPersonsRequest)
	if err != nil {
		t.Fatal(err)
	}
	bench.list(listedFile("film.mp4", int64(len("film")), 1))
	bench.examineAs(probedRecord("film.mp4", "film", filmProbe()))
	want := []string{
		"loudness 1", "loudness 2", "picture 0 track 0", "sound 1 track 0", "sound 2 track 1", "stills 0",
		"text 3", "timeline 0",
	}
	if got := slices.Sorted(slices.Values(dropped.dropped)); !slices.Equal(got, want) {
		t.Errorf("a discarding rescan of an unchanged file threw out %v; want %v", got, want)
	}
	if !strings.Contains(logged.String(), discardedFact) {
		t.Errorf("the discarding rescan logged:\n%s\nwant the file's discard said", logged.String())
	}
}
