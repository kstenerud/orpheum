// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/api"
)

// scannedServiceCodes is the paid service with one television waiting with abcd-efgh, as a core claims
// a scanned code from it: with no nonce, its key's hash read from the television's QR code (D321).
type scannedServiceCodes struct {
	waiting   discoveryproto.DeviceCodeClaimResponse
	activated []string
	// handOffs are the hand-offs' codes each activation sealed to the television, which it takes its
	// session by (POST /session/redeem).
	handOffs []string
	refused  []string
}

func (codes *scannedServiceCodes) ClaimDeviceCode(
	_ context.Context, code, _ string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	return codes.waiting, code == "abcd-efgh", nil
}

func (codes *scannedServiceCodes) ActivateDeviceCode(_ context.Context, code, _, handOffCode, _ string) error {
	codes.activated = append(codes.activated, code)
	codes.handOffs = append(codes.handOffs, handOffCode)
	return nil
}

// newScannedServiceCodes is the paid service with a television of platform waiting with abcd-efgh,
// served to harness's core, and the hash of its key its QR code shows.
func newScannedServiceCodes(t *testing.T, harness testHarness, platform string) (*scannedServiceCodes, string) {
	t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
	codes := &scannedServiceCodes{waiting: discoveryproto.DeviceCodeClaimResponse{
		PublicKey: publicKey, Platform: platform, AskedSecondsAgo: 5,
	}}
	harness.authService.ClaimDeviceCodesFrom(codes)
	keyHash, err := proof.DeviceKeyHash(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return codes, keyHash
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
	codes, keyHash := newScannedServiceCodes(t, harness, "Tizen")
	entered := harness.do(t, http.MethodPost, "/pairings/code", admin,
		map[string]any{"code": "abcd-efgh", "keyHash": keyHash})
	var claim api.ActivationOutput
	if err := json.Unmarshal([]byte(entered.body), &claim.Body); err != nil || entered.status != http.StatusOK ||
		claim.Body.CheckCode != "" || claim.Body.DeviceName != "Tizen" {
		t.Fatalf("POST /pairings/code with the key's hash = %d %s; want no check code, the TV named Tizen",
			entered.status, entered.body)
	}
	if answered := harness.do(t, http.MethodPost, "/pairings/check", admin,
		map[string]any{"code": "abcd-efgh", "isMatch": true}); answered.status != http.StatusNoContent ||
		len(codes.activated) != 1 {
		t.Errorf("activating the scanned code = %d %s, activated %v", answered.status, answered.body, codes.activated)
	}
	other := harness.do(t, http.MethodPost, "/pairings/code", admin,
		map[string]any{"code": "abcd-efgh", "keyHash": "AAAAAAAAAAAAAAAAAAAAAA"})
	wantError(t, other, http.StatusConflict, api.CodeConflict)
	if len(codes.refused) != 1 {
		t.Errorf("refused %v; want the code whose key the hash does not name ended", codes.refused)
	}
}

// A web build Connect still serves enters a code at /activations and answers at /activations/answer,
// which pair the device for one release beside /pairings/code and /pairings/check, outside the API
// document (D425 §3).
func TestActivation_TheOldPathsPairForOneReleaseOutsideTheDocument(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	asked := harness.do(t, http.MethodPost, "/pairings", "", map[string]any{"platform": "web"})
	var pairing api.PairingOutput
	if err := json.Unmarshal([]byte(asked.body), &pairing.Body); err != nil || pairing.Body.Code == "" {
		t.Fatalf("POST /pairings = %d %s", asked.status, asked.body)
	}
	entered := harness.do(t, http.MethodPost, "/activations", admin,
		map[string]any{"code": pairing.Body.Code, "deviceName": "TV"})
	if entered.status != http.StatusOK || !strings.Contains(entered.body, `"checkCode":"`) {
		t.Fatalf("POST /activations = %d %s; want the check code, as /pairings/code answers", entered.status,
			entered.body)
	}
	if answered := harness.do(t, http.MethodPost, "/activations/answer", admin,
		map[string]any{"code": pairing.Body.Code, "isMatch": true}); answered.status != http.StatusNoContent {
		t.Fatalf("POST /activations/answer = %d %s", answered.status, answered.body)
	}
	secret := map[string]string{"secret": pairing.Body.Secret}
	if state := harness.do(t, http.MethodPost, "/pairings/state", "", secret); !strings.Contains(state.body,
		`"state":"activated"`) {
		t.Errorf("the device's pairing answered at the old path = %s; want it activated", state.body)
	}
	for _, operation := range api.OperationsIn(harness.server.OpenAPI()) {
		if strings.HasPrefix(operation.Path(), "/activations") {
			t.Errorf("the API document names %s %s; want the old path outside it", operation.Method(),
				operation.Path())
		}
	}
}
