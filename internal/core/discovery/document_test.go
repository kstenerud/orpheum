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
		if _, err := built.service.Connect(t.Context(), typed, ""); !errors.Is(err, addressdoc.ErrNotAnIssuer) {
			t.Errorf("Connect(%q) = %v; want ErrNotAnIssuer", typed, err)
		}
	}
	if built.fake.documentFetches != 0 || built.fake.enrolCount != 0 {
		t.Errorf("a refused issuer asked %d documents and enrolled %d times", built.fake.documentFetches,
			built.fake.enrolCount)
	}
}

// The core takes its discovery's core API, client origin and loader origin from the issuer's signed
// address document and from nowhere else; a document of another issuer, or one no key of the chain
// signed, enrols nothing (D391 §3; RFC 8414 §3.3).
func TestConnect_TakesItsAddressesFromTheIssuersSignedDocumentAlone(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.fake.document.ClientOrigins = "https://" + addressdoc.CorePlace + ".client.example"
	built.fake.document.LoaderOrigin = "https://loader.example"
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(t.Context())
	status := built.service.Status()
	if status.ClientOrigin != "https://"+fakeCoreID+".client.example" ||
		status.LoaderOrigin != "https://loader.example" || status.Name != "Fake" || status.BaseURL != built.issuer ||
		status.Identity != built.fake.chain.ID() || len(status.RootKeys) != 2 ||
		!strings.HasPrefix(status.RootKeys[0], "recovery SHA256:") {
		t.Errorf("the core holds %+v; want the document's addresses and the chain's keys", status)
	}

	other := newWorld(t)
	another := other.fake.document
	another.Issuer = "https://elsewhere.example"
	other.fake.files[addressdoc.DocumentPath] = signDocument(t, another, other.fake.e1)
	if _, err := other.service.Connect(t.Context(), other.issuer, other.fake.chain.ID()); !errors.Is(
		err, addressdoc.ErrWrongIssuer) {
		t.Errorf("another issuer's document: %v; want ErrWrongIssuer", err)
	}
	other.fake.files[addressdoc.DocumentPath] = signDocument(t, other.fake.document, trusttest.HardwareKey(t, "x"))
	if _, err := other.service.Connect(t.Context(), other.issuer, other.fake.chain.ID()); !errors.Is(
		err, trust.ErrUntrustedKey) {
		t.Errorf("a document no key of the chain signed: %v; want ErrUntrustedKey", err)
	}
	if other.fake.enrolCount != 0 || other.service.Status().BaseURL != "" {
		t.Error("a document refused enrolled the core")
	}
}

// A discovery the core does not know is enrolled only by the identity its admin compared with what
// its operator shows: none given is refused with the identity found, another refused, and the
// identity after "#" or given enrols (the root key audit's §3.8; D393 §4).
func TestConnect_AnOwnDiscoveryIsEnrolledOnlyByTheIdentityItsAdminConfirms(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	identified, err := built.service.Identify(t.Context(), built.issuer)
	if err != nil || identified.Identity != built.fake.chain.ID() || identified.Name != "Fake" ||
		identified.IsSuggested {
		t.Fatalf("identified %+v, %v", identified, err)
	}
	var confirm IdentityToConfirmError
	if _, err = built.service.Connect(t.Context(), built.issuer, ""); !errors.As(err, &confirm) ||
		confirm.Identity != built.fake.chain.ID() {
		t.Errorf("no identity given: %v; want IdentityToConfirm naming it", err)
	}
	other, _, _ := trusttest.NewDiscovery(t)
	if _, err = built.service.Connect(t.Context(), built.issuer, other.ID()); !errors.Is(err, ErrNotTheIdentity) {
		t.Errorf("another identity: %v; want ErrNotTheIdentity", err)
	}
	if built.fake.enrolCount != 0 {
		t.Fatal("an unconfirmed discovery was enrolled with")
	}
	if _, err = built.service.Connect(t.Context(), built.issuer+"#"+built.fake.chain.ID(), ""); err != nil {
		t.Errorf("the identity after #: %v", err)
	}
}

// The suggested issuer is trusted only from the genesis this build holds, never one the issuer shows;
// a build holding none refuses it at once, saying so, asking nothing (D391 §7; §3.8 step 3).
func TestConnect_TheSuggestedIssuerIsTrustedByItsCompiledGenesisAndNoneFailsLoudly(t *testing.T) {
	t.Parallel()
	unfilled := newWorld(t)
	unfilled.suggested = Suggestion{Issuer: unfilled.issuer}
	unfilled.service = unfilled.restart(t)
	if _, err := unfilled.service.Connect(t.Context(), unfilled.issuer, ""); !errors.Is(err, ErrSuggestedHasNoGenesis) {
		t.Errorf("no genesis compiled in: %v; want ErrSuggestedHasNoGenesis", err)
	}
	if unfilled.fake.documentFetches != 0 || unfilled.service.Status().SuggestedIssuer != unfilled.issuer {
		t.Error("with no genesis compiled in, the issuer was asked, or the suggestion is not named")
	}
	wrong := newWorld(t)
	otherChain, _, _ := trusttest.NewDiscovery(t)
	wrong.suggested = Suggestion{Issuer: wrong.issuer, Genesis: otherChain.Statements[0]}
	wrong.service = wrong.restart(t)
	if _, err := wrong.service.Connect(t.Context(), wrong.issuer, ""); !errors.Is(err, ErrNotTheSuggestedRoot) {
		t.Errorf("another genesis: %v; want ErrNotTheSuggestedRoot", err)
	}
	right := newWorld(t)
	right.suggested = Suggestion{Issuer: right.issuer, Genesis: right.fake.chain.Statements[0]}
	right.service = right.restart(t)
	if status, err := right.service.Connect(t.Context(), right.issuer, ""); err != nil || !status.IsSuggested {
		t.Errorf("the compiled genesis' discovery: %+v, %v; want it enrolled as the suggested one", status, err)
	}
}

// claimed is a world whose core is enrolled and claimed.
func claimed(t *testing.T) *world {
	t.Helper()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.mutex.Lock()
	built.fake.state = discoveryproto.StateClaimed
	built.fake.mutex.Unlock()
	built.service.step(t.Context())
	return built
}

// look moves the clock past a look's wait and has the core look.
func (built *world) look(t *testing.T) Status {
	t.Helper()
	built.clock.Advance(documentCheckEvery)
	built.service.step(t.Context())
	return built.service.Status()
}

// A newer signed document, from any place the core has used, moves the core's addresses: its loader,
// and its core API, which it then calls under the same identity, its signatures bound to the identity
// and not a URL (the audit's F14). A forged one, or none, keeps the last good document, and says why.
// A place no document names any more is still asked (the audit's F6).
func TestDocument_ANewerSignedDocumentFromAnyPlaceMovesTheCoreAndAForgedOneDoesNot(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	moved, server := startDiscovery(t)
	moved.chain, moved.e1 = built.fake.chain, built.fake.e1
	built.fake.document.Locations = []string{built.issuer, server.URL + "/mirror"}
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	// The issuer's name is lost to someone else, who serves a document of their own; the mirror serves
	// the owner's newer one, naming the mirror alone, which moves the API to another host.
	forged := built.fake.document
	forged.Sequence, forged.CoreAPI = 9, "https://attacker.example"
	newer := built.fake.document
	newer.Sequence, newer.CoreAPI, newer.LoaderOrigin = 2, server.URL, "https://loader.example"
	newer.Locations = []string{server.URL + "/mirror"}
	built.fake.mutex.Lock()
	built.fake.files[addressdoc.DocumentPath] = signDocument(t, forged, trusttest.HardwareKey(t, "x"))
	built.fake.mutex.Unlock()
	moved.mutex.Lock()
	moved.issuer, moved.publicKey, moved.state = built.issuer, built.key.Public(), discoveryproto.StateClaimed
	moved.files[addressdoc.DocumentPath] = signDocument(t, newer, built.fake.e1)
	moved.mutex.Unlock()

	status := built.look(t)
	if status.BaseURL != server.URL || status.LoaderOrigin != "https://loader.example" ||
		!strings.Contains(status.DocumentProblem, "no key trusted") {
		t.Fatalf("after the newer document: %+v; want the mirror's API and loader, and the forgery said", status)
	}
	built.clock.Advance(time.Minute)
	built.service.step(t.Context())
	moved.mutex.Lock()
	sessions := moved.sessionCount
	moved.mutex.Unlock()
	if sessions == 0 {
		t.Error("the core opened no session at the moved API under its identity")
	}
	fetches := built.fake.documentFetches
	built.look(t)
	if built.fake.documentFetches == fetches {
		t.Error("the issuer, which the newer document no longer names, was not asked again")
	}
}

// A rotation the owner publishes is followed: the root statement taken, the floor lifted to its
// version, a document under it taken, and one the removed key signs refused; the heartbeat then
// names the new version (§3.4, F17).
func TestDocument_TheCoreFollowsARotationAndTheRemovedKeySignsNoMore(t *testing.T) {
	t.Parallel()
	built := claimed(t)
	replacement := trusttest.HardwareKey(t, "e2")
	statement, root := built.fake.chain.Next(t, 2, trusttest.Change{
		Remove: []trusttest.Key{built.fake.e1}, AddEveryday: []trusttest.Key{replacement},
	}, built.fake.r1)
	stolen := built.fake.document
	stolen.RootVersion, stolen.Sequence, stolen.CoreAPI = 2, 5, "https://attacker.example"
	built.fake.mutex.Lock()
	built.fake.chain.Take(statement, root)
	built.fake.files[addressdoc.DocumentPath] = signDocument(t, stolen, built.fake.e1)
	built.fake.mutex.Unlock()
	status := built.look(t)
	if status.RootVersion != 2 || status.BaseURL == "https://attacker.example" || status.DocumentProblem == "" {
		t.Fatalf("after the rotation: %+v; want version 2, the removed key's document refused and said", status)
	}
	current := built.fake.document
	current.RootVersion = 2
	built.fake.mutex.Lock()
	built.fake.files[addressdoc.DocumentPath] = signDocument(t, current, replacement)
	built.fake.mutex.Unlock()
	if status = built.look(t); status.DocumentProblem != "" {
		t.Errorf("a document the new key signed: problem %q", status.DocumentProblem)
	}
	built.clock.Advance(heartbeatEvery)
	built.service.step(t.Context())
	built.fake.mutex.Lock()
	last := built.fake.heartbeats[len(built.fake.heartbeats)-1]
	built.fake.mutex.Unlock()
	published, err := trust.Load(trust.ChainDiscovery, built.fake.chain.ID(), built.fake.chain.Statements)
	if err != nil {
		t.Fatal(err)
	}
	if last.RootVersion != 2 || last.RootHash != published.Hash() {
		t.Errorf("the heartbeat names root %d %q; want version 2 and its hash", last.RootVersion, last.RootHash)
	}
}

// A document of the floor's own sequence with other bytes is equivocation, refused and said; an older
// one changes nothing; two root statements of one version, each an everyday key's, are both refused
// and said (§3.4 step 8, §3.5 step 3).
func TestDocument_EquivocationARollbackAndAConflictAreRefusedAndSaid(t *testing.T) {
	t.Parallel()
	built := claimed(t)
	twin := built.fake.document
	twin.LoaderOrigin = "https://other.example"
	built.fake.mutex.Lock()
	built.fake.files[addressdoc.DocumentPath] = signDocument(t, twin, built.fake.e1)
	built.fake.mutex.Unlock()
	if status := built.look(t); !strings.Contains(status.DocumentProblem, "another address document of sequence 1") ||
		status.LoaderOrigin != fakeLoaderOrigin {
		t.Errorf("equivocation: %+v", status)
	}
	e2, e3 := trusttest.HardwareKey(t, "e2"), trusttest.HardwareKey(t, "e3")
	one, _ := built.fake.chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{e2}}, built.fake.e1)
	other, _ := built.fake.chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{e3}}, built.fake.e1)
	built.fake.mutex.Lock()
	delete(built.fake.files, addressdoc.DocumentPath)
	built.fake.files[addressdoc.RootPath(2)] = one
	built.fake.files[addressdoc.RootLatestPath] = other
	built.fake.mutex.Unlock()
	if status := built.look(t); !strings.Contains(status.DocumentProblem, "two different root statements") ||
		status.RootVersion != 1 {
		t.Errorf("a conflict: %+v; want it refused at version 1 and said", status)
	}
}

// A trust state that does not hold, its statement changed after it was kept, is never used: the core
// starts enrolled with no discovery (§3.3).
func TestTrust_AStateThatDoesNotHoldIsNeverUsed(t *testing.T) {
	t.Parallel()
	built := claimed(t)
	if again := built.restart(t).Status(); again.Identity != built.fake.chain.ID() {
		t.Fatalf("a state that holds was not used: %+v", again)
	}
	kept, err := built.files.ReadPersistentFile(DiscoveryTrustFile)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(kept), `chain discovery`, `chain discovery `, 1)
	if err = built.files.WritePersistentFile(DiscoveryTrustFile, []byte(tampered)); err != nil {
		t.Fatal(err)
	}
	if again := built.restart(t).Status(); again.BaseURL != "" || again.Identity != "" {
		t.Errorf("a state that does not hold was used: %+v", again)
	}
}
