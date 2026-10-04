// SPDX-License-Identifier: GPL-3.0-or-later

package instancedir

// refuseNetworkFilesystem does not check on macOS, a development platform only; the server is
// deployed on Linux, where the check is real (netfs_linux.go).
func refuseNetworkFilesystem(string) error {
	return nil
}
