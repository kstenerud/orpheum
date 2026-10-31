// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/certificate/certificatetest"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// memoryFiles is a persistent directory in memory.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func (files *memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	contents, found := files.files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files *memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	files.files[name] = contents
	return nil
}

func (files *memoryFiles) RemovePersistentFile(name string) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	delete(files.files, name)
	return nil
}

// world is a core's discovery service against a fake discovery, on a fake clock.
type world struct {
	service    *Service
	files      *memoryFiles
	key        identity.Key
	fake       *fakeDiscovery
	issuer     string
	clock      *clocktest.Fake
	candidates []discoveryproto.Candidate
	// claimedCount is how many times the service said the core was claimed.
	claimedCount atomic.Int32
	// probeNonces is how many nonces the service made for the probe.
	probeNonces atomic.Int32
	// hasCertificate is whether the certificate for the discovery's names is held.
	hasCertificate atomic.Bool
	// version is the build's, v0.1.0 unless a test names another before the service is made again.
	version string
	// suggested is the discovery the core suggests, none unless a test names one before the service
	// is made again; noPublisher has the build hold no publisher root.
	suggested   Suggestion
	noPublisher bool
}

// publisherRoot is the publisher root the core is built with: the fake's, unless it holds none.
func (built *world) publisherRoot() PublisherRoot {
	if built.noPublisher {
		return PublisherRoot{}
	}
	return PublisherRoot{Genesis: built.fake.publisher.Statements[0]}
}

func newWorld(t *testing.T) *world {
	t.Helper()
	key, err := identity.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fake, server := startDiscovery(t)
	built := &world{
		files: &memoryFiles{files: map[string][]byte{}}, key: key, fake: fake, issuer: server.URL,
		clock:      clocktest.NewFake(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)),
		candidates: []discoveryproto.Candidate{{Address: "2a0f:1000::1", Port: 443, Source: "interface"}},
		version:    "v0.1.0",
	}
	built.service = built.restart(t)
	return built
}

// restart builds the service again over the same files, as a core that started again does.
func (built *world) restart(t *testing.T) *Service {
	t.Helper()
	service, err := New(Parts{
		Files: built.files, Key: built.key, HTTP: &http.Client{}, Clock: built.clock,
		Logger: slog.New(slog.DiscardHandler), Version: built.version, APIMajor: 1, Suggested: built.suggested,
		Publisher:      built.publisherRoot(),
		Candidates:     func() []discoveryproto.Candidate { return built.candidates },
		OnClaimed:      func(context.Context) { built.claimedCount.Add(1) },
		Random:         unjittered{},
		ProbeNonce:     func() (string, error) { return fmt.Sprintf("probe-%d", built.probeNonces.Add(1)), nil },
		HasCertificate: built.hasCertificate.Load,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestConnect_EnrolsWithASignatureAndKeepsTheDiscoveryAcrossARestart(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	status, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != discoveryproto.StatePending || status.ClaimCode != "abcd-efgh" || status.CoreID != fakeCoreID {
		t.Errorf("after connecting the status is %+v; want pending, with its claim code", status)
	}
	if _, err = built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); !errors.Is(
		err, ErrAlreadyEnrolled) {
		t.Errorf("connecting again = %v; want ErrAlreadyEnrolled", err)
	}
	again := built.restart(t).Status()
	if again.Issuer != built.issuer || again.BaseURL != built.issuer || again.CoreID != fakeCoreID ||
		again.ClientOrigin != "http://"+fakeCoreID+".localhost:8443" || again.RootVersion != 1 {
		t.Errorf("after a restart the status is %+v; want the discovery and the core's id kept", again)
	}
}

// startingPendingEvery is how often a pending core asks whether it is claimed, where no setting says.
var startingPendingEvery = settings.StartingDuration(settings.DiscoveryPendingCheckEverySeconds)

// The other waits, where no setting says.
var (
	endedEvery       = settings.StartingDuration(settings.DiscoveryEndedCheckEverySeconds)
	anotherCopyEvery = settings.StartingDuration(settings.DiscoveryAnotherCopyCheckEverySeconds)
)

// durations is a settings reader that answers every length of time with one, or with err.
type durations struct {
	every time.Duration
	err   error
}

func (read durations) Duration(context.Context, settings.Key) (time.Duration, error) {
	return read.every, read.err
}

// A pending core asks as often as the setting says, and at its starting value when it cannot be read:
// its 10 s kept every claim waiting, the owner and the browser tier's tests alike (2026-10-20).
func TestStep_APendingCoreAsksWhetherItIsClaimedAsOftenAsTheSettingSays(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.parts.Settings = durations{every: 1500 * time.Millisecond}
	if wait := built.service.step(ctx); wait != 1500*time.Millisecond {
		t.Errorf("a pending core with the setting at 1.5 s waited %s", wait)
	}
	built.service.parts.Settings = durations{err: errors.New("the database is gone")}
	if wait := built.service.step(ctx); wait != startingPendingEvery || startingPendingEvery != 2*time.Second {
		t.Errorf("a pending core whose setting cannot be read waited %s; want its starting value, 2 s", wait)
	}
}

// byKey is a settings reader that answers each key with its own length of time.
type byKey map[settings.Key]time.Duration

func (read byKey) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	return read[key], nil
}

// Each state's wait is its own setting's (the owner, 2026-10-20: constants become settings): a core
// with no discovery, a claimed core whose addresses did not change, and one refused as a copy.
func TestStep_EachStateWaitsAsItsOwnSettingSays(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	waits := byKey{}
	waits[settings.DiscoveryEndedCheckEverySeconds] = 7 * time.Minute
	waits[settings.DiscoveryAddressCheckEverySeconds] = 11 * time.Second
	waits[settings.DiscoveryAnotherCopyCheckEverySeconds] = 13 * time.Second
	waits[settings.DiscoveryPendingCheckEverySeconds] = time.Second
	alone := newWorld(t)
	alone.service.parts.Settings = waits
	if wait := alone.service.step(ctx); wait != 7*time.Minute {
		t.Errorf("a core with no discovery waited %s; want the ended setting's 7 min", wait)
	}
	claimed := newWorld(t)
	claimed.service.parts.Settings = waits
	if _, err := claimed.service.Connect(ctx, claimed.issuer, claimed.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	claimed.service.step(ctx)
	claimed.fake.claim()
	claimed.service.step(ctx) // learns it is claimed
	claimed.service.step(ctx) // sends its addresses
	if wait := claimed.service.step(ctx); wait != 11*time.Second {
		t.Errorf("a claimed core whose addresses did not change waited %s; want the address setting's 11 s", wait)
	}
	copied := newWorld(t)
	copied.service.parts.Settings = waits
	if _, err := copied.service.Connect(ctx, copied.issuer, copied.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	copied.service.step(ctx)
	copied.fake.refuse(discoveryproto.RefusalAnotherInstance)
	if wait := copied.service.step(ctx); wait != 13*time.Second {
		t.Errorf("a core refused as a copy waited %s; want the copy setting's 13 s", wait)
	}
}

func TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	if wait := built.service.step(ctx); wait != startingPendingEvery || len(built.fake.heartbeats) != 0 {
		t.Fatalf("a pending core waited %s and sent %d heartbeats; want %s and none", wait,
			len(built.fake.heartbeats), startingPendingEvery)
	}
	built.fake.claim()
	built.service.step(ctx) // learns it is claimed
	built.service.step(ctx) // sends its addresses
	status := built.service.Status()
	if status.State != discoveryproto.StateClaimed || status.Zone != "orpheum-core.example" ||
		status.ReachableFrom != discoveryproto.ReachableIPv6 || len(status.Probe) != 1 || status.DeadEndsLastWeek != 4 {
		t.Errorf("once claimed the status is %+v; want claimed, its zone and what the probe found", status)
	}
	built.service.step(ctx) // nothing changed, nothing due
	built.candidates = append(built.candidates, discoveryproto.Candidate{
		Address: "203.0.113.7", Port: 51234, Source: "mapped", Method: "pcp",
	})
	built.service.step(ctx) // an address appeared: sent at once
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx) // five minutes on: sent again
	if count := len(built.fake.heartbeats); count != 3 {
		t.Fatalf("%d heartbeats; want one when claimed, one when an address appeared, one five minutes on", count)
	}
	sent := built.fake.heartbeats[1]
	if sent.APIMajor != 1 || sent.OrpheumVersion != "v0.1.0" || len(sent.Candidates) != 2 {
		t.Errorf("the heartbeat was %+v; want the version, the API's major and both candidates", sent)
	}
	// Each heartbeat carries a new nonce for the probe to ask the core's echo for (§2.6).
	if first, next := built.fake.heartbeats[1].ProbeNonce, built.fake.heartbeats[2].ProbeNonce; first != "probe-2" ||
		next != "probe-3" {
		t.Errorf("the heartbeats' probe nonces were %q and %q; want a new one each", first, next)
	}
}

func TestStep_OpensANewSessionWhenTheDiscoveryRefusesTheOld(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	built.fake.mutex.Lock()
	built.fake.tokens = map[string]bool{} // the discovery restarted and forgot its sessions
	built.fake.mutex.Unlock()
	// The refused call opens a new session at once and is made again with it, rather than failing
	// and waiting a retry's length (the protocol's §2.3; C11).
	if wait := built.service.step(ctx); wait != startingPendingEvery || built.service.Status().Problem != "" {
		t.Errorf("a refused session waited %s with problem %q; want the step's own wait, %s, and no problem",
			wait, built.service.Status().Problem, startingPendingEvery)
	}
	if count := built.fake.sessionCount; count != 2 {
		t.Errorf("%d sessions opened; want a second one opened at once", count)
	}
}

func TestCall_KeepsTheDeprecationTheDiscoveryNames(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.fake.deprecated = "2027-04-09"
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	if until := built.service.Status().DeprecatedUntil; until != "2027-04-09" {
		t.Errorf("the deprecation kept is %q; want the discovery's date (D273 §7)", until)
	}
}

func TestLeave_TellsTheDiscoveryAndForgetsIt(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	if err := built.service.Leave(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !built.fake.isLeft || built.service.Status().BaseURL != "" {
		t.Error("leaving did not tell the discovery, or the core still names it")
	}
	if _, err := built.files.ReadPersistentFile(StateFileName); !errors.Is(err, instancedir.ErrNotFound) {
		t.Errorf("%s is still there after leaving: %v", StateFileName, err)
	}
}

// answering answers the first offer it is given and refuses the rest, as a core at its limit does;
// proven is what it tells of the connection it answered, as rtcapi tells one that proved itself from
// home, when set.
type answering struct {
	mutex   sync.Mutex
	offered []string
	proven  *rtcapi.ProvenAtHome
}

// taken are the offers rtcapi was given.
func (answerer *answering) taken() []string {
	answerer.mutex.Lock()
	defer answerer.mutex.Unlock()
	return append([]string{}, answerer.offered...)
}

func (answerer *answering) AnswerRelayed(
	_ context.Context, offer rtcapi.Description, onProvenAtHome func(rtcapi.ProvenAtHome),
) (rtcapi.Description, error) {
	answerer.mutex.Lock()
	defer answerer.mutex.Unlock()
	answerer.offered = append(answerer.offered, offer.SDP)
	if len(answerer.offered) > 1 {
		return rtcapi.Description{}, rtcapi.ErrTooManyConnections
	}
	if answerer.proven != nil {
		// As rtcapi does, once the connection the answer made connects, after the answer.
		proven := *answerer.proven
		defer onProvenAtHome(proven)
	}
	return rtcapi.Description{Type: "answer", SDP: "answer to " + offer.SDP}, nil
}

func TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	answerer := &answering{}
	built.service.parts.Answerer = answerer
	built.service.parts.OfferKey = offerKeys
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the signalling loop to end")
	})
	for index, offer := range []string{
		"offer one\r\na=fingerprint:sha-256 AB:CD", "offer two\r\na=fingerprint:sha-256 EF:01",
	} {
		built.fake.signals <- provenOffer(fmt.Sprint(index), offer)
	}
	answered := testutil.Receive(t, built.fake.replies, "the core's answer")
	refused := testutil.Receive(t, built.fake.replies, "the core's refusal")
	if answered.Type != discoveryproto.SignalAnswer || answered.OfferID != "0" ||
		answered.SDP != "answer to offer one\r\na=fingerprint:sha-256 AB:CD" {
		t.Errorf("the first offer was answered %+v; want its answer, under its id", answered)
	}
	// The answer is signed over its exact SDP, the offer and the offer's fingerprint (§2.5 item 4).
	signed := proof.Answer{CoreID: fakeCoreID, OfferID: "0", OfferFingerprint: "sha-256 AB:CD", SDP: answered.SDP}
	if !bytes.Equal(answered.PublicKey, built.key.Public()) || !proof.VerifyAnswer(answered.PublicKey, signed,
		answered.Signature) {
		t.Error("the answer's signature does not verify over its SDP, its offer and the offer's fingerprint")
	}
	if refused.Type != discoveryproto.SignalRefusal || refused.OfferID != "1" ||
		!strings.Contains(refused.Reason, "as many connections") {
		t.Errorf("the offer past the limit was answered %+v; want a refusal saying why", refused)
	}
}

// aLocatorHash is the one hash offerKeys knows, and aSessionKey its key.
const aLocatorHash = "a-locators-hash"

var aSessionKey = proof.SessionOfferKey("a session's token")

// offerKeys is the core's keys as auth keeps them: one locator's, by its hash.
func offerKeys(_ context.Context, hash string) ([]byte, error) {
	if hash != aLocatorHash {
		return nil, errors.New("no such locator")
	}
	return aSessionKey, nil
}

// provenOffer is an offer passed on under aLocatorHash with the proof its client made.
func provenOffer(offerID, sdp string) discoveryproto.SignalMessage {
	return discoveryproto.SignalMessage{
		Type: discoveryproto.SignalOffer, OfferID: offerID, SDP: sdp, LocatorHash: aLocatorHash,
		Proof: proof.Offer(aSessionKey, rtcapi.FingerprintOf(sdp)),
	}
}

// claimedWorld is a world whose core the discovery has claimed, its first heartbeat sent.
func claimedWorld(t *testing.T) *world {
	t.Helper()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	built.service.step(t.Context())
	return built
}

// The core learns its zone when the discovery claims it, says so once, and keeps it, so its
// certificate's name is known from the start after a restart.
func TestStep_SaysOnceWhenTheCoreIsClaimedAndKeepsItsZone(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if count := built.claimedCount.Load(); count != 0 {
		t.Fatalf("a pending core was said claimed %d times; want none", count)
	}
	built.fake.claim()
	for range 3 {
		built.clock.Advance(heartbeatEvery)
		built.service.step(ctx)
	}
	want := "*." + fakeCoreID + "." + fakeZone
	if count, name := built.claimedCount.Load(), built.service.Status().CertificateName(); count != 1 || name != want {
		t.Errorf("once claimed: said %d times, certificate name %q; want once, %q", count, name, want)
	}
	if servers := built.service.Status().ICEServers; len(servers) != 1 || servers[0].URLs[0] != fakeSTUN {
		t.Errorf("the STUN servers kept are %+v; want the discovery's, %s", servers, fakeSTUN)
	}
	if name := built.restart(t).Status().CertificateName(); name != want {
		t.Errorf("after a restart the certificate name is %q; want %q, kept", name, want)
	}
}

// A claimed core asks how it stands with each heartbeat, and stops sending its addresses once its
// subscription has ended.
func TestStep_AClaimedCoreLearnsWhenItsSubscriptionEnds(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	sent := len(built.fake.heartbeats)
	built.fake.become(discoveryproto.StateFrozen)
	built.clock.Advance(heartbeatEvery)
	if wait := built.service.step(t.Context()); wait != endedEvery || len(built.fake.heartbeats) != sent {
		t.Errorf("a frozen core waited %s, %d heartbeats after %d; want %s and none more", wait,
			len(built.fake.heartbeats), sent, endedEvery)
	}
	if state := built.service.Status().State; state != discoveryproto.StateFrozen {
		t.Errorf("the core's state is %q; want frozen", state)
	}
}

// The certificate of the core's names is obtained through its discovery, which holds the DNS-01
// record for it: a wildcard for every name under the core's, set and cleared again (the spec's §3.7).
func TestSetTXT_ACertificateForTheCoresNamesIsObtainedThroughTheDiscovery(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	now := built.clock.Now()
	authority := certificatetest.NewCA(t, built.fake, now, 90*24*time.Hour)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	name := built.service.Status().CertificateName()
	certificates, err := certificate.New(certificate.Parts{
		Domain: name, DirectoryURL: authority.Directory(), HTTP: httpClient, DNS: built.service,
		Files: certificateFiles{}, Clock: built.clock, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = certificates.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	served, err := certificates.GetCertificate(nil)
	if err != nil || served.Leaf == nil || !slices.Equal(served.Leaf.DNSNames, []string{name}) {
		t.Fatalf("GetCertificate = %+v, %v; want a certificate for %s", served, err, name)
	}
	if left := built.fake.TXT("_acme-challenge." + fakeCoreID + "." + fakeZone); left != "" {
		t.Errorf("the challenge's record is still %q; want it cleared", left)
	}
	// The core told the discovery its ACME account first, for the CAA to name (RFC 8657).
	built.fake.mutex.Lock()
	accounts := slices.Clone(built.fake.accounts)
	built.fake.mutex.Unlock()
	if len(accounts) != 1 || !strings.HasSuffix(accounts[0], "/account/1") {
		t.Errorf("the discovery was told the accounts %q; want the order's", accounts)
	}
}

// certificateFiles keeps a certificate's files in memory.
type certificateFiles map[string][]byte

func (files certificateFiles) ReadPersistentFile(name string) ([]byte, error) {
	contents, found := files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files certificateFiles) WritePersistentFile(name string, contents []byte) error {
	files[name] = contents
	return nil
}

// refuse has the discovery refuse every session with code, and forget the sessions it opened.
func (fake *fakeDiscovery) refuse(code string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.refusal = &discoveryproto.Refusal{Code: code, Message: "refused: " + code}
	fake.tokens = map[string]bool{}
}

// A core whose discovery refuses it as a copy of another instance, connected and still answering,
// says so on its System page and asks again every few minutes; each run of the core names an
// instance of its own (D293 §1).
func TestStep_ACoreRefusedAsACopySaysSoAndAsksAgainLater(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	built.fake.refuse(discoveryproto.RefusalAnotherInstance)
	// Its session refused, it opens another at once, which is refused as a copy.
	if wait := built.service.step(ctx); wait != anotherCopyEvery ||
		!strings.Contains(built.service.Status().Problem, ErrAnotherCopy.Error()) {
		t.Errorf("a core refused as a copy waited %s with problem %q; want %s and that it is a copy", wait,
			built.service.Status().Problem, anotherCopyEvery)
	}
	built.restart(t).step(ctx)
	ids := built.fake.instanceIDs
	if len(ids) != 3 || ids[0] == "" || ids[0] != ids[1] || ids[2] == ids[0] || ids[2] == "" {
		t.Errorf("the sessions named instances %q; want one run's twice, then a restart's own", ids)
	}
}

// A core its discovery no longer holds enrols again, and shows the new code (the protocol's §2.2,
// C12): it does not retry a session that will never open.
func TestStep_EnrolsAgainWhenTheDiscoveryNoLongerHoldsTheCore(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(ctx)
	built.fake.refuse(discoveryproto.RefusalEnrolAgain)
	built.service.step(ctx) // its session refused: it opens another, refused as unknown
	if wait := built.service.step(ctx); wait != startingPendingEvery {
		t.Errorf("a core its discovery no longer holds waited %s; want %s, enrolled again", wait, startingPendingEvery)
	}
	status := built.service.Status()
	if built.fake.enrolCount != 2 || status.State != discoveryproto.StatePending || status.ClaimCode == "" {
		t.Errorf("after the refusal: %d enrolments, status %+v; want a second, pending with its code",
			built.fake.enrolCount, status)
	}
}

// The core's home addresses are its house addresses under its names, marked or, as a core in a
// container configures them, not; none until the discovery publishes it, and ready once the
// certificate they need is held (D292 §6, D297 §6).
func TestStatus_NamesTheHomeAddressesOnceClaimedAndSaysWhenTheirCertificateIsHeld(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	lan := discoveryproto.ScopeLAN
	built.candidates = append(built.candidates,
		discoveryproto.Candidate{Address: "192.168.1.250", Port: 8443, Source: "configured"},
		discoveryproto.Candidate{Address: "192.168.1.251", Port: 443, Source: "interface", Scope: lan},
		discoveryproto.Candidate{Address: "2001:db8:1::5", Port: 8443, Source: "configured"})
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	if pending := built.service.Status(); len(pending.HomeAddresses) != 0 || pending.IsHomeAddressReady {
		t.Errorf("a pending core's status = %+v; want no home address", pending)
	}
	built.fake.claim()
	built.service.step(ctx) // learns it is claimed
	built.service.step(ctx) // sends its addresses
	want := []string{
		"https://192-168-1-250." + fakeCoreID + ".orpheum-core.example:8443",
		"https://192-168-1-251." + fakeCoreID + ".orpheum-core.example",
	}
	claimed := built.service.Status()
	if !slices.Equal(claimed.HomeAddresses, want) || claimed.IsHomeAddressReady {
		t.Errorf("claimed, before the certificate: %v ready %v; want %v, not ready",
			claimed.HomeAddresses, claimed.IsHomeAddressReady, want)
	}
	built.hasCertificate.Store(true)
	if ready := built.service.Status(); !ready.IsHomeAddressReady {
		t.Error("with the certificate held, the home addresses are not ready")
	}
}
