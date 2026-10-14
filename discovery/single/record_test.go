// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"os"
	"path/filepath"
	"testing"
)

// A write puts a whole new file in the record's place, never rewrites the old one where it lies, so
// a crash mid-write leaves the old record readable, and leaves nothing else beside it.
func TestFileStore_AWriteReplacesTheRecordWithAWholeNewFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store := FileStore{Path: filepath.Join(directory, "discovery.json")}
	set := func(id string) {
		t.Helper()
		if err := store.Update(func(record *Record) error {
			record.Core = &Core{ID: id}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	set("first")
	before, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	set("second")
	after, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("the record was rewritten in place; want a new file renamed over it")
	}
	if after.Mode().Perm() != 0o600 {
		t.Errorf("the record's mode is %v; want 0600", after.Mode().Perm())
	}
	record, err := store.Read()
	if err != nil || record.Core == nil || record.Core.ID != "second" {
		t.Errorf("the record reads %+v, %v; want the second write whole", record.Core, err)
	}
	listed, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed {
		if name := entry.Name(); name != "discovery.json" && name != "discovery.json.lock" {
			t.Errorf("the directory holds %s; want no temporary left", name)
		}
	}
}
