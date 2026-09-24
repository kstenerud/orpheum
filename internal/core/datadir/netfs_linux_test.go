// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"strings"
	"testing"
)

func TestCheckFilesystemType_RefusesNetworkFilesystems(t *testing.T) {
	t.Parallel()
	const nfsMagic, ext4Magic = 0x6969, 0xEF53
	err := checkFilesystemType("/srv/state", nfsMagic)
	if err == nil || !strings.Contains(err.Error(), "NFS") {
		t.Errorf("NFS: error = %v, want a refusal naming NFS", err)
	}
	if err := checkFilesystemType("/srv/state", ext4Magic); err != nil {
		t.Errorf("ext4: error = %v, want nil", err)
	}
	if err := refuseNetworkFilesystem(t.TempDir()); err != nil {
		t.Errorf("the test's own temporary directory: %v", err)
	}
}
