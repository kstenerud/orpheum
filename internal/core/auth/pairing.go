// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/safe32code"
	"github.com/kstenerud/orpheum/internal/core/clientnet"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// How a device is activated (D288): it asks for a code, a person signed in elsewhere enters it, both
// show a check code, and the person says whether they match.
const (
	// How long a pairing works from its code's making: fixed, not a setting, since it bounds how long
	// a code shown on a screen can be taken by someone else (D218 §3).
	pairingLife = 10 * time.Minute
	// How many pairings wait at once, and how many not yet entered one network may hold (clientnet: an
	// IPv6 /64 or an IPv4 address). Past either a new one is refused, never an older one dropped: the
	// oldest giving way let a stranger push out a television's code before it was typed (the security
	// audit's S13, 2026-10-13; D288 §6). A bound on memory and on one network, not on who asks.
	pairingsAtMost           = 1000
	pairingsPerNetworkAtMost = 5
	// A site (clientnet.SiteOf: an IPv6 /48) holds many networks: twenty networks' five codes each
	// filled a hundred, and no television could get a code (the second audit's K5, 2026-10-13).
	pairingsPerSiteAtMost = 20
)

// The states of a pairing, as the device asking sees them (D288).
const (
	// PairingWaiting is a code no one has entered yet.
	PairingWaiting = "waiting"
	// PairingChecking is a code entered, the check code shown on both devices, waiting for the person.
	PairingChecking = "checking"
	// PairingActivated is a pairing the person confirmed: the device takes its session.
	PairingActivated = "activated"
	// PairingEnded is a pairing that is no more: refused, expired, taken, or lost to a restart.
	PairingEnded = "ended"
)

var (
	// ErrNoPairing is a code no device waits with, or a pairing the person answering did not enter.
	ErrNoPairing = errors.New("no device is waiting with that code; check it, or ask the device for a new one")
	// ErrTooManyPairings is a code asked for while as many wait as may, or as many from the asking
	// network: the device asks again once one is entered or expires.
	ErrTooManyPairings = errors.New("too many devices are waiting with a code; try again in a few minutes")
	// ErrNoDeviceName is a new device entered with no name to list it by.
	ErrNoDeviceName = errors.New("a new device needs a name")
	// ErrActivatesAnAdmin is a code entered for another person who is an admin: an admin's devices
	// are their own to activate.
	ErrActivatesAnAdmin = errors.New("an admin's devices are activated by that admin alone")
)

// PreviousDevice is the device a client asking for a code says it was signed in as, and that device's
// secret, its proof (NewDevice): without the proof it is a new device.
type PreviousDevice struct {
	ID    int64
	Proof string
}

// Pairing is a device's code, the secret by which it alone asks how its pairing stands, and when it
// stops working.
type Pairing struct {
	Secret    string
	Code      string
	ExpiresAt time.Time
}

// PairingState is how a device's pairing stands: one of the Pairing states, and the check code while
// it is checked.
type PairingState struct {
	State     string
	CheckCode string
}

// PairingClaim is what entering a code gives the person: the check code the device now shows too,
// and the name the device will be listed by.
type PairingClaim struct {
	CheckCode  string
	DeviceName string
	// Platform is the device's own name for what it is, and AskedAt when it asked for its code: what
	// the person entering it is shown, a code asked for long before being more likely one someone
	// sent them (the device code audit's M3).
	Platform string
	AskedAt  time.Time
}

// pairing is one device's pairing as the core keeps it.
type pairing struct {
	secret, code string
	platform     string
	// previous is the device the asking client says it was, and its proof (NewDevice).
	previous PreviousDevice
	// network and site are where the device asked from (clientnet), by which its codes are counted.
	network, site string
	madeAt        time.Time
	// The person's entering of its code, once entered: by whom, as whom, under what name, and the
	// check code.
	isClaimed             bool
	byUserID, forUserID   int64
	deviceName, checkCode string
	// isConfirmed is the person's match, and issued the session it made, until the device takes it.
	isConfirmed bool
	issued      Issued
	// deviceKey is the public key of the device a code the service issued is for (claimFromService),
	// which its outcome is sealed to, and askedAt when it asked the service; empty for the core's own
	// codes, whose madeAt is when they were asked for.
	deviceKey string
	askedAt   time.Time
}

// pairings are the pairings under way, in memory (D288 §6): a restart ends them.
type pairings struct {
	mutex    sync.Mutex
	bySecret map[string]*pairing
}

// forgetExpiredLocked forgets every pairing expired at now; the mutex is held.
func (kept *pairings) forgetExpiredLocked(now time.Time) {
	for secret, known := range kept.bySecret {
		if now.Sub(known.madeAt) >= pairingLife {
			delete(kept.bySecret, secret)
		}
	}
}

// liveLocked is the pairing a secret names, live at now; the mutex is held.
func (kept *pairings) liveLocked(secret string, now time.Time) (*pairing, bool) {
	kept.forgetExpiredLocked(now)
	known, found := kept.bySecret[secret]
	return known, found
}

// byCodeLocked is the live pairing with code; the mutex is held and expired ones are forgotten.
func (kept *pairings) byCodeLocked(code string) (*pairing, bool) {
	for _, known := range kept.bySecret {
		if known.code == code {
			return known, true
		}
	}
	return nil, false
}

// hasRoomLocked is whether another pairing may wait, asked for from network in site: fewer than
// pairingsAtMost wait, and fewer than pairingsPerNetworkAtMost of network's and
// pairingsPerSiteAtMost of site's not yet entered.
func (kept *pairings) hasRoomLocked(network, site string) bool {
	fromNetwork, fromSite := 0, 0
	for _, known := range kept.bySecret {
		if known.isClaimed {
			continue
		}
		if known.network == network {
			fromNetwork++
		}
		if known.site == site {
			fromSite++
		}
	}
	return len(kept.bySecret) < pairingsAtMost && fromNetwork < pairingsPerNetworkAtMost &&
		fromSite < pairingsPerSiteAtMost
}

// RequestPairing gives a device not signed in a code to show (D288 §1): the one it already has while
// secret names a pairing not yet entered, or a new one, eight characters of safe32 for ten minutes.
// The device names its platform, and the device it was signed in as before, which a person of its
// own resumes (D221); address is where it asks from. ErrTooManyPairings while no new one has room.
func (service *Service) RequestPairing(
	_ context.Context, secret, platform string, previous PreviousDevice, address string,
) (Pairing, error) {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	now := service.clock.Now()
	if known, found := service.pairings.liveLocked(secret, now); found && !known.isClaimed {
		return Pairing{Secret: known.secret, Code: known.code, ExpiresAt: known.madeAt.Add(pairingLife)}, nil
	}
	network, site := clientnet.Of(address), clientnet.SiteOf(address)
	if !service.pairings.hasRoomLocked(network, site) {
		return Pairing{}, ErrTooManyPairings
	}
	newSecret, err := randomSecret(service.random)
	if err != nil {
		return Pairing{}, err
	}
	code, err := service.unusedCodeLocked()
	if err != nil {
		return Pairing{}, err
	}
	service.pairings.bySecret[newSecret] = &pairing{
		secret: newSecret, code: code, platform: textindex.Line(platform), previous: previous,
		network: network, site: site, madeAt: now,
	}
	return Pairing{Secret: newSecret, Code: code, ExpiresAt: now.Add(pairingLife)}, nil
}

// unusedCodeLocked is a new code no live pairing has; the mutex is held.
func (service *Service) unusedCodeLocked() (string, error) {
	for {
		code, err := service.safe32Code()
		if err != nil {
			return "", err
		}
		if _, isTaken := service.pairings.byCodeLocked(code); !isTaken {
			return code, nil
		}
	}
}

// safe32Code is eight random characters of safe32, "xxxx-xxxx" (D218 §3).
func (service *Service) safe32Code() (string, error) {
	raw, err := randbytes.Read(service.random, safe32code.DeviceBytes)
	if err != nil {
		return "", fmt.Errorf("make a code: %w", err)
	}
	return safe32code.Device([safe32code.DeviceBytes]byte(raw)), nil
}

// PairingStateOf is how the pairing secret names stands, for the device that holds it; one not
// known, expired or taken has ended.
func (service *Service) PairingStateOf(_ context.Context, secret string) PairingState {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	known, found := service.pairings.liveLocked(secret, service.clock.Now())
	switch {
	case !found:
		return PairingState{State: PairingEnded}
	case known.issued.Token != "":
		return PairingState{State: PairingActivated}
	case known.isConfirmed:
		return PairingState{State: PairingChecking, CheckCode: known.checkCode}
	case known.isClaimed:
		return PairingState{State: PairingChecking, CheckCode: known.checkCode}
	default:
		return PairingState{State: PairingWaiting}
	}
}

// TakePairedSession is the session a confirmed pairing made, for the device that holds its secret,
// once: the pairing then ends. ErrNoPairing until the person has confirmed it.
func (service *Service) TakePairedSession(_ context.Context, secret string) (Issued, error) {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	known, found := service.pairings.liveLocked(secret, service.clock.Now())
	if !found || known.issued.Token == "" {
		return Issued{}, ErrNoPairing
	}
	delete(service.pairings.bySecret, secret)
	return known.issued, nil
}

// ClaimPairing is a person signed in, byUserID, entering the code a device shows, to activate it as
// forUserID, themselves or, for an admin, someone else, under deviceName (D288 §2): the pairing gets
// a check code, which both devices then show (§3). A device that was forUserID's before resumes its
// own name. What was typed is read as safe32 reads it; a code no device waits with counts against
// the person's failed sign-ins, as a wrong password does.
func (service *Service) ClaimPairing(
	ctx context.Context, byUserID, forUserID int64, typed, deviceName string,
) (PairingClaim, error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return PairingClaim{}, err
	}
	limitKey := "pairing-user:" + strconv.FormatInt(byUserID, 10)
	if service.limiter.isLimited(maxFailures, window, limitKey) {
		return PairingClaim{}, ErrRateLimited
	}
	if userErr := service.mayActivateFor(ctx, byUserID, forUserID); userErr != nil {
		return PairingClaim{}, userErr
	}
	code, readErr := safe32code.Read(typed, safe32code.DeviceLength)
	// A code the core did not issue may be the paid service's (D293 §2).
	claim, isServices, claimErr := service.claimFromService(ctx, byUserID, forUserID, code, deviceName)
	if claimErr != nil || isServices {
		return claim, claimErr
	}
	previous, found := service.waitingDevice(code)
	if readErr != nil || !found {
		service.limiter.recordFailure(window, limitKey)
		return PairingClaim{}, ErrNoPairing
	}
	name := textindex.Line(deviceName)
	isProven, err := isProvenDevice(ctx, service.database.Read(), service.clock.Now().UnixMilli(), forUserID,
		previous.ID, previous.Proof)
	if err != nil {
		return PairingClaim{}, err
	}
	if isProven {
		if known, deviceErr := liveDevice(ctx, service.database.Read(), forUserID, previous.ID); deviceErr == nil {
			name = known.Name
		}
	}
	if name == "" {
		return PairingClaim{}, ErrNoDeviceName
	}
	checkCode, err := service.safe32Code()
	if err != nil {
		return PairingClaim{}, err
	}
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	known, found := service.pairings.byCodeLocked(code)
	if !found || known.isClaimed {
		return PairingClaim{}, ErrNoPairing
	}
	known.isClaimed, known.byUserID, known.forUserID = true, byUserID, forUserID
	known.deviceName, known.checkCode = name, checkCode
	return PairingClaim{CheckCode: checkCode, DeviceName: name, Platform: known.platform, AskedAt: known.madeAt}, nil
}

// waitingDevice is the device a live pairing not yet entered says it was signed in as before, none
// when it says none, and whether a pairing with code waits.
func (service *Service) waitingDevice(code string) (PreviousDevice, bool) {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	known, found := service.pairings.byCodeLocked(code)
	if !found || known.isClaimed {
		return PreviousDevice{}, false
	}
	return known.previous, true
}

// AnswerPairing is the person who entered a code saying whether the check codes match (D288 §3):
// only a match activates the device, signed in as the person it was entered for (§4); no match ends
// the pairing. ErrNoPairing for a code they did not enter.
func (service *Service) AnswerPairing(ctx context.Context, byUserID int64, typed string, isMatch bool) error {
	code, err := safe32code.Read(typed, safe32code.DeviceLength)
	if err != nil {
		return ErrNoPairing
	}
	known, err := service.answered(byUserID, code, isMatch)
	if err != nil {
		return err
	}
	if known.deviceKey != "" && !isMatch {
		return service.answerService(ctx, known, Issued{}, false)
	}
	if !isMatch {
		return nil
	}
	// Made with the pairing let go, as a database write is: confirmed, it is answered no more. A
	// device a code activated carries the device scope, whoever entered the code (D292 §4; A11).
	issued, err := service.issueSession(ctx, known.forUserID, NewDevice{
		Name: known.deviceName, Platform: known.platform, PreviousDeviceID: known.previous.ID,
		PreviousDeviceProof: known.previous.Proof,
	}, grantScopeDevice)
	if err != nil {
		return err
	}
	if known.deviceKey != "" {
		if answerErr := service.answerService(ctx, known, issued, true); answerErr != nil {
			return answerErr
		}
		service.tellActivated(ctx, known)
		return nil
	}
	service.pairings.mutex.Lock()
	if kept, found := service.pairings.bySecret[known.secret]; found {
		kept.issued = issued
	}
	service.pairings.mutex.Unlock()
	service.tellActivated(ctx, known)
	return nil
}

// answered is the pairing byUserID entered with code, as it was, now confirmed, or forgotten when
// the check codes did not match; ErrNoPairing for one they did not enter or have answered.
func (service *Service) answered(byUserID int64, code string, isMatch bool) (pairing, error) {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	known, found := service.pairings.byCodeLocked(code)
	if !found || !known.isClaimed || known.byUserID != byUserID || known.isConfirmed {
		return pairing{}, ErrNoPairing
	}
	if !isMatch {
		delete(service.pairings.bySecret, known.secret)
		return *known, nil
	}
	known.isConfirmed = true
	return *known, nil
}

// mayActivateFor is whether byUserID may enter a code for forUserID: a live person, and an admin's
// device activated by that admin alone, since another admin's session, stolen or not, would
// otherwise plant a device that administers as them (the Plex-chain review's T1).
func (service *Service) mayActivateFor(ctx context.Context, byUserID, forUserID int64) error {
	forUser, err := liveUser(ctx, service.database.Read(), forUserID)
	if err != nil {
		return err
	}
	if forUserID != byUserID && slices.Contains(strings.Split(forUser.Capabilities, ","), capabilityAdminister) {
		return ErrActivatesAnAdmin
	}
	return nil
}
