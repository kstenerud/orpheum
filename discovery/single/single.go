// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/safe32code"
)

// The lifetimes of what it issues: a nonce, a session's token, a claim code (the spec's §3.2, §3.3).
const (
	nonceLasts   = 5 * time.Minute
	sessionLasts = time.Hour
	claimLasts   = 30 * time.Minute
)

// A nonce carries its own expiry and a MAC under a key only this discovery holds, so issuing one
// keeps nothing: a stranger asking for challenges again and again fills no table. A cap of 256
// outstanding nonces let one refuse the core's own sessions (the security audit's S13, 2026-10-13).
// What is kept is the nonces a signature has spent, until they expire, so each works once.
const (
	nonceRandomBytes = 16
	nonceMACBytes    = 16
	nonceKeyBytes    = 32
)

// secretBytes is the random bytes behind a nonce, a session's token and a core's id.
const secretBytes = 24

// coreIDChars is a core's id's length, in safe32 without dashes: a label in the core's names, of
// 130 bits, too many to guess (the protocol's §2.7; the security audit's A3).
const coreIDChars = 26

// bodyMost is the largest request body it reads: a heartbeat's candidates are a few kilobytes.
const bodyMost = 64 << 10

// Errors Claim returns.
var (
	ErrNoCore       = errors.New("no core has enrolled with this discovery")
	ErrNotPending   = errors.New("the core is not waiting to be claimed")
	ErrWrongCode    = errors.New("that is not the core's claim code")
	ErrCodeExpired  = errors.New("the claim code has expired: the core's System page shows a new one")
	errAnotherCore  = errors.New("this discovery holds another core")
	errNoDNS        = errors.New("this discovery sets no DNS records: it was given no DNS provider")
	errNotCoresName = errors.New("not the core's name")
	errNotDigest    = errors.New("not a DNS-01 value: a SHA-256 digest in base64url, 43 characters")
	errNotClaimed   = errors.New("the core is not claimed yet: its owner claims it by the code its System page shows")
)

// TXTRecords sets and clears the TXT records of the core's certificate's DNS-01 challenge, in the
// owner's own DNS (the spec's §3.7).
type TXTRecords interface {
	SetTXT(ctx context.Context, name, value string) error
	ClearTXT(ctx context.Context, name string) error
}

// Parts is what a single-target discovery is made of.
type Parts struct {
	// BaseURL is the address the core calls this discovery at, path included, which its signatures
	// are bound to (the protocol's §2.1; X12).
	BaseURL string
	// Zone is the owner's zone: the core's names are under <coreId>.<Zone>.
	Zone string
	// ClientOrigin is where the owner serves the client, which the core's links lead to (D293 §4).
	ClientOrigin string
	Store        Store
	// Now is the time; Random, where its nonces, tokens, ids and codes come from.
	Now    func() time.Time
	Random io.Reader
	// ICEServers are the STUN servers the core gathers its candidates through: the discovery's own.
	ICEServers []discoveryproto.ICEServer
	// TXT sets the certificate's records; nil refuses them.
	TXT    TXTRecords
	Logger *slog.Logger
}

// Discovery is a single-target discovery: its record in its store, and in memory its nonces, its
// sessions and its core's signalling connection, which a restart forgets (the core opens a new
// session on its next call's refusal).
type Discovery struct {
	parts Parts
	mutex sync.Mutex
	// nonceKey signs the nonces it issues; spent is each nonce a signature has used, until it expires.
	nonceKey []byte
	spent    map[string]time.Time
	// tokens are the sessions, by their tokens; instance is the core's live instance, empty when no
	// session is open (D293 §1).
	tokens   map[string]session
	instance string
	signal   *signalling
	// pingWait is how long a live instance has to answer: instancePingWait, shorter in the tests.
	pingWait time.Duration
}

// New returns a discovery over parts, with a key for its nonces from parts.Random.
func New(parts Parts) (*Discovery, error) {
	if parts.Logger == nil {
		parts.Logger = slog.New(slog.DiscardHandler)
	}
	nonceKey := make([]byte, nonceKeyBytes)
	if _, err := io.ReadFull(parts.Random, nonceKey); err != nil {
		return nil, fmt.Errorf("make the nonces' key: %w", err)
	}
	return &Discovery{
		parts: parts, nonceKey: nonceKey, spent: map[string]time.Time{}, tokens: map[string]session{},
		signal: newSignalling(parts.Logger), pingWait: instancePingWait,
	}, nil
}

// Handler answers the core's calls, at paths relative to the discovery's base URL.
func (discovery *Discovery) Handler() http.Handler {
	return http.HandlerFunc(discovery.serve)
}

func (discovery *Discovery) serve(writer http.ResponseWriter, request *http.Request) {
	// Every call says its version, the signalling connection's too; any minor of a served major is
	// answered (the protocol's §5).
	version := request.Header.Get(discoveryproto.ProtocolHeader)
	if !discoveryproto.Serves([]int{discoveryproto.ProtocolMajor}, version) {
		refuse(writer, http.StatusBadRequest, discoveryproto.RefusalVersion, fmt.Sprintf(
			"%s %q: this discovery serves %d.x", discoveryproto.ProtocolHeader, version, discoveryproto.ProtocolMajor))
		return
	}
	if request.URL.Path == discoveryproto.PathSignal && request.Method == http.MethodGet {
		held, opened, isSignedIn := discovery.signedIn(writer, request)
		if !isSignedIn {
			return
		}
		if held.State != discoveryproto.StateClaimed {
			refuse(writer, http.StatusForbidden, discoveryproto.RefusalNotClaimed, errNotClaimed.Error())
			return
		}
		discovery.signal.accept(writer, request, opened.instanceID)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, bodyMost)
	switch request.Method + " " + request.URL.Path {
	case http.MethodPost + " " + discoveryproto.PathChallenge:
		discovery.challenge(writer, request)
	case http.MethodPost + " " + discoveryproto.PathEnrol:
		discovery.enrol(writer, request)
	case http.MethodPost + " " + discoveryproto.PathSession:
		discovery.session(writer, request)
	default:
		held, _, isSignedIn := discovery.signedIn(writer, request)
		if !isSignedIn {
			return
		}
		discovery.serveSignedIn(writer, request, held)
	}
}

// serveSignedIn answers the calls a session's token is needed for. A pending core asks how it stands
// and may leave; its addresses and its certificate's record wait until it is claimed, so a stranger
// holding a pending core's place publishes nothing and gets no certificate (the security audit's A3).
func (discovery *Discovery) serveSignedIn(writer http.ResponseWriter, request *http.Request, held Core) {
	call := request.Method + " " + request.URL.Path
	switch call {
	case http.MethodGet + " " + discoveryproto.PathStatus:
		discovery.status(writer, request)
		return
	case http.MethodDelete + " " + discoveryproto.PathCore:
		discovery.leave(writer, request)
		return
	}
	if held.State != discoveryproto.StateClaimed {
		refuse(writer, http.StatusForbidden, discoveryproto.RefusalNotClaimed, errNotClaimed.Error())
		return
	}
	cleared, isClearing := strings.CutPrefix(request.URL.Path, discoveryproto.PathACMEChallenge+"/")
	switch {
	case call == http.MethodPut+" "+discoveryproto.PathAddresses:
		discovery.addresses(writer, request)
	case call == http.MethodPut+" "+discoveryproto.PathACMEChallenge:
		var challenge discoveryproto.ACMEChallengeRequest
		if read(writer, request, &challenge) {
			discovery.acmeChallenge(writer, request, held, challenge)
		}
	case request.Method == http.MethodDelete && isClearing:
		discovery.acmeChallenge(writer, request, held, discoveryproto.ACMEChallengeRequest{Name: cleared})
	default:
		http.NotFound(writer, request)
	}
}

// challenge issues a nonce to sign, for enrolling or a session: its expiry, random bytes and their
// MAC, keeping nothing.
func (discovery *Discovery) challenge(writer http.ResponseWriter, request *http.Request) {
	expires := uint64(discovery.parts.Now().Add(nonceLasts).UnixMilli())
	body := binary.BigEndian.AppendUint64(make([]byte, 0, 8+nonceRandomBytes+nonceMACBytes), expires)
	body = body[:8+nonceRandomBytes]
	if _, err := io.ReadFull(discovery.parts.Random, body[8:]); err != nil {
		discovery.fail(writer, request, fmt.Errorf("make a nonce: %w", err))
		return
	}
	nonce := slices.Concat(body, discovery.nonceMAC(body))
	answer(writer, discoveryproto.ChallengeResponse{Nonce: base64.RawURLEncoding.EncodeToString(nonce)})
}

// nonceMAC is the MAC of a nonce's expiry and random bytes under the discovery's key.
func (discovery *Discovery) nonceMAC(body []byte) []byte {
	mac := hmac.New(sha256.New, discovery.nonceKey)
	mac.Write(body)
	return mac.Sum(nil)[:nonceMACBytes]
}

// nonceExpiry is when a nonce this discovery issued expires, and whether it is one.
func (discovery *Discovery) nonceExpiry(nonce string) (time.Time, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(raw) != 8+nonceRandomBytes+nonceMACBytes {
		return time.Time{}, false
	}
	body := raw[:8+nonceRandomBytes]
	if !hmac.Equal(raw[8+nonceRandomBytes:], discovery.nonceMAC(body)) {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(binary.BigEndian.Uint64(body))), true //nolint:gosec // written from a time above
}

// isSigned is whether signature is publicKey's over a nonce this discovery issued, not expired and
// not yet spent, for purpose, bound to this discovery and core; one a valid signature used is spent,
// and kept until it expires.
func (discovery *Discovery) isSigned(publicKey ed25519.PublicKey, purpose, core, nonce string, signature []byte) bool {
	expires, isIssued := discovery.nonceExpiry(nonce)
	now := discovery.parts.Now()
	if !isIssued || !now.Before(expires) ||
		!proof.Verify(publicKey, purpose, discovery.parts.BaseURL, core, nonce, signature) {
		return false
	}
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	for each, until := range discovery.spent {
		if !now.Before(until) {
			delete(discovery.spent, each)
		}
	}
	if _, isSpent := discovery.spent[nonce]; isSpent {
		return false
	}
	discovery.spent[nonce] = expires
	return true
}

// enrol takes the core whose key signed: a new one, the same one again, or a pending one whose code
// expired unclaimed. While its core is claimed, or pending with a live code, it refuses any other key:
// it holds one core, and a stranger cannot take a pending core's place (the security audit's A3).
// A core replaced takes every session and the signalling connection with it.
func (discovery *Discovery) enrol(writer http.ResponseWriter, request *http.Request) {
	var enrolling discoveryproto.EnrolRequest
	if !read(writer, request, &enrolling) {
		return
	}
	if !discovery.isSigned(enrolling.PublicKey, discoveryproto.PurposeEnrol, proof.KeyLabel(enrolling.PublicKey),
		enrolling.Nonce, enrolling.Signature) {
		http.Error(writer, "the signature is not over a nonce this discovery issued", http.StatusForbidden)
		return
	}
	var enrolled Core
	isReplaced := false
	err := discovery.parts.Store.Update(func(record *Record) error {
		held := record.Core
		switch {
		case held != nil && string(held.PublicKey) == string(enrolling.PublicKey):
		case held != nil && (held.State != discoveryproto.StatePending ||
			discovery.parts.Now().Before(time.UnixMilli(held.ClaimExpiresAt))):
			return errAnotherCore
		default:
			isReplaced = held != nil
			id, idErr := discovery.coreID()
			if idErr != nil {
				return idErr
			}
			held = &Core{ID: id, PublicKey: enrolling.PublicKey, State: discoveryproto.StatePending}
		}
		if held.State == discoveryproto.StatePending {
			if codeErr := discovery.newClaimCode(held); codeErr != nil {
				return codeErr
			}
		}
		record.Core, enrolled = held, *held
		return nil
	})
	if errors.Is(err, errAnotherCore) {
		http.Error(writer, "this discovery holds another core: it holds one core only", http.StatusConflict)
		return
	}
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	if isReplaced {
		discovery.forgetSessions("another core took the place of the one whose code expired")
	}
	discovery.parts.Logger.InfoContext(request.Context(), "discovery: a core enrolled",
		"coreId", enrolled.ID, "state", enrolled.State)
	answer(writer, discoveryproto.EnrolResponse{
		CoreID: enrolled.ID, ClaimCode: enrolled.ClaimCode, ClaimExpiresAt: enrolled.ClaimExpiresAt,
	})
}

// status is where the core stands: pending with its claim code, a new one once the last expired, or
// claimed with its zone; and the STUN servers it gathers its candidates through.
func (discovery *Discovery) status(writer http.ResponseWriter, request *http.Request) {
	var held Core
	err := discovery.parts.Store.Update(func(record *Record) error {
		if record.Core == nil {
			return ErrNoCore
		}
		if record.Core.State == discoveryproto.StatePending &&
			!discovery.parts.Now().Before(time.UnixMilli(record.Core.ClaimExpiresAt)) {
			if codeErr := discovery.newClaimCode(record.Core); codeErr != nil {
				return codeErr
			}
		}
		held = *record.Core
		return nil
	})
	if errors.Is(err, ErrNoCore) {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	status := discoveryproto.StatusResponse{
		State: held.State, ICEServers: discovery.parts.ICEServers, ClientOrigin: discovery.parts.ClientOrigin,
	}
	if held.State == discoveryproto.StatePending {
		status.ClaimCode, status.ClaimExpiresAt = held.ClaimCode, held.ClaimExpiresAt
	} else {
		status.Zone = discovery.parts.Zone
	}
	answer(writer, status)
}

// addresses keeps the core's heartbeat. It publishes no names and probes nothing yet (W4b.4): its
// answer says so by listing none.
func (discovery *Discovery) addresses(writer http.ResponseWriter, request *http.Request) {
	var heartbeat discoveryproto.AddressesRequest
	if !read(writer, request, &heartbeat) {
		return
	}
	err := discovery.parts.Store.Update(func(record *Record) error {
		if record.Core == nil {
			return ErrNoCore
		}
		record.Core.Addresses, record.Core.LastSeenAt = &heartbeat, discovery.parts.Now().UnixMilli()
		return nil
	})
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	discovery.parts.Logger.InfoContext(request.Context(), "discovery: the core sent its addresses",
		"candidates", len(heartbeat.Candidates), "orpheumVersion", heartbeat.OrpheumVersion)
	answer(writer, discoveryproto.AddressesResponse{Published: []string{}, Probe: []discoveryproto.ProbeResult{}})
}

// acmeChallenge sets the core's certificate's DNS-01 record, or clears it on a DELETE: the core's own
// name alone, and a value only of the shape DNS-01 gives (S18).
func (discovery *Discovery) acmeChallenge(writer http.ResponseWriter, request *http.Request, held Core,
	challenge discoveryproto.ACMEChallengeRequest,
) {
	var err error
	switch {
	case challenge.Name != "_acme-challenge."+held.ID+"."+discovery.parts.Zone:
		err = errNotCoresName
	case request.Method != http.MethodDelete && !isDNS01Digest(challenge.Value):
		err = errNotDigest
	case discovery.parts.TXT == nil:
		err = errNoDNS
	case request.Method == http.MethodDelete:
		err = discovery.parts.TXT.ClearTXT(request.Context(), challenge.Name)
	default:
		err = discovery.parts.TXT.SetTXT(request.Context(), challenge.Name, challenge.Value)
	}
	switch {
	case errors.Is(err, errNotCoresName):
		http.Error(writer, "not the core's name: _acme-challenge.<coreId>."+discovery.parts.Zone, http.StatusForbidden)
	case errors.Is(err, errNotDigest):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	case errors.Is(err, errNoDNS):
		http.Error(writer, err.Error(), http.StatusNotImplemented)
	case err != nil:
		discovery.fail(writer, request, err)
	}
}

// isDNS01Digest reports whether value is what DNS-01 puts in a TXT record: a SHA-256 digest in
// unpadded base64url, 43 characters (RFC 8555 §8.4).
func isDNS01Digest(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

// leave forgets the core, at its asking, with its sessions and its signalling connection.
func (discovery *Discovery) leave(writer http.ResponseWriter, request *http.Request) {
	if err := discovery.parts.Store.Update(func(record *Record) error {
		record.Core = nil
		return nil
	}); err != nil {
		discovery.fail(writer, request, err)
		return
	}
	discovery.forgetSessions("the core left")
	discovery.parts.Logger.InfoContext(request.Context(), "discovery: the core left")
	writer.WriteHeader(http.StatusNoContent)
}

// Claim binds the pending core whose claim code was typed: `orpheum discovery claim <code>` (D274 §5).
func (discovery *Discovery) Claim(typed string) error {
	code, err := safe32code.Read(typed, safe32code.DeviceLength)
	if err != nil {
		return ErrWrongCode
	}
	return discovery.parts.Store.Update(func(record *Record) error {
		held := record.Core
		switch {
		case held == nil:
			return ErrNoCore
		case held.State != discoveryproto.StatePending:
			return ErrNotPending
		case held.ClaimCode != code:
			return ErrWrongCode
		case !discovery.parts.Now().Before(time.UnixMilli(held.ClaimExpiresAt)):
			return ErrCodeExpired
		}
		held.State, held.ClaimCode, held.ClaimExpiresAt = discoveryproto.StateClaimed, "", 0
		return nil
	})
}

// newClaimCode gives a pending core a new claim code, lasting claimLasts.
func (discovery *Discovery) newClaimCode(held *Core) error {
	var raw [safe32code.DeviceBytes]byte
	if _, err := io.ReadFull(discovery.parts.Random, raw[:]); err != nil {
		return fmt.Errorf("make a claim code: %w", err)
	}
	held.ClaimCode = safe32code.Device(raw)
	held.ClaimExpiresAt = discovery.parts.Now().Add(claimLasts).UnixMilli()
	return nil
}

// coreID is a new core's id: coreIDChars safe32 characters, a label in its names.
func (discovery *Discovery) coreID() (string, error) {
	raw := make([]byte, (coreIDChars*5+7)/8)
	if _, err := io.ReadFull(discovery.parts.Random, raw); err != nil {
		return "", fmt.Errorf("make a core's id: %w", err)
	}
	written, err := safe32code.WriteBytes(raw, coreIDChars)
	if err != nil {
		return "", fmt.Errorf("write a core's id: %w", err)
	}
	return strings.ReplaceAll(written, "-", ""), nil
}

// secret is a new nonce or token.
func (discovery *Discovery) secret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := io.ReadFull(discovery.parts.Random, raw); err != nil {
		return "", fmt.Errorf("make a secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// fail answers a failure of its own, logging why.
func (discovery *Discovery) fail(writer http.ResponseWriter, request *http.Request, err error) {
	discovery.parts.Logger.ErrorContext(request.Context(), "discovery: a call failed", "err", err)
	http.Error(writer, "the discovery failed: "+err.Error(), http.StatusInternalServerError)
}

// read reads request's JSON body into value, answering a body it cannot read as a bad request.
func read(writer http.ResponseWriter, request *http.Request, value any) bool {
	if err := json.NewDecoder(request.Body).Decode(value); err != nil {
		http.Error(writer, "the body is not the call's JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// answer writes value as the JSON answer; one it cannot write, the core reads as a failed call.
func answer(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		http.Error(writer, "the discovery could not write its answer", http.StatusInternalServerError)
	}
}
