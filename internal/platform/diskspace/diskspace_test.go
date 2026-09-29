// SPDX-License-Identifier: GPL-3.0-or-later

package diskspace

import (
	"path/filepath"
	"testing"
)

func TestMeasure_SaysTheSizeAndFreeSpaceOfTheFilesystemAtAPath(t *testing.T) {
	t.Parallel()
	space, err := Measure(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if space.TotalBytes <= 0 || space.AvailableBytes < 0 || space.AvailableBytes > space.TotalBytes {
		t.Errorf("the temporary directory's filesystem is %+v; want a size and free space within it", space)
	}
	if percent := (Space{TotalBytes: 200, AvailableBytes: 50}).UsedPercent(); percent != 75 {
		t.Errorf("200 bytes with 50 free is %d%% used, want 75", percent)
	}
	if _, err := Measure(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("a path that does not exist was measured")
	}
}
