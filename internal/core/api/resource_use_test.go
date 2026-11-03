// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/selfuse"
)

// readOwnUse is what the harness's core reads of its own use: a tenth of a second of CPU more at
// each reading, so a minute apart it used a sixth of a percent of one core.
func readOwnUse() func() (selfuse.Sample, error) {
	var cpuMicros int64
	return func() (selfuse.Sample, error) {
		cpuMicros += 100_000
		return selfuse.Sample{Scope: selfuse.ScopeGroup, CPUMicros: cpuMicros, HeldBytes: 1 << 20}, nil
	}
}

// An admin sees what the core and each node used of their machines, minute by minute; anyone else
// is refused.
func TestResourceUse_AnAdminSeesWhatTheCoreAndEachNodeUsed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.nodes.selfUses = []corenodes.NodeSelfUse{{NodeID: 1, Name: "local", SentAt: 1, Use: nodeproto.SelfUse{
		Scope: "cgroup", HeldBytes: 2 << 20,
	}}}
	recorder := harness.resourceUse
	if err := recorder.Record(t.Context()); err != nil {
		t.Fatal(err)
	}
	harness.clock.Advance(time.Minute)
	if err := recorder.Record(t.Context()); err != nil {
		t.Fatal(err)
	}
	anonymous := harness.do(t, http.MethodGet, "/system/resource-use", "", nil)
	if anonymous.status != http.StatusUnauthorized {
		t.Errorf("signed out, GET /system/resource-use = %d; want 401", anonymous.status)
	}
	admin := harness.createAdminAndSignIn(t)
	got := harness.do(t, http.MethodGet, "/system/resource-use", admin, nil)
	var use api.ResourceUseOutput
	if err := json.Unmarshal([]byte(got.body), &use.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /system/resource-use = %d %s", got.status, got.body)
	}
	checkCoreAndLocal(t, use)
}

// checkCoreAndLocal checks the core's figures, with its CPU time, and local's one reading, without.
func checkCoreAndLocal(t *testing.T, use api.ResourceUseOutput) {
	t.Helper()
	if len(use.Body.Summaries) != 2 {
		t.Fatalf("summaries = %+v; want the core's and local's", use.Body.Summaries)
	}
	core, node := use.Body.Summaries[0], use.Body.Summaries[1]
	if core.IsNode || core.CPUPercent == nil || *core.CPUPercent <= 0 || core.HeldBytes != 1<<20 {
		t.Errorf("the core's summary = %+v; want its CPU time and the memory it held", core)
	}
	if !node.IsNode || node.NodeName != "local" || node.CPUPercent != nil || node.HeldBytes != 2<<20 {
		t.Errorf("local's summary = %+v; want its one reading, with no CPU time", node)
	}
	if len(use.Body.Hours) != 2 {
		t.Errorf("hours = %+v; want the core's and local's", use.Body.Hours)
	}
}
