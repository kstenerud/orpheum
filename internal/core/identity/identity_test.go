// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
)

// memoryFiles is a persistent directory in memory.
type memoryFiles map[string][]byte

func (files memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	contents, found := files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files[name] = contents
	return nil
}

func TestLoadOrMake_MakesTheKeyOnceAndKeepsIt(t *testing.T) {
	t.Parallel()
	files := memoryFiles{}
	first, err := LoadOrMake(files, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(files[KeyFileName]), keyFileHeader+"\n") {
		t.Errorf("%s holds %q; want its header first", KeyFileName, files[KeyFileName])
	}
	again, err := LoadOrMake(files, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Public().Equal(again.Public()) {
		t.Error("the second start made another key; want the one kept")
	}
}

func TestSign_NamesItsPurposeSoOneCannotBeUsedForAnother(t *testing.T) {
	t.Parallel()
	key, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := key.Sign("enrol", "https://discovery.example/core/v1", "k7xq2", "n0nce")
	if !proof.Verify(key.Public(), "enrol", "https://discovery.example/core/v1", "k7xq2", "n0nce", signature) {
		t.Error("the signature does not verify for its own purpose")
	}
	if proof.Verify(key.Public(), "session", "https://discovery.example/core/v1", "k7xq2", "n0nce", signature) {
		t.Error("an enrolment's signature verifies as a session's")
	}
}

func TestParseKeyFile_RefusesWhatIsNotAKey(t *testing.T) {
	t.Parallel()
	for _, contents := range []string{
		"", "orpheum-secret-key v1\nAAAA\n", keyFileHeader + "\nnot base64!\n",
		keyFileHeader + "\nAAAA\n",
	} {
		if _, err := ParseKeyFile([]byte(contents)); err == nil {
			t.Errorf("ParseKeyFile(%q) succeeded; want it refused", contents)
		}
	}
	if _, err := LoadOrMake(failingFiles{}, rand.Reader); err == nil || !strings.Contains(err.Error(), "disk") {
		t.Errorf("a read that failed for another reason = %v; want it said, not a new key made", err)
	}
}

// failingFiles fail every read for a reason other than the file's absence.
type failingFiles struct{}

func (failingFiles) ReadPersistentFile(string) ([]byte, error) {
	return nil, errors.New("the disk failed")
}

func (failingFiles) WritePersistentFile(string, []byte) error { return nil }
