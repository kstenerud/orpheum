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
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
	// Suggested is the discovery an admin is offered first, and the root key set the core pins for it
	// (Suggested); empty offers none.
	Suggested Suggestion
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

// kept is the state file: which discovery, by its issuer, the root key set pinned for it and the
// address document it last held, the core's id there, and its zone once claimed, so its certificate
// is served from the start; and the house networks it learnt, so a restart keeps them (D356 §4).
type kept struct {
	Issuer        string              `json:"issuer"`
	RootSet       trust.Pinned        `json:"rootSet"`
	Document      addressdoc.Document `json:"document"`
	CoreID        string              `json:"coreId"`
	Zone          string              `json:"zone,omitempty"`
	HouseNetworks []keptHouseNetwork  `json:"houseNetworks,omitempty"`
}

// Service keeps the core enrolled with its discovery: it connects when an admin asks, then keeps a
// session, sends its addresses, and says how it stands (D273).
type Service struct {
	parts      Parts
	wake       chan struct{}
	claimedNow chan struct{} // wakes the signalling loop when the core is newly claimed
	// instanceID is this run's, made at its start and never kept, so the discovery tells a restart
	// or a copy from the instance it holds (D293 §1).
	instanceID string

	// connecting is held through a Connect, so two at once (a double click) enrol once (C12).
	connecting sync.Mutex
	// renewal opens one session at a time, for every caller.
	renewal renewal

	mutex sync.Mutex
	// Guarded by mutex:
	status Status
	// held is the root key set pinned and the address document held; documentCheckedAt when the core
	// last looked for a newer one (checkDocument).
	held              held
	documentCheckedAt time.Time
	token             string
	tokenExpiresAt    time.Time
	lastSent          []discoveryproto.Candidate
	// failures is how many calls of the step loop failed in a row, which its next wait doubles by.
	failures int64
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
	// release is what the discovery last said of releases newer than this one; releaseAskedAt when
	// the core last asked, and releaseWait how long after it asks again (checkRelease).
	release        Release
	releaseAskedAt time.Time
	releaseWait    time.Duration

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
		parts: parts, wake: make(chan struct{}, 1), claimedNow: make(chan struct{}, 1), instanceID: rand.Text(),
	}
	contents, err := parts.Files.ReadPersistentFile(StateFileName)
	switch {
	case errors.Is(err, instancedir.ErrNotFound):
		return service, nil
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", StateFileName, err)
	}
	var state kept
	if err = json.Unmarshal(contents, &state); err != nil {
		return nil, fmt.Errorf("read %s: %w", StateFileName, err)
	}
	if state.Issuer == "" || state.Document.CoreAPI == "" {
		// A discovery held by no issuer is none: the admin enrols the core again (D391 §1).
		parts.Logger.Warn("discovery: discovery.json names no issuer; the core is enrolled with none")
		return service, nil
	}
	service.status.Issuer, service.status.BaseURL = state.Issuer, state.Document.CoreAPI
	service.status.CoreID, service.status.Zone = state.CoreID, state.Zone
	service.held = held{pinned: state.RootSet, document: state.Document}
	service.learnt = learntOf(state.HouseNetworks)
	return service, nil
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
		status.LoaderOrigin, status.Homepage = document.LoaderOrigin, document.Homepage
		status.RootKeys, status.RootSetVersion = service.held.pinned.Fingerprints(), service.held.pinned.Version
		status.DocumentIssuedAt = time.UnixMilli(document.IssuedAt)
	}
	status.SuggestedIssuer = service.parts.Suggested.Issuer
	return status
}

// ErrAlreadyEnrolled is a connect while the core is enrolled with a discovery: it leaves first.
var ErrAlreadyEnrolled = errors.New("the core is enrolled with a discovery already; leave it first")

// Connect enrols the core with the discovery whose issuer the admin typed, "discovery.example" or
// its https origin: it fetches the issuer's address document and opens it, pinning the root key set
// it is signed by (the suggestion's, for the suggested issuer), then proves to the core API the
// document names that it holds its key, and is given the code its owner claims it by, which Status
// then holds until the discovery says it is claimed (D391 §3).
func (service *Service) Connect(ctx context.Context, typed string) (Status, error) {
	issuer, err := addressdoc.Issuer(typed)
	if err != nil {
		return Status{}, err
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
	return service.enrol(ctx, issuer, opened)
}

// enrol proves the core holds its key to the discovery issuer names, at the core API its document
// names, and keeps the set, the document, and the id and code it is given.
func (service *Service) enrol(ctx context.Context, issuer string, opened held) (Status, error) {
	baseURL := opened.document.CoreAPI
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
			Signature: service.parts.Key.Sign(discoveryproto.PurposeEnrol, issuer, proof.KeyLabel(public), nonce),
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
	service.token, service.lastSent, service.sentHouseNetworks = "", nil, nil
	service.mutex.Unlock()
	if keepErr := service.keep(); keepErr != nil {
		service.mutex.Lock()
		service.status, service.held = was, wasHeld
		service.mutex.Unlock()
		return Status{}, keepErr
	}
	service.parts.Logger.InfoContext(ctx, "discovery: enrolled; waiting to be claimed", "issuer", issuer,
		"coreApi", baseURL, "coreId", enrolled.CoreID, "rootSetVersion", opened.pinned.Version)
	service.nudge()
	return service.Status(), nil
}

// Leave takes the core out of its discovery: it says so to the discovery, which may fail without
// keeping it in, and forgets it.
func (service *Service) Leave(ctx context.Context) error {
	status := service.Status()
	if status.BaseURL == "" {
		return nil
	}
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodDelete, path: discoveryproto.PathCore,
	})
	if err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: the discovery did not take the core's leaving", "err", err)
	}
	if err := service.parts.Files.RemovePersistentFile(StateFileName); err != nil {
		return fmt.Errorf("forget %s: %w", StateFileName, err)
	}
	service.mutex.Lock()
	service.status, service.token, service.lastSent = Status{}, "", nil
	service.learnt, service.sentHouseNetworks = nil, nil
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
		wait := service.step(ctx)
		timer := service.parts.Clock.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-service.wake:
			timer.Stop()
		case <-timer.C():
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

// step does what the core's state calls for now, and returns how long to wait before the next.
func (service *Service) step(ctx context.Context) time.Duration {
	status := service.Status()
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
	service.checkRelease(ctx, status)
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

// ErrAnotherCopy is the discovery refusing this core because another instance of it, a copy, is
// connected and still answers: what the admin is told.
var ErrAnotherCopy = errors.New("another copy of this server is connected to the discovery; this one stays " +
	"off it, and asks again every few minutes")

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
	if _, err := service.enrol(ctx, issuer, opened); err != nil {
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
	service.failures = 0
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
		Issuer: service.status.Issuer, RootSet: service.held.pinned, Document: service.held.document,
		CoreID: service.status.CoreID, Zone: service.status.Zone, HouseNetworks: keptHouseNetworksOf(service.learnt),
	}
	service.mutex.Unlock()
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode %s: %w", StateFileName, err)
	}
	if err = service.parts.Files.WritePersistentFile(StateFileName, encoded); err != nil {
		return fmt.Errorf("keep %s: %w", StateFileName, err)
	}
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
	request := discoveryproto.AddressesRequest{
		OrpheumVersion: service.parts.Version,
		APIMajor:       service.parts.APIMajor, Candidates: candidates, HouseNetworks: houseNetworks,
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
	service.mutex.Unlock()
	return service.wait(ctx, settings.DiscoveryAddressCheckEverySeconds)
}

// failed records why a call failed, and returns the wait before trying again. A session refused
// because another copy of the core holds the discovery is no ordinary failure: it says so, and asks
// again in a few minutes, however it came (D293 §1).
func (service *Service) failed(ctx context.Context, what string, err error) time.Duration {
	var refused *refusal
	isCopy := errors.As(err, &refused) && refused.code == discoveryproto.RefusalAnotherInstance
	if isCopy {
		what, err = "open a session", ErrAnotherCopy
	}
	problem := what + ": " + err.Error()
	service.mutex.Lock()
	isNew := service.status.Problem != problem
	service.status.Problem = problem
	if errors.Is(err, errUnauthorised) {
		service.token = ""
	}
	failures := service.failures
	if !isCopy {
		service.failures++
	}
	service.mutex.Unlock()
	if isNew {
		service.parts.Logger.WarnContext(ctx, "discovery: a call failed; trying again", "what", what, "err", err)
	}
	if isCopy {
		return service.wait(ctx, settings.DiscoveryAnotherCopyCheckEverySeconds)
	}
	return service.retryWait(ctx, failures)
}

// retryWait is the wait after a failure that followed earlier ones in a row, as many as earlier
// counts: discovery_retry_first_seconds doubled for each, to discovery_retry_most_seconds, jittered
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
