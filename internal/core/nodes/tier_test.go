// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A read of a video's index alone is fast work, milliseconds, which bumps a read through and does
// not wait for a scan's quick pass; a read through is slow work, which does (D255, D269 §5).
func TestTierOf_AReadOfAVideosIndexAloneIsFastWork(t *testing.T) {
	t.Parallel()
	indexOnly := nodeproto.Work{ReadThrough: &nodeproto.ReadThrough{
		VideoTimeline: &nodeproto.VideoTimelineAsk{IsFromIndexOnly: true},
	}}
	whole := nodeproto.Work{ReadThrough: &nodeproto.ReadThrough{VideoTimeline: &nodeproto.VideoTimelineAsk{}}}
	if got := tierOf(indexOnly, jobs.PriorityIndexFirst); got != tierFast || readsWholeFiles(indexOnly) {
		t.Errorf("a read of the index alone is tier %d, reading whole files %t; want fast, not", got,
			readsWholeFiles(indexOnly))
	}
	if got := tierOf(whole, jobs.PriorityBackground); got != tierSlow || !readsWholeFiles(whole) {
		t.Errorf("a read through is tier %d, reading whole files %t; want slow, reading them", got,
			readsWholeFiles(whole))
	}
}
