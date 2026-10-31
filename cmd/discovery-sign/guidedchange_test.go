// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// changeFolder is a software ceremony's folder: everyday key e1, recovery keys r1 and r2.
func changeFolder(t *testing.T) (string, *fakeKeys) {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "records")
	fake := newFakeKeys(t)
	status, said := softwareCeremony(t, fake, folder, t.TempDir(), "\n2\nhttps://orpheum.dev\n\n")
	if status != exitSuccess {
		t.Fatalf("the ceremony = %d: %s", status, said)
	}
	return folder, fake
}

// counting is whether the key named name counts in each chain the folder's ledger holds, in that order.
func counting(t *testing.T, folder, name string) []bool {
	t.Helper()
	var found []bool
	for _, kind := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
		chain, _, err := ledger(filepath.Join(folder, ledgerDirectory)).chain(kind)
		if err != nil {
			t.Fatal(err)
		}
		key, err := readKey(filepath.Join(folder, keysDirectory, string(kind)+"-"+name+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, slices.ContainsFunc(chain.Fingerprints(), func(shown string) bool {
			return strings.HasSuffix(shown, " "+key.Fingerprint())
		}))
	}
	return found
}

// The carried key lost, by the guided command (D394 §4): suspending e1 offers the other keys to sign,
// never e1 itself, and stops its signatures counting in both chains; the address document e1 signed is
// signed again, so cores still take it; resuming it offers only the
// recovery keys, and its signatures count again.
func TestGuidedChange_ASuspendedKeyIsResumedOnlyByARecoveryKey(t *testing.T) {
	t.Parallel()
	folder, fake := changeFolder(t)
	status, said := guidedRun(t, fake, "\n\n\n\n\n", "suspend", "--folder", folder, "--key", "e1")
	if status != exitSuccess {
		t.Fatalf("suspend = %d: %s", status, said)
	}
	if !strings.Contains(said, "+ !suspended ") ||
		!strings.Contains(said, "an everyday or a recovery key. Which one? (r1, r2)") {
		t.Errorf("the suspension said %q; want the suspended line shown and r1 or r2 offered, not e1", said)
	}
	if got := counting(t, folder, "e1"); slices.Contains(got, true) {
		t.Errorf("e1 counts after its suspension, by chain: %v", got)
	}
	discovery, err := trust.Genesis(trust.ChainDiscovery,
		readPublished(t, filepath.Join(folder, publishDirectory), addressdoc.RootPath(1)))
	if err != nil {
		t.Fatal(err)
	}
	if discovery, err = discovery.Accept(readPublished(t, filepath.Join(folder, publishDirectory),
		addressdoc.RootPath(2))); err != nil {
		t.Fatal(err)
	}
	if _, err = addressdoc.Open(readPublished(t, filepath.Join(folder, publishDirectory), addressdoc.DocumentPath),
		discovery, "https://orpheum.dev"); err != nil {
		t.Errorf("the published address document, which e1 signed, does not open once e1 is suspended: %v", err)
	}
	status, said = guidedRun(t, fake, "\n\n\n\n", "resume", "--folder", folder, "--key", "e1")
	if status != exitSuccess {
		t.Fatalf("resume = %d: %s", status, said)
	}
	if !strings.Contains(said, "- !suspended ") || !strings.Contains(said, "a recovery key. Which one? (r1, r2)") {
		t.Errorf("the resume said %q; want the suspended line gone and only recovery keys offered", said)
	}
	if got := counting(t, folder, "e1"); slices.Contains(got, false) {
		t.Errorf("e1 does not count after its resumption, by chain: %v", got)
	}
}

// Removing a recovery key needs a recovery key's signature, so the one left signs without being asked
// for; the key is gone from both chains.
func TestGuidedChange_ARemovedRecoveryKeyIsSignedOffByTheOtherRecoveryKey(t *testing.T) {
	t.Parallel()
	folder, fake := changeFolder(t)
	status, said := guidedRun(t, fake, "\n\n", "remove", "--folder", folder, "--key", "r2")
	if status != exitSuccess {
		t.Fatalf("remove = %d: %s", status, said)
	}
	if !strings.Contains(said, `It is signed by a recovery key: "r1".`) {
		t.Errorf("the removal said %q; want r1 to sign it", said)
	}
	if got := counting(t, folder, "r2"); slices.Contains(got, true) {
		t.Errorf("r2 counts after its removal, by chain: %v", got)
	}
}

// A new everyday key is made for both chains and added to each, signed by a key already in the set.
func TestGuidedChange_AnAddedKeyIsMadeForBothChainsAndCounts(t *testing.T) {
	t.Parallel()
	folder, fake := changeFolder(t)
	status, said := guidedRun(t, fake, "\n\n\n\n\n", "add", "--folder", folder, "--key", "e2")
	if status != exitSuccess {
		t.Fatalf("add = %d: %s", status, said)
	}
	if fake.makes != 8 {
		t.Errorf("%d keys made; want the ceremony's 6 and one for each chain", fake.makes)
	}
	if got := counting(t, folder, "e2"); slices.Contains(got, false) {
		t.Errorf("e2 does not count after it was added, by chain: %v", got)
	}
	if status, said = guidedRun(t, fake, "", "add", "--folder", folder, "--key", "e2"); status != exitFailure ||
		!strings.Contains(said, "in the folder already") {
		t.Errorf("adding e2 again = %d %q; want refused", status, said)
	}
}
