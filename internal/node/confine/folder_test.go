// SPDX-License-Identifier: AGPL-3.0-or-later

package confine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
)

// folderTree makes, under a configured root, films/Wing, films/.cache, a file, a link to a folder
// inside the root and one to a folder outside it, and returns the root and the outside folder.
func folderTree(t *testing.T) (*confine.MediaRoot, string) {
	t.Helper()
	directory, outside := t.TempDir(), t.TempDir()
	for _, folder := range []string{"films/Wing", "films/.cache", "shows/Dark"} {
		if err := os.MkdirAll(filepath.Join(directory, folder), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "films", "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "shows"), filepath.Join(directory, "films", "inside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "films", "outside")); err != nil {
		t.Fatal(err)
	}
	return nodetest.OpenMediaRoot(t, directory), outside
}

func TestMediaRoot_SubfoldersAreTheRealFoldersButTheHiddenOnes(t *testing.T) {
	t.Parallel()
	media, _ := folderTree(t)
	names, err := media.Subfolders("films")
	if err != nil || !slices.Equal(names, []string{"Wing"}) {
		t.Errorf("films' subfolders: %v, %v; want only Wing: no file, no link, no hidden folder (D359 §2)",
			names, err)
	}
	names, err = media.Subfolders(".")
	if err != nil || !slices.Equal(names, []string{"films", "shows"}) {
		t.Errorf("the root's subfolders: %v, %v; want films and shows", names, err)
	}
	for _, through := range []string{"films/inside", "films/outside", "films/notes.txt", "../", "films/../films"} {
		if _, err := media.Subfolders(through); !errors.Is(err, confine.ErrNotARealFolder) {
			t.Errorf("subfolders of %s: %v, want ErrNotARealFolder", through, err)
		}
	}
	if _, err := media.Subfolders("films/gone"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a folder not there: %v, want os.ErrNotExist", err)
	}
}

func TestMediaRoot_AFolderIsARootOfItsOwnReachedWithoutALink(t *testing.T) {
	t.Parallel()
	media, _ := folderTree(t)
	folder, err := media.Folder("films/Wing")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = folder.Close() }) //nolint:errcheck // read-only
	if want := filepath.Join(media.Path(), "films", "Wing"); folder.Path() != want ||
		folder.ConfiguredPath() != media.Path() {
		t.Errorf("the folder is at %s in %s; want %s in %s", folder.Path(), folder.ConfiguredPath(), want,
			media.Path())
	}
	if _, err := folder.Open("../notes.txt"); err == nil {
		t.Error("the folder opened a file outside itself")
	}
	for _, through := range []string{"films/inside", "films/outside", "films/notes.txt", "..", "/etc"} {
		if _, err := media.Folder(through); !errors.Is(err, confine.ErrNotARealFolder) {
			t.Errorf("folder %s: %v, want ErrNotARealFolder (D359 §1)", through, err)
		}
	}
}

func TestHolding_IsTheConfiguredRootAFolderIsAtOrUnderByAPathThatCannotClimbOut(t *testing.T) {
	t.Parallel()
	media, outside := folderTree(t)
	roots := map[string]*confine.MediaRoot{media.Path(): media}
	for _, each := range []struct {
		path, relative string
		isHeld         bool
	}{
		{media.Path(), ".", true},
		{media.Path() + "/films/Wing", "films/Wing", true},
		{media.Path() + "/films/../../etc", "", false},
		{media.Path() + "/films/", "", false},
		{media.Path() + "X", "", false},
		{outside, "", false},
	} {
		root, relative, isHeld := confine.Holding(roots, each.path)
		if isHeld != each.isHeld || relative != each.relative || (isHeld && root != media) {
			t.Errorf("Holding(%s) = %v, %q, %v; want %q, %v", each.path, root, relative, isHeld, each.relative,
				each.isHeld)
		}
	}
}
