// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// The carried key lost, through the tool (D394 §4): e2 suspends e1, the change shown, and e1 signing
// its own suspension is refused; e1's address document no longer holds; e2 cannot resume it, the tool
// saying a recovery key signs; the bank's key resumes it and its signatures count again.
func TestRoot_AKeyIsSuspendedAndOnlyARecoveryKeyResumesIt(t *testing.T) {
	t.Parallel()
	person := newOwner(t)
	carried, home := trusttest.HardwareKey(t, "e1"), trusttest.HardwareKey(t, "e2")
	bank := trusttest.HardwareKey(t, "r1")
	person.does("genesis", "--chain", "discovery", "--everyday", person.pub(carried), "--everyday",
		person.pub(home), "--recovery", person.pub(bank))
	person.does("check", "--kind", "root", "--payload",
		person.signs("discovery-root-1", trust.NamespaceDiscoveryRoot, carried))

	said := person.does("root", "--chain", "discovery", "--suspend", person.pub(carried))
	if !strings.Contains(said, "+ !suspended "+trust.Fingerprint(carried.PublicKey())) ||
		!strings.Contains(said, "an everyday or a recovery key") {
		t.Errorf("the suspension said %q; want the suspended line added and an everyday key to sign", said)
	}
	if status, refused := person.runs("check", "--kind", "root", "--payload",
		person.signs("discovery-root-2", trust.NamespaceDiscoveryRoot, carried)); status != exitFailure {
		t.Errorf("e1's own suspension, signed by it alone: %d %q; want refused", status, refused)
	}
	person.does("check", "--kind", "root", "--payload",
		person.signs("discovery-root-2", trust.NamespaceDiscoveryRoot, home))
	chain, _, err := ledger(person.ledger).chain(trust.ChainDiscovery)
	if err != nil {
		t.Fatal(err)
	}
	document := trusttest.Sign(t, trust.NamespaceDiscoveryAddress, []byte("orpheum-signed address 1\n"), carried)
	if err = chain.Verify(trust.NamespaceDiscoveryAddress, document); err == nil {
		t.Error("a document the suspended key signed holds")
	}

	said = person.does("root", "--chain", "discovery", "--resume", person.pub(carried))
	if !strings.Contains(said, "- !suspended ") || !strings.Contains(said, "Sign it with a recovery key") {
		t.Errorf("the resume said %q; want the suspended line gone and a recovery key to sign", said)
	}
	if status, refused := person.runs("check", "--kind", "root", "--payload",
		person.signs("discovery-root-3", trust.NamespaceDiscoveryRoot, home)); status != exitFailure {
		t.Errorf("e1 resumed by e2: %d %q; want refused", status, refused)
	}
	person.does("check", "--kind", "root", "--payload",
		person.signs("discovery-root-3", trust.NamespaceDiscoveryRoot, bank))
	if chain, _, err = ledger(person.ledger).chain(trust.ChainDiscovery); err != nil {
		t.Fatal(err)
	}
	if err = chain.Verify(trust.NamespaceDiscoveryAddress, document); err != nil {
		t.Errorf("a document the resumed key signed: %v", err)
	}
	if status, refused := person.runs("root", "--chain", "discovery", "--resume", person.pub(carried)); status !=
		exitFailure || !strings.Contains(refused, "not suspended") {
		t.Errorf("resuming a key not suspended: %d %q; want refused", status, refused)
	}
}

// The install's bootstrap trusts no suspended key: its allowed signers are the publisher's keys whose
// signatures count.
func TestBootstrap_TrustsNoSuspendedKey(t *testing.T) {
	t.Parallel()
	publishing, home := trusttest.HardwareKey(t, "p1"), trusttest.HardwareKey(t, "p2")
	publisher := trusttest.NewChain(t, trust.ChainPublisher, []trusttest.Key{publishing, home},
		[]trusttest.Key{trusttest.HardwareKey(t, "pr1")})
	_, root := publisher.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{publishing}}, home)
	script, err := renderBootstrap(root, []string{"https://site.example"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, publishing.RootKey("").Text) {
		t.Error("the bootstrap trusts the suspended key")
	}
	if !strings.Contains(script, home.RootKey("").Text) {
		t.Error("the bootstrap lacks a key whose signatures count")
	}
}
