// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"golang.org/x/crypto/ssh"
)

// Kind is a kind of signed object, and the SSH signature namespace it is signed in (`ssh-keygen -Y
// sign -n <kind>`): a signature made for one kind is never another's.
type Kind string

// The kinds of signed object.
const (
	// KindRootSet is a statement of the root key set: its keys and its version.
	KindRootSet Kind = "orpheum-root-set-v1"
	// KindDelegation is a root key's authorisation of an online key, for a time, to sign the address
	// documents of one issuer.
	KindDelegation Kind = "orpheum-delegation-v1"
	// KindAddressDocument is a discovery's address document (discovery/addressdoc).
	KindAddressDocument Kind = "orpheum-address-document-v1"
	// KindReleaseRecord is the release record (discovery/release).
	KindReleaseRecord Kind = "orpheum-release-record-v1"
)

// Signed is a signed object as it is served: the exact bytes signed, the SSH signature over them,
// and for an address document an online key signed, the delegation that authorises that key.
type Signed struct {
	Payload    string  `json:"payload"`
	Signature  string  `json:"signature"`
	Delegation *Signed `json:"delegation,omitempty"`
}

// RootSet is a root key set's statement, a KindRootSet's payload: its keys, as authorised keys'
// lines, its version, which only grows, and when it was made.
type RootSet struct {
	Version  int64    `json:"version"`
	IssuedAt int64    `json:"issuedAt"`
	Keys     []string `json:"keys"`
}

// Delegation is a KindDelegation's payload: the online key authorised, the issuer whose documents it
// may sign, and the Unix milliseconds it may from and until.
type Delegation struct {
	Key       string `json:"key"`
	Issuer    string `json:"issuer"`
	NotBefore int64  `json:"notBefore"`
	NotAfter  int64  `json:"notAfter"`
}

// Pinned is the root key set a core holds its discovery by, as it keeps it: the set's version and its
// keys.
type Pinned struct {
	Version int64    `json:"version"`
	Keys    []string `json:"keys"`
}

// Errors the checks return besides the signature's own.
var (
	ErrNoRootKeys     = errors.New("no root keys: the discovery's root key set is empty")
	ErrNotARootSet    = errors.New("not a root key set's statement")
	ErrNoFirstSet     = errors.New("the discovery sent no root key set")
	ErrDelegation     = errors.New("the online key's delegation does not hold")
	ErrRootKeyOnly    = errors.New("a release record is signed by a root key itself, not an online key")
	ErrNotTheFirstSet = errors.New("the first root key set is not signed by one of its own keys")
)

// Anchor is a set the core is given, not shown: the suggested discovery's, compiled in. An empty set
// is refused, never trusted as anything.
func Anchor(version int64, keys []string) (Pinned, error) {
	if len(keys) == 0 {
		return Pinned{}, ErrNoRootKeys
	}
	if _, err := parseKeys(keys); err != nil {
		return Pinned{}, err
	}
	return Pinned{Version: version, Keys: normalised(keys)}, nil
}

// FirstUse is the set a discovery shows a core enrolling with it unanchored: its lowest version,
// signed by one of its own keys, then each later one in turn as Advance takes it. The admin's
// enrolment, with their password, is what trusts it (D391 §3); the System page shows its keys'
// fingerprints to compare.
func FirstUse(sets []Signed) (Pinned, error) {
	ordered, err := readSets(sets)
	if err != nil {
		return Pinned{}, err
	}
	if len(ordered) == 0 {
		return Pinned{}, ErrNoFirstSet
	}
	first := ordered[0]
	keys, err := parseKeys(first.set.Keys)
	if err != nil {
		return Pinned{}, err
	}
	if err = verifySSH(first.signed.Signature, string(KindRootSet), []byte(first.signed.Payload), keys); err != nil {
		return Pinned{}, fmt.Errorf("%w: %w", ErrNotTheFirstSet, err)
	}
	pinned := Pinned{Version: first.set.Version, Keys: normalised(first.set.Keys)}
	return pinned.Advance(sets)
}

// Advance is the set after every statement in sets newer than this one, taken in order of version:
// each signed by a key of the set before it. A statement that does not chain stops it, the set
// reached so far returned with the error.
//
// The rotation's further rules (a removed key never trusted again, two sets of one version both
// refused) wait on the outside audit of D391 §2's design.
func (pinned Pinned) Advance(sets []Signed) (Pinned, error) {
	ordered, err := readSets(sets)
	if err != nil {
		return pinned, err
	}
	for _, each := range ordered {
		if each.set.Version <= pinned.Version {
			continue
		}
		if _, err = pinned.verifyRoot(KindRootSet, each.signed); err != nil {
			return pinned, fmt.Errorf("the root key set of version %d: %w", each.set.Version, err)
		}
		if _, err = parseKeys(each.set.Keys); err != nil {
			return pinned, fmt.Errorf("the root key set of version %d: %w", each.set.Version, err)
		}
		pinned = Pinned{Version: each.set.Version, Keys: normalised(each.set.Keys)}
	}
	return pinned, nil
}

// Fingerprints are the set's keys' fingerprints, as `ssh-keygen -l` shows each.
func (pinned Pinned) Fingerprints() []string {
	keys, err := parseKeys(pinned.Keys)
	if err != nil {
		return nil
	}
	fingerprints := make([]string, 0, len(keys))
	for _, key := range keys {
		fingerprints = append(fingerprints, Fingerprint(key))
	}
	return fingerprints
}

// OpenDocument is an address document's payload once its signature holds: by a root key of the set,
// or by an online key a root key authorised for issuer at now.
func (pinned Pinned) OpenDocument(signed Signed, issuer string, now time.Time) ([]byte, error) {
	if signed.Delegation == nil {
		return pinned.verifyRoot(KindAddressDocument, signed)
	}
	payload, err := pinned.verifyRoot(KindDelegation, *signed.Delegation)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDelegation, err)
	}
	var delegation Delegation
	if err = json.Unmarshal(payload, &delegation); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDelegation, err)
	}
	at := now.UnixMilli()
	switch {
	case delegation.Issuer != issuer:
		return nil, fmt.Errorf("%w: it is for %q, not %q", ErrDelegation, delegation.Issuer, issuer)
	case at < delegation.NotBefore || at >= delegation.NotAfter:
		return nil, fmt.Errorf("%w: it is not valid now", ErrDelegation)
	}
	online, err := ParseKey(delegation.Key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDelegation, err)
	}
	if verifyErr := verifySSH(signed.Signature, string(KindAddressDocument), []byte(signed.Payload),
		[]ssh.PublicKey{online}); verifyErr != nil {
		return nil, verifyErr
	}
	return []byte(signed.Payload), nil
}

// OpenRecord is a release record's payload once its signature holds: a root key's, never an online
// key's, since a record decides what a core may run.
func (pinned Pinned) OpenRecord(signed Signed) ([]byte, error) {
	if signed.Delegation != nil {
		return nil, ErrRootKeyOnly
	}
	return pinned.verifyRoot(KindReleaseRecord, signed)
}

// Sign signs payload as kind with signer, an online key or a test's: the signature a root key makes
// with `ssh-keygen -Y sign -n <kind>`.
func Sign(signer ssh.Signer, kind Kind, payload []byte) (Signed, error) {
	signature, err := signSSH(signer, string(kind), payload)
	if err != nil {
		return Signed{}, err
	}
	return Signed{Payload: string(payload), Signature: signature}, nil
}

// verifyRoot is signed's payload once a key of the set signed it as kind.
func (pinned Pinned) verifyRoot(kind Kind, signed Signed) ([]byte, error) {
	keys, err := parseKeys(pinned.Keys)
	if err != nil {
		return nil, err
	}
	if verifyErr := verifySSH(signed.Signature, string(kind), []byte(signed.Payload), keys); verifyErr != nil {
		return nil, verifyErr
	}
	return []byte(signed.Payload), nil
}

// readSet is one root set's statement with the set it states.
type readSet struct {
	signed Signed
	set    RootSet
}

// readSets are the statements' sets, in order of version.
func readSets(sets []Signed) ([]readSet, error) {
	read := make([]readSet, 0, len(sets))
	for _, signed := range sets {
		var set RootSet
		if err := json.Unmarshal([]byte(signed.Payload), &set); err != nil || len(set.Keys) == 0 {
			return nil, ErrNotARootSet
		}
		read = append(read, readSet{signed: signed, set: set})
	}
	slices.SortStableFunc(read, func(one, other readSet) int { return cmp.Compare(one.set.Version, other.set.Version) })
	return read, nil
}

// parseKeys are lines' keys; none is ErrNoRootKeys.
func parseKeys(lines []string) ([]ssh.PublicKey, error) {
	if len(lines) == 0 {
		return nil, ErrNoRootKeys
	}
	keys := make([]ssh.PublicKey, 0, len(lines))
	for _, line := range lines {
		key, err := ParseKey(line)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// normalised are lines as KeyLine writes them, which every line here parses as (parseKeys).
func normalised(lines []string) []string {
	keys := make([]string, 0, len(lines))
	for _, line := range lines {
		if key, err := ParseKey(line); err == nil {
			keys = append(keys, KeyLine(key))
		}
	}
	return keys
}
