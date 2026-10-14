// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// StateFileName is where the core keeps which discovery it is enrolled with, in the persistent
// directory, so a restart carries on: the discovery's base URL and the core's id there.
const StateFileName = "discovery.json"

// ServiceURL is the paid discovery's base URL, the one an admin is offered first (D273 §2).
const ServiceURL = "https://api.orpheum-app.com/core/v1"

// The waits between the loop's looks are discovery-core.md's.

// pendingEvery is how often a pending core asks whether it has been claimed: soon after its
// owner types the code.
const pendingEvery = 10 * time.Second

// addressesEvery is how often a core looks whether its addresses changed, sending them at once
// if they did.
const addressesEvery = time.Minute

// heartbeatEvery is how often a core sends its addresses when they have not changed (the
// spec's §3.4).
const heartbeatEvery = 5 * time.Minute

// anotherCopyEvery is how often a core its discovery refused as a copy of another, still connected,
// asks again: if the other stops, this one takes its place (D293 §1).
const anotherCopyEvery = 3 * time.Minute

// endedEvery is how often a core whose discovery has ended asks again, in case its owner pays.
const endedEvery = time.Hour

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

// Parts are what the service is built from.
type Parts struct {
	Files Files
	Key   identity.Key
	// HTTP is the one client every call to another host goes through.
	HTTP   *http.Client
	Clock  clock.Clock
	Logger *slog.Logger
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
	// OnClaimed is told when the discovery has claimed the core, or moved it to another zone: its
	// certificate's name (Status.CertificateName) is then known. Nil tells nothing.
	OnClaimed func(ctx context.Context)
	// RetryFirst is the wait after a call failed, or the signalling connection dropped; each failure
	// in a row doubles it, to RetryMost, every wait jittered from Random, so a discovery's restart is
	// not met by every core at once (the protocol's §2; C7). The config's discovery table sets both;
	// the development stack keeps RetryMost near a second (D292 §9).
	RetryFirst, RetryMost time.Duration
	Random                io.Reader
	// OfferKey is the key an offer passed on under hash is proven under: a live session's, by its
	// locator's hash, or a live link's, by its locate hash (auth.Service.OfferKey; the protocol's §2.5
	// item 3). Nil proves none, and every offer is refused.
	OfferKey func(ctx context.Context, hash string) ([]byte, error)
	// Links are what a discovery finds the core by, read before each look: every live link's locate
	// hash and when it ends, and every live session's device locator's hash (auth.Service's
	// LiveLinks and LiveLocators; discovery-core.md K7). Nil registers none.
	Links func(ctx context.Context) ([]Link, []string, error)
}

// retryJitter is how far a retry's wait may move either way.
const retryJitter = 0.1

// Answerer answers a client's WebRTC offer a discovery passed on: rtcapi.Server.AnswerRelayed, which
// checks the offer itself before any connection is made, since the discovery's check is only a
// first filter (the protocol's §2.5; the security audit's A2). It is the one way an offer through
// a discovery reaches rtcapi.
type Answerer interface {
	AnswerRelayed(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error)
}

// kept is the state file: which discovery, the core's id there, and its zone once claimed, so its
// certificate is served from the start.
type kept struct {
	BaseURL string `json:"baseUrl"`
	CoreID  string `json:"coreId"`
	Zone    string `json:"zone,omitempty"`
}

// Status is where the core stands with its discovery, for its admin.
type Status struct {
	// BaseURL is the discovery's, empty when the core is enrolled with none.
	BaseURL        string
	CoreID         string
	State          discoveryproto.State
	Zone           string
	ClaimCode      string
	ClaimExpiresAt time.Time
	ReachableFrom  string
	// DeadEndsLastWeek is how many times a client found no path in the last week, as the discovery
	// counted (D293 §7).
	DeadEndsLastWeek int
	Published        []string
	Probe            []discoveryproto.ProbeResult
	// LastHeartbeatAt is when the discovery last took the core's addresses.
	LastHeartbeatAt time.Time
	// Problem is why the last call failed, empty when it did not.
	Problem string
	// DeprecatedUntil is the date the discovery stops answering this protocol, when it said so.
	DeprecatedUntil string
	// ICEServers are the STUN servers the discovery names for the core's WebRTC candidates.
	ICEServers []discoveryproto.ICEServer
	// ClientOrigin is where the discovery serves its client, which the core's links lead to; empty
	// until it says, or when what it said is not an https origin (D293 §4).
	ClientOrigin string
	// Fingerprint is the identity key's, which the core's links carry (proof.Fingerprint), and
	// FingerprintText the same as a person reads and compares it, in safe32 groups
	// (proof.FingerprintText).
	Fingerprint, FingerprintText string
}

// Service keeps the core enrolled with its discovery: it connects when an admin asks, then keeps a
// session, sends its addresses, and says how it stands (D273).
type Service struct {
	parts Parts
	wake  chan struct{}
	// instanceID is this run's, made at its start and never kept, so the discovery tells a restart
	// or a copy from the instance it holds (D293 §1).
	instanceID string

	// connecting is held through a Connect, so two at once (a double click) enrol once (C12).
	connecting sync.Mutex
	// renewal opens one session at a time, for every caller.
	renewal renewal

	mutex sync.Mutex
	// Guarded by mutex:
	status         Status
	token          string
	tokenExpiresAt time.Time
	lastSent       []discoveryproto.Candidate
	// failures is how many calls of the step loop failed in a row, which its next wait doubles by.
	failures int64
	// sentLinks and sentLocators are what the discovery was last told it may find the core by, and
	// sentUnder the session's token it was told under: a new session, a discovery that may have
	// forgotten, is told again.
	sentLinks    []Link
	sentLocators []string
	sentUnder    string
}

// IsPublished is whether the discovery publishes the core's addresses: it has claimed the core, and
// its subscription has not ended.
func (status Status) IsPublished() bool {
	return status.State == discoveryproto.StateClaimed || status.State == discoveryproto.StateGrace
}

// CertificateName is the wildcard every name the discovery publishes for the core is under,
// *.<coreId>.<zone>, which its certificate is for; empty before the discovery has given it a zone.
func (status Status) CertificateName() string {
	if status.CoreID == "" || status.Zone == "" {
		return ""
	}
	return "*." + status.CoreID + "." + status.Zone
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
	service := &Service{parts: parts, wake: make(chan struct{}, 1), instanceID: rand.Text()}
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
	service.status.BaseURL, service.status.CoreID, service.status.Zone = state.BaseURL, state.CoreID, state.Zone
	return service, nil
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
	return status
}

// ErrAlreadyEnrolled is a connect while the core is enrolled with a discovery: it leaves first.
var ErrAlreadyEnrolled = errors.New("the core is enrolled with a discovery already; leave it first")

// Connect enrols the core with the discovery at baseURL: it proves it holds its key, and is given
// the code its owner claims it by, which Status then holds until the discovery says it is claimed.
func (service *Service) Connect(ctx context.Context, baseURL string) (Status, error) {
	if err := checkBaseURL(baseURL); err != nil {
		return Status{}, err
	}
	service.connecting.Lock()
	defer service.connecting.Unlock()
	if service.Status().BaseURL != "" {
		return Status{}, ErrAlreadyEnrolled
	}
	return service.enrol(ctx, strings.TrimSuffix(baseURL, "/"))
}

// enrol proves the core holds its key to the discovery at baseURL and keeps the id and code it is
// given.
func (service *Service) enrol(ctx context.Context, baseURL string) (Status, error) {
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
			Signature: service.parts.Key.Sign(discoveryproto.PurposeEnrol, baseURL, proof.KeyLabel(public), nonce),
		}, into: &enrolled,
	})
	if err != nil {
		return Status{}, fmt.Errorf("enrol with %s: %w", baseURL, err)
	}
	if keepErr := service.keep(kept{BaseURL: baseURL, CoreID: enrolled.CoreID}); keepErr != nil {
		return Status{}, keepErr
	}
	service.mutex.Lock()
	service.status = Status{
		BaseURL: baseURL, CoreID: enrolled.CoreID, State: discoveryproto.StatePending,
		ClaimCode: enrolled.ClaimCode, ClaimExpiresAt: time.UnixMilli(enrolled.ClaimExpiresAt),
		DeprecatedUntil: service.status.DeprecatedUntil, // what this connect's calls were told
	}
	service.token, service.lastSent = "", nil
	service.mutex.Unlock()
	service.parts.Logger.InfoContext(ctx, "discovery: enrolled; waiting to be claimed", "baseUrl", baseURL,
		"coreId", enrolled.CoreID)
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
	service.mutex.Unlock()
	service.parts.Logger.InfoContext(ctx, "discovery: left", "baseUrl", status.BaseURL)
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
		return endedEvery
	}
	_, err := service.session(ctx)
	var refused *refusal
	switch {
	case errors.As(err, &refused) && refused.code == discoveryproto.RefusalEnrolAgain:
		return service.enrolAgain(ctx, status.BaseURL)
	case err != nil:
		return service.failed(ctx, "open a session", err)
	}
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

// enrolAgain enrols the core again with its discovery, which no longer holds it, and returns the wait
// before the next step: the new code shows on the System page (the protocol's §2.2, C12).
func (service *Service) enrolAgain(ctx context.Context, baseURL string) time.Duration {
	service.parts.Logger.WarnContext(ctx, "discovery: the discovery no longer holds the core; enrolling again",
		"baseUrl", baseURL)
	if _, err := service.enrol(ctx, baseURL); err != nil {
		return service.failed(ctx, "enrol again", err)
	}
	return pendingEvery
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
	service.failures = 0
	service.status.ClaimCode, service.status.ClaimExpiresAt = answered.ClaimCode, time.Time{}
	service.status.ICEServers = answered.ICEServers
	service.status.ClientOrigin = originOf(answered.ClientOrigin)
	if answered.ClaimCode != "" {
		service.status.ClaimExpiresAt = time.UnixMilli(answered.ClaimExpiresAt)
	}
	service.mutex.Unlock()
	service.changed(ctx, was, answered)
	switch answered.State {
	case discoveryproto.StatePending:
		return pendingEvery
	case discoveryproto.StateClaimed, discoveryproto.StateGrace:
		return 0
	case discoveryproto.StateFrozen, discoveryproto.StateReleased, discoveryproto.StateDisabled:
		return endedEvery
	}
	return endedEvery
}

// changed says what changed in the core's standing: a new state is logged, a new zone kept, and a
// core newly claimed told to its OnClaimed.
func (service *Service) changed(ctx context.Context, was Status, answered discoveryproto.StatusResponse) {
	if answered.State != was.State {
		service.parts.Logger.InfoContext(ctx, "discovery: the core's state changed", "from", string(was.State),
			"to", string(answered.State))
	}
	if answered.Zone != was.Zone {
		if err := service.keep(kept{BaseURL: was.BaseURL, CoreID: was.CoreID, Zone: answered.Zone}); err != nil {
			service.parts.Logger.WarnContext(ctx, "discovery: could not keep the core's zone", "err", err)
		}
	}
	isClaimed := func(state discoveryproto.State) bool { return Status{State: state}.IsPublished() }
	if isClaimed(answered.State) && (!isClaimed(was.State) || answered.Zone != was.Zone) &&
		service.parts.OnClaimed != nil {
		service.parts.OnClaimed(ctx)
	}
}

// keep writes the state file.
func (service *Service) keep(state kept) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode %s: %w", StateFileName, err)
	}
	if err = service.parts.Files.WritePersistentFile(StateFileName, encoded); err != nil {
		return fmt.Errorf("keep %s: %w", StateFileName, err)
	}
	return nil
}

// heartbeat sends the core's addresses when they changed, or every heartbeatEvery, and returns the
// wait until it looks again.
func (service *Service) heartbeat(ctx context.Context, status Status) time.Duration {
	candidates := service.parts.Candidates()
	service.mutex.Lock()
	isDue := !slices.Equal(candidates, service.lastSent) ||
		service.parts.Clock.Now().Sub(service.status.LastHeartbeatAt) >= heartbeatEvery
	service.mutex.Unlock()
	if !isDue {
		return addressesEvery
	}
	// A claimed core's standing can change too, its subscription lapsing or renewed: it asks with
	// each heartbeat, and sends its addresses only while it is still claimed.
	if wait := service.askStatus(ctx, status); wait != 0 {
		return wait
	}
	request := discoveryproto.AddressesRequest{
		OrpheumVersion: service.parts.Version,
		APIMajor:       service.parts.APIMajor, Candidates: candidates,
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
	service.lastSent = candidates
	service.status.LastHeartbeatAt = service.parts.Clock.Now()
	service.status.Published, service.status.Probe = answered.Published, answered.Probe
	service.status.ReachableFrom, service.status.Problem = answered.ReachableFrom, ""
	service.status.DeadEndsLastWeek = answered.DeadEndsLastWeek
	service.mutex.Unlock()
	return addressesEvery
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
		return anotherCopyEvery
	}
	return service.retryWait(ctx, failures)
}

// retryWait is the wait after a failure that followed earlier ones in a row, as many as earlier
// counts: RetryFirst doubled for each, to RetryMost, jittered.
func (service *Service) retryWait(ctx context.Context, earlier int64) time.Duration {
	most := service.parts.RetryMost
	wait, err := backoff.Jittered(service.parts.Random, backoff.Doubled(service.parts.RetryFirst, most, earlier),
		most, retryJitter)
	if err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not jitter a retry's wait", "err", err)
	}
	return wait
}

// signedCall makes a call with the session's token, renewing the session at once, and only once, when the
// discovery refuses its token, rather than failing and waiting a retry's length (the protocol's
// §2.3; C11). A second refusal is the call's failure.
func (service *Service) signedCall(ctx context.Context, call callOf) error {
	token, err := service.session(ctx)
	if err != nil {
		return err
	}
	call.token = token
	err = service.call(ctx, call)
	if !errors.Is(err, errUnauthorised) {
		return err
	}
	service.dropToken(token)
	if call.token, err = service.session(ctx); err != nil {
		return err
	}
	return service.call(ctx, call)
}

// dropToken forgets token, the discovery having refused it, unless another caller has renewed it.
func (service *Service) dropToken(token string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.token == token {
		service.token = ""
	}
}

// session is a bearer token for the discovery, opening a session when there is none or it ends soon:
// one at a time, every caller sharing it (renewal).
func (service *Service) session(ctx context.Context) (string, error) {
	if token, isLive := service.liveToken(); isLive {
		return token, nil
	}
	return service.renewal.do(ctx, service.openSession)
}

// liveToken is the session's token, while it does not end within sessionMargin.
func (service *Service) liveToken() (string, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	isLive := service.token != "" && service.parts.Clock.Now().Add(sessionMargin).Before(service.tokenExpiresAt)
	return service.token, isLive
}

// openSession opens a session, unless the renewal a caller waited for left one live.
func (service *Service) openSession(ctx context.Context) (string, error) {
	if token, isLive := service.liveToken(); isLive {
		return token, nil
	}
	status := service.Status()
	// The session's life counts from before the asking, so a slow answer only shortens it.
	asked := service.parts.Clock.Now()
	nonce, err := service.challenge(ctx, status.BaseURL)
	if err != nil {
		return "", err
	}
	var opened discoveryproto.SessionResponse
	err = service.call(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathSession,
		body: discoveryproto.SessionRequest{
			CoreID: status.CoreID, InstanceID: service.instanceID, Nonce: nonce,
			Signature: service.parts.Key.Sign(discoveryproto.PurposeSession, status.BaseURL, status.CoreID, nonce),
		}, into: &opened,
	})
	if err != nil {
		return "", err
	}
	service.mutex.Lock()
	service.token, service.tokenExpiresAt = opened.Token, asked.Add(time.Duration(opened.ExpiresIn)*time.Second)
	service.mutex.Unlock()
	return opened.Token, nil
}

// challenge is a nonce from the discovery at baseURL, for the core's key to sign.
func (service *Service) challenge(ctx context.Context, baseURL string) (string, error) {
	var answered discoveryproto.ChallengeResponse
	err := service.call(ctx, callOf{
		base: baseURL, method: http.MethodPost, path: discoveryproto.PathChallenge,
		body: discoveryproto.ChallengeRequest{PublicKey: service.parts.Key.Public()}, into: &answered,
	})
	if err != nil {
		return "", fmt.Errorf("ask for a challenge: %w", err)
	}
	if answered.Nonce == "" {
		return "", errors.New("the discovery's challenge has no nonce")
	}
	return answered.Nonce, nil
}

// errUnauthorised is a discovery refusing the core's session: a new one is opened.
var errUnauthorised = errors.New("the discovery refused the core's session")

// refusal is a refusal the discovery names, which the core acts on (discoveryproto.Refusal).
type refusal struct {
	code, message string
}

func (refused *refusal) Error() string {
	return "the discovery refused: " + refused.message
}

// callOf is one call to a discovery.
type callOf struct {
	base, method, path, token string
	body                      any
	into                      any
}

// call makes one call, carrying the protocol's version and the session's token, and reads its answer
// into call.into; a deprecation the discovery names is kept for the admin.
func (service *Service) call(ctx context.Context, call callOf) error {
	var body io.Reader
	if call.body != nil {
		encoded, err := json.Marshal(call.body)
		if err != nil {
			return fmt.Errorf("encode the request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, call.method, call.base+call.path, body)
	if err != nil {
		return fmt.Errorf("make the request: %w", err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	if call.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if call.token != "" {
		request.Header.Set("Authorization", "Bearer "+call.token)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("reach the discovery: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	if deprecated := response.Header.Get(discoveryproto.DeprecatedHeader); deprecated != "" {
		service.mutex.Lock()
		service.status.DeprecatedUntil = deprecated
		service.mutex.Unlock()
	}
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("read the discovery's answer: %w", err)
	}
	switch {
	case response.StatusCode < 200 || response.StatusCode > 299:
		return failure(response, answer)
	case call.into == nil:
		return nil
	}
	if err = json.Unmarshal(answer, call.into); err != nil {
		return fmt.Errorf("the discovery's answer is not what it should be: %w", err)
	}
	return nil
}

// failure is the error a refused call's answer stands for: a refused session, a refusal the
// discovery names, or the status and what it said.
func failure(response *http.Response, answer []byte) error {
	if response.StatusCode == http.StatusUnauthorized {
		return errUnauthorised
	}
	var named discoveryproto.Refusal
	if json.Unmarshal(answer, &named) == nil && named.Code != "" {
		return &refusal{code: named.Code, message: named.Message}
	}
	return fmt.Errorf("the discovery answered %s: %s", response.Status, bytes.TrimSpace(answer))
}

// originOf is a client origin a discovery names, as the core takes it: an https origin with no path,
// query or credentials, written scheme://host[:port]; anything else is none. The discovery already
// serves its client's code, so its word gives it nothing more, and this keeps the core's links from
// leading anywhere but a web origin (D293 §4).
func originOf(named string) string {
	parsed, err := url.Parse(named)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return "https://" + strings.ToLower(parsed.Host)
}

// ErrNotADiscoveryURL is a base URL a core will not enrol with.
var ErrNotADiscoveryURL = errors.New("a discovery's address is https://, or http:// only on this machine")

// checkBaseURL refuses a base URL that is not HTTPS, but for one on this machine's loopback, so a
// claim code and a session never cross a network in the clear.
func checkBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return ErrNotADiscoveryURL
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if address := net.ParseIP(parsed.Hostname()); address != nil && address.IsLoopback() ||
			parsed.Hostname() == "localhost" {
			return nil
		}
	}
	return ErrNotADiscoveryURL
}
