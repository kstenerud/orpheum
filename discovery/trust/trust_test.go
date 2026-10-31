// SPDX-License-Identifier: AGPL-3.0-or-later

package trust_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
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

// A signature holds only in the namespace expected, over the exact bytes, by a key expected; one
// ssh-keygen made holds (the root key audit's §3.2).
func TestVerify_ASignatureSSHKeygenMadeHoldsAndNothingElse(t *testing.T) {
	t.Parallel()
	key, err := trust.ParseKey(vectorKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = trust.VerifyForTest(vectorSignature, "orpheum-test-v1", vectorPayload, false, key); err != nil {
		t.Fatalf("ssh-keygen's signature: %v", err)
	}
	for name, check := range map[string]error{
		"another namespace": trust.VerifyForTest(vectorSignature, trust.NamespaceRelease, vectorPayload, false, key),
		"a byte changed":    trust.VerifyForTest(vectorSignature, "orpheum-test-v1", vectorPayload+" ", false, key),
		"another key": trust.VerifyForTest(vectorSignature, "orpheum-test-v1", vectorPayload, false,
			trusttest.PlainKey(t, "x").PublicKey()),
		// A plain key cannot say its PIN was given: one that must cannot sign.
		"a user-verified key": trust.VerifyForTest(vectorSignature, "orpheum-test-v1", vectorPayload, true, key),
	} {
		if check == nil {
			t.Errorf("%s: taken", name)
		}
	}
}

// A hardware key marked uv signs only with the user-verified flag set, which Orpheum checks itself, as
// neither x/crypto nor ssh-keygen does (D393 §3); a key of a type the design does not take signs
// nothing.
func TestVerify_AKeyMarkedUVSignsOnlyWithItsPIN(t *testing.T) {
	t.Parallel()
	withPIN, withoutPIN := trusttest.HardwareKey(t, "e1"), trusttest.HardwareKeyWithoutPIN(t, "e2")
	payload := "orpheum-signed release 1\n"
	signed := trusttest.Sign(t, trust.NamespaceRelease, []byte(payload), withPIN)
	if err := trust.VerifyForTest(signed.Signature, trust.NamespaceRelease, payload, true,
		withPIN.PublicKey()); err != nil {
		t.Errorf("a signature with the PIN: %v", err)
	}
	unverified := trusttest.Sign(t, trust.NamespaceRelease, []byte(payload), withoutPIN)
	if err := trust.VerifyForTest(unverified.Signature, trust.NamespaceRelease, payload, true,
		withoutPIN.PublicKey()); !errors.Is(err, trust.ErrNotUserVerified) {
		t.Errorf("a signature without the PIN: %v; want ErrNotUserVerified", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecSigner, err := ssh.NewSignerFromKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = trust.ParseKey(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(ecSigner.PublicKey())))); !errors.Is(
		err, trust.ErrKeyType) {
		t.Errorf("a software ECDSA key: %v; want ErrKeyType", err)
	}
}

// The format is strict text: LF lines, the header first, keys in order, repeated only where they may;
// an unknown line is skipped unless it is critical, and a format number this build does not know is
// refused (§3.1).
func TestParse_TheFormatIsStrictAndAnUnknownCriticalLineRefuses(t *testing.T) {
	t.Parallel()
	schema := trust.Schema{Type: "release", Format: 1, Fields: []trust.Field{
		{Name: "id", IsRequired: true}, {Name: "image", IsRepeated: true},
	}}
	good := "orpheum-signed release 1\nid op1-x\nfuture yes\nimage a\nimage b\n"
	lines, err := schema.Parse([]byte(good))
	if err != nil || lines.One("id") != "op1-x" || len(lines["image"]) != 2 || lines["future"] != nil {
		t.Fatalf("a good payload: %v, %v", lines, err)
	}
	for text, want := range map[string]error{
		"orpheum-signed release 1\r\nid x\n":          trust.ErrNotStrict,
		"orpheum-signed release 1\nid x":              trust.ErrNotStrict,
		"orpheum-signed release 1\nid x \n":           trust.ErrNotStrict,
		"orpheum-signed release 1\n\nid x\n":          trust.ErrNotStrict,
		"orpheum-signed release 1\nid\tx\n":           trust.ErrNotStrict,
		"orpheum-signed release 2\nid x\n":            trust.ErrUnknownFormat,
		"orpheum-signed address 1\nid x\n":            trust.ErrWrongType,
		"orpheum-signed release 1\nid x\n!future y\n": trust.ErrCritical,
		"orpheum-signed release 1\nimage a\nid x\n":   trust.ErrOutOfOrder,
		"orpheum-signed release 1\nid x\nid y\n":      trust.ErrOutOfOrder,
		"orpheum-signed release 1\nimage a\n":         trust.ErrMissingLine,
	} {
		if _, parseErr := schema.Parse([]byte(text)); !errors.Is(parseErr, want) {
			t.Errorf("%q: %v; want %v", text, parseErr, want)
		}
	}
}

// A chain's id is its genesis' hash, od1- or op1-, and its genesis is signed by one of its own keys;
// a state kept is taken again only from the genesis of the id pinned (D393 §4; §3.3).
func TestGenesis_TheIdIsTheGenesisHashAndAStateIsCheckedAgainFromIt(t *testing.T) {
	t.Parallel()
	chain, everyday, _ := trusttest.NewDiscovery(t)
	held := chain.Follower(t)
	if !strings.HasPrefix(held.ID(), "od1-") || held.ID() != chain.ID() || len(held.ID()) != len("od1-")+52 {
		t.Errorf("the id %q", held.ID())
	}
	next, root := chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{trusttest.HardwareKey(t, "e2")}},
		everyday)
	chain.Take(next, root)
	loaded, err := trust.Load(trust.ChainDiscovery, chain.ID(), chain.Statements)
	if err != nil || loaded.Version() != 2 {
		t.Fatalf("the state loaded: %v, %v", loaded.Version(), err)
	}
	other, _, _ := trusttest.NewDiscovery(t)
	if _, err = trust.Load(trust.ChainDiscovery, other.ID(), chain.Statements); !errors.Is(
		err, trust.ErrNotTheIdentity) {
		t.Errorf("another identity's state: %v; want ErrNotTheIdentity", err)
	}
	// A state whose statement was changed after it was kept does not load.
	tampered := chain.Statements
	tampered[1].Payload = strings.Replace(tampered[1].Payload, "version 2", "version 3", 1)
	if _, err = trust.Load(trust.ChainDiscovery, chain.ID(), tampered); err == nil {
		t.Error("a tampered state loaded")
	}
	stranger := trusttest.HardwareKey(t, "x")
	forged := trusttest.SignRoot(t, trust.ChainDiscovery, chain.Last, stranger)
	forged.Payload = chain.Statements[0].Payload
	if _, err = trust.Genesis(trust.ChainDiscovery, forged); !errors.Is(err, trust.ErrNotGenesis) {
		t.Errorf("a genesis none of its keys signed: %v; want ErrNotGenesis", err)
	}
	if _, err = trust.Genesis(trust.ChainPublisher, chain.Statements[0]); !errors.Is(err, trust.ErrNotTheChain) {
		t.Errorf("a discovery's genesis as a publisher's: %v; want ErrNotTheChain", err)
	}
}
