// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// A device code the paid service issued, which a person entered on this core (D293 §2; the
// protocol's §3.1): the core claims it from the service, learning the device's key, and once the
// person says the check codes match, seals what the device needs to that key.

// ClaimDeviceCode is the device waiting at the discovery with code: its public key and platform, and
// whether one waits. None waits while the discovery does not publish the core, or when it has no
// device with that code, or another core claimed it; an error is a discovery not reached.
func (service *Service) ClaimDeviceCode(
	ctx context.Context, code string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	status := service.Status()
	if !status.IsPublished() {
		return discoveryproto.DeviceCodeClaimResponse{}, false, nil
	}
	var claimed discoveryproto.DeviceCodeClaimResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathDeviceCodeClaim,
		body: discoveryproto.DeviceCodeClaimRequest{Code: code}, into: &claimed,
	})
	var refused *refusal
	switch {
	case errors.As(err, &refused) && refused.code == discoveryproto.RefusalNotFound:
		return discoveryproto.DeviceCodeClaimResponse{}, false, nil
	case errors.As(err, &refused) && refused.code == discoveryproto.RefusalTooManyClaims:
		return discoveryproto.DeviceCodeClaimResponse{}, false, discoveryproto.ErrTooManyClaims
	case err != nil:
		return discoveryproto.DeviceCodeClaimResponse{}, false, fmt.Errorf("claim a device code: %w", err)
	}
	return claimed, true, nil
}

// ActivateDeviceCode tells the discovery the device asking with code and publicKey is activated: it
// seals the core's id, its identity key's fingerprint, its realm, handOffCode, the session's the core
// keeps for the device's client origin, and the name of the person it signs in as to the device's key
// (proof.SealDeviceOutcome), which the discovery passes on unread.
func (service *Service) ActivateDeviceCode(ctx context.Context, code, publicKey, handOffCode, person string) error {
	status := service.Status()
	sealed, err := proof.SealDeviceOutcome(publicKey, proof.DeviceOutcome{
		CoreID: status.CoreID, Fingerprint: status.Fingerprint, Realm: status.Realm, HandOffCode: handOffCode,
		Person: person,
	})
	if err != nil {
		return fmt.Errorf("seal a device's outcome: %w", err)
	}
	return service.endDeviceCode(ctx, status.BaseURL, code, sealed)
}

// RefuseDeviceCode tells the discovery the person said the check codes do not match, which ends the
// code.
func (service *Service) RefuseDeviceCode(ctx context.Context, code string) error {
	return service.endDeviceCode(ctx, service.Status().BaseURL, code, "")
}

// endDeviceCode posts how the code ended: sealed, or refused when it is empty.
func (service *Service) endDeviceCode(ctx context.Context, baseURL, code, sealed string) error {
	if baseURL == "" {
		return ErrNotEnrolled
	}
	if err := service.signedCall(ctx, callOf{
		base: baseURL, method: http.MethodPost, path: discoveryproto.PathDeviceCodeOutcome,
		body: discoveryproto.DeviceCodeOutcomeRequest{Code: code, Sealed: sealed},
	}); err != nil {
		return fmt.Errorf("tell the discovery how a device code ended: %w", err)
	}
	return nil
}
