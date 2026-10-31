// SPDX-License-Identifier: AGPL-3.0-or-later

package addressdoc_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

const issuer = "https://discovery.example"

func document(id string) addressdoc.Document {
	return addressdoc.Document{
		ID: id, RootVersion: 1, Sequence: 1, Issued: trusttest.Issued(),
		Expires: trusttest.Issued().Add(365 * 24 * time.Hour),
		Name:    "Discovery", Issuer: issuer, Locations: []string{issuer, "https://mirror.example/discovery"},
		CoreAPI: "https://api.discovery.example/core/v1", ClientAPI: "https://api.discovery.example/client/v1",
		LoaderOrigin: issuer, ClientOrigins: "https://{core}.client.example", Homepage: "https://about.example",
	}
}

// sign is the document's payload signed by key in the address namespace.
func sign(t *testing.T, made addressdoc.Document, key trusttest.Key) trust.Signed {
	t.Helper()
	payload, err := made.Payload()
	if err != nil {
		t.Fatal(err)
	}
	return trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, key)
}

func TestIssuer_ANameTypedIsItsHTTPSOriginAndAnythingButAnOriginIsRefused(t *testing.T) {
	t.Parallel()
	for typed, want := range map[string]string{
		"discovery.example":           "https://discovery.example",
		"Discovery.Example/":          "https://discovery.example",
		"https://discovery.example":   "https://discovery.example",
		"http://localhost:8080":       "http://localhost:8080",
		"http://127.0.0.1:9000":       "http://127.0.0.1:9000",
		"https://discovery.example:8": "https://discovery.example:8",
	} {
		if got, err := addressdoc.Issuer(typed); err != nil || got != want {
			t.Errorf("Issuer(%q) = %q, %v; want %q", typed, got, err, want)
		}
	}
	for _, typed := range []string{
		"http://discovery.example", "https://discovery.example/core/v1", "https://discovery.example?x",
		"https://user@discovery.example", "ftp://discovery.example", "",
	} {
		if got, err := addressdoc.Issuer(typed); !errors.Is(err, addressdoc.ErrNotAnIssuer) {
			t.Errorf("Issuer(%q) = %q, %v; want ErrNotAnIssuer", typed, got, err)
		}
	}
}

// A document is taken only signed by a key of the chain's current set in the address namespace,
// naming the chain's id and a root version it has reached, for exactly the issuer asked; its lines
// read back as written (the root key audit's §3.1, §3.5).
func TestOpen_ADocumentIsTheChainsTheAskedIssuersAndNoOtherIsTaken(t *testing.T) {
	t.Parallel()
	chain, everyday, recovery := trusttest.NewDiscovery(t)
	held := chain.Follower(t)
	made := document(chain.ID())
	opened, err := addressdoc.Open(sign(t, made, everyday), held, issuer)
	if err != nil {
		t.Fatal(err)
	}
	if opened.CoreAPI != made.CoreAPI || !opened.Expires.Equal(made.Expires) || len(opened.Places()) != 2 ||
		opened.ClientOrigin("k7xq2") != "https://k7xq2.client.example" {
		t.Errorf("opened %+v", opened)
	}
	if _, err = addressdoc.Open(sign(t, made, recovery), held, issuer); err != nil {
		t.Errorf("a recovery key's document: %v", err)
	}
	if _, err = addressdoc.Open(sign(t, made, everyday), held, "https://other.example"); !errors.Is(
		err, addressdoc.ErrWrongIssuer) {
		t.Errorf("another issuer's: %v; want ErrWrongIssuer", err)
	}
	other, _, _ := trusttest.NewDiscovery(t)
	if _, err = addressdoc.Open(sign(t, document(other.ID()), everyday), held, issuer); !errors.Is(
		err, addressdoc.ErrWrongID) {
		t.Errorf("another discovery's: %v; want ErrWrongID", err)
	}
	ahead := made
	ahead.RootVersion = 2
	if _, err = addressdoc.Open(sign(t, ahead, everyday), held, issuer); !errors.Is(err, addressdoc.ErrAheadOfChain) {
		t.Errorf("a root version not reached: %v; want ErrAheadOfChain", err)
	}
	if _, err = addressdoc.Open(sign(t, made, trusttest.HardwareKey(t, "x")), held, issuer); !errors.Is(
		err, trust.ErrUntrustedKey) {
		t.Errorf("a stranger's: %v; want ErrUntrustedKey", err)
	}
	asRoot := trusttest.Sign(t, trust.NamespaceDiscoveryRoot, []byte(sign(t, made, everyday).Payload), everyday)
	if _, err = addressdoc.Open(asRoot, held, issuer); err == nil {
		t.Error("a document signed in the root namespace was taken")
	}
	newer := made
	newer.Sequence = 2
	if !newer.IsNewerThan(made) || made.IsNewerThan(newer) || !ahead.IsNewerThan(newer) {
		t.Error("documents are ordered by root version, then sequence")
	}
}

func TestCheck_EveryAddressIsHTTPSAndThePatternPutsTheCoreInALabel(t *testing.T) {
	t.Parallel()
	type change = func(*addressdoc.Document)
	for name, change := range map[string]change{
		"core API in the clear":   func(d *addressdoc.Document) { d.CoreAPI = "http://api.discovery.example/v1" },
		"loader with a path":      func(d *addressdoc.Document) { d.LoaderOrigin = issuer + "/app" },
		"account in the clear":    func(d *addressdoc.Document) { d.AccountOrigin = "http://account.example" },
		"core in no whole label":  func(d *addressdoc.Document) { d.ClientOrigins = "https://c-{core}.client.example" },
		"two cores":               func(d *addressdoc.Document) { d.ClientOrigins = "https://{core}.{core}.example" },
		"a location in the clear": func(d *addressdoc.Document) { d.Locations = []string{"http://mirror.example"} },
		"a homepage, no page":     func(d *addressdoc.Document) { d.Homepage = "javascript:alert(1)" },
	} {
		changed := document("od1-x")
		change(&changed)
		if err := changed.Check(issuer); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	single := document("od1-x")
	single.ClientOrigins = "https://home.example"
	if err := single.Check(issuer); err != nil {
		t.Errorf("one household's origin: %v", err)
	}
}
