// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
)

// Signed is a signed object as it is served and kept: the payload's exact bytes, and its signature
// file, one or more armoured SSH signatures.
type Signed struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// Errors following a chain returns.
var (
	ErrNotTheChain     = errors.New("the statement is of another chain")
	ErrNotGenesis      = errors.New("not a chain's first statement: it names an id, or none of its own keys signed it")
	ErrNotTheIdentity  = errors.New("the chain's first statement is not the identity pinned")
	ErrNotAllowed      = errors.New("no key allowed to sign this statement signed it")
	ErrRemovedReturned = errors.New("the statement does not keep every removed key removed")
	ErrNotHeldByNewSet = errors.New("the statement's signer leaves the set, and no key of the new set signed it too")
	ErrNotNext         = errors.New("the statement does not follow the chain held")
)

// Chain is a root key chain as a core follows it (the root key audit's §3.4): every statement it
// accepted, as received, from the genesis, whose hash is the chain's id.
type Chain struct {
	kind     ChainKind
	id       string
	accepted []Signed
	roots    []Root
}

// Genesis is the chain whose first statement is genesis: of kind, naming no id, signed by a key of
// its own set.
func Genesis(kind ChainKind, genesis Signed) (Chain, error) {
	root, err := ParseRoot([]byte(genesis.Payload))
	if err != nil {
		return Chain{}, err
	}
	if root.Chain != kind {
		return Chain{}, fmt.Errorf("%w: %s, not %s", ErrNotTheChain, root.Chain, kind)
	}
	keys := append(root.usable(RoleRecovery, nil), root.usable(RoleEveryday, nil)...)
	if root.ID != "" {
		return Chain{}, ErrNotGenesis
	}
	if _, err = verifyBlocks(genesis.Signature, kind.RootNamespace(), []byte(genesis.Payload), keys); err != nil {
		return Chain{}, fmt.Errorf("%w: %w", ErrNotGenesis, err)
	}
	return Chain{
		kind: kind, id: IdentityOf(kind, []byte(genesis.Payload)), accepted: []Signed{genesis}, roots: []Root{root},
	}, nil
}

// Load is the chain of statements, a core's kept state, checked again from its genesis, every
// statement taken by the rules a new one is (§3.3): a state that does not hold is refused. The id,
// when given, is the identity the genesis must be.
func Load(kind ChainKind, id string, statements []Signed) (Chain, error) {
	if len(statements) == 0 {
		return Chain{}, fmt.Errorf("%w: no statements", ErrNotGenesis)
	}
	chain, err := Genesis(kind, statements[0])
	if err != nil {
		return Chain{}, err
	}
	if id != "" && chain.id != id {
		return Chain{}, fmt.Errorf("%w: %s, not %s", ErrNotTheIdentity, chain.id, id)
	}
	for _, statement := range statements[1:] {
		if chain, err = chain.Accept(statement); err != nil {
			return Chain{}, err
		}
	}
	return chain, nil
}

// Accept is the chain with statement after its current set if the chain's rules take it (§3.4 steps
// 2–6), or why not: how the signing tool checks a statement before it is published.
func (chain Chain) Accept(statement Signed) (Chain, error) {
	next, _, err := chain.judge(statement)
	if err != nil {
		return chain, fmt.Errorf("the statement after version %d: %w", chain.Version(), err)
	}
	return chain.accept(statement, next), nil
}

// VerifyAny is whether signed is a signature of any key of the current set in namespace, whatever its
// role: a drill's, which proves a key and its PIN still work (the audit's §4 step 8).
func (chain Chain) VerifyAny(namespace string, signed Signed) ([]string, error) {
	current := chain.Current()
	keys := append(current.usable(RoleEveryday, current.Removed), current.usable(RoleRecovery, current.Removed)...)
	signers, err := verifyBlocks(signed.Signature, namespace, []byte(signed.Payload), keys)
	fingerprints := make([]string, 0, len(signers))
	for _, signer := range signers {
		fingerprints = append(fingerprints, Fingerprint(signer.Public))
	}
	return fingerprints, err
}

// ID is the chain's identity.
func (chain Chain) ID() string { return chain.id }

// Kind is which root the chain is.
func (chain Chain) Kind() ChainKind { return chain.kind }

// Current is the set the chain stands at; none for a chain never made.
func (chain Chain) Current() Root {
	if len(chain.roots) == 0 {
		return Root{}
	}
	return chain.roots[len(chain.roots)-1]
}

// Version is the version the chain stands at.
func (chain Chain) Version() int64 { return chain.Current().Version }

// Statements are every statement accepted, as received, from the genesis.
func (chain Chain) Statements() []Signed { return slices.Clone(chain.accepted) }

// Hash is the current statement's payload's SHA-256, in hexadecimal: what a core's heartbeat names
// with its version, so the operator sees two cores holding different statements of one version (the
// audit's F17).
func (chain Chain) Hash() string {
	if len(chain.accepted) == 0 {
		return ""
	}
	digest := sha256.Sum256([]byte(chain.accepted[len(chain.accepted)-1].Payload))
	return hex.EncodeToString(digest[:])
}

// Fingerprints are the fingerprints of the current set's keys whose signatures count, each with its
// role: a suspended key is not among them (D394 §4).
func (chain Chain) Fingerprints() []string {
	current := chain.Current()
	var shown []string
	for _, key := range current.Keys {
		if key.Public != nil && !current.IsSuspended(key) {
			shown = append(shown, string(key.Role)+" "+key.Fingerprint())
		}
	}
	return shown
}

// Verify is whether signed is a signature of the current set's, everyday or recovery, over its
// payload in namespace: how an address document or a release record is checked (§3.5).
func (chain Chain) Verify(namespace string, signed Signed) error {
	current := chain.Current()
	keys := append(current.usable(RoleEveryday, current.Removed), current.usable(RoleRecovery, current.Removed)...)
	_, err := verifyBlocks(signed.Signature, namespace, []byte(signed.Payload), keys)
	return err
}

// Followed is what following a chain did: how many statements it accepted, and the versions at which
// two valid statements conflict, which every admin is told of (§3.4 step 8).
type Followed struct {
	Accepted  int
	Conflicts []int64
}

// Follow is the chain with the statements among candidates its rules accept, from its current version
// on, as far as they lead (§3.4): the next version, signed by an everyday key of the set when it changes
// the everyday tier alone, else by a recovery key; or any later version, skipping, by a recovery key
// alone. Two valid statements of one version are a conflict: a recovery key's wins over an everyday
// key's; otherwise both are refused, the chain stays, and a recovery key's statement of a later
// version resolves it. A statement differing from one already accepted at its version, and valid
// there, is a conflict too.
func (chain Chain) Follow(candidates []Signed) (Chain, Followed) {
	followed := Followed{Conflicts: chain.equivocations(candidates)}
	for {
		next, conflict := chain.choose(candidates)
		if conflict != 0 {
			followed.Conflicts = append(followed.Conflicts, conflict)
		}
		if next == nil {
			return chain, followed
		}
		root, _, err := chain.judge(*next)
		if err != nil {
			return chain, followed
		}
		chain = chain.accept(*next, root)
		followed.Accepted++
	}
}

// valid is one candidate the chain's rules accept now, with its statement and whether a recovery key
// signed it.
type valid struct {
	signed     Signed
	root       Root
	byRecovery bool
}

// choose is the candidate to accept next, if any, and the version of a conflict found on the way.
func (chain Chain) choose(candidates []Signed) (*Signed, int64) {
	byVersion := map[int64][]valid{}
	for _, candidate := range candidates {
		root, byRecovery, err := chain.judge(candidate)
		if err == nil {
			byVersion[root.Version] = appendDistinct(byVersion[root.Version], valid{candidate, root, byRecovery})
		}
	}
	next := chain.Version() + 1
	var conflict int64
	if found := byVersion[next]; len(found) > 0 {
		if winner, isOne := winnerOf(found); isOne {
			return &winner.signed, 0
		}
		conflict = next
	}
	// A skip, a recovery key's alone: the highest version one signed, if it is one statement.
	versions := make([]int64, 0, len(byVersion))
	for version := range byVersion {
		if version > next {
			versions = append(versions, version)
		}
	}
	slices.Sort(versions)
	for _, version := range slices.Backward(versions) {
		if winner, isOne := winnerOf(byVersion[version]); isOne {
			return &winner.signed, conflict
		}
		conflict = version
	}
	return nil, conflict
}

// appendDistinct adds one to found unless its payload is there already, where the copy a recovery
// key signed is kept.
func appendDistinct(found []valid, one valid) []valid {
	for index, held := range found {
		if held.signed.Payload == one.signed.Payload {
			if one.byRecovery && !held.byRecovery {
				found[index] = one
			}
			return found
		}
	}
	return append(found, one)
}

// winnerOf is the one statement of found to take: the only one, or the only one a recovery key
// signed; none when two remain.
func winnerOf(found []valid) (valid, bool) {
	if len(found) == 1 {
		return found[0], true
	}
	recovered := slices.DeleteFunc(slices.Clone(found), func(one valid) bool { return !one.byRecovery })
	if len(recovered) == 1 {
		return recovered[0], true
	}
	return valid{}, false
}

// judge is candidate's statement once the chain's rules take it after the current set, and whether a
// recovery key signed it (§3.4 steps 2–6).
func (chain Chain) judge(candidate Signed) (Root, bool, error) {
	return judgeAfter(chain.kind, chain.id, chain.Current(), candidate)
}

// judgeAfter is judge after the set current.
func judgeAfter(kind ChainKind, id string, current Root, candidate Signed) (Root, bool, error) {
	next, err := ParseRoot([]byte(candidate.Payload))
	switch {
	case err != nil:
		return Root{}, false, err
	case next.Chain != kind || next.ID != id:
		return Root{}, false, ErrNotTheChain
	case next.Version <= current.Version:
		return Root{}, false, ErrNotNext
	}
	message := []byte(candidate.Payload)
	namespace := kind.RootNamespace()
	// A block by a key of no role signs nothing: verifyBlocks' error is only that none did.
	recovery, _ := verifyBlocks( //nolint:errcheck // as said
		candidate.Signature, namespace, message, current.usable(RoleRecovery, current.Removed))
	everyday, _ := verifyBlocks( //nolint:errcheck // as said
		candidate.Signature, namespace, message, current.usable(RoleEveryday, current.Removed))
	byRecovery := len(recovery) >= current.Thresholds[RoleRecovery]
	byEveryday := len(everyday) >= current.Thresholds[RoleEveryday] && next.Version == current.Version+1 &&
		!current.changesRecoveryTier(next)
	if !byRecovery && !byEveryday {
		return Root{}, false, ErrNotAllowed
	}
	if removedErr := keepsRemoved(current, next); removedErr != nil {
		return Root{}, false, removedErr
	}
	role, signers := RoleRecovery, recovery
	if !byRecovery {
		role, signers = RoleEveryday, everyday
	}
	if !slices.ContainsFunc(signers, func(signer Key) bool { return next.roleOf(signer) != "" }) {
		// The new set signs too, so a set no one holds is never accepted (§3.4 step 6).
		held, _ := verifyBlocks( //nolint:errcheck // none held is the refusal below
			candidate.Signature, namespace, message, next.usable(role, next.Removed))
		if len(held) < next.Thresholds[role] {
			return Root{}, false, ErrNotHeldByNewSet
		}
	}
	return next, byRecovery, nil
}

// keepsRemoved is whether next's removed keys hold every key current removed, and every key of
// current that next does not hold (§3.4 step 4).
func keepsRemoved(current, next Root) error {
	for _, fingerprint := range current.Removed {
		if !slices.Contains(next.Removed, fingerprint) {
			return fmt.Errorf("%w: %s", ErrRemovedReturned, fingerprint)
		}
	}
	for _, key := range current.Keys {
		isHeld := slices.ContainsFunc(next.Keys, func(held RootKey) bool { return held.Text == key.Text })
		if key.Public != nil && !isHeld && !slices.Contains(next.Removed, key.Fingerprint()) {
			return fmt.Errorf("%w: %s leaves the set without being named removed", ErrRemovedReturned,
				key.Fingerprint())
		}
	}
	return nil
}

// accept is the chain with a statement judged good after its current set.
func (chain Chain) accept(signed Signed, root Root) Chain {
	chain.accepted = append(slices.Clone(chain.accepted), signed)
	chain.roots = append(slices.Clone(chain.roots), root)
	return chain
}

// equivocations are the versions already accepted for which a candidate holds another statement the
// rules would have taken there.
func (chain Chain) equivocations(candidates []Signed) []int64 {
	var conflicts []int64
	for _, candidate := range candidates {
		root, err := ParseRoot([]byte(candidate.Payload))
		if err != nil {
			continue
		}
		for index := 1; index < len(chain.roots); index++ {
			if chain.roots[index].Version != root.Version ||
				bytes.Equal([]byte(chain.accepted[index].Payload), []byte(candidate.Payload)) {
				continue
			}
			if _, _, judgeErr := judgeAfter(chain.kind, chain.id, chain.roots[index-1], candidate); judgeErr == nil &&
				!slices.Contains(conflicts, root.Version) {
				conflicts = append(conflicts, root.Version)
			}
		}
	}
	return conflicts
}
