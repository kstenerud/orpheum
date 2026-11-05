// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// Hellos of one name at once enrol one node: each looks the name up and enrols it in one write, and
// writes take turns, so the second finds the first's node. A live node's name being unique in the
// database (D432 section 6) is never met as a conflict by a hello.
func TestEnrol_HellosOfOneNameAtOnceEnrolOneNode(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	const hellos = 8
	ids := make([]int64, hellos)
	errs := make([]error, hellos)
	var group sync.WaitGroup
	for index := range hellos {
		group.Go(func() {
			node, err := enrol(t.Context(), harness.database, harness.clock, "attic")
			ids[index], errs[index] = node.ID, err
		})
	}
	group.Wait()
	for index := range hellos {
		if errs[index] != nil || ids[index] != ids[0] {
			t.Errorf("hello %d: node %d, %v; want node %d, as every other hello of the name", index, ids[index],
				errs[index], ids[0])
		}
	}
	got := storetest.QueryRaw(t, harness.database.Path(), "SELECT count(*) FROM node WHERE name = 'attic'")
	if len(got) != 1 || got[0] != "1" {
		t.Errorf("nodes called attic = %v; want 1", got)
	}
}
