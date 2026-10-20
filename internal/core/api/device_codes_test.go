// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// scannedServiceCodes is the paid service with one television waiting with abcd-efgh, as a core claims
// a scanned code from it: with no nonce, its key's hash read from the television's QR code (D321).
type scannedServiceCodes struct {
	waiting   discoveryproto.DeviceCodeClaimResponse
	activated []string
	refused   []string
}

func (codes *scannedServiceCodes) ClaimDeviceCode(
	_ context.Context, code, _ string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	return codes.waiting, code == "abcd-efgh", nil
}

func (codes *scannedServiceCodes) ActivateDeviceCode(_ context.Context, code, _, _, _ string) error {
	codes.activated = append(codes.activated, code)
	return nil
}

func (codes *scannedServiceCodes) RefuseDeviceCode(_ context.Context, code string) error {
	codes.refused = append(codes.refused, code)
	return nil
}

// A code scanned from a television's QR code, with its key's hash, is shown with no check code and the
// television named by its platform, and activated by one answer; a hash naming another key is refused
// as another device, and the code ended (D321).
func TestActivation_AScannedCodeNeedsNoCheckCodeAndOnlyItsKeyIsActivated(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
	codes := &scannedServiceCodes{waiting: discoveryproto.DeviceCodeClaimResponse{
		PublicKey: publicKey, Platform: "Tizen", AskedSecondsAgo: 5,
	}}
	harness.authService.ClaimDeviceCodesFrom(codes)
	keyHash, err := proof.DeviceKeyHash(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	entered := harness.do(t, http.MethodPost, "/activations", admin,
		map[string]any{"code": "abcd-efgh", "keyHash": keyHash})
	var claim ActivationOutput
	if err = json.Unmarshal([]byte(entered.body), &claim.Body); err != nil || entered.status != http.StatusOK ||
		claim.Body.CheckCode != "" || claim.Body.DeviceName != "Tizen" {
		t.Fatalf("POST /activations with the key's hash = %d %s; want no check code, the TV named Tizen",
			entered.status, entered.body)
	}
	if answered := harness.do(t, http.MethodPost, "/activations/answer", admin,
		map[string]any{"code": "abcd-efgh", "isMatch": true}); answered.status != http.StatusNoContent ||
		len(codes.activated) != 1 {
		t.Errorf("activating the scanned code = %d %s, activated %v", answered.status, answered.body, codes.activated)
	}
	other := harness.do(t, http.MethodPost, "/activations", admin,
		map[string]any{"code": "abcd-efgh", "keyHash": "AAAAAAAAAAAAAAAAAAAAAA"})
	wantError(t, other, http.StatusConflict, CodeConflict)
	if len(codes.refused) != 1 {
		t.Errorf("refused %v; want the code whose key the hash does not name ended", codes.refused)
	}
}
