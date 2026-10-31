// SPDX-License-Identifier: AGPL-3.0-or-later

package trusttest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// Issued is when the tests' statements are issued.
func Issued() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }

// The flags a hardware key sets in a signature: user-present, and user-verified once its PIN is given.
const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
)

// application is the application the owner's keys are made with (`-O application=ssh:orpheum-root`).
const application = "ssh:orpheum-root"

// Key is a root key made for a test: its signer, its label, and whether its key line marks it as one
// whose signatures must be user-verified.
type Key struct {
	ssh.Signer
	Label        string
	MustVerifyUV bool
}

// HardwareKey is a YubiKey's Ed25519 key, made in software, whose signatures carry the user-verified
// flag, as one made with `-O verify-required` signs with its PIN.
func HardwareKey(tb testing.TB, label string) Key {
	tb.Helper()
	return Key{Signer: newSKSigner(tb, flagUserPresent|flagUserVerified), Label: label, MustVerifyUV: true}
}

// HardwareKeyWithoutPIN is a hardware key marked uv whose signatures lack the user-verified flag: a
// key made without `-O verify-required`, or a forged signature that skipped the PIN.
func HardwareKeyWithoutPIN(tb testing.TB, label string) Key {
	tb.Helper()
	return Key{Signer: newSKSigner(tb, flagUserPresent), Label: label, MustVerifyUV: true}
}

// WithoutPIN is the same hardware key signing without its PIN: its signatures lack the user-verified
// flag, as a thief's would who has the key but not its PIN.
func (key Key) WithoutPIN() Key {
	hardware, isHardware := key.Signer.(*skSigner)
	if !isHardware {
		return key
	}
	copied := *hardware
	copied.flags = flagUserPresent
	key.Signer = &copied
	return key
}

// PlainKey is a plain Ed25519 key, as the paper recovery key or an operator without a hardware key
// has, marked "-".
func PlainKey(tb testing.TB, label string) Key {
	tb.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		tb.Fatal(err)
	}
	return Key{Signer: signer, Label: label}
}

// RootKey is the key as a root statement holds it, in role.
func (key Key) RootKey(role trust.Role) trust.RootKey {
	return trust.RootKey{
		Role: role, MustVerifyUV: key.MustVerifyUV, Public: key.PublicKey(),
		Text: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key.PublicKey()))), Label: key.Label,
	}
}

// Sign is payload signed in namespace by each of signers, their blocks one file.
func Sign(tb testing.TB, namespace string, payload []byte, signers ...Key) trust.Signed {
	tb.Helper()
	var file strings.Builder
	for _, signer := range signers {
		block, err := trust.Sign(signer, namespace, payload)
		if err != nil {
			tb.Fatal(err)
		}
		file.WriteString(block)
	}
	return trust.Signed{Payload: string(payload), Signature: file.String()}
}

// Chain is a root chain a test signs: its kind, every statement in order, and the last as it stands.
type Chain struct {
	Kind       trust.ChainKind
	Statements []trust.Signed
	Last       trust.Root
}

// ID is the chain's identity, its genesis' hash.
func (chain *Chain) ID() string {
	return trust.IdentityOf(chain.Kind, []byte(chain.Statements[0].Payload))
}

// NewChain is a chain of kind whose genesis, version 1, holds everyday and recovery keys, thresholds
// 1, signed by the first everyday key.
func NewChain(tb testing.TB, kind trust.ChainKind, everyday, recovery []Key, locations ...string) *Chain {
	tb.Helper()
	root := trust.Root{
		Chain: kind, Version: 1, Issued: Issued(), Locations: locations,
		Thresholds: map[trust.Role]int{trust.RoleEveryday: 1, trust.RoleRecovery: 1},
	}
	for _, key := range recovery {
		root.Keys = append(root.Keys, key.RootKey(trust.RoleRecovery))
	}
	for _, key := range everyday {
		root.Keys = append(root.Keys, key.RootKey(trust.RoleEveryday))
	}
	chain := &Chain{Kind: kind, Last: root}
	chain.Statements = append(chain.Statements, SignRoot(tb, kind, root, everyday[0]))
	return chain
}

// NewDiscovery is a discovery's chain of two hardware keys, everyday e1 and recovery r1, and the keys.
func NewDiscovery(tb testing.TB, locations ...string) (chain *Chain, everyday, recovery Key) {
	tb.Helper()
	everyday, recovery = HardwareKey(tb, "e1"), HardwareKey(tb, "r1")
	return NewChain(tb, trust.ChainDiscovery, []Key{everyday}, []Key{recovery}, locations...), everyday, recovery
}

// SignRoot is root's payload signed by signers in kind's namespace.
func SignRoot(tb testing.TB, kind trust.ChainKind, root trust.Root, signers ...Key) trust.Signed {
	tb.Helper()
	payload, err := root.Payload()
	if err != nil {
		tb.Fatal(err)
	}
	return Sign(tb, kind.RootNamespace(), payload, signers...)
}

// Change is a root statement's change: keys added in their roles, keys removed, and keys suspended
// and resumed.
type Change struct {
	AddEveryday, AddRecovery []Key
	Remove                   []Key
	Suspend, Resume          []Key
	Thresholds               map[trust.Role]int
}

// Next is the statement of version after the last with change made, every key removed named in its
// removed lines, signed by signers; it does not join the chain's statements until Take.
func (chain *Chain) Next(tb testing.TB, version int64, change Change, signers ...Key) (trust.Signed, trust.Root) {
	tb.Helper()
	root := chain.Last
	root.ID, root.Version, root.Issued = chain.ID(), version, Issued().Add(time.Duration(version)*time.Hour)
	root.Keys, root.Removed = slices.Clone(root.Keys), slices.Clone(root.Removed)
	root.Suspended = slices.Clone(root.Suspended)
	for _, key := range change.Suspend {
		root.Suspended = append(root.Suspended, trust.Fingerprint(key.PublicKey()))
	}
	for _, key := range change.Resume {
		fingerprint := trust.Fingerprint(key.PublicKey())
		root.Suspended = slices.DeleteFunc(root.Suspended, func(held string) bool { return held == fingerprint })
	}
	if change.Thresholds != nil {
		root.Thresholds = change.Thresholds
	}
	for _, key := range change.Remove {
		text := key.RootKey("").Text
		root.Keys = slices.DeleteFunc(root.Keys, func(held trust.RootKey) bool { return held.Text == text })
		fingerprint := trust.Fingerprint(key.PublicKey())
		root.Removed = append(root.Removed, fingerprint)
		root.Suspended = slices.DeleteFunc(root.Suspended, func(held string) bool { return held == fingerprint })
	}
	for _, key := range change.AddRecovery {
		root.Keys = append(root.Keys, key.RootKey(trust.RoleRecovery))
	}
	for _, key := range change.AddEveryday {
		root.Keys = append(root.Keys, key.RootKey(trust.RoleEveryday))
	}
	return SignRoot(tb, chain.Kind, root, signers...), root
}

// Take adds a statement Next made to the chain, as the owner publishes it.
func (chain *Chain) Take(signed trust.Signed, root trust.Root) {
	chain.Statements = append(chain.Statements, signed)
	chain.Last = root
}

// Follower is the chain as a core holds it from its genesis alone, for a test to have it follow.
func (chain *Chain) Follower(tb testing.TB) trust.Chain {
	tb.Helper()
	held, err := trust.Genesis(chain.Kind, chain.Statements[0])
	if err != nil {
		tb.Fatal(err)
	}
	return held
}

// skSigner is a hardware key's Ed25519 made in software (sk-ssh-ed25519@openssh.com): the signature
// is over the application's hash, the flags, the counter and the message's hash (OpenSSH's
// PROTOCOL.u2f).
type skSigner struct {
	private ed25519.PrivateKey
	public  ssh.PublicKey
	flags   byte
	counter uint32
}

func newSKSigner(tb testing.TB, flags byte) *skSigner {
	tb.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	wire := ssh.Marshal(struct {
		Type        string
		Key         []byte
		Application string
	}{ssh.KeyAlgoSKED25519, public, application})
	parsed, err := ssh.ParsePublicKey(wire)
	if err != nil {
		tb.Fatal(err)
	}
	return &skSigner{private: private, public: parsed, flags: flags}
}

// PublicKey is the key's public half, sk-ssh-ed25519@openssh.com.
func (key *skSigner) PublicKey() ssh.PublicKey { return key.public }

// Sign signs data as a hardware key does, its counter one more each time.
func (key *skSigner) Sign(_ io.Reader, data []byte) (*ssh.Signature, error) {
	key.counter++
	applicationHash := sha256.Sum256([]byte(application))
	message := sha256.Sum256(data)
	counter := binary.BigEndian.AppendUint32(nil, key.counter)
	signed := slices.Concat(applicationHash[:], []byte{key.flags}, counter, message[:])
	return &ssh.Signature{
		Format: ssh.KeyAlgoSKED25519, Blob: ed25519.Sign(key.private, signed),
		Rest: append([]byte{key.flags}, counter...),
	}, nil
}
