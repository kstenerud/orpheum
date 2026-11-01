// SPDX-License-Identifier: AGPL-3.0-or-later

package trust_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// owner is the owner's set as the root key audit's §3.6 has it: everyday e1 (carried) and e2 (home),
// recovery r1 (the bank) and r2 (a second site), the chain at version 1.
type owner struct {
	chain          *trusttest.Chain
	e1, e2, r1, r2 trusttest.Key
}

func newOwner(t *testing.T) owner {
	t.Helper()
	held := owner{
		e1: trusttest.HardwareKey(t, "e1"), e2: trusttest.HardwareKey(t, "e2"),
		r1: trusttest.HardwareKey(t, "r1"), r2: trusttest.PlainKey(t, "r2"),
	}
	held.chain = trusttest.NewChain(t, trust.ChainDiscovery, []trusttest.Key{held.e1, held.e2},
		[]trusttest.Key{held.r1, held.r2})
	return held
}

// stands is where a core must stand after following: its version, the keys its set holds and those
// it does not, and the versions it found in conflict.
type stands struct {
	version    int64
	want, gone []trusttest.Key
	conflicts  []int64
}

// follows has core follow candidates, and fails the test unless it then stands as want says.
func follows(t *testing.T, core *trust.Chain, candidates []trust.Signed, want stands) {
	t.Helper()
	var followed trust.Followed
	*core, followed = core.Follow(candidates)
	if core.Version() != want.version {
		t.Fatalf("the core stands at %d; want %d", core.Version(), want.version)
	}
	held := core.Fingerprints()
	for _, key := range want.want {
		if !holds(held, key) {
			t.Errorf("the set lacks %s", key.Label)
		}
	}
	for _, key := range want.gone {
		if holds(held, key) {
			t.Errorf("the set still holds %s", key.Label)
		}
	}
	slices.Sort(followed.Conflicts)
	if !slices.Equal(followed.Conflicts, want.conflicts) {
		t.Errorf("conflicts %v; want %v", followed.Conflicts, want.conflicts)
	}
}

// holds is whether the fingerprints shown hold key's.
func holds(shown []string, key trusttest.Key) bool {
	return slices.ContainsFunc(shown, func(line string) bool { return strings.HasSuffix(line, fingerprintOf(key)) })
}

func fingerprintOf(key trusttest.Key) string { return trust.Fingerprint(key.PublicKey()) }

// Row 1: e1 lost or broken: e2 suspends it at n+1 at once, and later a recovery key's statement removes
// it and adds e3, the new key signing too. The other everyday key alone cannot replace it: an everyday
// key only suspends (D395).
func TestRotation_E1LostIsSuspendedByE2AndReplacedByARecoveryKey(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	core := set.chain.Follower(t)
	replacement := trusttest.HardwareKey(t, "e3")
	replacing := trusttest.Change{Remove: []trusttest.Key{set.e1}, AddEveryday: []trusttest.Key{replacement}}
	byEveryday, _ := set.chain.Next(t, 2, replacing, set.e2, replacement)
	follows(t, &core, []trust.Signed{byEveryday}, stands{version: 1})
	suspension, suspensionRoot := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e1}}, set.e2)
	follows(t, &core, []trust.Signed{suspension}, stands{
		version: 2, want: []trusttest.Key{set.e2}, gone: []trusttest.Key{set.e1},
	})
	set.chain.Take(suspension, suspensionRoot)
	// The lost key, found by another, signs nothing more.
	theirs, _ := set.chain.Next(t, 3, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1)
	follows(t, &core, []trust.Signed{theirs}, stands{version: 2})
	next, _ := set.chain.Next(t, 3, replacing, set.r1, replacement)
	follows(t, &core, []trust.Signed{next}, stands{
		version: 3, want: []trusttest.Key{set.e2, replacement}, gone: []trusttest.Key{set.e1},
	})
}

// Row 2: e1 stolen, its PIN safe: e2 suspends it at once, and a recovery key removes it later; the
// thief cannot sign without the PIN.
func TestRotation_E1StolenWithoutItsPINIsSuspendedAtOnce(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	core := set.chain.Follower(t)
	// The thief signs without the PIN: the flag is missing, and the core takes nothing.
	theirs, _ := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1.WithoutPIN())
	follows(t, &core, []trust.Signed{theirs}, stands{version: 1})
	ours, oursRoot := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e1}}, set.e2)
	follows(t, &core, []trust.Signed{ours}, stands{
		version: 2, want: []trusttest.Key{set.e2}, gone: []trusttest.Key{set.e1},
	})
	set.chain.Take(ours, oursRoot)
	e3 := trusttest.HardwareKey(t, "e3")
	replacing := trusttest.Change{Remove: []trusttest.Key{set.e1}, AddEveryday: []trusttest.Key{e3}}
	removal, _ := set.chain.Next(t, 3, replacing, set.r2, e3)
	follows(t, &core, []trust.Signed{removal}, stands{
		version: 3, want: []trusttest.Key{set.e2, e3}, gone: []trusttest.Key{set.e1},
	})
}

// Row 3: e1 stolen with its PIN: the thief cannot add a key of theirs, an everyday key's statement
// changing the set by nothing but a suspension (the owner, 2026-11-01); the most it can sign is n+1
// suspending e2. The owner's recovery key signs a statement far ahead, removing e1, resuming e2 and
// adding e3. A core that took the thief's, and one that never saw it, both end at the owner's set,
// and e1 signs nothing more.
func TestRotation_E1StolenWithItsPINIsRecoveredBySkippingPastTheThief(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	thiefKey := trusttest.HardwareKey(t, "x")
	added, _ := set.chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{thiefKey}}, set.e1, thiefKey)
	if _, err := set.chain.Follower(t).Accept(added); !errors.Is(err, trust.ErrNotAllowed) {
		t.Errorf("the thief adding a key of theirs: %v; want ErrNotAllowed", err)
	}
	theirs, theirRoot := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1)
	misled := set.chain.Follower(t)
	follows(t, &misled, []trust.Signed{theirs}, stands{
		version: 2, want: []trusttest.Key{set.e1}, gone: []trusttest.Key{set.e2},
	})
	set.chain.Take(theirs, theirRoot)
	fresh := trusttest.HardwareKey(t, "e3")
	recovered, _ := set.chain.Next(t, 1002, trusttest.Change{
		Remove: []trusttest.Key{set.e1}, Resume: []trusttest.Key{set.e2}, AddEveryday: []trusttest.Key{fresh},
	}, set.r1)
	follows(t, &misled, []trust.Signed{recovered}, stands{
		version: 1002, want: []trusttest.Key{set.e2, fresh}, gone: []trusttest.Key{set.e1},
	})
	unaware := set.chain.Follower(t)
	follows(t, &unaware, []trust.Signed{theirs, recovered}, stands{
		version: 1002, want: []trusttest.Key{set.e2, fresh}, gone: []trusttest.Key{set.e1},
	})
	// The thief, again, at the next version: removed keys stay removed.
	set.chain.Take(recovered, mustRoot(t, recovered))
	again, _ := set.chain.Next(t, 1003, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1)
	follows(t, &misled, []trust.Signed{again}, stands{version: 1002})
}

// Row 4: both everyday keys lost: the recovery key signs two new everyday keys.
func TestRotation_BothEverydayKeysLostAreReplacedByARecoveryKey(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	core := set.chain.Follower(t)
	e3, e4 := trusttest.HardwareKey(t, "e3"), trusttest.HardwareKey(t, "e4")
	next, _ := set.chain.Next(t, 2, trusttest.Change{
		Remove: []trusttest.Key{set.e1, set.e2}, AddEveryday: []trusttest.Key{e3, e4},
	}, set.r1)
	follows(t, &core, []trust.Signed{next}, stands{
		version: 2, want: []trusttest.Key{e3, e4}, gone: []trusttest.Key{set.e1, set.e2},
	})
}

// Row 5: r1 lost: r2 signs n+1, removing r1 and adding a new recovery key; an everyday key cannot.
func TestRotation_R1LostIsReplacedByR2AndNeverByAnEverydayKey(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	core := set.chain.Follower(t)
	r3 := trusttest.HardwareKey(t, "r3")
	change := trusttest.Change{Remove: []trusttest.Key{set.r1}, AddRecovery: []trusttest.Key{r3}}
	byEveryday, _ := set.chain.Next(t, 2, change, set.e1, r3)
	follows(t, &core, []trust.Signed{byEveryday}, stands{version: 1})
	byRecovery, _ := set.chain.Next(t, 2, change, set.r2, r3)
	follows(t, &core, []trust.Signed{byRecovery}, stands{
		version: 2, want: []trusttest.Key{set.r2, r3}, gone: []trusttest.Key{set.r1},
	})
}

// Row 6: r1 stolen with its PIN: the thief and r2 each sign a statement n+1; both are a recovery key's,
// so both are refused and every admin told, until r2's statement far ahead resolves it.
func TestRotation_R1StolenWithItsPINIsResolvedByR2sSkip(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	core := set.chain.Follower(t)
	thiefKey := trusttest.HardwareKey(t, "x")
	theirs, _ := set.chain.Next(t, 2, trusttest.Change{AddRecovery: []trusttest.Key{thiefKey}}, set.r1)
	newRecovery := trusttest.HardwareKey(t, "r3")
	replacing := trusttest.Change{Remove: []trusttest.Key{set.r1}, AddRecovery: []trusttest.Key{newRecovery}}
	ours, _ := set.chain.Next(t, 2, replacing,
		set.r2, newRecovery)
	follows(t, &core, []trust.Signed{theirs, ours}, stands{version: 1, conflicts: []int64{2}})
	replaced := trusttest.Change{Remove: []trusttest.Key{set.r1}, AddRecovery: []trusttest.Key{newRecovery}}
	skip, _ := set.chain.Next(t, 1001, replaced, set.r2, newRecovery)
	follows(t, &core, []trust.Signed{theirs, ours, skip}, stands{
		version: 1001, want: []trusttest.Key{set.r2, newRecovery}, gone: []trusttest.Key{set.r1, thiefKey},
		conflicts: []int64{2},
	})
}

// Row 7: all keys lost: a new genesis is another identity, which a core enrolled with the old one
// never takes; its admin enrols again.
func TestRotation_AllKeysLostIsANewIdentity(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	fresh := newOwner(t)
	if _, err := trust.Load(trust.ChainDiscovery, set.chain.ID(), fresh.chain.Statements); !errors.Is(
		err, trust.ErrNotTheIdentity) {
		t.Errorf("a new genesis under the old id: %v; want ErrNotTheIdentity", err)
	}
	core := set.chain.Follower(t)
	follows(t, &core, fresh.chain.Statements, stands{version: 1})
}

// The rules beside the table (§3.4), with the owner's of 2026-11-01: an everyday key changes the set
// by nothing but suspending an everyday key, and never skips; two everyday statements of one version
// are both refused, and a recovery key's wins over an everyday key's; a key leaving the set has the
// new set sign too; and removed keys stay named removed.
func TestFollow_TheTiersRulesHold(t *testing.T) {
	t.Parallel()
	set := newOwner(t)
	third, fourth := trusttest.HardwareKey(t, "e3"), trusttest.HardwareKey(t, "e4")
	refused := map[string]trust.Signed{}
	refused["an everyday skip"], _ = set.chain.Next(t, 5, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1)
	refused["an everyday threshold"], _ = set.chain.Next(t, 2, trusttest.Change{
		Thresholds: map[trust.Role]int{trust.RoleEveryday: 1, trust.RoleRecovery: 2},
	}, set.e1)
	refused["an everyday key adding one"], _ = set.chain.Next(t, 2,
		trusttest.Change{AddEveryday: []trusttest.Key{third}}, set.e1, third)
	refused["an everyday key removing one"], _ = set.chain.Next(t, 2,
		trusttest.Change{Remove: []trusttest.Key{set.e2}}, set.e1)
	refused["an everyday key replacing itself"], _ = set.chain.Next(t, 2, trusttest.Change{
		Remove: []trusttest.Key{set.e1}, AddEveryday: []trusttest.Key{third},
	}, set.e2, third)
	moved := set.chain.Last
	moved.ID, moved.Version, moved.Locations = set.chain.ID(), 2, []string{"https://elsewhere.example/root"}
	refused["an everyday key moving the chain"] = trusttest.SignRoot(t, trust.ChainDiscovery, moved, set.e1)
	unmarked := set.chain.Last
	unmarked.ID, unmarked.Version, unmarked.Keys = set.chain.ID(), 2, slices.Clone(unmarked.Keys)
	for index, key := range unmarked.Keys {
		if key.Label == "e2" {
			unmarked.Keys[index].MustVerifyUV = !key.MustVerifyUV
		}
	}
	refused["an everyday key re-marking one"] = trusttest.SignRoot(t, trust.ChainDiscovery, unmarked, set.e1)
	refused["r1 leaving with no new key signing"], _ = set.chain.Next(t, 2, trusttest.Change{
		Remove: []trusttest.Key{set.r1}, AddRecovery: []trusttest.Key{trusttest.HardwareKey(t, "r3")},
	}, set.r1)
	unnamed := set.chain.Last
	unnamed.ID, unnamed.Version = set.chain.ID(), 2
	unnamed.Keys = slices.DeleteFunc(slices.Clone(unnamed.Keys), func(key trust.RootKey) bool {
		return key.Label == "e2"
	})
	refused["e2 dropped unnamed"] = trusttest.SignRoot(t, trust.ChainDiscovery, unnamed, set.r1)
	for name, statement := range refused {
		core := set.chain.Follower(t)
		if after, _ := core.Follow([]trust.Signed{statement}); after.Version() != 1 {
			t.Errorf("%s: taken", name)
		}
	}
	one, _ := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e2}}, set.e1)
	other, _ := set.chain.Next(t, 2, trusttest.Change{Suspend: []trusttest.Key{set.e1}}, set.e2)
	core := set.chain.Follower(t)
	follows(t, &core, []trust.Signed{one, other}, stands{version: 1, conflicts: []int64{2}})
	recovery, _ := set.chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{fourth}}, set.r1)
	follows(t, &core, []trust.Signed{one, other, recovery}, stands{
		version: 2, want: []trusttest.Key{set.e1, set.e2, fourth},
	})
	// A core that took one of two everyday statements hears of the other, and says so.
	early := set.chain.Follower(t)
	follows(t, &early, []trust.Signed{one}, stands{
		version: 2, want: []trusttest.Key{set.e1}, gone: []trusttest.Key{set.e2},
	})
	follows(t, &early, []trust.Signed{other}, stands{version: 2, conflicts: []int64{2}})
}

func mustRoot(t *testing.T, signed trust.Signed) trust.Root {
	t.Helper()
	root, err := trust.ParseRoot([]byte(signed.Payload))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
