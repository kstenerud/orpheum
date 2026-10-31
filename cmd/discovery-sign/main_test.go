// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// signingAt is when the tests sign.
var signingAt = time.UnixMilli(1_800_000_000_000)

// yubiKey is a key loaded into an agent, as `ssh-add -K` loads a YubiKey's resident key: the agent
// holding it, and its public half's line.
type yubiKey struct {
	agent agent.Agent
	line  string
}

func newYubiKey(t *testing.T) yubiKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err = keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return yubiKey{agent: keyring, line: trust.KeyLine(signer.PublicKey())}
}

// sign runs the helper with key plugged in, and fails the test unless it succeeds.
func (key yubiKey) sign(t *testing.T, arguments ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if status := run(arguments, signingAt, key.agent.Signers, &stdout, &stderr); status != exitSuccess {
		t.Fatalf("discovery-sign %v = %d: %s", arguments, status, stderr.String())
	}
	return stdout.String()
}

// refused runs the helper with key plugged in, and fails the test unless it is refused saying want.
func (key yubiKey) refused(t *testing.T, want string, arguments ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if status := run(arguments, signingAt, key.agent.Signers, &stdout, &stderr); status != exitFailure ||
		!strings.Contains(stderr.String(), want) {
		t.Errorf("discovery-sign %v = %d, %q; want refused saying %q", arguments, status, stderr.String(), want)
	}
}

func readJSON(t *testing.T, file string, into any) {
	t.Helper()
	contents, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(contents, into); err != nil {
		t.Fatal(err)
	}
}

// The owner makes the first root key set of three YubiKeys with any one of them, and a later one
// that removes a lost key with any other; a key no longer in the set signs nothing more, and the
// chain a core catches up through holds (D391 §2).
func TestRootSet_AnyOneKeyMakesTheNextSetAndARemovedKeySignsNoMore(t *testing.T) {
	t.Parallel()
	first, second, third := newYubiKey(t), newYubiKey(t), newYubiKey(t)
	sets := filepath.Join(t.TempDir(), "root-sets.json")
	said := first.sign(t, "root-set", "--sets", sets, "--add", first.line, "--add", second.line, "--add", third.line)
	if !strings.Contains(said, "root key set 1, of 3 keys") {
		t.Errorf("the first set: %q", said)
	}
	// The first key lost: the second removes it.
	second.sign(t, "root-set", "--sets", sets, "--remove", first.line)
	var chain []trust.Signed
	readJSON(t, sets, &chain)
	pinned, err := trust.FirstUse(chain)
	if err != nil || pinned.Version != 2 || len(pinned.Keys) != 2 {
		t.Fatalf("the chain reaches %+v, %v; want version 2 of two keys", pinned, err)
	}
	first.refused(t, "none of the keys at hand", "root-set", "--sets", sets, "--add", first.line)
	// A key outside the set cannot start a set of its own in the chain either.
	stranger := newYubiKey(t)
	stranger.refused(t, "none of the keys at hand", "root-set", "--sets", sets, "--remove", second.line)
}

// A document the helper signs, with the chain of sets beside it, is one a core opens for its issuer;
// a delegation and a release record it signs hold too, each by a root key (D391 §3, §6).
func TestDocument_WhatTheHelperSignsACoreOpens(t *testing.T) {
	t.Parallel()
	key := newYubiKey(t)
	directory := t.TempDir()
	sets := filepath.Join(directory, "root-sets.json")
	key.sign(t, "root-set", "--sets", sets, "--add", key.line)
	served := filepath.Join(directory, "orpheum-discovery.json")
	key.sign(t, "document", "--sets", sets, "--out", served, "--issuer", "discovery.example",
		"--core-api", "https://api.discovery.example/core/v1",
		"--client-api", "https://api.discovery.example/client/v1",
		"--loader", "https://discovery.example", "--client-origins", "https://{core}.client.example",
		"--location", "https://mirror.example/discovery")
	var document addressdoc.Served
	readJSON(t, served, &document)
	var chain []trust.Signed
	readJSON(t, sets, &chain)
	pinned, err := trust.FirstUse(chain)
	if err != nil {
		t.Fatal(err)
	}
	opened, _, err := addressdoc.Open(document, pinned, "https://discovery.example", signingAt)
	if err != nil || opened.IssuedAt != signingAt.UnixMilli() || len(opened.Locations) != 1 {
		t.Fatalf("the document opens as %+v, %v", opened, err)
	}

	_, online, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	onlineSigner, err := ssh.NewSignerFromKey(online)
	if err != nil {
		t.Fatal(err)
	}
	delegation := filepath.Join(directory, "delegation.json")
	key.sign(t, "delegation", "--sets", sets, "--issuer", "discovery.example", "--key",
		trust.KeyLine(onlineSigner.PublicKey()), "--for", "2160h", "--out", delegation)
	var delegated trust.Signed
	readJSON(t, delegation, &delegated)
	byOnline, err := trust.Sign(onlineSigner, trust.KindAddressDocument, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	byOnline.Delegation = &delegated
	if _, err = pinned.OpenDocument(byOnline, "https://discovery.example", signingAt.Add(time.Hour)); err != nil {
		t.Errorf("a document the delegated key signed: %v", err)
	}

	unsigned := filepath.Join(directory, "release.json")
	record := release.Record{
		IssuedAt: 1, Latest: "v1.0.0",
		Releases: map[string]release.Note{"v1.0.0": {Digest: "sha256:" + strings.Repeat("a", 64)}},
	}
	if err = release.Write(unsigned, record); err != nil {
		t.Fatal(err)
	}
	signedRecord := filepath.Join(directory, "release.signed.json")
	key.sign(t, "record", "--sets", sets, "--record", unsigned, "--out", signedRecord)
	var sealed trust.Signed
	readJSON(t, signedRecord, &sealed)
	if opened, openErr := release.Open(sealed, pinned); openErr != nil || opened.Latest != "v1.0.0" {
		t.Errorf("the signed record opens as %+v, %v", opened, openErr)
	}
}

// A record signed by hand with ssh-keygen is sealed only when its signature holds under the chain.
func TestSeal_TakesAHandMadeSignatureOnlyWhenItHolds(t *testing.T) {
	t.Parallel()
	key := newYubiKey(t)
	directory := t.TempDir()
	sets := filepath.Join(directory, "root-sets.json")
	key.sign(t, "root-set", "--sets", sets, "--add", key.line)
	payload := filepath.Join(directory, "release.json")
	if err := release.Write(payload, release.Record{
		IssuedAt: 1, Latest: "v1.0.0",
		Releases: map[string]release.Note{"v1.0.0": {Digest: "sha256:" + strings.Repeat("b", 64)}},
	}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	signers, err := key.agent.Signers()
	if err != nil {
		t.Fatal(err)
	}
	byHand, err := trust.Sign(signers[0], trust.KindReleaseRecord, contents)
	if err != nil {
		t.Fatal(err)
	}
	signature := payload + ".sig"
	if err = os.WriteFile(signature, []byte(byHand.Signature), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(directory, "release.signed.json")
	key.sign(t, "seal", "--sets", sets, "--kind", string(trust.KindReleaseRecord), "--payload", payload,
		"--signature", signature, "--out", out)
	if err = os.WriteFile(payload, append(contents, ' '), 0o600); err != nil { //nolint:gosec // the test's own file
		t.Fatal(err)
	}
	key.refused(t, "does not hold", "seal", "--sets", sets, "--kind", string(trust.KindReleaseRecord),
		"--payload", payload, "--signature", signature, "--out", out)
}
