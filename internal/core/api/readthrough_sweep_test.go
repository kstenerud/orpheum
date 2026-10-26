// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// The read-through sweep queues its files in one write, not one each, and a sweep again over files
// whose read-throughs are queued writes no row: after an import every sweep took cache.db's writer
// once per unread file (the audit of 2026-10-25, S4).
func TestReadThrough_ASweepQueuesInOneWriteAndAgainChangesNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelvePictures(t)
	ctx, tally := querytally.New(t.Context())
	if queued, err := harness.readThrough.QueueReadThroughs(ctx); err != nil || queued != 2 || tally.Writes() != 1 {
		t.Fatalf("the sweep queued %d (%v) in %d writes; want both films, in one", queued, err, tally.Writes())
	}
	ctx, again := querytally.New(t.Context())
	queued, err := harness.readThrough.QueueReadThroughs(ctx)
	if err != nil || queued != 0 || again.Writes() != 1 || strings.Contains(again.String(), "MergeQueuedJob") {
		t.Errorf("the sweep again queued %d (%v) in %d writes, running %s; want none, in one write that "+
			"merges nothing", queued, err, again.Writes(), again)
	}
}
