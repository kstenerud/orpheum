// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// ServiceDeviceCodes are the device codes the paid service issues to devices that know no core
// (D293 §2): a code a person enters that this core did not issue is claimed there, and its outcome
// sealed to the device's key. The core's discovery is one (discovery.Service).
type ServiceDeviceCodes interface {
	// ClaimDeviceCode is the device waiting with code, and whether one waits; an error is the service
	// not reached.
	ClaimDeviceCode(ctx context.Context, code string) (discoveryproto.DeviceCodeClaimResponse, bool, error)
	// ActivateDeviceCode seals what the device needs, with the code of the session kept for it, to its
	// key, and passes it on.
	ActivateDeviceCode(ctx context.Context, code, publicKey, handOffCode string) error
	// RefuseDeviceCode ends the code, the check codes not matching.
	RefuseDeviceCode(ctx context.Context, code string) error
}

// ClaimDeviceCodesFrom has a code this core did not issue claimed from codes; set before the core
// serves. None set, and such a code is one no device waits with.
func (service *Service) ClaimDeviceCodesFrom(codes ServiceDeviceCodes) {
	service.serviceDeviceCodes = codes
}

// claimFromService is ClaimPairing for a code the core did not issue: the service's device waiting
// with it becomes a pairing entered by byUserID for forUserID under deviceName, its check code made
// of the device's key, so the device shows it without asking the core (proof.DeviceCheckCode).
// Whether the code is the service's: none that is not a code, or that one of the core's own devices
// waits with; entering it again gives the same claim.
func (service *Service) claimFromService(
	ctx context.Context, byUserID, forUserID int64, code, deviceName string,
) (PairingClaim, bool, error) {
	if service.serviceDeviceCodes == nil || code == "" {
		return PairingClaim{}, false, nil
	}
	if _, isOwn := service.waitingDevice(code); isOwn {
		return PairingClaim{}, false, nil
	}
	waiting, isWaiting, err := service.serviceDeviceCodes.ClaimDeviceCode(ctx, code)
	if err != nil || !isWaiting {
		return PairingClaim{}, false, err
	}
	name := textindex.Line(deviceName)
	if name == "" {
		return PairingClaim{}, true, ErrNoDeviceName
	}
	checkCode, err := proof.DeviceCheckCode(waiting.PublicKey)
	if err != nil {
		return PairingClaim{}, false, nil
	}
	secret, err := randomSecret(service.random)
	if err != nil {
		return PairingClaim{}, true, err
	}
	service.pairings.mutex.Lock()
	defer service.pairings.mutex.Unlock()
	service.pairings.forgetExpiredLocked(service.clock.Now())
	if known, found := service.pairings.byCodeLocked(code); found {
		if known.deviceKey == "" || known.byUserID != byUserID || known.isConfirmed {
			return PairingClaim{}, false, nil
		}
		return PairingClaim{CheckCode: known.checkCode, DeviceName: known.deviceName}, true, nil
	}
	service.pairings.bySecret[secret] = &pairing{
		secret: secret, code: code, platform: textindex.Line(waiting.Platform), madeAt: service.clock.Now(),
		isClaimed: true, byUserID: byUserID, forUserID: forUserID, deviceName: name, checkCode: checkCode,
		deviceKey: waiting.PublicKey,
	}
	return PairingClaim{CheckCode: checkCode, DeviceName: name}, true, nil
}

// answerService is AnswerPairing's end for a pairing the service's code made: a match keeps the
// session issued for the device's client origin to take, under the code's person, and passes its
// code on sealed; no match ends the code at the service.
func (service *Service) answerService(ctx context.Context, known pairing, issued Issued, isMatch bool) error {
	if !isMatch {
		return service.serviceDeviceCodes.RefuseDeviceCode(ctx, known.code)
	}
	handOffCode, err := service.keepHandOff(ctx, issued, proof.DeviceCodePerson(known.code))
	if err != nil {
		return err
	}
	service.pairings.mutex.Lock()
	delete(service.pairings.bySecret, known.secret)
	service.pairings.mutex.Unlock()
	return service.serviceDeviceCodes.ActivateDeviceCode(ctx, known.code, known.deviceKey, handOffCode)
}
