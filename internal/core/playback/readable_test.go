// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// connectedNodes says every node is connected.
type connectedNodes struct{}

func (connectedNodes) NodeConnection(context.Context, int64) (nodes.Connection, time.Time) {
	return nodes.Connected, time.Time{}
}

// probedRoots is a node whose root answers again at its backAt'th probe, which brings it back with
// bringBack; 0 never.
type probedRoots struct {
	bringBack func()
	backAt    int
	probes    int
}

func (roots *probedRoots) ProbeAndBringBack(
	_ context.Context, _ int64, root string,
) (nodeproto.PlaceProbe, bool, error) {
	roots.probes++
	if roots.probes != roots.backAt {
		return nodeproto.PlaceProbe{Path: root, State: nodeproto.ProbeNotAnswering}, false, nil
	}
	roots.bringBack()
	return nodeproto.PlaceProbe{Path: root, State: nodeproto.ProbeReadable}, true, nil
}

// unavailableFilm is a service over one film, file 2, under a root that is unavailable.
func unavailableFilm(t *testing.T, roots *probedRoots) *Service {
	t.Helper()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "mixed", MetadataSource: "nfo", Root: "/media"})
	librarytest.File(t, database, librarytest.FileSpec{
		ID: 2, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Film.mkv", SizeBytes: 10,
		PayloadHash: []byte{0x0c, 0x0d}, Filing: "listed",
	})
	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: 2})
	storetest.ExecRaw(t, database.Path(),
		fmt.Sprintf(`INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome,
		 last_scan_at) VALUES (%[1]d, '/media', 7, 1, 'complete', 1)`, nodeID),
		fmt.Sprintf(`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (%d, '/media', '/media', 1, 'not_answering')`, nodeID))
	roots.bringBack = func() {
		storetest.ExecRaw(t, database.Path(), "DELETE FROM unavailable_place")
	}
	return &Service{
		database: database, nodes: connectedNodes{}, roots: roots,
		clock: clocktest.NewFake(time.Date(2026, 10, 21, 19, 7, 12, 0, time.UTC)),
	}
}

// An ask whether a file can be read, held, probes its root's node while the root is unavailable and
// answers as soon as a probe brings it back: a page waiting on a share's return asked every ten
// seconds and heard of it only from the node's next status (D327 §3; staging, 2026-10-21).
func TestAwaitReadable_AnswersAsSoonAsAProbeBringsTheRootBack(t *testing.T) {
	t.Parallel()
	roots := &probedRoots{backAt: 3}
	service := unavailableFilm(t, roots)
	started := service.clock.Now()
	unreadable, err := service.AwaitReadable(t.Context(), 2, 20*time.Second)
	if err != nil || unreadable != nil {
		t.Fatalf("a film whose root came back at the third probe: %+v, %v; want it readable", unreadable, err)
	}
	if waited := service.clock.Now().Sub(started); roots.probes != 3 || waited != 2*probeEvery {
		t.Errorf("probed %d times over %s; want three, %s apart", roots.probes, waited, probeEvery)
	}
}

// A held ask whose root does not come back answers that it cannot be read at its bound, and one not
// held probes once and answers at once.
func TestAwaitReadable_AnswersUnreadableAtItsBound(t *testing.T) {
	t.Parallel()
	for _, wait := range []time.Duration{0, 5 * time.Second} {
		roots := &probedRoots{}
		service := unavailableFilm(t, roots)
		started := service.clock.Now()
		unreadable, err := service.AwaitReadable(t.Context(), 2, wait)
		if err != nil || unreadable == nil || unreadable.Reason != library.RootMissing || unreadable.Path != "/media" {
			t.Errorf("held %s: %+v, %v; want /media missing", wait, unreadable, err)
		}
		if waited := service.clock.Now().Sub(started); waited != wait || roots.probes < 1 {
			t.Errorf("held %s: answered after %s, %d probes; want at the bound, probed", wait, waited, roots.probes)
		}
	}
}
