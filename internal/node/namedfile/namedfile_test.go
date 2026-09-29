// SPDX-License-Identifier: GPL-3.0-or-later

package namedfile_test

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// openRoot is a media root over directory.
func openRoot(t *testing.T, directory string) *confine.MediaRoot {
	t.Helper()
	root, err := confine.OpenMediaRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // read-only
	return root
}

// payloadHashOf is the payload hash of contents.
func payloadHashOf(t *testing.T, contents string) []byte {
	t.Helper()
	identity, err := hash.PayloadIdentity(strings.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	return identity.PayloadHash[:]
}

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
	root := openRoot(t, directory)
	file, info, err := namedfile.Open(root, "film.mkv", payloadHashOf(t, "a film"))
	if err != nil || info.Size() != 6 {
		t.Fatalf("the regular file: %v, %v", info, err)
	}
	_ = file.Close() //nolint:errcheck // read-only
	for _, name := range []string{"Extras", "."} {
		if _, _, err := namedfile.Open(root, name, payloadHashOf(t, "")); !errors.Is(err, namedfile.ErrNotRegular) {
			t.Errorf("the directory %q: %v, want ErrNotRegular", name, err)
		}
	}
	// A pipe opened for reading waits for a writer; it is opened without waiting and refused.
	done := make(chan error, 1)
	go func() {
		_, _, err := namedfile.Open(root, "pipe.mkv", payloadHashOf(t, ""))
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
	root, hashes, named := openRoot(t, directory), namedfile.NewHashes(), payloadHashOf(t, "a film")
	_, _, err := namedfile.Open(root, "film.mkv", payloadHashOf(t, "another"))
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
	root := openRoot(t, directory)
	roots := map[string]*confine.MediaRoot{root.Path(): root}
	payloadHash := hex.EncodeToString(payloadHashOf(t, "a film"))
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
