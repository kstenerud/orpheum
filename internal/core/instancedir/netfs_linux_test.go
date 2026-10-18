// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"strings"
	"testing"
)

func TestRefuseNetworkFilesystemType_RefusesNetworkFilesystems(t *testing.T) {
	t.Parallel()
	const nfsMagic, ext4Magic = 0x6969, 0xEF53
	err := refuseNetworkFilesystemType("/srv/orpheum/persistent", nfsMagic)
	// The refusal names the directory as the instance's top-level config does (D242 §3).
	if err == nil || !strings.Contains(err.Error(), "NFS") || !strings.Contains(err.Error(), "persistent directory") {
		t.Errorf("NFS: error = %v, want a refusal naming NFS and the persistent directory", err)
	}
	// A Windows drive under WSL, or a virtual machine's shared folder.
	const ninePMagic = 0x01021997
	if err := refuseNetworkFilesystemType("/mnt/c/orpheum/persistent", ninePMagic); err == nil ||
		!strings.Contains(err.Error(), "9P") {
		t.Errorf("9P: error = %v, want a refusal naming 9P", err)
	}
	if err := refuseNetworkFilesystemType("/srv/orpheum/persistent", ext4Magic); err != nil {
		t.Errorf("ext4: error = %v, want nil", err)
	}
	if err := refuseNetworkFilesystem(t.TempDir()); err != nil {
		t.Errorf("the test's own temporary directory: %v", err)
	}
}
