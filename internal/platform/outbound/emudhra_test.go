// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// chain is a root, an intermediate and a leaf for example.com, the intermediate's key given.
type chain struct {
	Root, Intermediate, Leaf *x509.Certificate
}

// newChain makes a chain whose intermediate holds intermediateKey: a sound root and leaf.
func newChain(t *testing.T, intermediateKey *ecdsa.PrivateKey) chain {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authority := func(serial int64, name string) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, NotBefore: madeAt.Add(-time.Hour),
			NotAfter: madeAt.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
			KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		}
	}
	sign := func(template, parent *x509.Certificate, key *ecdsa.PublicKey, signer *ecdsa.PrivateKey) *x509.Certificate {
		der, err := x509.CreateCertificate(rand.Reader, template, parent, key, signer)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	rootTemplate := authority(1, "Test Root")
	root := sign(rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	intermediate := sign(authority(2, "EM OV TLS CA - G3C"), root, &intermediateKey.PublicKey, rootKey)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "example.com"}, DNSNames: []string{"example.com"},
		NotBefore: madeAt.Add(-time.Hour), NotAfter: madeAt.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leaf := sign(leafTemplate, intermediate, &leafKey.PublicKey, intermediateKey)
	return chain{Root: root, Intermediate: intermediate, Leaf: leaf}
}

// recoverableKey is the P-384 key whose private scalar is scalar, as eMudhra's G3C OV CA's was 4.
func recoverableKey(t *testing.T, scalar byte) *ecdsa.PrivateKey {
	t.Helper()
	raw := make([]byte, 48)
	raw[47] = scalar
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P384(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// The Go this is built with refuses any chain through eMudhra's eight intermediate CAs, whose
// private keys were the numbers 4, 56, 66, 115, 155, 156, 230 and 237 on P-384 (CVE-2026-111943,
// fixed in Go 1.26.10 and 1.27.3): whoever has the keys, anyone, makes a certificate for any name,
// and Go checks neither revocation nor Certificate Transparency, while the system's trust store
// still trusts the chains through Sectigo's AAA Certificate Services. A chain whose intermediate
// holds one of those keys, which is that CA's public key whatever the root, does not verify; a
// sound one does (the owner, 2026-10-21: "Make sure we are not vulnerable to this").
func TestGoRefusesEMudhrasEightCompromisedCAs(t *testing.T) {
	t.Parallel()
	for _, scalar := range []byte{4, 56, 66, 115, 155, 156, 230, 237} {
		if _, err := verify(newChain(t, recoverableKey(t, scalar))); err == nil {
			t.Errorf("a chain through the CA whose key is %d verifies; build with Go 1.26.10 or later", scalar)
		}
	}
	sound, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verify(newChain(t, sound)); err != nil {
		t.Errorf("a chain of sound keys: %v; want it verified", err)
	}
}

// madeAt is when a test's certificates begin; each lasts an hour either side.
var madeAt = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// verify is chain verified as a client of example.com verifies it at madeAt, trusting its root.
func verify(built chain) ([][]*x509.Certificate, error) {
	roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(built.Root)
	intermediates.AddCert(built.Intermediate)
	return built.Leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, DNSName: "example.com", CurrentTime: madeAt,
	})
}
