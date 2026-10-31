// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ChainKind is which root a chain is: a discovery's, which signs its address documents, or the
// publisher's, Orpheum's own, which signs release records (D393 §2).
type ChainKind string

// The two kinds of chain.
const (
	ChainDiscovery ChainKind = "discovery"
	ChainPublisher ChainKind = "publisher"
)

// The SSH signature namespaces, one per type of object (the root key audit's §3.1). A verifier takes
// the namespace from the type it expects, never from the signature.
const (
	NamespaceDiscoveryRoot    = "orpheum-discovery-root-v1"
	NamespaceDiscoveryAddress = "orpheum-discovery-address-v1"
	NamespacePublisherRoot    = "orpheum-publisher-root-v1"
	NamespaceRelease          = "orpheum-release-v1"
	// NamespaceFreshness is reserved for an online key's freshness statement, which is not built
	// (the audit's F15; D393 §9); NamespaceDrill is a yearly drill's, which nothing accepts.
	NamespaceFreshness = "orpheum-discovery-freshness-v1"
	NamespaceDrill     = "orpheum-drill-v1"
)

// RootNamespace is the namespace of kind's root statements.
func (kind ChainKind) RootNamespace() string {
	if kind == ChainPublisher {
		return NamespacePublisherRoot
	}
	return NamespaceDiscoveryRoot
}

// idPrefix is the start of kind's ids: od1- for a discovery, op1- for a publisher.
func (kind ChainKind) idPrefix() string {
	if kind == ChainPublisher {
		return "op1-"
	}
	return "od1-"
}

// Role is a root key's tier (D393 §1): everyday keys sign documents, records and changes of the
// everyday tier; recovery keys sign anything, and alone change the recovery tier, the thresholds and
// the roles, or skip versions.
type Role string

// The two roles.
const (
	RoleEveryday Role = "everyday"
	RoleRecovery Role = "recovery"
)

// RootKey is one key line of a root statement: its role, whether its signatures must be
// user-verified, the key, and its neutral label. A key of a type this build does not know is kept,
// its Public nil, and signs nothing here.
type RootKey struct {
	Role         Role
	MustVerifyUV bool
	Public       ssh.PublicKey
	// Text is the key as written, "<type> <base64>"; Label the line's comment, "e1".
	Text, Label string
}

// Fingerprint is the key's SHA-256 fingerprint, as `ssh-keygen -l` shows it.
func (key RootKey) Fingerprint() string {
	if key.Public == nil {
		return ""
	}
	return Fingerprint(key.Public)
}

// Root is a root statement (the audit's §3.1): its chain and id (none in the genesis, whose hash is
// the id), its version, when it was issued, its keys, each role's threshold, every key ever removed,
// and where the chain is served.
type Root struct {
	Chain      ChainKind
	ID         string
	Version    int64
	Issued     time.Time
	Keys       []RootKey
	Thresholds map[Role]int
	Removed    []string
	Locations  []string
}

// rootSchema is the root statement's lines, format 1. A "delegate" line is reserved for an online
// key's delegation, not built (D393 §9): a core that does not know it ignores it.
func rootSchema() Schema {
	return Schema{Type: "root", Format: 1, Fields: []Field{
		{Name: "chain", IsRequired: true},
		{Name: "id"},
		{Name: "version", IsRequired: true},
		{Name: "issued", IsRequired: true},
		{Name: "key", IsRepeated: true, IsRequired: true},
		{Name: "threshold", IsRepeated: true, IsRequired: true},
		{Name: "removed", IsRepeated: true},
		{Name: "location", IsRepeated: true},
	}}
}

// ParseRoot reads a root statement's payload.
func ParseRoot(payload []byte) (Root, error) {
	lines, err := rootSchema().Parse(payload)
	if err != nil {
		return Root{}, err
	}
	root := Root{Chain: ChainKind(lines.One("chain")), ID: lines.One("id"), Thresholds: map[Role]int{}}
	if root.Chain != ChainDiscovery && root.Chain != ChainPublisher {
		return Root{}, fmt.Errorf("%w: chain %q", ErrBadValue, root.Chain)
	}
	if root.Version, err = ParseCount(lines.One("version")); err != nil {
		return Root{}, err
	}
	if root.Issued, err = ParseTime(lines.One("issued")); err != nil {
		return Root{}, err
	}
	for _, line := range lines["key"] {
		key, keyErr := parseRootKey(line)
		if keyErr != nil {
			return Root{}, keyErr
		}
		root.Keys = append(root.Keys, key)
	}
	for _, line := range lines["threshold"] {
		role, count, isCut := strings.Cut(line, " ")
		number, countErr := strconv.Atoi(count)
		if !isCut || countErr != nil || number < 1 {
			return Root{}, fmt.Errorf("%w: threshold %q", ErrBadValue, line)
		}
		root.Thresholds[Role(role)] = number
	}
	root.Removed, root.Locations = lines["removed"], lines["location"]
	return root, root.check()
}

// check is whether the statement can sign anything: a key and a threshold for each role, and no key
// it holds among those it removed.
func (root Root) check() error {
	for _, role := range []Role{RoleEveryday, RoleRecovery} {
		if root.Thresholds[role] == 0 || len(root.usable(role, nil)) == 0 {
			return fmt.Errorf("%w: the set has no %s key it can use, or no threshold for it", ErrBadValue, role)
		}
	}
	for _, key := range root.Keys {
		if key.Public != nil && slices.Contains(root.Removed, key.Fingerprint()) {
			return fmt.Errorf("%w: %s is in the set and among its removed keys", ErrBadValue, key.Fingerprint())
		}
	}
	return nil
}

// parseRootKey reads "<role> <uv|-> <type> <base64> [label]".
func parseRootKey(line string) (RootKey, error) {
	fields := strings.Split(line, " ")
	if len(fields) < 4 || len(fields) > 5 || (fields[1] != "uv" && fields[1] != "-") {
		return RootKey{}, fmt.Errorf("%w: key %q", ErrBadValue, line)
	}
	key := RootKey{Role: Role(fields[0]), MustVerifyUV: fields[1] == "uv", Text: fields[2] + " " + fields[3]}
	if len(fields) == 5 {
		key.Label = fields[4]
	}
	if public, err := ParseKey(key.Text); err == nil {
		key.Public = public
	}
	return key, nil
}

// Payload is the statement written as its lines, which a key then signs.
func (root Root) Payload() ([]byte, error) {
	lines := Lines{
		"chain": {string(root.Chain)}, "version": {strconv.FormatInt(root.Version, 10)},
		"issued": {TimeText(root.Issued)}, "removed": root.Removed, "location": root.Locations,
	}
	if root.ID != "" {
		lines["id"] = []string{root.ID}
	}
	for _, key := range root.Keys {
		mark := "-"
		if key.MustVerifyUV {
			mark = "uv"
		}
		line := string(key.Role) + " " + mark + " " + key.Text
		if key.Label != "" {
			line += " " + key.Label
		}
		lines["key"] = append(lines["key"], line)
	}
	for _, role := range []Role{RoleRecovery, RoleEveryday} {
		if count, isNamed := root.Thresholds[role]; isNamed {
			lines["threshold"] = append(lines["threshold"], string(role)+" "+strconv.Itoa(count))
		}
	}
	return rootSchema().Write(lines)
}

// usable are the role's keys this build can check a signature by, but those in removed.
func (root Root) usable(role Role, removed []string) []Key {
	var keys []Key
	for _, key := range root.Keys {
		if key.Role == role && key.Public != nil && !slices.Contains(removed, key.Fingerprint()) {
			keys = append(keys, Key{Public: key.Public, MustVerifyUV: key.MustVerifyUV})
		}
	}
	return keys
}

// roleOf is the role the statement gives key, empty when it holds no such key.
func (root Root) roleOf(key Key) Role {
	for _, held := range root.Keys {
		if held.Public != nil && sameKey(held.Public, key.Public) {
			return held.Role
		}
	}
	return ""
}

// recoveryTier is what only a recovery key may change: each key's role, the recovery keys with their
// marks, and the thresholds.
func (root Root) recoveryTier() string {
	var parts []string
	for _, key := range root.Keys {
		mark := "-"
		if key.MustVerifyUV {
			mark = "uv"
		}
		parts = append(parts, key.Text+" "+string(key.Role)+" "+mark)
	}
	recovery := slices.DeleteFunc(slices.Clone(parts), func(part string) bool {
		return strings.HasSuffix(part, " "+string(RoleEveryday)+" uv") ||
			strings.HasSuffix(part, " "+string(RoleEveryday)+" -")
	})
	slices.Sort(recovery)
	return strings.Join(recovery, ",") + "|" + fmt.Sprint(root.Thresholds[RoleRecovery], root.Thresholds[RoleEveryday])
}

// changesRecoveryTier is whether next changes what only a recovery key may: a recovery key added,
// removed or re-marked, a key moved between roles, or a threshold.
func (root Root) changesRecoveryTier(next Root) bool {
	if root.recoveryTier() != next.recoveryTier() {
		return true
	}
	for _, key := range next.Keys {
		for _, held := range root.Keys {
			if key.Text == held.Text && key.Role != held.Role {
				return true
			}
		}
	}
	return false
}

// IdentityOf is the id of the chain whose genesis payload is genesis: od1- or op1-, then the
// lowercase unpadded base32 of its SHA-256 (the audit's §3.1; D393 §4).
func IdentityOf(kind ChainKind, genesis []byte) string {
	digest := sha256.Sum256(genesis)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:])
	return kind.idPrefix() + strings.ToLower(encoded)
}
