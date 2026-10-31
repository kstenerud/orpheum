// SPDX-License-Identifier: AGPL-3.0-or-later

package trusttest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// Root is a discovery's one root key, a plain Ed25519 made for the test: its signer, its public
// half's line, its set's first statement and the set as a core pins it.
type Root struct {
	Signer ssh.Signer
	Line   string
	Set    trust.Signed
	Pinned trust.Pinned
}

// NewRoot is a root key and its set of one, version 1, signed by itself.
func NewRoot(tb testing.TB) Root {
	tb.Helper()
	signer, line := NewKey(tb)
	set := Sign(tb, signer, trust.KindRootSet, trust.RootSet{Version: 1, Keys: []string{line}})
	pinned, err := trust.FirstUse([]trust.Signed{set})
	if err != nil {
		tb.Fatal(err)
	}
	return Root{Signer: signer, Line: line, Set: set, Pinned: pinned}
}

// NewKey is a plain Ed25519 key's signer and its public half's line.
func NewKey(tb testing.TB) (ssh.Signer, string) {
	tb.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		tb.Fatal(err)
	}
	return signer, trust.KeyLine(signer.PublicKey())
}

// Sign is payload, as JSON, signed as kind by signer.
func Sign(tb testing.TB, signer ssh.Signer, kind trust.Kind, payload any) trust.Signed {
	tb.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		tb.Fatal(err)
	}
	signed, err := trust.Sign(signer, kind, encoded)
	if err != nil {
		tb.Fatal(err)
	}
	return signed
}

// Signs is payload signed by the root key as kind.
func (root Root) Signs(tb testing.TB, kind trust.Kind, payload any) trust.Signed {
	tb.Helper()
	return Sign(tb, root.Signer, kind, payload)
}
