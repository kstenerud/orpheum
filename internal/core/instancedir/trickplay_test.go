// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTrickplaySheets_AreStoredReadListedAndRemovedByTheirSHA256Alone(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	sheet := strings.Repeat("cd", 32)
	if listed, err := directories.ListTrickplaySheets(); err != nil || len(listed) != 0 {
		t.Fatalf("ListTrickplaySheets before any = %v, %v; want none", listed, err)
	}
	if err := directories.WriteTrickplaySheet(sheet, []byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directories.cacheDirectory, "trickplay", sheet+".webp"))
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("the sheet's file = %v, %v; want it at trickplay/<sha256>.webp, mode 0600", info, err)
	}
	if got, err := directories.ReadTrickplaySheet(sheet); err != nil || string(got) != "RIFF" {
		t.Errorf("ReadTrickplaySheet = %q, %v", got, err)
	}
	if listed, err := directories.ListTrickplaySheets(); err != nil || !slices.Equal(listed, []string{sheet}) {
		t.Errorf("ListTrickplaySheets = %v, %v; want %s", listed, err, sheet)
	}
	for _, name := range []string{"../art", strings.Repeat("CD", 32), "cd", strings.Repeat("cd", 32) + ".webp"} {
		if err := directories.WriteTrickplaySheet(name, nil); !errors.Is(err, ErrBadTrickplayName) {
			t.Errorf("WriteTrickplaySheet(%q) = %v, want ErrBadTrickplayName", name, err)
		}
	}
	if err := directories.RemoveTrickplaySheet(sheet); err != nil {
		t.Fatal(err)
	}
	if _, err := directories.ReadTrickplaySheet(sheet); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadTrickplaySheet after its removal = %v, want ErrNotFound", err)
	}
	if err := directories.RemoveTrickplaySheet(sheet); err != nil {
		t.Errorf("removing it again = %v, want nil", err)
	}
}
