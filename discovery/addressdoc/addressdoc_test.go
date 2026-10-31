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

var now = time.UnixMilli(1_800_000_000_000)

func document() addressdoc.Document {
	return addressdoc.Document{
		Issuer: issuer, CoreAPI: "https://api.discovery.example/core/v1",
		ClientAPI: "https://api.discovery.example/client/v1", LoaderOrigin: issuer,
		ClientOrigins: "https://{core}.client.example", Homepage: "https://about.example",
		Locations: []string{"https://mirror.example/discovery"}, IssuedAt: now.UnixMilli(),
	}
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

func TestOpen_ADocumentSignedByTheSetIsTheAskedIssuersAndNoOtherIsTaken(t *testing.T) {
	t.Parallel()
	root := trusttest.NewRoot(t)
	served := addressdoc.Served{
		Document: root.Signs(t, trust.KindAddressDocument, document()), RootSets: []trust.Signed{root.Set},
	}
	opened, pinned, err := addressdoc.Open(served, root.Pinned, issuer, now)
	if err != nil {
		t.Fatal(err)
	}
	if opened.CoreAPI != document().CoreAPI || pinned.Version != 1 {
		t.Fatalf("opened %+v, %+v", opened, pinned)
	}
	if opened.ClientOrigin("k7xq2") != "https://k7xq2.client.example" {
		t.Errorf("the client origin: %q", opened.ClientOrigin("k7xq2"))
	}
	if urls := opened.URLs(); len(urls) != 2 || urls[1] != "https://mirror.example/discovery"+addressdoc.WellKnownPath {
		t.Errorf("the URLs: %v", urls)
	}
	// The same document asked for at another issuer: RFC 8414 §3.3, never used.
	if _, _, err = addressdoc.Open(served, root.Pinned, "https://other.example", now); !errors.Is(
		err, addressdoc.ErrWrongIssuer) {
		t.Fatalf("another issuer's: %v, want ErrWrongIssuer", err)
	}
	// Signed by a key outside the set.
	stranger := trusttest.NewRoot(t)
	forged := addressdoc.Served{Document: stranger.Signs(t, trust.KindAddressDocument, document())}
	if _, _, err = addressdoc.Open(forged, root.Pinned, issuer, now); !errors.Is(err, trust.ErrUntrustedKey) {
		t.Fatalf("a stranger's: %v, want ErrUntrustedKey", err)
	}
}

func TestOpen_CatchesUpTheRootSetFromTheStatementsServedWithIt(t *testing.T) {
	t.Parallel()
	root := trusttest.NewRoot(t)
	next, nextLine := trusttest.NewKey(t)
	second := root.Signs(t, trust.KindRootSet, trust.RootSet{Version: 2, Keys: []string{nextLine}})
	served := addressdoc.Served{
		Document: trusttest.Sign(t, next, trust.KindAddressDocument, document()),
		RootSets: []trust.Signed{root.Set, second},
	}
	_, pinned, err := addressdoc.Open(served, root.Pinned, issuer, now)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Version != 2 || pinned.Keys[0] != nextLine {
		t.Fatalf("the set reached: %+v", pinned)
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
		"no client API":           func(d *addressdoc.Document) { d.ClientAPI = "" },
		"never issued":            func(d *addressdoc.Document) { d.IssuedAt = 0 },
		"a homepage, no page":     func(d *addressdoc.Document) { d.Homepage = "javascript:alert(1)" },
	} {
		changed := document()
		change(&changed)
		if err := changed.Check(issuer); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	single := document()
	single.ClientOrigins = "https://home.example"
	if err := single.Check(issuer); err != nil {
		t.Errorf("one household's origin: %v", err)
	}
}
