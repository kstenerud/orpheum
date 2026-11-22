// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file read by an older probe is read again on its own (D535 §2): the sweep asks for a scan that
// reads it again, as a full rescan does (D392 §2), so what its differences touch is thrown out, and
// nothing else. It asks gently: only while no scan runs, so a node's hello scans and a person's come
// first, and at most olderProbesAtOnce files of one node at a time, in the background; each scan's
// end asks for the sweep again, so the next follow the last.

// olderProbesAtOnce bounds the files one sweep asks read again: a few minutes of a node's probing.
const olderProbesAtOnce = 100

// olderProbesRead is how far, on each node, the files an older probe read have been asked read
// again since the server started: the last file asked, by node id. A file is asked once a start, so
// one whose reading again fails, its disk away or its probe the same, is not asked for ever; the
// next start asks it again.
type olderProbesRead struct {
	mutex       sync.Mutex
	afterFileID map[int64]int64
}

// ReadAgainOlderProbes is the sweep that reads again on its own each file an older probe read
// (D535 §2): while no scan is in progress, it asks for a scan, in the background, reading again the
// next files a probe older than the node's (nodeproto.ProbeVersion) read on the first connected node
// that has any, in every library, whatever it holds. It says how many files it asked read again.
func (admin *Admin) ReadAgainOlderProbes(ctx context.Context) (int, error) {
	admin.olderProbes.mutex.Lock()
	defer admin.olderProbes.mutex.Unlock()
	if admin.olderProbes.afterFileID == nil {
		admin.olderProbes.afterFileID = map[int64]int64{}
	}
	progress, err := admin.scans.InProgress(ctx)
	if err != nil {
		return 0, err
	}
	if len(progress) > 0 {
		return 0, nil // a scan runs: the files wait for it to end
	}
	enrolled, err := admin.database.Read().ListLiveNodes(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the nodes: %w", err)
	}
	for _, node := range enrolled {
		nodeID, _, connected := admin.nodes.ConnectedNodeRoots(ctx, node.Name)
		if !connected || nodeID != node.ID {
			continue
		}
		asked, err := admin.readAgainOlderProbesOn(ctx, node.ID)
		if err != nil || asked > 0 {
			return asked, err
		}
	}
	return 0, nil
}

// readAgainOlderProbesOn asks for the scans reading again the next files an older probe read on a
// node, in the library folders that hold them, and says how many files it asked; none once there are
// none left to ask. A file under no live library folder is passed over.
func (admin *Admin) readAgainOlderProbesOn(ctx context.Context, nodeID int64) (int, error) {
	queries := admin.database.Read()
	for {
		files, err := queries.ListFilesProbedBefore(ctx, sqlcgen.ListFilesProbedBeforeParams{
			AfterID: admin.olderProbes.afterFileID[nodeID], NodeID: nodeID, ProbeVersion: nodeproto.ProbeVersion,
			MaxRows: olderProbesAtOnce,
		})
		if err != nil {
			return 0, fmt.Errorf("list the files an older probe read on node %d: %w", nodeID, err)
		}
		if len(files) == 0 {
			return 0, nil
		}
		admin.olderProbes.afterFileID[nodeID] = files[len(files)-1].ID
		asked, err := admin.askToReadAgain(ctx, nodeID, files)
		if err != nil || asked > 0 {
			return asked, err
		}
	}
}

// askToReadAgain asks, for each live library folder on a node holding any of files, the innermost
// first, for a background scan of the one folder in it holding them, reading them again; and says
// how many it asked.
func (admin *Admin) askToReadAgain(
	ctx context.Context, nodeID int64, files []sqlcgen.ListFilesProbedBeforeRow,
) (int, error) {
	roots, err := admin.database.Read().ListLiveRootPathsOnNode(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("list node %d's library folders: %w", nodeID, err)
	}
	byRoot := map[string][]filePlace{}
	for _, file := range files {
		for _, root := range roots { // longest first: the innermost folder holding it
			if _, under := RelativeToRoot([]byte(file.Path), root); under {
				byRoot[root] = append(byRoot[root], filePlace{nodeID: nodeID, path: file.Path})
				break
			}
		}
	}
	asked := 0
	for _, root := range roots {
		scope, readAgain, holdsAny := filesUnder(nodeID, root, byRoot[root])
		if !holdsAny {
			continue
		}
		err := admin.scans.RequestReadingAgain(ctx, nodeID, root, scope, readAgain, jobs.PriorityBackground)
		if err != nil {
			return asked, err
		}
		admin.log().InfoContext(ctx, "files an older probe read are read again", slogx.EventKey, "olderProbesReadAgain",
			"nodeId", nodeID, "rootPath", root, "scopePath", scope, "fileCount", len(readAgain),
			"probeVersion", nodeproto.ProbeVersion)
		asked += len(readAgain)
	}
	return asked, nil
}
