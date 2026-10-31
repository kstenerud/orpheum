// SPDX-License-Identifier: AGPL-3.0-or-later

package trust_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// suspended is the owner's set after e2 suspended e1 at version 2, and a core following it.
func suspended(t *testing.T, kind trust.ChainKind) (owner, trust.Chain) {
	t.Helper()
	set := owner{
		e1: trusttest.HardwareKey(t, "e1"), e2: trusttest.HardwareKey(t, "e2"),
		r1: trusttest.HardwareKey(t, "r1"), r2: trusttest.PlainKey(t, "r2"),
	}
	set.chain = trusttest.NewChain(t, kind, []trusttest.Key{set.e1, set.e2}, []trusttest.Key{set.r1, set.r2})
	core := set.chain.Follower(t)
	suspension, root := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e1}}, set.e2)
	follows(t, &core, []trust.Signed{suspension}, stands{
		version: 2, want: []trusttest.Key{set.e2, set.r1, set.r2}, gone: []trusttest.Key{set.e1},
	})
	set.chain.Take(suspension, root)
	return set, core
}

// A suspended key stays listed in the set, and its signatures count for nothing: not on an address
// document, a release record, a drill, nor the next root statement (D394 §4).
func TestSuspend_ASuspendedKeysSignaturesCountForNothing(t *testing.T) {
	t.Parallel()
	payload := []byte("orpheum-signed address 1\n")
	set, core := suspended(t, trust.ChainDiscovery)
	if !slices.ContainsFunc(core.Current().Keys, func(key trust.RootKey) bool { return key.Label == "e1" }) {
		t.Error("the suspended key left the set's key lines")
	}
	if err := core.Verify(trust.NamespaceDiscoveryAddress,
		trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, set.e1)); err == nil {
		t.Error("an address document the suspended key signed holds")
	}
	if err := core.Verify(trust.NamespaceDiscoveryAddress,
		trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, set.e2)); err != nil {
		t.Errorf("an address document e2 signed: %v", err)
	}
	if _, err := core.VerifyAny(trust.NamespaceDrill, trusttest.Sign(t, trust.NamespaceDrill, payload, set.e1)); err == nil {
		t.Error("a drill the suspended key signed holds")
	}
	added := trusttest.Change{AddEveryday: []trusttest.Key{trusttest.HardwareKey(t, "x")}}
	next, _ := set.chain.Next(t, 3, added, set.e1)
	follows(t, &core, []trust.Signed{next}, stands{version: 2})

	publisher, publisherCore := suspended(t, trust.ChainPublisher)
	if err := publisherCore.Verify(trust.NamespaceRelease,
		trusttest.Sign(t, trust.NamespaceRelease, payload, publisher.e1)); err == nil {
		t.Error("a release record the suspended key signed holds")
	}
	if err := publisherCore.Verify(trust.NamespaceRelease,
		trusttest.Sign(t, trust.NamespaceRelease, payload, publisher.e2)); err != nil {
		t.Errorf("a release record e2 signed: %v", err)
	}
}

// The tiers' rules for suspension: an everyday key suspends an everyday key but not a recovery key;
// only a recovery key resumes, and the key's signatures count again; a statement signed only by the
// key it suspends is refused; a removed key never comes back; the recovery keys are never all
// suspended.
func TestSuspend_OnlyARecoveryKeyResumesAndTheTiersRulesHold(t *testing.T) {
	t.Parallel()
	payload := []byte("orpheum-signed address 1\n")
	fresh := newOwner(t)
	everydayOnRecovery, _ := fresh.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{fresh.r1}}, fresh.e1)
	if _, err := fresh.chain.Follower(t).Accept(everydayOnRecovery); !errors.Is(err, trust.ErrNotAllowed) {
		t.Errorf("an everyday key suspending a recovery key: %v; want ErrNotAllowed", err)
	}
	everydayOnEveryday, _ := fresh.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{fresh.e2}}, fresh.e1)
	if _, err := fresh.chain.Follower(t).Accept(everydayOnEveryday); err != nil {
		t.Errorf("an everyday key suspending another: %v", err)
	}
	selfSuspended, _ := fresh.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{fresh.e1}}, fresh.e1)
	if _, err := fresh.chain.Follower(t).Accept(selfSuspended); !errors.Is(err, trust.ErrNotHeldByNewSet) {
		t.Errorf("a statement signed only by the key it suspends: %v; want ErrNotHeldByNewSet", err)
	}
	byRecovery, _ := fresh.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{fresh.r2}}, fresh.r1)
	if _, err := fresh.chain.Follower(t).Accept(byRecovery); err != nil {
		t.Errorf("a recovery key suspending another: %v", err)
	}
	allRecovery := fresh.chain.Last
	allRecovery.ID, allRecovery.Version = fresh.chain.ID(), 2
	allRecovery.Suspended = []string{fingerprintOf(fresh.r1), fingerprintOf(fresh.r2)}
	if _, err := fresh.chain.Follower(t).Accept(
		trusttest.SignRoot(t, trust.ChainDiscovery, allRecovery, fresh.r1)); !errors.Is(err, trust.ErrBadValue) {
		t.Errorf("both recovery keys suspended: %v; want ErrBadValue", err)
	}

	set, core := suspended(t, trust.ChainDiscovery)
	resume := trusttest.Change{Resume: []trusttest.Key{set.e1}}
	for name, signer := range map[string]trusttest.Key{"another everyday key": set.e2, "the key itself": set.e1} {
		statement, _ := set.chain.Next(t, 3, resume, signer)
		if _, err := core.Accept(statement); !errors.Is(err, trust.ErrNotAllowed) {
			t.Errorf("a resume signed by %s: %v; want ErrNotAllowed", name, err)
		}
	}
	resumed, resumedRoot := set.chain.Next(t, 3, resume, set.r1)
	follows(t, &core, []trust.Signed{resumed}, stands{version: 3, want: []trusttest.Key{set.e1}})
	if err := core.Verify(trust.NamespaceDiscoveryAddress,
		trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, set.e1)); err != nil {
		t.Errorf("an address document the resumed key signed: %v", err)
	}
	set.chain.Take(resumed, resumedRoot)

	// Removed for good: e1, removed, is not resumed by putting it back in the set, with or without its
	// removed line.
	removal, removalRoot := set.chain.Next(t, 4, trusttest.Change{Remove: []trusttest.Key{set.e1}}, set.e2)
	follows(t, &core, []trust.Signed{removal}, stands{version: 4, gone: []trusttest.Key{set.e1}})
	set.chain.Take(removal, removalRoot)
	back := set.chain.Last
	back.ID, back.Version = set.chain.ID(), 5
	back.Keys = append(slices.Clone(back.Keys), set.e1.RootKey(trust.RoleEveryday))
	if _, err := core.Accept(trusttest.SignRoot(t, trust.ChainDiscovery, back, set.r1)); !errors.Is(
		err, trust.ErrBadValue) {
		t.Errorf("a removed key back in the set: %v; want ErrBadValue", err)
	}
	back.Removed = nil
	if _, err := core.Accept(trusttest.SignRoot(t, trust.ChainDiscovery, back, set.r1)); !errors.Is(
		err, trust.ErrRemovedReturned) {
		t.Errorf("a removed key back in the set, unnamed removed: %v; want ErrRemovedReturned", err)
	}
	back = set.chain.Last
	back.ID, back.Version, back.Suspended = set.chain.ID(), 5, []string{fingerprintOf(set.e1)}
	if _, err := core.Accept(trusttest.SignRoot(t, trust.ChainDiscovery, back, set.r1)); !errors.Is(
		err, trust.ErrBadValue) {
		t.Errorf("a removed key named suspended: %v; want ErrBadValue", err)
	}
}

// A suspended key's line is critical: a build that does not know it refuses the statement, so the key
// is never counted by a core too old to see the suspension.
func TestSuspend_TheSuspendedLineIsCritical(t *testing.T) {
	t.Parallel()
	set, _ := suspended(t, trust.ChainDiscovery)
	payload := set.chain.Statements[1].Payload
	unknown := trust.Schema{Type: "root", Format: 1, Fields: []trust.Field{
		{Name: "chain"},
		{Name: "id"},
		{Name: "version"},
		{Name: "issued"},
		{Name: "key", IsRepeated: true},
		{Name: "threshold", IsRepeated: true},
	}}
	if _, err := unknown.Parse([]byte(payload)); !errors.Is(err, trust.ErrCritical) {
		t.Errorf("a build without the suspended line read the statement: %v; want ErrCritical", err)
	}
}
