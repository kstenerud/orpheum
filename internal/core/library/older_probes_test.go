// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// currentRecord is observedRecord as the node's probe reads it now, saying its version.
func currentRecord(path, content string) nodeproto.ScanRecord {
	record := observedRecord(path, content)
	record.Observed.Probe.Version = nodeproto.ProbeVersion
	return record
}

// D535 §2: a file an older probe read is read again on its own, by a background scan of the folder
// that holds it reading it again, as a full rescan does; a file the node's own probe read is not;
// nothing is asked while a scan runs, and a file is asked once.
func TestReadAgainOlderProbes_AFileAnOlderProbeReadIsReadAgainOnItsOwn(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{
		observedRecord("Arkon (1979)/Arkon.mkv", "arkon"),
		currentRecord("Arkon (1979)/Trailer.mkv", "trailer"),
		currentRecord("Metropolis (1927)/Metropolis.mkv", "metropolis"),
	}, true)
	ctx := t.Context()
	nodes := &fakeNodes{id: bench.nodeID, roots: []string{root}}
	admin := NewAdmin(bench.database, bench.clock, nodes, bench.scans, retirementOf(&fakeWork{}))

	bench.requestUnder("Metropolis (1927)")
	if asked, err := admin.ReadAgainOlderProbes(ctx); err != nil || asked != 0 {
		t.Fatalf("ReadAgainOlderProbes while a scan runs = %d, %v; want nothing asked till it ends", asked, err)
	}
	bench.listedIn("Metropolis (1927)", listedDirectory("Metropolis (1927)"),
		listedFile("Metropolis (1927)/Metropolis.mkv", int64(len("metropolis")), 1))
	if _, isScanning := bench.inProgressOf(); isScanning {
		t.Fatal("the scan of the one folder is still in progress after its listing changed nothing")
	}

	if asked, err := admin.ReadAgainOlderProbes(ctx); err != nil || asked != 1 {
		t.Fatalf("ReadAgainOlderProbes = %d, %v; want the one file an older probe read asked read again", asked,
			err)
	}
	bench.listedIn("Arkon (1979)", listedDirectory("Arkon (1979)"),
		listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1),
		listedFile("Arkon (1979)/Trailer.mkv", int64(len("trailer")), 1))
	_, files := bench.examine()
	if got, want := examinedPaths(files), []string{"Arkon (1979)/Arkon.mkv"}; !slices.Equal(got, want) {
		t.Errorf("the scan read %v; want %v: the file an older probe read, not the one the node's read", got, want)
	}
	bench.examineAs(currentRecord("Arkon (1979)/Arkon.mkv", "arkon"))
	if asked, err := admin.ReadAgainOlderProbes(ctx); err != nil || asked != 0 {
		t.Errorf("ReadAgainOlderProbes after the file was read again = %d, %v; want none left to ask", asked, err)
	}
}

// D535 §2: a file whose reading again leaves it as an older probe read it, its node's probe older or
// its disk away, is asked once a start, not again at every sweep.
func TestReadAgainOlderProbes_AFileIsAskedOnceAStart(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	ctx := t.Context()
	nodes := &fakeNodes{id: bench.nodeID, roots: []string{root}}
	admin := NewAdmin(bench.database, bench.clock, nodes, bench.scans, retirementOf(&fakeWork{}))
	if asked, err := admin.ReadAgainOlderProbes(ctx); err != nil || asked != 1 {
		t.Fatalf("ReadAgainOlderProbes = %d, %v; want the file asked", asked, err)
	}
	bench.listedIn("Arkon (1979)", listedDirectory("Arkon (1979)"),
		listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1))
	bench.examineAs(observedRecord("Arkon (1979)/Arkon.mkv", "arkon"))
	if asked, err := admin.ReadAgainOlderProbes(ctx); err != nil || asked != 0 {
		t.Errorf("ReadAgainOlderProbes again = %d, %v; want the file not asked twice in one start", asked, err)
	}
	restarted := NewAdmin(bench.database, bench.clock, nodes, bench.scans, retirementOf(&fakeWork{}))
	if asked, err := restarted.ReadAgainOlderProbes(ctx); err != nil || asked != 1 {
		t.Errorf("ReadAgainOlderProbes after a start = %d, %v; want the file asked again", asked, err)
	}
}
