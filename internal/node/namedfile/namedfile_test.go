// SPDX-License-Identifier: AGPL-3.0-or-later

package namedfile_test

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestOpen_RefusesWhatIsNotARegularFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "Extras"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(directory, "pipe.mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := nodetest.OpenMediaRoot(t, directory)
	file, info, err := namedfile.Open(root, "film.mkv", nodetest.PayloadHash(t, "a film"))
	if err != nil || info.Size() != 6 {
		t.Fatalf("the regular file: %v, %v", info, err)
	}
	_ = file.Close() //nolint:errcheck // read-only
	for _, name := range []string{"Extras", "."} {
		_, _, err := namedfile.Open(root, name, nodetest.PayloadHash(t, ""))
		if !errors.Is(err, namedfile.ErrNotRegular) {
			t.Errorf("the directory %q: %v, want ErrNotRegular", name, err)
		}
	}
	// A pipe opened for reading waits for a writer; it is opened without waiting and refused.
	done := make(chan error, 1)
	go func() {
		_, _, err := namedfile.Open(root, "pipe.mkv", nodetest.PayloadHash(t, ""))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, namedfile.ErrNotRegular) {
			t.Errorf("the pipe: %v, want ErrNotRegular", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("opening a pipe waited for a writer; want it refused at once")
	}
}

func TestOpen_RefusesAFileThatIsNoLongerTheOneNamed(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	full := filepath.Join(directory, "film.mkv")
	if err := os.WriteFile(full, []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, hashes := nodetest.OpenMediaRoot(t, directory), namedfile.NewHashes()
	named := nodetest.PayloadHash(t, "a film")
	_, _, err := namedfile.Open(root, "film.mkv", nodetest.PayloadHash(t, "another"))
	if !errors.Is(err, namedfile.ErrChanged) {
		t.Errorf("naming another hash: %v, want ErrChanged", err)
	}
	file, _, err := hashes.Open(root, "film.mkv", named)
	if err != nil {
		t.Fatalf("the file named: %v", err)
	}
	_ = file.Close() //nolint:errcheck // read-only
	// Replaced in place after its payload hash was remembered: the new size makes it hashed again.
	if err := os.WriteFile(full, []byte("a different film"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0) // any time but the one it was written at
	if err := os.Chtimes(full, later, later); err != nil {
		t.Fatal(err)
	}
	if _, _, err := hashes.Open(root, "film.mkv", named); !errors.Is(err, namedfile.ErrChanged) {
		t.Errorf("the file replaced after its hash was remembered: %v, want ErrChanged", err)
	}
	if _, _, err := namedfile.Open(root, "gone.mkv", named); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v, want os.ErrNotExist", err)
	}
}

func TestOpenNamed_FindsTheRootByItsPathAndRefusesABadName(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := nodetest.OpenMediaRoot(t, directory)
	roots := map[string]*confine.MediaRoot{root.Path(): root}
	payloadHash := hex.EncodeToString(nodetest.PayloadHash(t, "a film"))
	named := nodeproto.BytesFile{RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: payloadHash}
	file, _, err := namedfile.OpenNamed(roots, named)
	if err != nil {
		t.Fatalf("the file named: %v", err)
	}
	_ = file.Close() //nolint:errcheck // read-only
	elsewhere := named
	elsewhere.RootPath = "/elsewhere"
	if _, _, err = namedfile.OpenNamed(roots, elsewhere); !errors.Is(err, namedfile.ErrNoRoot) {
		t.Errorf("under a root the node was not given: %v, want ErrNoRoot", err)
	}
	for _, bad := range []nodeproto.BytesFile{
		{RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: "not hex"},
		{RootPath: root.Path(), Path: []byte("film.mkv")},
		{RootPath: root.Path(), PayloadHash: payloadHash},
	} {
		if _, _, err = namedfile.NewHashes().OpenNamed(roots, bad); !errors.Is(err, namedfile.ErrBadName) {
			t.Errorf("%+v: %v, want ErrBadName", bad, err)
		}
	}
}

// A command names a file under one of the node's roots, and nothing it names reaches past that
// root, or past the library folder it names inside one: not by climbing, not by an absolute path,
// not through a link that leaves the root, nor one followed by a climb. Each place outside holds a
// file whose payload hash is the one named, so only where it is refuses it.
func TestOpenNamed_NothingANameCarriesReachesPastItsRootOrFolder(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	directory, outside := filepath.Join(parent, "media"), filepath.Join(parent, "elsewhere")
	for _, folder := range []string{filepath.Join(directory, "films"), filepath.Join(outside, "deep", "er")} {
		if err := os.MkdirAll(folder, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(directory, "film.mkv"), filepath.Join(outside, "secret.mkv"),
		filepath.Join(outside, "deep", "secret.mkv"),
	} {
		if err := os.WriteFile(path, []byte("a film"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"escape": filepath.Join(outside, "secret.mkv"), "away": filepath.Join(outside, "deep", "er"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	root := nodetest.OpenMediaRoot(t, directory)
	roots := map[string]*confine.MediaRoot{root.Path(): root}
	payloadHash := nodetest.PayloadHashHex(t, "a film")
	named := func(rootPath, path string) nodeproto.BytesFile {
		return nodeproto.BytesFile{RootPath: rootPath, Path: []byte(path), PayloadHash: payloadHash}
	}
	file, _, err := namedfile.OpenNamed(roots, named(root.Path(), "film.mkv"))
	if err != nil {
		t.Fatalf("the file inside the root: %v", err)
	}
	_ = file.Close() //nolint:errcheck // read-only

	films := filepath.Join(root.Path(), "films")
	for name, outsider := range map[string]nodeproto.BytesFile{
		"a climb out of the root":                  named(root.Path(), "../elsewhere/secret.mkv"),
		"a climb from inside":                      named(root.Path(), "films/../../elsewhere/secret.mkv"),
		"an absolute path":                         named(root.Path(), filepath.Join(outside, "secret.mkv")),
		"a link out of the root":                   named(root.Path(), "escape"),
		"a climb after a link out of the root":     named(root.Path(), "away/../secret.mkv"),
		"a root path that climbs out":              named(root.Path()+"/..", "elsewhere/secret.mkv"),
		"a root path through the link":             named(filepath.Join(root.Path(), "away"), "../secret.mkv"),
		"a climb out of a library folder":          named(films, "../film.mkv"),
		"an absolute path from a library folder":   named(films, filepath.Join(outside, "secret.mkv")),
		"a library folder's climb out of the root": named(films, "../../elsewhere/secret.mkv"),
	} {
		if file, _, err := namedfile.NewHashes().OpenNamed(roots, outsider); err == nil {
			_ = file.Close() //nolint:errcheck // read-only
			t.Errorf("%s (%s under %s): opened, want it refused", name, outsider.Path, outsider.RootPath)
		}
	}
}
