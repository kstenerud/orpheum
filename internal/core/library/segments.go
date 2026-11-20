// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SegmentType is what a media segment is, as media_segment.type holds it (D82, D159;
// plans/media-segments.md): what a player does with it follows from it.
type SegmentType string

// The types of segment.
const (
	SegmentIntro      SegmentType = "intro"
	SegmentRecap      SegmentType = "recap"
	SegmentPreview    SegmentType = "preview"
	SegmentCredits    SegmentType = "credits"
	SegmentCommercial SegmentType = "commercial"
	// SegmentUnknown is a range marked but of no type known: shown, never acted on.
	SegmentUnknown SegmentType = "unknown"
	// SegmentEpisodeBoundary is where one episode ends and the next begins inside one file (D159).
	SegmentEpisodeBoundary SegmentType = "episode_boundary"
)

// SegmentProvider is what found a segment, as media_segment.provider holds it.
type SegmentProvider string

// The providers of segments.
const (
	// ProviderChapters found it as a chapter of the file whose title names it.
	ProviderChapters SegmentProvider = "chapters"
	// ProviderFingerprint is audio found repeated across a season's episodes (D115); not built yet.
	ProviderFingerprint SegmentProvider = "fingerprint"
	// ProviderPerson is a person's correction, which beats every detector and survives re-detection.
	ProviderPerson SegmentProvider = "person"
)

// chapterConfidence is how sure a segment found from a chapter's title is: the file's author named it.
const chapterConfidence = 1.0

// chapterTitleType is the segment a chapter's title names, given lowercased and trimmed, as its whole
// title: any other chapter is a chapter and nothing more. "op" and "ed" are anime's opening and ending.
func chapterTitleType(title string) (SegmentType, bool) {
	switch title {
	case "intro", "opening", "op", "opening credits", "opening theme", "title sequence":
		return SegmentIntro, true
	case "recap", "previously", "previously on":
		return SegmentRecap, true
	case "preview", "next episode", "next time":
		return SegmentPreview, true
	case "credits", "ending", "ed", "end credits", "closing credits", "outro":
		return SegmentCredits, true
	}
	return "", false
}

// Segment is one typed range of a file's timeline, and what found it.
type Segment struct {
	Type     SegmentType     `json:"type"`
	StartMs  int64           `json:"startMs"`
	EndMs    int64           `json:"endMs"`
	Provider SegmentProvider `json:"-"`
}

// chapterSegments are the segments a probe's chapters name, in order, each once: a chapter whose title,
// trimmed, is one chapterTitleType names, whatever its case. A chapter that ends where or before it
// starts, or past the file's length where the probe gives one, marks nothing it can be trusted for.
func chapterSegments(probe nodeproto.Probe) []Segment {
	found := []Segment{}
	for _, chapter := range probe.Chapters {
		segmentType, isNamed := chapterTitleType(strings.ToLower(strings.TrimSpace(chapter.ContainerTitle)))
		if !isNamed || chapter.StartMs < 0 || chapter.EndMs <= chapter.StartMs {
			continue
		}
		if length := probe.Format.DurationMs; length > 0 && chapter.EndMs > length {
			continue
		}
		found = append(found, Segment{Type: segmentType, StartMs: chapter.StartMs, EndMs: chapter.EndMs})
	}
	slices.SortFunc(found, func(one, other Segment) int {
		return cmp.Or(cmp.Compare(one.StartMs, other.StartMs), cmp.Compare(one.EndMs, other.EndMs),
			strings.Compare(string(one.Type), string(other.Type)))
	})
	return slices.Compact(found)
}

// syncChapterSegments makes a file's chapter-named segments those its probe's chapters name: the chapter
// provider's rows it no longer names go and those it newly names come, and no other provider's row is
// touched, a person's above all. A file read again unchanged writes nothing.
func (batch *ingestBatch) syncChapterSegments(ctx context.Context, fileID int64, probe nodeproto.Probe) error {
	encoded, err := json.Marshal(chapterSegments(probe))
	if err != nil {
		return fmt.Errorf("encode the chapter segments of file %d: %w", fileID, err)
	}
	err = batch.queries.DeleteDetectedSegmentsNotIn(ctx, sqlcgen.DeleteDetectedSegmentsNotInParams{
		FileID: fileID, Provider: string(ProviderChapters), Found: string(encoded),
	})
	if err == nil {
		err = batch.queries.InsertDetectedSegmentsMissing(ctx, sqlcgen.InsertDetectedSegmentsMissingParams{
			FileID: fileID, Provider: string(ProviderChapters), Confidence: chapterConfidence, Found: string(encoded),
		})
	}
	if err != nil {
		return fmt.Errorf("keep the chapter segments of file %d: %w", fileID, err)
	}
	return nil
}

// SegmentsOfFile is a file's segments as its player takes them, in order: of each type, a person's
// where they set any, else every detector's. The caller has checked the person reaches the file.
func (browser *Browser) SegmentsOfFile(ctx context.Context, fileID int64) ([]Segment, error) {
	rows, err := browser.database.Read().ListSegmentsOfFile(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("read the segments of file %d: %w", fileID, err)
	}
	segments := make([]Segment, 0, len(rows))
	for _, row := range rows {
		segments = append(segments, Segment{
			Type: SegmentType(row.Type), StartMs: row.StartMs, EndMs: row.EndMs,
			Provider: SegmentProvider(row.Provider),
		})
	}
	return segments, nil
}
