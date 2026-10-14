// SPDX-License-Identifier: AGPL-3.0-or-later

package atomics_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/atomics"
)

func openRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // the test is over
	return root, directory
}

// entries are the names in directory.
func entries(t *testing.T, directory string) []string {
	t.Helper()
	listed, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range listed {
		names = append(names, entry.Name())
	}
	return names
}

func TestWriteFile_ReplacesTheFileWholeWithItsMode(t *testing.T) {
	t.Parallel()
	root, directory := openRoot(t)
	for _, contents := range []string{"first", "second, longer"} {
		if err := atomics.WriteFile(root, "kept.json", []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(directory, "kept.json"))
		if err != nil || string(got) != contents {
			t.Errorf("the file holds %q, %v; want %q", got, err, contents)
		}
	}
	info, err := os.Stat(filepath.Join(directory, "kept.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file's mode is %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if names := entries(t, directory); len(names) != 1 {
		t.Errorf("the directory holds %v; want the file alone", names)
	}
}

func TestReplace_ReplacesTheFileWhole(t *testing.T) {
	t.Parallel()
	root, directory := openRoot(t)
	for _, contents := range []string{"a chunk", "the chunk made again"} {
		if err := atomics.Replace(root, "00001.m4s", []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(directory, "00001.m4s"))
	if err != nil || string(got) != "the chunk made again" {
		t.Errorf("the file holds %q, %v; want the last write", got, err)
	}
	if names := entries(t, directory); len(names) != 1 {
		t.Errorf("the directory holds %v; want the file alone", names)
	}
}

func TestWriteStream_AFailedWriteLeavesTheOldFileAndNoTemporary(t *testing.T) {
	t.Parallel()
	root, directory := openRoot(t)
	if err := atomics.WriteFile(root, "kept.json", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	failed := errors.New("the writer failed")
	err := atomics.WriteStream(root, "kept.json", 0o600, func(output io.Writer) error {
		if _, writeErr := output.Write([]byte("half")); writeErr != nil {
			return writeErr
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Errorf("a failing writer = %v; want its error", err)
	}
	got, readErr := os.ReadFile(filepath.Join(directory, "kept.json"))
	if readErr != nil || string(got) != "old" {
		t.Errorf("after a failed write the file holds %q, %v; want the old file", got, readErr)
	}
	if names := entries(t, directory); len(names) != 1 {
		t.Errorf("after a failed write the directory holds %v; want no temporary left", names)
	}
}
