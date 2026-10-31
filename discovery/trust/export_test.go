// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

// CheckForTest checks a signature in any namespace, as ssh-keygen's vector was made in one of its
// own.
func CheckForTest(pinned Pinned, namespace, payload, signature string) error {
	keys, err := parseKeys(pinned.Keys)
	if err != nil {
		return err
	}
	return verifySSH(signature, namespace, []byte(payload), keys)
}
