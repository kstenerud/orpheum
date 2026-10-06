// SPDX-License-Identifier: GPL-3.0-or-later

package storetest_test

import (
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ExecRaw's connection waits for a lock as the store's own do, 15 s: with none, a lock held for an
// instant just after a server stopped failed a test about one run in fifty (2026-10-06). The
// statement fails, and so ExecRaw, unless the connection waits that long: abs of the smallest
// integer overflows.
func TestExecRaw_WaitsForALockAsTheStoresOwnConnectionsDo(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "raw.db")
	storetest.ExecRaw(t, path,
		"SELECT CASE WHEN (SELECT timeout FROM pragma_busy_timeout) >= 15000 THEN 1 "+
			"ELSE abs(-9223372036854775808) END")
}
