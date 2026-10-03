// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The core lists a root, then asks for the files it needs read: the node answers each as a result
// of the job (D240 §2, §4).
func TestAgent_ListsARootAndExaminesTheFilesTheCoreAsksFor(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "Wing.mkv"), []byte("alien"), 0o600); err != nil {
		t.Fatal(err)
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	listed := testutil.Receive(t, core.scans, "the listing")
	if listed.JobID != 1 || listed.Listing == nil || !slices.ContainsFunc(listed.Listing.Records,
		func(record nodeproto.ScanRecord) bool {
			return record.Listed != nil && record.Listed.Path.Text == "Wing.mkv"
		}) {
		t.Errorf("job 1's answer = %+v, want a listing naming Wing.mkv", listed)
	}
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 2, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
			RootPath: root.Path(),
			Files:    []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "Wing.mkv"}, Kind: nodeproto.ExamineVideo}},
		},
	}}
	examined := testutil.Receive(t, core.scans, "the examining")
	if examined.JobID != 2 || examined.Examined == nil || len(examined.Examined.Records) != 1 ||
		examined.Examined.Records[0].Observed == nil {
		t.Errorf("job 2's answer = %+v, want Wing.mkv observed", examined)
	}
}

// A listing or an examining under a root the node was not given is refused, whatever the core asks
// (D107).
func TestAgent_RefusesToListOrExamineARootItWasNotGiven(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	core.commands <- nodeproto.Command{CommandSeq: 3, JobID: 3, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: "/etc", Path: "."},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused listing"); refused.Listing == nil ||
		refused.Listing.Refusal == "" || len(refused.Listing.Records) != 0 {
		t.Errorf("listing /etc = %+v, want it refused: the node was not given it", refused.Listing)
	}
	core.commands <- nodeproto.Command{CommandSeq: 4, JobID: 4, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{RootPath: "/etc", Files: []nodeproto.ExamineFile{
			{Path: nodeproto.Path{Text: "passwd"}, Kind: nodeproto.ExamineNFO},
		}},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused examining"); refused.Examined == nil ||
		refused.Examined.Refusal == "" || len(refused.Examined.Records) != 0 {
		t.Errorf("examining under /etc = %+v, want it refused", refused.Examined)
	}
}

// List lists as the node's scanner does.
func (fakeScanner) List(ctx context.Context, root *confine.MediaRoot, start string, depth int) (
	[]nodeproto.ScanRecord, error,
) {
	return realScanner().List(ctx, root, start, depth)
}

// Examine examines as the node's scanner does, with a prober that finds every video a Matroska.
func (fakeScanner) Examine(
	ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	return realScanner().Examine(ctx, root, files, between)
}

// realScanner is the node's scanner over no other roots, probing nothing.
func realScanner() *scan.Scanner {
	return scan.NewScanner(nil, matroskaProber{}, scan.Options{VideoExtensions: []string{"mkv"}})
}

// matroskaProber finds every file a Matroska.
type matroskaProber struct{}

func (matroskaProber) Probe(context.Context, string) (nodeproto.Probe, error) {
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}}}, nil
}
