// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"path/filepath"
	"testing"
)

// The node keeps 5 GiB of its prepared directory's disk free, whatever its 95% says: what else writes
// to the disk could fill a smaller floor before the node can act (D542; the owner, 2026-11-23).
func TestWriteConfigs_TheNodeKeepsFiveGiBOfItsDiskFree(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	installation := Installation{Route: BySynology, ComposeVersion: 2}
	if err := WriteConfigs(dir, filledValues(t, Staging), installation); err != nil {
		t.Fatal(err)
	}
	node := decodeTOML(t, filepath.Join(dir, nodeConfig))["node"]
	if node["prepared_disk_min_free_mib"] != int64(5120) || node["prepared_disk_used_limit_percent"] != int64(95) {
		t.Errorf("the node's prepared_disk_min_free_mib is %v and prepared_disk_used_limit_percent %v; want 5120 "+
			"(5 GiB) and 95", node["prepared_disk_min_free_mib"], node["prepared_disk_used_limit_percent"])
	}
}
