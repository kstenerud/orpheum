// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestSelfUses_AreWhatEachConnectedNodesLastStatusSaidItUses(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 1})
	if uses := harness.service.SelfUses(t.Context()); len(uses) != 0 {
		t.Errorf("a status saying nothing of the node's use gave %+v; want none", uses)
	}
	use := nodeproto.SelfUse{Scope: "cgroup", CPUMicros: 5, HeldBytes: 6, CacheBytes: 7, PeakBytes: 8}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 2, SelfUse: &use})
	uses := harness.service.SelfUses(t.Context())
	if len(uses) != 1 || uses[0].NodeID != reply.NodeID || uses[0].Name != "local" || uses[0].SentAt != 2 ||
		uses[0].Use != use {
		t.Errorf("the nodes' uses = %+v; want local's, as its last status said", uses)
	}
}
