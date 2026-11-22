// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// fetchedAndConverted is a fetch bench whose video has one subtitle fetched and converted by its
// node, and where that text is kept; later conversions are queued and not run.
func fetchedAndConverted(t *testing.T) (*fetchBench, Fetched, sqlcgen.GetSubtitleTextParams) {
	t.Helper()
	bench := newFetchBench(t)
	bench.node.answer = func(nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return nodeproto.SubtitleText{ASS: []byte("[Script Info]"), VTT: []byte("WEBVTT")}
	}
	fetched, err := bench.fetch(t)
	if err != nil {
		t.Fatalf("the fetch: %v", err)
	}
	bench.mustHaveConvertedAtPlayPriority(t, fetched)
	row, err := bench.database.Read().GetFetchedSubtitle(t.Context(), fetched.Track.FetchedSubtitleID)
	if err != nil {
		t.Fatal(err)
	}
	bench.queue.onEnqueue = nil
	return bench, fetched, fetchedKey(row.BodyGzip)
}

// isTextKept says whether the text kept where key says is there.
func (bench *fetchBench) isTextKept(t *testing.T, key sqlcgen.GetSubtitleTextParams) bool {
	t.Helper()
	_, err := store.OrNotFound(bench.database.Read().GetSubtitleText(t.Context(), key))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	return err == nil
}

// D535 §6: a video page's discarding rescan throws out the subtitles fetched for its files and
// their text: they are found again from the player.
func TestDiscardFetched_AVideoPagesThrowsOutTheSubtitlesAndTheirText(t *testing.T) {
	t.Parallel()
	bench, _, key := fetchedAndConverted(t)
	if err := bench.service.DiscardFetched(t.Context(), []int64{bareVideoFileID}, true); err != nil {
		t.Fatal(err)
	}
	if count, isKept := bench.fetchedCount(t), bench.isTextKept(t, key); count != 0 || isKept {
		t.Errorf("after the discard, %d fetched subtitles, text kept %v; want neither", count, isKept)
	}
}

// D535 §6: a library's discarding rescan keeps each fetched subtitle, which cost a download, and
// throws out only its text, converted again from the kept download in the background.
func TestDiscardFetched_ALibrarysKeepsTheSubtitlesAndConvertsThemAgain(t *testing.T) {
	t.Parallel()
	bench, fetched, key := fetchedAndConverted(t)
	if err := bench.service.DiscardFetched(t.Context(), []int64{bareVideoFileID}, false); err != nil {
		t.Fatal(err)
	}
	if count, isKept := bench.fetchedCount(t), bench.isTextKept(t, key); count != 1 || isKept {
		t.Errorf("after the discard, %d fetched subtitles, text kept %v; want the one kept, its text not",
			count, isKept)
	}
	last := bench.queue.requests[len(bench.queue.requests)-1]
	want := jobs.Target{FetchedSubtitleID: fetched.Track.FetchedSubtitleID}
	if last.Kind != jobs.KindConvertFetchedSubtitle || last.Target != want || last.Priority != jobs.PriorityBackground {
		t.Errorf("the last job asked for is %+v; want the subtitle's conversion again, in the background", last)
	}
	if count := bench.subtitler.downloadCount(); count != 1 {
		t.Errorf("downloaded %d times; want once, the kept download converted again", count)
	}
}
