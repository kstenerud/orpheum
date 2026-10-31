// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// claimedWithDevice is a core its discovery has claimed, under the realm karlnas, with a device
// waiting at the discovery with abcd-efgh, and that device's key.
func claimedWithDevice(t *testing.T) (*world, *ecdh.PrivateKey) {
	t.Helper()
	built := newWorld(t)
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	built.fake.deviceCodes["abcd-efgh"] = discoveryproto.DeviceCodeClaimResponse{
		PublicKey: base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), Platform: "Tizen",
	}
	if _, err = built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	return built, private
}

// coreNonce is the nonce the core claims with in these tests (D321).
const coreNonce = "EBESExQVFhcYGRobHB0eHw"

// A device code the discovery issued is claimed only once the discovery publishes the core, and one
// no device waits with is none (D293 §2).
func TestDeviceCode_IsClaimedOnlyOnceTheDiscoveryPublishesTheCore(t *testing.T) {
	t.Parallel()
	built, _ := claimedWithDevice(t)
	ctx := t.Context()
	if _, isWaiting, err := built.service.ClaimDeviceCode(ctx, "abcd-efgh", coreNonce); isWaiting || err != nil {
		t.Errorf("a pending core claimed a device code: %v, %v; want none", isWaiting, err)
	}
	built.fake.claim()
	built.service.step(ctx)
	if _, isWaiting, err := built.service.ClaimDeviceCode(ctx, "zzzz-zzzz", ""); isWaiting || err != nil {
		t.Errorf("a code no device waits with: %v, %v; want none waiting, no error", isWaiting, err)
	}
	claimed, isWaiting, err := built.service.ClaimDeviceCode(ctx, "abcd-efgh", coreNonce)
	if !isWaiting || err != nil || claimed != built.fake.deviceCodes["abcd-efgh"] {
		t.Errorf("claimed %+v, %v, %v; want the device's key and platform", claimed, isWaiting, err)
	}
	if sent := built.fake.coreNonces["abcd-efgh"]; sent != coreNonce {
		t.Errorf("the claim carried the core's nonce %q; want the one given (D321)", sent)
	}
	built.fake.claimsRefused = true
	_, _, err = built.service.ClaimDeviceCode(ctx, "abcd-efgh", coreNonce)
	if !errors.Is(err, discoveryproto.ErrTooManyClaims) {
		t.Errorf("a claim the discovery refused as too many: %v; want ErrTooManyClaims", err)
	}
}

// The outcome the core sends opens with the device's key alone, naming this core, its fingerprint,
// its realm and the hand-off's code; a refusal sends none (D293 §2).
func TestDeviceCode_ItsOutcomeOpensWithTheDevicesKeyAndARefusalSendsNone(t *testing.T) {
	t.Parallel()
	built, private := claimedWithDevice(t)
	ctx := t.Context()
	built.fake.claim()
	built.fake.realm = "karlnas"
	built.service.step(ctx)
	publicKey := built.fake.deviceCodes["abcd-efgh"].PublicKey
	if err := built.service.ActivateDeviceCode(ctx, "abcd-efgh", publicKey, "a-hand-off-code", "karl"); err != nil {
		t.Fatal(err)
	}
	key, err := hpke.NewDHKEMPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := proof.OpenDeviceOutcome(key, built.fake.deviceOutcomes["abcd-efgh"])
	want := proof.DeviceOutcome{
		CoreID: fakeCoreID, Fingerprint: built.service.Status().Fingerprint, Realm: "karlnas",
		HandOffCode: "a-hand-off-code", Person: "karl",
	}
	if err != nil || opened != want {
		t.Errorf("the device opened %+v, %v; want %+v", opened, err, want)
	}
	built.fake.deviceOutcomes["abcd-efgj"] = "unset"
	err = built.service.RefuseDeviceCode(ctx, "abcd-efgj")
	if err != nil || built.fake.deviceOutcomes["abcd-efgj"] != "" {
		t.Errorf("a refusal sent %q, %v; want no outcome", built.fake.deviceOutcomes["abcd-efgj"], err)
	}
}
