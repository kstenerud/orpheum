// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"os"
)

// partsEnd ends the name of the folder beside a payload that holds each signing attempt's own copy of
// it, so a signature is never mistaken for an older one's.
const partsEnd = ".parts"

// removeParts removes the attempts' copies of the payload at path once its signatures are written
// beside it: they hold the payload again and spent signatures, and nothing reads them after.
func removeParts(path string) error {
	if err := os.RemoveAll(path + partsEnd); err != nil {
		return fmt.Errorf("remove the signing attempts' copies: %w", err)
	}
	return nil
}
