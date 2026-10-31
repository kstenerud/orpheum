// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import "golang.org/x/crypto/ssh"

// VerifyForTest is verifyBlocks under keys, each marked as must, in any namespace: ssh-keygen's
// vector was made in one of its own.
func VerifyForTest(file, namespace, payload string, mustVerifyUV bool, keys ...ssh.PublicKey) error {
	held := make([]Key, 0, len(keys))
	for _, key := range keys {
		held = append(held, Key{Public: key, MustVerifyUV: mustVerifyUV})
	}
	_, err := verifyBlocks(file, namespace, []byte(payload), held)
	return err
}
