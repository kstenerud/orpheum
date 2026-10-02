// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakePreparedShelf is a prepared shelf whose last eviction left usage.
type fakePreparedShelf struct{ usage prepared.Usage }

func (kept fakePreparedShelf) Evict() (prepared.Usage, error)    { return kept.usage, nil }
func (kept fakePreparedShelf) LastUsage() (prepared.Usage, bool) { return kept.usage, true }

func (fakePreparedShelf) Inventory() ([]nodeproto.RenditionManifest, error) { return nil, nil }

func (fakePreparedShelf) Reconcile([]string, []string) ([]string, error) { return nil, nil }

// reconcilingShelf holds renditions, which it lists for the hello, and tells reconciled of each
// reconciliation: the renditions listed and those kept.
type reconcilingShelf struct {
	fakePreparedShelf
	renditions []nodeproto.RenditionManifest
	reconciled chan [2][]string
}

func (shelf reconcilingShelf) Inventory() ([]nodeproto.RenditionManifest, error) {
	return shelf.renditions, nil
}

func (shelf reconcilingShelf) Reconcile(listed, kept []string) ([]string, error) {
	shelf.reconciled <- [2][]string{listed, kept}
	return nil, nil
}

// The hello lists the renditions the node holds, by their manifests, and the node evicts those of
// them the core's reply does not keep (D238 §2).
func TestAgent_TheHelloListsTheRenditionsHeldAndThoseTheCoreDoesNotKeepAreEvicted(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.mutex.Lock()
	core.keptRenditionDirs = []string{"1/remux/aaaaaaaaaaaaaaaa"}
	core.mutex.Unlock()
	shelf := reconcilingShelf{
		renditions: []nodeproto.RenditionManifest{
			{Prepare: nodeproto.Prepare{RenditionDir: "1/remux/aaaaaaaaaaaaaaaa"}},
			{Prepare: nodeproto.Prepare{RenditionDir: "2/remux/bbbbbbbbbbbbbbbb"}},
		},
		reconciled: make(chan [2][]string, 4),
	}
	startAgentWithShelf(t, socket, shelf)
	reconciled := testutil.Receive(t, shelf.reconciled, "the first hello's reconciliation")
	core.mutex.Lock()
	hello := core.hellos[0]
	core.mutex.Unlock()
	if len(hello.Renditions) != 2 || hello.Renditions[1].Prepare.RenditionDir != "2/remux/bbbbbbbbbbbbbbbb" {
		t.Errorf("the hello listed %+v, want both renditions the node holds", hello.Renditions)
	}
	if !slices.Equal(reconciled[0], []string{"1/remux/aaaaaaaaaaaaaaaa", "2/remux/bbbbbbbbbbbbbbbb"}) ||
		!slices.Equal(reconciled[1], []string{"1/remux/aaaaaaaaaaaaaaaa"}) {
		t.Errorf("reconciled %v listed against %v kept, want both listed and the core's one kept",
			reconciled[0], reconciled[1])
	}
}
