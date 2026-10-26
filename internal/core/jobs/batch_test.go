// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// A batch of requests is queued 500 to a write, and every one of them is queued.
func TestEnqueueBatch_QueuesFiveHundredToAWrite(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	batch := make([]Request, 0, 1200)
	for itemID := int64(1); itemID <= 1200; itemID++ {
		batch = append(batch, Request{Kind: KindFetchMetadata, Target: Target{ItemID: itemID}, Reason: "test"})
	}
	ctx, tally := querytally.New(t.Context())
	if queued, err := fixed.queue.EnqueueBatch(ctx, batch); err != nil || queued != 1200 || tally.Writes() != 3 {
		t.Errorf("1200 requests queued %d (%v) in %d writes; want all of them, in 3", queued, err, tally.Writes())
	}
}
