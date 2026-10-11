// SPDX-License-Identifier: AGPL-3.0-or-later

package diskspace

import (
	"os"
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

func TestTreeBytes_CountsOnlyTheRegularFilesUnderADirectory(t *testing.T) {
	t.Parallel()
	root, elsewhere := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "video"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", "seg00000.m4s"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.m3u8"), []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(elsewhere, "large.mkv")
	if err := os.WriteFile(large, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(large, filepath.Join(root, "video", "link.mkv")); err != nil {
		t.Fatal(err)
	}
	total, err := TreeBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	if total != 8 {
		t.Errorf("%d bytes; want 8, the two regular files, and nothing for the link or the directories", total)
	}
	if _, err := TreeBytes(filepath.Join(root, "gone")); err == nil {
		t.Error("a directory that does not exist was measured")
	}
}
