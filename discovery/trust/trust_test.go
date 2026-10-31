// SPDX-License-Identifier: AGPL-3.0-or-later

package trust_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// The vector ssh-keygen made (OpenSSH 10.0): `ssh-keygen -t ed25519`, then `ssh-keygen -Y sign -n
// orpheum-test-v1` over vectorPayload. It proves the check reads what the owner's tool writes.
const (
	vectorKey       = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIF4IiVhrC5Sp5o4uTU5Cxqpo1hmysmvZSJqoB/vNc/9/"
	vectorPayload   = `{"version":1}`
	vectorSignature = `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAADMAAAALc3NoLWVkMjU1MTkAAAAgXgiJWGsLlKnmji5NTkLGqmjWGb
Kya9lImqgH+81z/38AAAAPb3JwaGV1bS10ZXN0LXYxAAAAAAAAAAZzaGE1MTIAAABTAAAA
C3NzaC1lZDI1NTE5AAAAQJjA7DqQj0eghiJu9ai6OYHpV9q6H8qN7XfCufPkaAYRTYhdVq
Nah3M066gqGcNm0MumInGd+OnO+onH/NzjyAA=
-----END SSH SIGNATURE-----
`
)

func TestVerify_ASignatureSSHKeygenMadeHoldsAndNothingElse(t *testing.T) {
	t.Parallel()
	pinned, err := trust.Anchor(1, []string{vectorKey})
	if err != nil {
		t.Fatal(err)
	}
	// The vector is in a namespace of its own: a release record's is checked in its own, so the same
	// bytes are refused there, and accepted by the namespace they were made for.
	if _, err = pinned.OpenRecord(trust.Signed{Payload: vectorPayload, Signature: vectorSignature}); !errors.Is(
		err, trust.ErrWrongNamespace) {
		t.Fatalf("a signature for another namespace: %v, want ErrWrongNamespace", err)
	}
	if err = trust.CheckForTest(pinned, "orpheum-test-v1", vectorPayload, vectorSignature); err != nil {
		t.Fatalf("ssh-keygen's signature: %v", err)
	}
	if err = trust.CheckForTest(pinned, "orpheum-test-v1", vectorPayload+" ", vectorSignature); !errors.Is(
		err, trust.ErrBadSignature) {
		t.Fatalf("a payload changed by a byte: %v, want ErrBadSignature", err)
	}
	other, _ := newKey(t)
	stranger, err := trust.Anchor(1, []string{trust.KeyLine(other.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	if err = trust.CheckForTest(stranger, "orpheum-test-v1", vectorPayload, vectorSignature); !errors.Is(
		err, trust.ErrUntrustedKey) {
		t.Fatalf("a key outside the set: %v, want ErrUntrustedKey", err)
	}
}

func TestSign_AHardwareKeysSignatureHoldsAndAKeyOfAnotherTypeIsRefused(t *testing.T) {
	t.Parallel()
	hardware := newSKKey(t)
	pinned, err := trust.Anchor(1, []string{trust.KeyLine(hardware.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := trust.Sign(hardware, trust.KindReleaseRecord, []byte(`{"latest":"v1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pinned.OpenRecord(signed); err != nil {
		t.Fatalf("an sk-ssh-ed25519 key's record: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecSigner, err := ssh.NewSignerFromKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = trust.Anchor(1, []string{trust.KeyLine(ecSigner.PublicKey())}); !errors.Is(err, trust.ErrKeyType) {
		t.Fatalf("a software ECDSA root key: %v, want ErrKeyType", err)
	}
	if _, err = trust.Sign(ecSigner, trust.KindReleaseRecord, []byte("{}")); !errors.Is(err, trust.ErrKeyType) {
		t.Fatalf("signing with a software ECDSA key: %v, want ErrKeyType", err)
	}
}

func TestAnchor_AnEmptySetIsRefusedNeverTrusted(t *testing.T) {
	t.Parallel()
	if _, err := trust.Anchor(1, nil); !errors.Is(err, trust.ErrNoRootKeys) {
		t.Fatalf("an empty anchor: %v, want ErrNoRootKeys", err)
	}
	if _, err := trust.Anchor(1, []string{"not a key"}); err == nil {
		t.Fatal("a line that is no key was taken")
	}
}

func TestFirstUse_TakesASelfSignedSetAndCatchesUpThroughEachLaterOne(t *testing.T) {
	t.Parallel()
	first, firstLine := newKey(t)
	second, secondLine := newKey(t)
	third, thirdLine := newKey(t)
	sets := []trust.Signed{
		rootSet(t, first, 1, firstLine, secondLine),
		// Several versions on, in any order: each signed by a key of the set before it.
		rootSet(t, third, 3, thirdLine),
		rootSet(t, second, 2, secondLine, thirdLine),
	}
	pinned, err := trust.FirstUse(sets)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Version != 3 || len(pinned.Keys) != 1 || pinned.Keys[0] != thirdLine {
		t.Fatalf("caught up to %+v, want version 3 of the third key alone", pinned)
	}
	stranger, strangerLine := newKey(t)
	if _, err = trust.FirstUse([]trust.Signed{rootSet(t, stranger, 1, firstLine)}); !errors.Is(
		err, trust.ErrNotTheFirstSet) {
		t.Fatalf("a first set its own keys did not sign: %v, want ErrNotTheFirstSet", err)
	}
	_ = strangerLine
}

func TestAdvance_ASetSignedByAKeyNoLongerInTheSetIsRefused(t *testing.T) {
	t.Parallel()
	kept, keptLine := newKey(t)
	stolen, stolenLine := newKey(t)
	pinned, err := trust.FirstUse([]trust.Signed{rootSet(t, kept, 1, keptLine, stolenLine)})
	if err != nil {
		t.Fatal(err)
	}
	// The stolen key removed by the one kept, then signing a set of its own.
	pinned, err = pinned.Advance([]trust.Signed{rootSet(t, kept, 2, keptLine)})
	if err != nil || pinned.Version != 2 {
		t.Fatalf("the removal: %+v, %v", pinned, err)
	}
	after, err := pinned.Advance([]trust.Signed{rootSet(t, stolen, 3, stolenLine)})
	if !errors.Is(err, trust.ErrUntrustedKey) || after.Version != 2 {
		t.Fatalf("a set the removed key signed: %+v, %v; want version 2 kept and ErrUntrustedKey", after, err)
	}
	// One added by a key of the set is taken.
	added, addedLine := newKey(t)
	pinned, err = pinned.Advance([]trust.Signed{rootSet(t, kept, 4, keptLine, addedLine)})
	if err != nil || len(pinned.Keys) != 2 {
		t.Fatalf("an added key: %+v, %v", pinned, err)
	}
	if _, err = pinned.OpenRecord(sign(t, added, trust.KindReleaseRecord, "{}")); err != nil {
		t.Fatalf("the added key's record: %v", err)
	}
}

func TestOpenDocument_AnOnlineKeySignsOnlyForItsIssuerAndWhileItsDelegationLasts(t *testing.T) {
	t.Parallel()
	root, rootLine := newKey(t)
	online, onlineLine := newKey(t)
	pinned, err := trust.Anchor(1, []string{rootLine})
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1_800_000_000_000)
	delegation := sign(t, root, trust.KindDelegation, mustJSON(t, trust.Delegation{
		Key: onlineLine, Issuer: "https://discovery.example", NotBefore: now.Add(-time.Hour).UnixMilli(),
		NotAfter: now.Add(time.Hour).UnixMilli(),
	}))
	document := sign(t, online, trust.KindAddressDocument, `{"issuer":"https://discovery.example"}`)
	document.Delegation = &delegation
	if _, err = pinned.OpenDocument(document, "https://discovery.example", now); err != nil {
		t.Fatalf("a document the delegated key signed: %v", err)
	}
	for name, check := range map[string]func() error{
		"another issuer": func() error {
			_, openErr := pinned.OpenDocument(document, "https://other.example", now)
			return openErr
		},
		"after it ends": func() error {
			_, openErr := pinned.OpenDocument(document, "https://discovery.example", now.Add(2*time.Hour))
			return openErr
		},
		"a record": func() error {
			_, openErr := pinned.OpenRecord(document)
			return openErr
		},
	} {
		if err = check(); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	// A delegation the online key made itself holds nothing.
	selfMade := sign(t, online, trust.KindDelegation, delegation.Payload)
	document.Delegation = &selfMade
	if _, err = pinned.OpenDocument(document, "https://discovery.example", now); !errors.Is(err, trust.ErrDelegation) {
		t.Fatalf("a delegation no root key signed: %v, want ErrDelegation", err)
	}
	// A root key signs a document itself, with no delegation.
	if _, err = pinned.OpenDocument(sign(t, root, trust.KindAddressDocument, "{}"), "https://discovery.example",
		now); err != nil {
		t.Fatalf("a document a root key signed: %v", err)
	}
}

// newKey is a plain Ed25519 key, and its public half's line.
func newKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer, trust.KeyLine(signer.PublicKey())
}

// rootSet is a statement of version's set of keys, signed by signer.
func rootSet(t *testing.T, signer ssh.Signer, version int64, keys ...string) trust.Signed {
	t.Helper()
	return sign(t, signer, trust.KindRootSet, mustJSON(t, trust.RootSet{Version: version, Keys: keys}))
}

func sign(t *testing.T, signer ssh.Signer, kind trust.Kind, payload string) trust.Signed {
	t.Helper()
	signed, err := trust.Sign(signer, kind, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// skKey is a hardware key's Ed25519 as a YubiKey signs with it (sk-ssh-ed25519@openssh.com), made in
// software: the signature is over the application's hash, the flags, the counter and the message's
// hash (OpenSSH's PROTOCOL.u2f).
type skKey struct {
	private ed25519.PrivateKey
	public  ssh.PublicKey
}

// skApplication is the application `ssh-keygen -O application=ssh:orpheum` gives a key.
const skApplication = "ssh:orpheum"

func newSKKey(t *testing.T) *skKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wire := ssh.Marshal(struct {
		Type        string
		Key         []byte
		Application string
	}{ssh.KeyAlgoSKED25519, public, skApplication})
	parsed, err := ssh.ParsePublicKey(wire)
	if err != nil {
		t.Fatal(err)
	}
	return &skKey{private: private, public: parsed}
}

func (key *skKey) PublicKey() ssh.PublicKey { return key.public }

func (key *skKey) Sign(_ io.Reader, data []byte) (*ssh.Signature, error) {
	const userPresent = 0x01
	application := sha256.Sum256([]byte(skApplication))
	message := sha256.Sum256(data)
	counter := binary.BigEndian.AppendUint32(nil, 1)
	signed := append(append(append(application[:], userPresent), counter...), message[:]...)
	return &ssh.Signature{
		Format: ssh.KeyAlgoSKED25519, Blob: ed25519.Sign(key.private, signed),
		Rest: append([]byte{userPresent}, counter...),
	}, nil
}
