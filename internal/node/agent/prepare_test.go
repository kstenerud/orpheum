// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakePreparer reports half the source done, then a size that names the input it was given.
type fakePreparer struct{}

func (fakePreparer) Prepare(
	_ context.Context, input string, _ nodeproto.Prepare, progress func(doneMs int64),
) (int64, error) {
	progress(500)
	return int64(len(input)), nil
}

// finalReport is the core's last word from a prepare: done or failed.
func finalReport(t *testing.T, core *fakeCore) nodeproto.PrepareReport {
	t.Helper()
	for {
		select {
		case report := <-core.prepared:
			if report.State != nodeproto.PrepareRunning {
				return report
			}
		case <-t.Context().Done():
			t.Fatal("no prepare report reached the core")
		}
	}
}

func TestAgent_PreparesOnlyTheFileTheCoreNamed(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := running(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := hash.Payload(strings.NewReader("a film"), 6)
	if err != nil {
		t.Fatal(err)
	}
	named := nodeproto.BytesFile{
		Root: root.Path(), Path: []byte("film.mkv"), Hash: hex.EncodeToString(identity.Hash[:]),
	}
	send := func(job int64, source nodeproto.BytesFile) {
		core.commands <- nodeproto.Command{Seq: job, Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			Job: job, Source: source, Profile: nodeproto.ProfileRemux, Dir: "3/remux",
		}}
	}
	send(1, named)
	done := finalReport(t, core)
	wantInput := root.Path() + "/film.mkv"
	if done.Job != 1 || done.State != nodeproto.PrepareDone || done.Bytes != int64(len(wantInput)) {
		t.Errorf("a prepare of the named file ended %+v, want done, run on %s", done, wantInput)
	}
	changed := named
	changed.Hash = strings.Repeat("0", 64)
	send(2, changed)
	if refused := finalReport(t, core); refused.State != nodeproto.PrepareFailed ||
		!strings.Contains(refused.Error, "not the one named") {
		t.Errorf("a prepare of a changed file ended %+v, want refused", refused)
	}
	elsewhere := named
	elsewhere.Root = t.TempDir()
	send(3, elsewhere)
	if refused := finalReport(t, core); refused.State != nodeproto.PrepareFailed ||
		!strings.Contains(refused.Error, "none of this node's roots") {
		t.Errorf("a prepare under another root ended %+v, want refused", refused)
	}
}
