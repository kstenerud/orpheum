// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// ServiceDeviceCodes are the device codes the paid service issues to devices that know no core
// (D293 §2): a code a person enters that this core did not issue is claimed there, and its outcome
// sealed to the device's key. The core's discovery is one (discovery.Service).
type ServiceDeviceCodes interface {
	// ClaimDeviceCode is the device waiting with code, and whether one waits; an error is the service
	// not reached. coreNonce is the core's part of the check code, none for a scanned code (D321).
	ClaimDeviceCode(ctx context.Context, code, coreNonce string) (discoveryproto.DeviceCodeClaimResponse, bool, error)
	// ActivateDeviceCode seals what the device needs, with the code of the session kept for it, to its
	// key, and passes it on.
	ActivateDeviceCode(ctx context.Context, code, publicKey, handOffCode, person string) error
	// RefuseDeviceCode ends the code, the check codes not matching.
	RefuseDeviceCode(ctx context.Context, code string) error
}

// revealEvery is how often the core asks the service, while entering a typed code waits
// deviceCodeRevealWaitSeconds for the device to reveal its nonce: the device asks how its code
// stands every two seconds and reveals at once (D321).
const revealEvery = 500 * time.Millisecond

// ErrDeviceSilent is a device whose typed code was entered that did not reveal its nonce in time, and
// ErrNotThatDevice a device that is not the one that showed the code: its nonce breaks its commitment,
// or its key is not the one a scanned QR code names (D321). Either ends the code.
var (
	ErrDeviceSilent  = errors.New("the device did not answer; check that it still shows the code, and enter it again")
	ErrNotThatDevice = errors.New("the device that answered is not the one that showed the code; ask it for a new one")
)

// ClaimDeviceCodesFrom has a code this core did not issue claimed from codes; set before the core
// serves. None set, and such a code is one no device waits with.
func (service *Service) ClaimDeviceCodesFrom(codes ServiceDeviceCodes) {
	service.serviceDeviceCodes = codes
}

// claimFromService is ClaimPairing for a code the core did not issue: the service's device waiting
// with it becomes a pairing entered by byUserID for forUserID under deviceName. Typed, its check code
// is committed: the core claims with a nonce of its own, waits for the device's, and makes the check
// code of the key and both (proof.DeviceCheckCode). Scanned, keyHash is the device's QR code's, the
// core claims with no nonce, takes only the key it names, and there is no check code; with no name
// given, the device is listed by its platform (D321). Whether the code is the service's: none that is
// not a code, or that one of the core's own devices waits with; entering it again gives the same claim.
func (service *Service) claimFromService(
	ctx context.Context, byUserID, forUserID int64, code, keyHash, deviceName string,
) (PairingClaim, bool, error) {
	if service.serviceDeviceCodes == nil || code == "" {
		return PairingClaim{}, false, nil
	}
	if _, isOwn := service.waitingDevice(code); isOwn {
		return PairingClaim{}, false, nil
	}
	if known, found := service.claimedServiceCode(code); found {
		if known.byUserID != byUserID || known.isConfirmed {
			return PairingClaim{}, false, nil
		}
		return claimOf(known), true, nil
	}
	coreNonce, err := service.coreNonceFor(keyHash)
	if err != nil {
		return PairingClaim{}, true, err
	}
	waiting, isWaiting, err := service.serviceDeviceCodes.ClaimDeviceCode(ctx, code, coreNonce)
	if errors.Is(err, discoveryproto.ErrTooManyClaims) {
		return PairingClaim{}, false, ErrRateLimited
	}
	if err != nil || !isWaiting {
		return PairingClaim{}, false, err
	}
	if scannedErr := service.isScannedDevice(ctx, code, keyHash, waiting); scannedErr != nil {
		return PairingClaim{}, true, scannedErr
	}
	name := serviceDeviceName(deviceName, keyHash, waiting)
	if name == "" {
		return PairingClaim{}, true, ErrNoDeviceName
	}
	checkCode := ""
	if keyHash == "" {
		if checkCode, err = service.checkCodeOnceRevealed(ctx, code, coreNonce, waiting); err != nil {
			return PairingClaim{}, true, err
		}
	}
	known, err := service.keepServicePairing(byUserID, forUserID, code, name, checkCode, waiting)
	return claimOf(known), true, err
}

// coreNonceFor is the core's nonce for a typed code's check code, fresh each claim; none for a scanned
// code (keyHash), which has no check code.
func (service *Service) coreNonceFor(keyHash string) (string, error) {
	if keyHash != "" {
		return "", nil
	}
	return randbytes.Base64URLSecret(service.random, proof.DeviceNonceBytes)
}

// isScannedDevice is nil unless a scanned code's device has another key than its QR code's hash names,
// which ends the code; a typed code (no keyHash) is nil.
func (service *Service) isScannedDevice(
	ctx context.Context, code, keyHash string, waiting discoveryproto.DeviceCodeClaimResponse,
) error {
	if keyHash == "" {
		return nil
	}
	if hash, err := proof.DeviceKeyHash(waiting.PublicKey); err == nil && hash == keyHash {
		return nil
	}
	return errors.Join(ErrNotThatDevice, service.serviceDeviceCodes.RefuseDeviceCode(ctx, code))
}

// serviceDeviceName is the name a service's device is listed by: the one given, or for a scanned code
// (keyHash) with none given, its platform.
func serviceDeviceName(deviceName, keyHash string, waiting discoveryproto.DeviceCodeClaimResponse) string {
	name := textindex.Line(deviceName)
	if name == "" && keyHash != "" {
		return textindex.Line(waiting.Platform)
	}
	return name
}

// keepServicePairing keeps the pairing a service's code made, entered by byUserID for forUserID.
func (service *Service) keepServicePairing(
	byUserID, forUserID int64, code, name, checkCode string, waiting discoveryproto.DeviceCodeClaimResponse,
) (pairing, error) {
	secret, err := randomSecret(service.random)
	if err != nil {
		return pairing{}, err
	}
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	now := service.clock.Now()
	known := &pairing{
		secret: secret, code: code, platform: textindex.Line(waiting.Platform), madeAt: now,
		isClaimed: true, byUserID: byUserID, forUserID: forUserID, deviceName: name, checkCode: checkCode,
		deviceKey: waiting.PublicKey, askedAt: now.Add(-time.Duration(max(waiting.AskedSecondsAgo, 0)) * time.Second),
	}
	service.pairings.bySecret[secret] = known
	return *known, nil
}

// claimedServiceCode is the pairing a service's code made when it was entered before, if any.
func (service *Service) claimedServiceCode(code string) (pairing, bool) {
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	known, found := service.pairings.byCodeLocked(code)
	if !found || known.deviceKey == "" {
		return pairing{}, false
	}
	return *known, true
}

// checkCodeOnceRevealed is a typed code's check code, made once the device reveals the nonce it
// committed to: the core claims again, with the same nonce, until the service has it, at most
// deviceCodeRevealWaitSeconds. A nonce that breaks the commitment is another device's, and ends the
// code (D321).
func (service *Service) checkCodeOnceRevealed(
	ctx context.Context, code, coreNonce string, waiting discoveryproto.DeviceCodeClaimResponse,
) (string, error) {
	revealWait, readErr := service.settings.Duration(ctx, settings.DeviceCodeRevealWaitSeconds)
	if readErr != nil {
		return "", readErr
	}
	deadline := service.clock.Now().Add(revealWait)
	for waiting.DeviceNonce == "" {
		if !service.clock.Now().Before(deadline) {
			return "", errors.Join(ErrDeviceSilent, service.serviceDeviceCodes.RefuseDeviceCode(ctx, code))
		}
		if err := service.clock.Sleep(ctx, revealEvery); err != nil {
			return "", fmt.Errorf("wait for the device's nonce: %w", err)
		}
		again, isWaiting, err := service.serviceDeviceCodes.ClaimDeviceCode(ctx, code, coreNonce)
		if err != nil {
			return "", err
		}
		if !isWaiting {
			return "", ErrNoPairing
		}
		waiting = again
	}
	commitment, err := proof.DeviceCommitment(waiting.PublicKey, waiting.DeviceNonce)
	if err != nil || commitment != waiting.Commitment {
		return "", errors.Join(ErrNotThatDevice, service.serviceDeviceCodes.RefuseDeviceCode(ctx, code))
	}
	checkCode, err := proof.DeviceCheckCode(waiting.PublicKey, waiting.DeviceNonce, coreNonce)
	if err != nil {
		return "", errors.Join(ErrNotThatDevice, service.serviceDeviceCodes.RefuseDeviceCode(ctx, code))
	}
	return checkCode, nil
}

// claimOf is what entering a service's code shows the person: its check code, the device's name and
// platform, and when it asked.
func claimOf(known pairing) PairingClaim {
	return PairingClaim{
		CheckCode: known.checkCode, DeviceName: known.deviceName, Platform: known.platform, AskedAt: known.askedAt,
	}
}

// answerService is AnswerPairing's end for a pairing the service's code made: a match keeps the
// session issued for the device's client origin to take, under the code's person, and passes its
// code on sealed; no match ends the code at the service.
func (service *Service) answerService(ctx context.Context, known pairing, issued Issued, isMatch bool) error {
	if !isMatch {
		return service.serviceDeviceCodes.RefuseDeviceCode(ctx, known.code)
	}
	handOffCode, err := service.keepHandOff(ctx, issued, proof.DeviceCodePerson(known.code), deviceHandOffLife)
	if err != nil {
		return err
	}
	service.pairings.mutex.Lock()
	delete(service.pairings.bySecret, known.secret)
	service.pairings.mutex.Unlock()
	person, err := liveUser(ctx, service.database.Read(), known.forUserID)
	if err != nil {
		return err
	}
	return service.serviceDeviceCodes.ActivateDeviceCode(ctx, known.code, known.deviceKey, handOffCode, person.Name)
}

// Activation is a device a code activated, as the core's log records it (the device code audit's
// M4): who entered the code, for whom, the device's name and platform, and whether the code was the
// paid service's.
type Activation struct {
	ByUserID, ForUserID  int64
	DeviceName, Platform string
	IsThroughService     bool
}

// OnDeviceActivated tells activated of each device a code activates; set before the core serves.
func (service *Service) OnDeviceActivated(activated func(context.Context, Activation)) {
	service.onActivated = activated
}

// tellActivated tells OnDeviceActivated's listener of known's activation, when there is one.
func (service *Service) tellActivated(ctx context.Context, known pairing) {
	if service.onActivated == nil {
		return
	}
	service.onActivated(ctx, Activation{
		ByUserID: known.byUserID, ForUserID: known.forUserID, DeviceName: known.deviceName,
		Platform: known.platform, IsThroughService: known.deviceKey != "",
	})
}
