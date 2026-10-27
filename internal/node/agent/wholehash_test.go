// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// D363 §2, D116: the node reads a file the core named, checked to be the one it named, whole, and
// answers the SHA-256 of its every byte with its size and modified time; a file changed since is
// refused.
func TestAgent_ReadsAFileWholeForTheHashOfItsEveryByte(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}})
	const contents = "every byte of a film"
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	ask := func(job int64, payloadHash string) nodeproto.WholeHash {
		t.Helper()
		core.commands <- nodeproto.Command{
			CommandSeq: job, JobID: job,
			Work: nodeproto.Work{Type: nodeproto.CommandHashWhole, HashWhole: &nodeproto.HashWhole{
				File: nodeproto.BytesFile{RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: payloadHash},
			}},
		}
		return *testutil.Receive(t, core.wholeHashes, "the whole hash").WholeHash
	}
	sum := sha256.Sum256([]byte(contents))
	hashed := ask(1, nodetest.PayloadHashHex(t, contents))
	if hashed.SHA256 != hex.EncodeToString(sum[:]) || hashed.SizeBytes != int64(len(contents)) ||
		hashed.ModifiedAt == 0 || hashed.Refusal != "" || hashed.Failure != "" {
		t.Errorf("the whole hash = %+v; want the SHA-256 of its bytes, its size and its time", hashed)
	}
	if changed := ask(2, nodetest.PayloadHashHex(t, "another film")); changed.Refusal == "" || changed.SHA256 != "" {
		t.Errorf("a file not the one named: %+v; want it refused, unread", changed)
	}
}
