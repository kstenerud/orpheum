// SPDX-License-Identifier: AGPL-3.0-or-later

package atomics_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// This package's atomics.go is the module's internal/platform/atomics/atomics.go, byte for byte: a
// change made there and not here, or here and not there, fails (the owner, 2026-10-14).
func TestAtomics_IsInternalPlatformAtomicsByteForByte(t *testing.T) {
	t.Parallel()
	ours, err := os.ReadFile("atomics.go")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := os.ReadFile(filepath.Join("..", "..", "..", "internal", "platform", "atomics", "atomics.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ours, theirs) {
		t.Error("discovery/internal/atomics/atomics.go differs from internal/platform/atomics/atomics.go; " +
			"make the change in both")
	}
}
