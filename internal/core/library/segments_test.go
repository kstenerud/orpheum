// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// chaptered is a film observed with chapters, each titled as given and a minute long, one after
// another from the start, in a file of an hour.
func chaptered(titles ...string) nodeproto.ScanRecord {
	record := observedRecord("Metropolis.mkv", "metropolis")
	record.Observed.Probe.Format.DurationMs = 3_600_000
	for index, title := range titles {
		start := int64(index) * 60_000
		record.Observed.Probe.Chapters = append(record.Observed.Probe.Chapters,
			nodeproto.ProbeChapter{StartMs: start, EndMs: start + 60_000, ContainerTitle: title})
	}
	return record
}

// segmentRows are the file's segment rows, each type|start|end|provider, in order.
func (world *harness) segmentRows() []string {
	world.t.Helper()
	return storetest.QueryRaw(world.t, world.database.Path(),
		"SELECT type, start_ms, end_ms, provider FROM media_segment ORDER BY start_ms, provider")
}

// D82, plans/media-segments.md: a chapter whose whole title, trimmed and in any case, names an intro, a
// recap, a preview or the credits is a segment of that type, found by the chapter provider; any other
// chapter is none, a title that only holds one of the words too.
func TestChapterSegments_AChapterTitledAsASegmentTypeIsOne(t *testing.T) {
	t.Parallel()
	for title, want := range map[string]SegmentType{
		"Intro": SegmentIntro, "OPENING": SegmentIntro, "OP": SegmentIntro, "Opening credits": SegmentIntro,
		"opening THEME": SegmentIntro, "Title sequence": SegmentIntro,
		"Recap": SegmentRecap, "PREVIOUSLY": SegmentRecap, "Previously on": SegmentRecap,
		"Preview": SegmentPreview, "NEXT EPISODE": SegmentPreview, "Next time": SegmentPreview,
		"Credits": SegmentCredits, "ENDING": SegmentCredits, "ED": SegmentCredits, "End credits": SegmentCredits,
		"CLOSING CREDITS": SegmentCredits, " Outro\t": SegmentCredits,
	} {
		found := chapterSegments(chaptered("Chapter 1", title).Observed.Probe)
		if wantFound := []Segment{{Type: want, StartMs: 60_000, EndMs: 120_000}}; !slices.Equal(found, wantFound) {
			t.Errorf("a chapter titled %q makes %+v; want %+v", title, found, wantFound)
		}
	}
	others := []string{"", "Chapter 2", "Introduction", "Intro and recap", "Credits roll", "Opening night"}
	for _, title := range others {
		if found := chapterSegments(chaptered(title).Observed.Probe); len(found) != 0 {
			t.Errorf("a chapter titled %q makes %+v; want no segment: chapters stay chapters", title, found)
		}
	}
}

// A chapter that ends where or before it starts, or past the file's length, is no segment, whatever its
// title; a file whose probe gives no length keeps one however late it ends.
func TestChapterSegments_ABadChapterIsSkipped(t *testing.T) {
	t.Parallel()
	probe := chaptered().Observed.Probe
	probe.Chapters = []nodeproto.ProbeChapter{
		{StartMs: 60_000, EndMs: 60_000, ContainerTitle: "Intro"},
		{StartMs: 90_000, EndMs: 30_000, ContainerTitle: "Recap"},
		{StartMs: 3_500_000, EndMs: 3_700_000, ContainerTitle: "Credits"},
		{StartMs: 100_000, EndMs: 160_000, ContainerTitle: "Preview"},
	}
	want := []Segment{{Type: SegmentPreview, StartMs: 100_000, EndMs: 160_000}}
	if found := chapterSegments(probe); !slices.Equal(found, want) {
		t.Errorf("the segments = %+v; want the good chapter's alone, %+v", found, want)
	}
	probe.Format.DurationMs = 0
	if found := chapterSegments(probe); len(found) != 2 || found[1].Type != SegmentCredits {
		t.Errorf("with no length, the segments = %+v; want the preview and the credits", found)
	}
}

// A file's probe kept makes its chapter segments; read again with other chapters, its chapter-made
// segments are replaced and a person's correction stays (D82: it survives re-detection); read again
// unchanged, nothing is written.
func TestIngest_AProbeReadAgainReplacesTheChapterSegmentsAndLeavesAPersons(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{chaptered("Intro", "Chapter 2", "Credits")}, true)
	want := []string{"intro|0|60000|chapters", "credits|120000|180000|chapters"}
	if got := world.segmentRows(); !slices.Equal(got, want) {
		t.Fatalf("the segments = %v; want %v", got, want)
	}
	fileID := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	storetest.ExecRaw(t, world.database.Path(),
		"INSERT INTO media_segment (file_id, type, start_ms, end_ms, provider, confidence) VALUES "+
			"("+itoa(fileID)+", 'intro', 5000, 55000, 'person', 1)")
	ids := storetest.QueryRaw(t, world.database.Path(), "SELECT id FROM media_segment ORDER BY id")

	world.scan([]nodeproto.ScanRecord{chaptered("Intro", "Chapter 2", "Credits")}, true)
	if again := storetest.QueryRaw(t, world.database.Path(), "SELECT id FROM media_segment ORDER BY id"); !slices.Equal(
		again, ids) {
		t.Errorf("read again unchanged, the segment ids are %v; want %v, nothing written", again, ids)
	}

	world.scan([]nodeproto.ScanRecord{chaptered("Recap", "Chapter 2", "Chapter 3", "End credits")}, true)
	want = []string{
		"recap|0|60000|chapters", "intro|5000|55000|person", "credits|180000|240000|chapters",
	}
	if got := world.segmentRows(); !slices.Equal(got, want) {
		t.Errorf("read again with other chapters, the segments = %v; want %v", got, want)
	}

	// Read by its player, the person's intro stands for its type and the detector's others for theirs.
	segments, err := NewBrowser(world.database, nil).SegmentsOfFile(t.Context(), fileID)
	wantRead := []Segment{
		{Type: SegmentRecap, StartMs: 0, EndMs: 60_000, Provider: ProviderChapters},
		{Type: SegmentIntro, StartMs: 5_000, EndMs: 55_000, Provider: ProviderPerson},
		{Type: SegmentCredits, StartMs: 180_000, EndMs: 240_000, Provider: ProviderChapters},
	}
	if err != nil || !slices.Equal(segments, wantRead) {
		t.Errorf("SegmentsOfFile = %+v, %v; want %+v", segments, err, wantRead)
	}
	world.scan([]nodeproto.ScanRecord{chaptered("Intro")}, true)
	segments, err = NewBrowser(world.database, nil).SegmentsOfFile(t.Context(), fileID)
	wantRead = []Segment{{Type: SegmentIntro, StartMs: 5_000, EndMs: 55_000, Provider: ProviderPerson}}
	if err != nil || !slices.Equal(segments, wantRead) {
		t.Errorf("with a chapter intro beside the person's, SegmentsOfFile = %+v, %v; want the person's alone %+v",
			segments, err, wantRead)
	}
}
