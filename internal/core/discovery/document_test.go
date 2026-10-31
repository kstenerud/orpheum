// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// An issuer is an origin a person types, https, or plain http only on this machine; a URL with a
// path, an address in the clear or anything else enrols nothing and asks nothing (D391 §3).
func TestConnect_RefusesAnIssuerThatIsNotAnHTTPSOrigin(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	for _, typed := range []string{
		"http://discovery.example", built.issuer + "/core/v1", "ftp://discovery.example", "",
	} {
		if _, err := built.service.Connect(t.Context(), typed); !errors.Is(err, addressdoc.ErrNotAnIssuer) {
			t.Errorf("Connect(%q) = %v; want ErrNotAnIssuer", typed, err)
		}
	}
	if built.fake.documentFetches != 0 || built.fake.enrolCount != 0 {
		t.Errorf("a refused issuer asked %d documents and enrolled %d times", built.fake.documentFetches,
			built.fake.enrolCount)
	}
}

// The core takes its discovery's core API, client origin and loader origin from the issuer's signed
// address document and from nowhere else; a document of another issuer, or one its root key set did
// not sign, enrols nothing (D391 §3; RFC 8414 §3.3).
func TestConnect_TakesItsAddressesFromTheIssuersSignedDocumentAlone(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.fake.document.ClientOrigins = "https://" + addressdoc.CorePlace + ".client.example"
	built.fake.document.LoaderOrigin = "https://loader.example"
	built.fake.document.Homepage = "https://about.example"
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
		t.Fatal(err)
	}
	built.service.step(t.Context())
	status := built.service.Status()
	if status.ClientOrigin != "https://"+fakeCoreID+".client.example" ||
		status.LoaderOrigin != "https://loader.example" || status.Homepage != "https://about.example" ||
		status.BaseURL != built.issuer {
		t.Errorf("the core holds %+v; want the document's addresses", status)
	}
	if len(status.RootKeys) != 1 || !strings.HasPrefix(status.RootKeys[0], "SHA256:") {
		t.Errorf("the root keys shown are %v; want the one key's fingerprint", status.RootKeys)
	}

	other := newWorld(t)
	another := other.fake.document
	another.Issuer = "https://elsewhere.example"
	served := other.fake.signedDocument(another)
	other.fake.served = &served
	if _, err := other.service.Connect(t.Context(), other.issuer); !errors.Is(err, addressdoc.ErrWrongIssuer) {
		t.Errorf("another issuer's document: %v; want ErrWrongIssuer", err)
	}
	stranger := trusttest.NewRoot(t)
	forged := addressdoc.Served{
		Document: stranger.Signs(t, trust.KindAddressDocument, other.fake.document),
		RootSets: []trust.Signed{other.fake.root.Set},
	}
	other.fake.served = &forged
	if _, err := other.service.Connect(t.Context(), other.issuer); !errors.Is(err, trust.ErrUntrustedKey) {
		t.Errorf("a document outside the root key set: %v; want ErrUntrustedKey", err)
	}
	if other.fake.enrolCount != 0 || other.service.Status().BaseURL != "" {
		t.Error("a document refused enrolled the core")
	}
}

// The suggested issuer is trusted by the root keys this build names for it, never by the set the
// issuer shows; a build that names none refuses it at once, saying so, asking nothing (D391 §7).
func TestConnect_TheSuggestedIssuerIsTrustedByItsCompiledKeysAndNoneFilledInFailsLoudly(t *testing.T) {
	t.Parallel()
	unfilled := newWorld(t)
	unfilled.suggested = Suggestion{Issuer: unfilled.issuer, RootSetVersion: 1}
	unfilled.service = unfilled.restart(t)
	if _, err := unfilled.service.Connect(t.Context(), unfilled.issuer); !errors.Is(err, ErrSuggestedHasNoRootKeys) {
		t.Errorf("no root keys filled in: %v; want ErrSuggestedHasNoRootKeys", err)
	}
	if unfilled.fake.documentFetches != 0 {
		t.Error("with no root keys filled in, the issuer was asked")
	}
	if unfilled.service.Status().SuggestedIssuer != unfilled.issuer {
		t.Error("the status does not name the suggestion")
	}

	wrong := newWorld(t)
	stranger := trusttest.NewRoot(t)
	wrong.suggested = Suggestion{Issuer: wrong.issuer, RootSetVersion: 1, RootKeys: []string{stranger.Line}}
	wrong.service = wrong.restart(t)
	// The issuer shows its own set, self-signed: first use would take it; the suggestion's keys do not.
	if _, err := wrong.service.Connect(t.Context(), wrong.issuer); !errors.Is(err, trust.ErrUntrustedKey) {
		t.Errorf("a document no compiled key signed: %v; want ErrUntrustedKey", err)
	}

	right := newWorld(t)
	right.suggested = Suggestion{Issuer: right.issuer, RootSetVersion: 1, RootKeys: []string{right.fake.root.Line}}
	right.service = right.restart(t)
	if _, err := right.service.Connect(t.Context(), right.issuer); err != nil {
		t.Errorf("the document the compiled key signed: %v", err)
	}
}

// A newer document, signed, from any location the held one names, moves the core's addresses within
// the hour: its loader, and its core API, which the core then calls under the same issuer, its
// signatures bound to the issuer and not the API (D391 §3, §4). A forged one, or none, keeps the last
// good document, and says why.
func TestDocument_ANewerSignedDocumentFromAnyLocationMovesTheCoreAndAForgedOneDoesNot(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	moved, server := startDiscovery(t)
	built.fake.document.Locations = []string{server.URL + "/mirror"}
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
		t.Fatal(err)
	}
	built.fake.mutex.Lock()
	built.fake.state = discoveryproto.StateClaimed
	built.fake.mutex.Unlock()
	built.service.step(t.Context())

	// The issuer's name is lost to someone else, who serves a document of their own; the mirror
	// serves the owner's newer one, which moves the API to another host.
	stranger := trusttest.NewRoot(t)
	forgedDocument := built.fake.document
	forgedDocument.IssuedAt, forgedDocument.CoreAPI = 9, "https://attacker.example"
	forged := addressdoc.Served{
		Document: stranger.Signs(t, trust.KindAddressDocument, forgedDocument), RootSets: []trust.Signed{stranger.Set},
	}
	newer := built.fake.document
	newer.IssuedAt, newer.CoreAPI, newer.LoaderOrigin = 2, server.URL, "https://loader.example"
	mirrored := built.fake.signedDocument(newer)
	built.fake.mutex.Lock()
	built.fake.served = &forged
	built.fake.mutex.Unlock()
	moved.mutex.Lock()
	moved.issuer, moved.publicKey, moved.state, moved.served = built.issuer, built.key.Public(),
		discoveryproto.StateClaimed, &mirrored
	moved.mutex.Unlock()

	built.clock.Advance(documentCheckEvery)
	built.service.step(t.Context())
	status := built.service.Status()
	if status.BaseURL != server.URL || status.LoaderOrigin != "https://loader.example" || status.DocumentProblem != "" {
		t.Fatalf("after the newer document: %+v; want the mirror's API and loader", status)
	}
	built.clock.Advance(time.Minute)
	built.service.step(t.Context())
	moved.mutex.Lock()
	sessions := moved.sessionCount
	moved.mutex.Unlock()
	if sessions == 0 {
		t.Error("the core opened no session at the moved API under its issuer")
	}

	// Now nothing that holds is served anywhere: the last good document stays, and the page says why.
	moved.mutex.Lock()
	moved.served = &forged
	moved.mutex.Unlock()
	built.clock.Advance(documentCheckEvery)
	built.service.step(t.Context())
	status = built.service.Status()
	if status.BaseURL != server.URL || status.DocumentProblem == "" {
		t.Errorf("with no document that holds: %+v; want the last good one kept and the problem said", status)
	}
}
