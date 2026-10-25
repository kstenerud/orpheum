// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// A cancel of a run of chunks, on a player's request path, reads only its store's jobs, however
// many other jobs are open: a scan's thousands of fetches and read-throughs cost it nothing (audit
// 2026-10-25, C3). It cancels the match, and leaves every other job open.
func TestCancelMatching_ReadsOnlyItsTargetsOpenJobs(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	for itemID := range int64(50) {
		fixed.enqueue(t, itemID+1, 0)
	}
	store := Target{ChunkStoreID: 7}
	request := Request{Kind: KindMakeChunks, Target: store, ArgsJSON: `{"firstChunk":3}`, Reason: "test"}
	if _, err := fixed.queue.Enqueue(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	ctx, tally := querytally.New(t.Context())
	cancelled, err := fixed.queue.CancelMatching(ctx, KindMakeChunks, store, func(argsJSON string) bool {
		return strings.Contains(argsJSON, `"firstChunk":3`)
	})
	if err != nil || cancelled != 1 {
		t.Fatalf("CancelMatching = %d, %v; want the one run from chunk 3", cancelled, err)
	}
	if ran := tally.String(); strings.Contains(ran, "ListEveryOpenJob") || tally.Writes() != 1 {
		t.Errorf("the cancel ran %s in %d writes; want only the store's jobs read, in one write", ran, tally.Writes())
	}
	if open := fixed.job(t, 1); open.State != "queued" {
		t.Errorf("another target's job is %s; want it queued still", open.State)
	}
}
