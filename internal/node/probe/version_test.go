// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// D535 §1: each probe records the version that read it, so the core knows a file an older probe read
// and reads it again (§2).
func TestParse_AProbeSaysTheVersionThatReadIt(t *testing.T) {
	t.Parallel()
	record, err := Parse(recordedFFprobeOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != nodeproto.ProbeVersion || record.Version == 0 {
		t.Errorf("the probe's version = %d; want the node's, %d", record.Version, nodeproto.ProbeVersion)
	}
}
