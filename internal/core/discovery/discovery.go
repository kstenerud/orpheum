// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// StateFileName is where the core keeps which discovery it is enrolled with, in the persistent
// directory, so a restart carries on: the discovery's issuer, the root key set it pinned, its address
// document and the core's id there.
const StateFileName = "discovery.json"

// The waits between the loop's looks are discovery-core.md's.

// heartbeatEvery is how often a core sends its addresses when they have not changed (the
// spec's §3.4).
const heartbeatEvery = 5 * time.Minute

// sessionMargin is how long before its end a session is renewed.
const sessionMargin = 5 * time.Minute

// maxAnswerBytes bounds what a discovery's answer may hold.
const maxAnswerBytes = 1 << 20

// Files are where the core keeps its discovery: instancedir.Directories.
type Files interface {
	ReadPersistentFile(name string) ([]byte, error)
	WritePersistentFile(name string, contents []byte) error
	RemovePersistentFile(name string) error
}

// Durations reads a length-of-time setting: settings.Service.
type Durations interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// Parts are what the service is built from.
type Parts struct {
	Files Files
	// Settings are the server's settings, read for how long the loop waits between its looks in each
	// state; nil uses the starting values.
	Settings Durations
	Key      identity.Key
	// HTTP is the one client every call to another host goes through.
	HTTP   *http.Client
	Clock  clock.Clock
	Logger *slog.Logger
	// Suggested is the discovery an admin is offered first, and the genesis of its chain (Suggested);
	// empty offers none. Publisher is the publisher root release records are signed under (Publisher);
	// with no genesis, a release build checks no record.
	Suggested Suggestion
	Publisher PublisherRoot
	// Version is this build's, and APIMajor its API's, for the discovery to serve the right client.
	Version  string
	APIMajor int
	// Candidates are the addresses the core may be reached at, read afresh for each heartbeat.
	Candidates func() []discoveryproto.Candidate
	// TURN are the owner's own TURN servers, with credentials fresh for each heartbeat, which the
	// discovery passes to the devices it finds the core for (the protocol's §2.4); nil has none.
	TURN func(ctx context.Context) []discoveryproto.ICEServer
	// ProbeNonce makes a nonce for each heartbeat, which the core's reachability echo recognises for
	// the discovery's outside probe (reachability.Checker.ProbeNonce; the protocol's §2.6); nil sends
	// none.
	ProbeNonce func() (string, error)
	// Answerer answers the WebRTC offers the discovery passes on: rtcapi.Server, which checks each
	// itself first; nil answers none.
	Answerer Answerer
	// HasCertificate is whether the certificate for the names the discovery publishes is held, which
	// the core's home addresses need (Status.IsHomeAddressReady); nil says none is.
	HasCertificate func() bool
	// OnClaimed is told when the discovery has claimed the core, or moved it to another zone: its
	// certificate's name (Status.CertificateName) is then known. Nil tells nothing.
	OnClaimed func(ctx context.Context)
	// Random jitters the retry waits (the protocol's §2; C7).
	Random io.Reader
	// OfferKey is the key an offer passed on under hash is proven under: a live session's, by its
	// locator's hash, or a live link's, by its locate hash (auth.Service.OfferKey; the protocol's §2.5
	// item 3). Nil proves none, and every offer is refused.
	OfferKey func(ctx context.Context, hash string) ([]byte, error)
	// HandOffOfferKeys are the further keys an offer passed on under hash may be proven under: those
	// of the live hand-offs of the person a realm sign-in names by it (D308 §4); nil adds none.
	HandOffOfferKeys func(hash string) [][]byte
	// RealmSignIn takes a realm sign-in the discovery passed on, who it is for and its proof, and is
	// the person's realm key, which the core's answer is tagged under; an error refuses it (D297 §3).
	RealmSignIn func(ctx context.Context, person, ask string) ([]byte, error)
	// OnRealm is told the realm the discovery's status names whenever it is another than before:
	// what a person's realm key is made for (auth.Service.SetRealm). Nil tells no one.
	OnRealm func(realm string)
	// Links are what a discovery finds the core by, read before each look: every live link's locate
	// hash and when it ends, and every live session's device locator's hash (auth.Service's
	// LiveLinks and LiveLocators; discovery-core.md K7). Nil registers none.
	Links func(ctx context.Context) ([]Link, []string, error)
	// ConfiguredHouseNetworks are the house's public IPv6 /64s its admin named
	// (discovery.house_ipv6_networks), sent in each heartbeat as configured, for a house whose devices
	// make no direct pair to teach them (D356 §6). One that is no house network is never sent.
	ConfiguredHouseNetworks []netip.Prefix
	// InstallationID is the core's installation, kept across its restarts and never in a backup
	// (identity.LoadOrMakeInstallation), sent with each session asked for, so the discovery admits the
	// installation that last held the realm at once (D437 §5). Empty sends none.
	InstallationID string
	// Healthy is the core's own health check, which must pass for the core to hold its signalling
	// connection and so answer the discovery's ping: a core whose database or disk has failed does
	// not hold the realm against a real failover (D437 §2). Nil is always healthy.
	Healthy func(ctx context.Context) error
}

// retryJitter is how far a retry's wait may move either way.
const retryJitter = 0.1

// Answerer answers a client's WebRTC offer a discovery passed on: rtcapi.Server.AnswerRelayed, which
// checks the offer itself before any connection is made, since the discovery's check is only a
// first filter (the protocol's §2.5; the security audit's A2). It is the one way an offer through
// a discovery reaches rtcapi.
type Answerer interface {
	// AnswerRelayed answers offer, and tells onProvenAtHome when its connection proves itself from
	// home (D356 §1).
	AnswerRelayed(
		ctx context.Context, offer rtcapi.Description, onProvenAtHome func(rtcapi.ProvenAtHome),
	) (rtcapi.Description, error)
}

// kept is the state file: the identity of the discovery the core is enrolled with, whose trust
// state is trust-discovery's (trustfile.go), the core's id there, and its zone once claimed, so its
// certificate is served from the start; and the house networks it learnt, so a restart keeps them
// (D356 §4).
type kept struct {
	Identity      string             `json:"identity"`
	CoreID        string             `json:"coreId"`
	Zone          string             `json:"zone,omitempty"`
	HouseNetworks []keptHouseNetwork `json:"houseNetworks,omitempty"`
}

// Service keeps the core enrolled with its discovery: it connects when an admin asks, then keeps a
// session, sends its addresses, and says how it stands (D273).
type Service struct {
	parts      Parts
	wake       chan struct{}
	claimedNow chan struct{} // wakes the signalling loop when the core is newly claimed
	// reachedAgain wakes the signalling loop when the discovery answers again after the network
	// failed, so it dials at once rather than at its next backed-off try (D437 §5).
	reachedAgain chan struct{}
	// instanceID is this run's, made at its start and never kept, so the discovery tells a restart
	// or a copy from the instance it holds (D293 §1).
	instanceID string

	// connecting is held through a Connect, so two at once (a double click) enrol once (C12).
	connecting sync.Mutex
	// renewal opens one session at a time, for every caller.
	renewal renewal
	// stepping is held through each step of the loop and through a Leave, so the two take turns: a
	// Leave waits for the step under way and then takes out whatever the core holds, a core that step
	// enrolled again included, and a step that comes while a Leave is under way does nothing, so the
	// admin's own Leave never enrols the core again (the owner, 2026-11-09).
	stepping sync.Mutex
	// leaving is set through a Leave: the discovery ends the signalling connection as it lets the
	// core go, and the signalling loop, seeing it, says so and dials no more.
	leaving atomic.Bool

	mutex sync.Mutex
	// Guarded by mutex:
	status Status
	// held is the discovery's trust state; documentCheckedAt when the core last looked at its places
	// (checkDocument); isCaughtUp whether, in this run, a place answered a look at the chain: until one
	// has, a restored core moves no address and installs nothing (the audit's F12).
	held              held
	documentCheckedAt time.Time
	isCaughtUp        bool
	// publisher is the publisher root's trust state, nil when this build holds no publisher root.
	publisher      *publisherHeld
	token          string
	tokenExpiresAt time.Time
	lastSent       []discoveryproto.Candidate
	// failures is how many calls of the step loop failed in a row, which its next wait doubles by;
	// isUnreachable whether the last of them did not reach the discovery, so the loop's wait looks
	// for the network to come back (D437 §5).
	failures      int64
	isUnreachable bool
	// sentLinks and sentLocators are what the discovery was last told it may find the core by, and
	// sentUnder the session's token it was told under: a new session, a discovery that may have
	// forgotten, is told again.
	sentLinks    []Link
	sentLocators []string
	sentUnder    string
	// learnt are the house networks learnt from devices proven at home, the most recently witnessed
	// first, and sentHouseNetworks the house networks the last heartbeat carried (D356).
	learnt            []HouseNetwork
	sentHouseNetworks []discoveryproto.HouseNetwork
	// keptWitnessings are when discovery.json last kept each learnt network witnessed
	// (learnHouseNetwork writes again only a day on, houseNetworkKeepEvery).
	keptWitnessings map[netip.Prefix]time.Time
	// release is what the discovery last said of releases newer than this one; releaseAskedAt when
	// the core last asked, and releaseWait how long after it asks again (checkRelease).
	release        Release
	releaseAskedAt time.Time
	releaseWait    time.Duration
	// releaseNowAt is when an admin last had the record read at once (CheckReleaseNow), and
	// releaseChecking is held through each reading, so the loop's and an admin's never interleave.
	releaseNowAt    time.Time
	releaseChecking sync.Mutex

	// keeping is held through each write of the state file, so two never interleave.
	keeping sync.Mutex

	// pingsUnanswered is how many pings of the signalling connection wait for their answer: a test
	// moves its fake time on only once a ping answered in real time has been heard.
	pingsUnanswered atomic.Int64
}

// SetTXT sets the TXT record of name, its certificate's DNS-01 challenge, through the discovery,
// which holds the record for the core, and returns what clears it again (the spec's §3.7).
func (service *Service) SetTXT(ctx context.Context, name, value string) (func(context.Context) error, error) {
	if err := service.setTXT(ctx, name, value); err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { return service.setTXT(ctx, name, "") }, nil
}

// ReportACMEAccount tells the discovery the core's ACME account, before the core orders a
// certificate, so its CAA names that account alone (the protocol's §2.7; RFC 8657).
func (service *Service) ReportACMEAccount(ctx context.Context, accountURI string) error {
	status := service.Status()
	if status.BaseURL == "" {
		return ErrNotEnrolled
	}
	if err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPut, path: discoveryproto.PathACMEAccount,
		body: discoveryproto.ACMEAccountRequest{AccountURI: accountURI},
	}); err != nil {
		return fmt.Errorf("tell the discovery the core's ACME account: %w", err)
	}
	return nil
}

// setTXT sets name's record to value, or clears it when value is empty.
func (service *Service) setTXT(ctx context.Context, name, value string) error {
	status := service.Status()
	if status.BaseURL == "" {
		return ErrNotEnrolled
	}
	call := callOf{
		base: status.BaseURL, method: http.MethodPut, path: discoveryproto.PathACMEChallenge,
		body: discoveryproto.ACMEChallengeRequest{Name: name, Value: value},
	}
	if value == "" {
		call.method, call.body = http.MethodDelete, nil
		call.path = discoveryproto.PathACMEChallenge + "/" + url.PathEscape(name)
	}
	if err := service.signedCall(ctx, call); err != nil {
		return fmt.Errorf("have the discovery set the certificate's challenge: %w", err)
	}
	return nil
}

// ErrNotEnrolled is a call that needs a discovery, on a core enrolled with none.
var ErrNotEnrolled = errors.New("the core is enrolled with no discovery")

// New returns the service, carrying on with the discovery the core was enrolled with, if any.
func New(parts Parts) (*Service, error) {
	service := &Service{
		parts: parts, wake: make(chan struct{}, 1), claimedNow: make(chan struct{}, 1),
		reachedAgain: make(chan struct{}, 1), instanceID: rand.Text(),
	}
	contents, err := parts.Files.ReadPersistentFile(StateFileName)
	switch {
	case errors.Is(err, instancedir.ErrNotFound):
		publisherErr := service.loadPublisher()
		return service, publisherErr
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", StateFileName, err)
	}
	var state kept
	if err = strictjson.Unmarshal(contents, &state); err != nil {
		return nil, fmt.Errorf("read %s: %w", StateFileName, err)
	}
	opened, problem, err := service.loadTrust()
	if err != nil || opened.chain.ID() != state.Identity || state.Identity == "" {
		// A state that does not hold is never used: the admin enrols the core again (§3.3; D391 §1).
		parts.Logger.Error("discovery: the trust state does not hold; the core is enrolled with no discovery",
			"err", err)
		publisherErr := service.loadPublisher()
		return service, publisherErr
	}
	service.status.Issuer, service.status.BaseURL = opened.issuer, opened.document.CoreAPI
	service.status.CoreID, service.status.Zone = state.CoreID, state.Zone
	service.status.DocumentProblem = problem
	service.held = opened
	service.learnt = learntOf(state.HouseNetworks)
	service.keptWitnessings = witnessingsOf(service.learnt)
	publisherErr := service.loadPublisher()
	return service, publisherErr
}

// SignedClientManifest is the client's manifest signed by the core's identity key, and that key's
// public half, which a loader checks it with (D308 §6).
func (service *Service) SignedClientManifest(manifest []byte) (signature []byte, publicKey ed25519.PublicKey) {
	return service.parts.Key.SignClientManifest(manifest), service.parts.Key.Public()
}

// Status is where the core stands now.
func (service *Service) Status() Status {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	status := service.status
	status.Fingerprint = proof.Fingerprint(service.parts.Key.Public())
	status.FingerprintText = proof.FingerprintText(service.parts.Key.Public())
	status.Published = slices.Clone(status.Published)
	status.Probe = slices.Clone(status.Probe)
	status.ICEServers = slices.Clone(status.ICEServers)
	status.DeadEndCausesLastWeek = maps.Clone(status.DeadEndCausesLastWeek)
	if status.IsPublished() {
		status.HomeAddresses = homeAddresses(service.lastSent, status.CoreID, status.Zone)
		status.IsHomeAddressReady = service.parts.HasCertificate != nil && service.parts.HasCertificate()
	}
	if status.BaseURL != "" {
		status.HouseNetworks = service.houseNetworksAt(service.parts.Clock.Now())
		document := service.held.document
		if status.CoreID != "" {
			status.ClientOrigin = document.ClientOrigin(status.CoreID)
		}
		status.LoaderOrigin, status.Homepage, status.Name = document.LoaderOrigin, document.Homepage, document.Name
		status.Identity, status.RootKeys = service.held.chain.ID(), service.held.chain.Fingerprints()
		status.RootVersion, status.DocumentIssuedAt = service.held.chain.Version(), document.Issued
		status.IsSuggested = status.Issuer == service.parts.Suggested.Issuer
		if document.Expires.Before(service.parts.Clock.Now()) {
			status.DocumentStaleSince = document.Expires
		}
	}
	status.SuggestedIssuer = service.parts.Suggested.Issuer
	return status
}

// ErrAlreadyEnrolled is a connect while the core is enrolled with a discovery: it leaves first.
var ErrAlreadyEnrolled = errors.New("the core is enrolled with a discovery already; leave it first")

// Connect enrols the core with the discovery whose issuer the admin typed, "discovery.example" or
// its https origin, with "#od1-…" after it or identity naming the identity it must be (the root key
// audit's §3.8): it follows the issuer's root chain from its genesis, the suggested one's from the
// genesis this build holds, and opens its address document, then proves to the core API the document
// names that it holds its key, its signature bound to the identity, and is given the code its owner
// claims it by. A discovery not suggested, with no identity given, is refused with the identity found
// (IdentityToConfirm), for the admin to compare with what its operator shows.
func (service *Service) Connect(ctx context.Context, typed, confirmed string) (Status, error) {
	issuer, named, err := splitIssuer(typed)
	if err != nil {
		return Status{}, err
	}
	if confirmed == "" {
		confirmed = named
	}
	service.connecting.Lock()
	defer service.connecting.Unlock()
	if service.Status().BaseURL != "" {
		return Status{}, ErrAlreadyEnrolled
	}
	opened, err := service.openFirst(ctx, issuer)
	if err != nil {
		return Status{}, err
	}
	found := service.identified(opened)
	switch {
	case found.IsSuggested:
	case confirmed == "":
		return Status{}, IdentityToConfirmError{Identified: found}
	case confirmed != found.Identity:
		return Status{}, fmt.Errorf("%w: it is %s", ErrNotTheIdentity, found.Identity)
	}
	return service.enrol(ctx, opened)
}

// enrol proves the core holds its key to the discovery opened, at the core API its document names,
// and keeps its trust state, and the id and code it is given.
func (service *Service) enrol(ctx context.Context, opened held) (Status, error) {
	issuer, baseURL := opened.issuer, opened.document.CoreAPI
	nonce, err := service.challenge(ctx, baseURL)
	if err != nil {
		return Status{}, err
	}
	public := service.parts.Key.Public()
	var enrolled discoveryproto.EnrolResponse
	err = service.call(ctx, callOf{
		base: baseURL, method: http.MethodPost, path: discoveryproto.PathEnrol,
		body: discoveryproto.EnrolRequest{
			PublicKey: public, Nonce: nonce,
			Signature: service.parts.Key.Sign(discoveryproto.PurposeEnrol, opened.chain.ID(), proof.KeyLabel(public),
				nonce),
		}, into: &enrolled,
	})
	if err != nil {
		return Status{}, fmt.Errorf("enrol with %s: %w", issuer, err)
	}
	service.mutex.Lock()
	was := service.status
	service.status = Status{
		Issuer: issuer, BaseURL: baseURL, CoreID: enrolled.CoreID, State: discoveryproto.StatePending,
		ClaimCode: enrolled.ClaimCode, ClaimExpiresAt: time.UnixMilli(enrolled.ClaimExpiresAt),
		DeprecatedUntil: service.status.DeprecatedUntil, // what this connect's calls were told
	}
	wasHeld := service.held
	service.held, service.documentCheckedAt = opened, service.parts.Clock.Now()
	service.isCaughtUp = true // the chain was just followed at the issuer
	service.token, service.lastSent, service.sentHouseNetworks = "", nil, nil
	service.mutex.Unlock()
	if keepErr := errors.Join(service.keepTrust(), service.keep()); keepErr != nil {
		service.mutex.Lock()
		service.status, service.held = was, wasHeld
		service.mutex.Unlock()
		return Status{}, keepErr
	}
	service.parts.Logger.InfoContext(ctx, "discovery: enrolled; waiting to be claimed", "issuer", issuer,
		"identity", opened.chain.ID(), "coreApi", baseURL, "coreId", enrolled.CoreID,
		"rootVersion", opened.chain.Version())
	service.nudge()
	return service.Status(), nil
}

// Leave takes the core out of its discovery: it says so to the discovery, which may fail without
// keeping it in, and forgets it. It waits for a step under way, which may be enrolling the core again,
// and takes out what that step left.
func (service *Service) Leave(ctx context.Context) error {
	if !service.stepping.TryLock() {
		service.parts.Logger.InfoContext(ctx, "discovery: leaving once the step under way ends",
			slogx.EventKey, "leaveWaitsForStep")
		service.stepping.Lock()
	}
	defer service.stepping.Unlock()
	status := service.Status()
	if status.BaseURL == "" {
		return nil
	}
	service.leaving.Store(true)
	defer service.leaving.Store(false)
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodDelete, path: discoveryproto.PathCore,
	})
	if err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: the discovery did not take the core's leaving", "err", err)
	}
	for _, name := range []string{StateFileName, DiscoveryTrustFile} {
		err := service.parts.Files.RemovePersistentFile(name)
		if err != nil && !errors.Is(err, instancedir.ErrNotFound) {
			return fmt.Errorf("forget %s: %w", name, err)
		}
	}
	service.mutex.Lock()
	service.status, service.token, service.lastSent = Status{}, "", nil
	service.learnt, service.sentHouseNetworks, service.keptWitnessings = nil, nil, nil
	service.held, service.documentCheckedAt = held{}, time.Time{}
	service.mutex.Unlock()
	service.parts.Logger.InfoContext(ctx, "discovery: left", "issuer", status.Issuer)
	return nil
}

// Run keeps the core enrolled until ctx ends: it asks whether it is claimed while pending, sends its
// addresses once it is, and holds the signalling connection beside.
func (service *Service) Run(ctx context.Context) {
	var signalling sync.WaitGroup
	defer signalling.Wait()
	signalling.Go(func() { service.signal(ctx) })
	for {
		if !service.waitForNextStep(ctx, service.step(ctx)) {
			return
		}
	}
}

// nudge has the loop look again now.
func (service *Service) nudge() {
	select {
	case service.wake <- struct{}{}:
	default:
	}
}

// step does what the core's state calls for now, and returns how long to wait before the next. One
// that comes while a Leave is under way does nothing: the core is leaving its discovery.
func (service *Service) step(ctx context.Context) time.Duration {
	if !service.stepping.TryLock() {
		return service.wait(ctx, settings.DiscoveryEndedCheckEverySeconds)
	}
	defer service.stepping.Unlock()
	status := service.Status()
	if !status.StoodAsideAt.IsZero() {
		return standingAsideWait
	}
	// The release record is the publisher's, served from its own places too: a core with no
	// discovery, or one it cannot reach, reads it at its start and hourly all the same (D462 §8).
	service.checkRelease(ctx, status)
	if status.BaseURL == "" {
		return service.wait(ctx, settings.DiscoveryEndedCheckEverySeconds)
	}
	_, err := service.session(ctx)
	var refused *refusal
	switch {
	case errors.As(err, &refused) && refused.code == discoveryproto.RefusalEnrolAgain:
		return service.enrolAgain(ctx, status.Issuer)
	case err != nil:
		return service.failed(ctx, "open a session", err)
	}
	service.checkDocument(ctx)
	switch status.State {
	case discoveryproto.StateClaimed, discoveryproto.StateGrace:
		wait := service.heartbeat(ctx, status)
		// After the heartbeat, which may have opened a new session: one the discovery holds no links
		// under is told them in the same step.
		if current := service.Status(); current.IsPublished() && current.Problem == "" {
			if linkWait, err := service.registerLinks(ctx, current); err != nil {
				return linkWait
			}
		}
		return wait
	case "", discoveryproto.StatePending, discoveryproto.StateFrozen, discoveryproto.StateReleased,
		discoveryproto.StateDisabled:
		return service.askStatus(ctx, status)
	default: // a state a later protocol added: ask, as for one that has ended
		return service.askStatus(ctx, status)
	}
}

// wait is the setting key's length of time: how long the loop waits before its next look, or its
// starting value when it cannot be read (each setting's description says when it applies).
func (service *Service) wait(ctx context.Context, key settings.Key) time.Duration {
	var reader settings.DurationReader
	if service.parts.Settings != nil {
		reader = service.parts.Settings
	}
	return settings.DurationOrStarting(ctx, reader, key, service.parts.Logger)
}

// enrolAgain enrols the core again with its discovery, which no longer holds it, and returns the wait
// before the next step: the new code shows on the System page (the protocol's §2.2, C12).
func (service *Service) enrolAgain(ctx context.Context, issuer string) time.Duration {
	service.parts.Logger.WarnContext(ctx, "discovery: the discovery no longer holds the core; enrolling again",
		"issuer", issuer)
	service.mutex.Lock()
	opened := service.held
	service.mutex.Unlock()
	if _, err := service.enrol(ctx, opened); err != nil {
		return service.failed(ctx, "enrol again", err)
	}
	return service.wait(ctx, settings.DiscoveryPendingCheckEverySeconds)
}

// askStatus asks the discovery how the core stands, and returns the wait its state calls for.
func (service *Service) askStatus(ctx context.Context, status Status) time.Duration {
	var answered discoveryproto.StatusResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodGet, path: discoveryproto.PathStatus, into: &answered,
	})
	if err != nil {
		return service.failed(ctx, "ask the discovery how the core stands", err)
	}
	service.mutex.Lock()
	was := service.status
	service.status.State, service.status.Zone, service.status.Problem = answered.State, answered.Zone, ""
	service.status.Realm, service.status.Account = answered.Realm, answered.Account
	service.status.RemovedAt = time.Time{}
	if answered.RemovedAt != 0 {
		service.status.RemovedAt = time.UnixMilli(answered.RemovedAt)
	}
	service.failures, service.isUnreachable = 0, false
	service.status.ClaimCode, service.status.ClaimExpiresAt = answered.ClaimCode, time.Time{}
	service.status.ICEServers = answered.ICEServers
	if answered.ClaimCode != "" {
		service.status.ClaimExpiresAt = time.UnixMilli(answered.ClaimExpiresAt)
	}
	service.mutex.Unlock()
	service.changed(ctx, was, answered)
	switch answered.State {
	case discoveryproto.StatePending:
		return service.wait(ctx, settings.DiscoveryPendingCheckEverySeconds)
	case discoveryproto.StateClaimed, discoveryproto.StateGrace:
		return 0
	case discoveryproto.StateFrozen, discoveryproto.StateReleased, discoveryproto.StateDisabled:
		return service.wait(ctx, settings.DiscoveryEndedCheckEverySeconds)
	}
	return service.wait(ctx, settings.DiscoveryEndedCheckEverySeconds)
}

// changed says what changed in the core's standing: a new state is logged, a new zone kept, and a
// core newly claimed told to its OnClaimed.
func (service *Service) changed(ctx context.Context, was Status, answered discoveryproto.StatusResponse) {
	if answered.State != was.State {
		service.parts.Logger.InfoContext(ctx, "discovery: the core's state changed", "from", string(was.State),
			"to", string(answered.State))
	}
	if answered.Realm != was.Realm && service.parts.OnRealm != nil {
		service.parts.OnRealm(answered.Realm)
	}
	if answered.Zone != was.Zone {
		if err := service.keep(); err != nil {
			service.parts.Logger.WarnContext(ctx, "discovery: could not keep the core's zone", "err", err)
		}
	}
	isClaimed := func(state discoveryproto.State) bool { return Status{State: state}.IsPublished() }
	service.wakeSignallingIfClaimed(was.State, answered.State)
	if isClaimed(answered.State) && (!isClaimed(was.State) || answered.Zone != was.Zone) &&
		service.parts.OnClaimed != nil {
		service.parts.OnClaimed(ctx)
	}
}

// keep writes the state file from what the core holds now: its discovery, its id and zone there, and
// the house networks learnt.
func (service *Service) keep() error {
	service.keeping.Lock()
	defer service.keeping.Unlock()
	service.mutex.Lock()
	state := kept{
		Identity: service.held.chain.ID(), CoreID: service.status.CoreID, Zone: service.status.Zone,
		HouseNetworks: keptHouseNetworksOf(service.learnt),
	}
	witnessings := witnessingsOf(service.learnt)
	service.mutex.Unlock()
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode %s: %w", StateFileName, err)
	}
	if err = service.parts.Files.WritePersistentFile(StateFileName, encoded); err != nil {
		return fmt.Errorf("keep %s: %w", StateFileName, err)
	}
	service.mutex.Lock()
	service.keptWitnessings = witnessings
	service.mutex.Unlock()
	return nil
}

// heartbeat sends the core's addresses and the house's networks when they changed, or every
// heartbeatEvery, and returns the wait until it looks again.
func (service *Service) heartbeat(ctx context.Context, status Status) time.Duration {
	candidates := service.parts.Candidates()
	service.mutex.Lock()
	houseNetworks := heartbeatHouseNetworks(service.houseNetworksAt(service.parts.Clock.Now()))
	isDue := !slices.Equal(candidates, service.lastSent) ||
		!slices.Equal(houseNetworks, service.sentHouseNetworks) ||
		service.parts.Clock.Now().Sub(service.status.LastHeartbeatAt) >= heartbeatEvery
	service.mutex.Unlock()
	if !isDue {
		return service.wait(ctx, settings.DiscoveryAddressCheckEverySeconds)
	}
	// A claimed core's standing can change too, its subscription lapsing or renewed: it asks with
	// each heartbeat, and sends its addresses only while it is still claimed.
	if wait := service.askStatus(ctx, status); wait != 0 {
		return wait
	}
	service.mutex.Lock()
	rootVersion, rootHash := service.held.chain.Version(), service.held.chain.Hash()
	service.mutex.Unlock()
	request := discoveryproto.AddressesRequest{
		OrpheumVersion: service.parts.Version,
		APIMajor:       service.parts.APIMajor, Candidates: candidates, HouseNetworks: houseNetworks,
		// The root it pinned, so its operator sees a core holding another statement (the audit's F17).
		RootVersion: rootVersion, RootHash: rootHash,
	}
	if service.parts.TURN != nil {
		request.TURN = service.parts.TURN(ctx)
	}
	if service.parts.ProbeNonce != nil {
		nonce, err := service.parts.ProbeNonce()
		if err != nil {
			return service.failed(ctx, "make the probe's nonce", err)
		}
		request.ProbeNonce = nonce
	}
	var answered discoveryproto.AddressesResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPut,
		path: discoveryproto.PathAddresses, body: request, into: &answered,
	})
	if err != nil {
		return service.failed(ctx, "send the core's addresses", err)
	}
	service.mutex.Lock()
	service.lastSent, service.sentHouseNetworks = candidates, houseNetworks
	service.status.LastHeartbeatAt = service.parts.Clock.Now()
	service.status.Published, service.status.Probe = answered.Published, answered.Probe
	service.status.ReachableFrom, service.status.Problem = answered.ReachableFrom, ""
	service.status.DeadEndsLastWeek = answered.DeadEndsLastWeek
	service.status.DeadEndCausesLastWeek = answered.DeadEndCausesLastWeek
	service.mutex.Unlock()
	return service.wait(ctx, settings.DiscoveryAddressCheckEverySeconds)
}

// failed records why a call failed, and returns the wait before trying again. Two refusals of a
// session are no ordinary failure, however they came: another copy of the core serves, and this one
// stands aside, asking no more until an admin asks again or it restarts (D437 §3); or the discovery
// waits for the last holder of the realm, and this one asks again when it says (D437 §5).
func (service *Service) failed(ctx context.Context, what string, err error) time.Duration {
	if refused, isRefused := errors.AsType[*refusal](err); isRefused {
		switch refused.code {
		case discoveryproto.RefusalAnotherInstance:
			return service.standAside(ctx)
		case discoveryproto.RefusalHolderAway:
			return service.waitForTheHolder(ctx, refused.retryAfter)
		}
	}
	problem := what + ": " + err.Error()
	service.mutex.Lock()
	isNew := service.status.Problem != problem
	service.status.Problem = problem
	if errors.Is(err, errUnauthorised) {
		service.token = ""
	}
	failures := service.failures
	service.failures++
	service.isUnreachable = isUnreached(err)
	service.mutex.Unlock()
	if isNew {
		service.parts.Logger.WarnContext(ctx, "discovery: a call failed; trying again", "what", what, "err", err)
	}
	return service.retryWait(ctx, failures)
}

// retryWait is the wait after a failure that followed earlier ones in a row, as many as earlier
// counts: discoveryRetryFirstSeconds doubled for each, to discoveryRetryMostSeconds, jittered
// from Random, so a discovery's restart is not met by every core at once. A most below the first
// waits the first.
func (service *Service) retryWait(ctx context.Context, earlier int64) time.Duration {
	first := service.wait(ctx, settings.DiscoveryRetryFirstSeconds)
	most := max(service.wait(ctx, settings.DiscoveryRetryMostSeconds), first)
	wait, err := backoff.Jittered(service.parts.Random, backoff.Doubled(first, most, earlier), most, retryJitter)
	if err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not jitter a retry's wait", "err", err)
	}
	return wait
}
