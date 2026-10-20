// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// fakeServiceCodes is the paid service's device codes as the core sees them: the devices waiting, by
// code, the nonces the core claimed them with, and how the core ended each it claimed.
type fakeServiceCodes struct {
	waiting   map[string]fakeDevice
	claims    map[string][]string
	activated map[string]string
	people    []string
	refused   []string
	// claimErr is what every claim fails with, when set.
	claimErr error
}

// fakeDevice is a device waiting with a code: what the service holds of it, and the nonce it
// committed to, which it reveals once the core's nonce is given, as a device does on its next poll;
// a silent one never does, and a liar reveals one that breaks its commitment (D321).
type fakeDevice struct {
	claim          discoveryproto.DeviceCodeClaimResponse
	nonce          string
	silent, isLiar bool
}

func (fake *fakeServiceCodes) ClaimDeviceCode(
	_ context.Context, code, coreNonce string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	if fake.claimErr != nil {
		return discoveryproto.DeviceCodeClaimResponse{}, false, fake.claimErr
	}
	device, isWaiting := fake.waiting[code]
	if !isWaiting {
		return discoveryproto.DeviceCodeClaimResponse{}, false, nil
	}
	fake.claims[code] = append(fake.claims[code], coreNonce)
	answer := device.claim
	// The first claim's nonce is the one kept; the device reveals after it, so from the next claim on.
	if first := fake.claims[code][0]; first != "" && len(fake.claims[code]) > 1 && !device.silent {
		answer.DeviceNonce = device.nonce
		if device.isLiar {
			answer.DeviceNonce = "ICEiIyQlJicoKSorLC0uLw"
		}
	}
	return answer, true, nil
}

func (fake *fakeServiceCodes) ActivateDeviceCode(_ context.Context, code, publicKey, handOffCode, person string) error {
	if publicKey != fake.waiting[code].claim.PublicKey {
		return errors.New("sealed to another key than the device's")
	}
	fake.activated[code] = handOffCode
	fake.people = append(fake.people, person)
	return nil
}

func (fake *fakeServiceCodes) RefuseDeviceCode(_ context.Context, code string) error {
	fake.refused = append(fake.refused, code)
	return nil
}

// newFakeServiceCodes is a service with devices waiting by code.
func newFakeServiceCodes(waiting map[string]fakeDevice) *fakeServiceCodes {
	return &fakeServiceCodes{waiting: waiting, claims: map[string][]string{}, activated: map[string]string{}}
}

// newWaitingDevice is a device of platform that asked askedSecondsAgo, with a fresh key and nonce and
// its commitment to them.
func newWaitingDevice(t *testing.T, platform string, askedSecondsAgo int) fakeDevice {
	t.Helper()
	publicKey := newDevicePublicKey(t)
	nonce := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))
	commitment, err := proof.DeviceCommitment(publicKey, nonce)
	if err != nil {
		t.Fatal(err)
	}
	return fakeDevice{claim: discoveryproto.DeviceCodeClaimResponse{
		PublicKey: publicKey, Platform: platform, AskedSecondsAgo: askedSecondsAgo, Commitment: commitment,
	}, nonce: nonce}
}

// newDevicePublicKey is a device's P-256 public key as it sends it.
func newDevicePublicKey(t *testing.T) string {
	t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
}

// serviceCodes is the auth service of a core whose paid service has a television waiting with
// abcd-efgh and a phone with abcd-efgj, an admin to enter them, and the activations the core told of.
type serviceCodes struct {
	service *Service
	codes   *fakeServiceCodes
	id      int64
	told    *[]Activation
}

// serviceCodesFixture is the auth service of serviceCodesWorld, its service's codes and its admin.
func serviceCodesFixture(t *testing.T) (*Service, *fakeServiceCodes, int64) {
	t.Helper()
	world := serviceCodesWorld(t)
	return world.service, world.codes, world.id
}

// serviceCodesWorld is a serviceCodes.
func serviceCodesWorld(t *testing.T) serviceCodes {
	t.Helper()
	built := newFixture(t)
	told := &[]Activation{}
	built.service.OnDeviceActivated(func(_ context.Context, activated Activation) {
		*told = append(*told, activated)
	})
	id := built.createUser(t, "partner", "pw", RoleAdmin)
	codes := newFakeServiceCodes(map[string]fakeDevice{
		"abcd-efgh": newWaitingDevice(t, "Tizen", 40),
		"abcd-efgj": newWaitingDevice(t, "", 0),
	})
	built.service.ClaimDeviceCodesFrom(codes)
	return serviceCodes{service: built.service, codes: codes, id: id, told: told}
}

// wantDeviceScoped fails t unless the session issued is the person's, neither administering nor
// editing metadata (the device scope), but reaching every library their role does.
func wantDeviceScoped(t *testing.T, service *Service, issued Issued, id int64) {
	t.Helper()
	principal, err := service.Authenticate(t.Context(), issued.Token)
	if err != nil || principal.UserID != id || principal.IsAdmin() || principal.CanEditMetadata() ||
		!principal.Reach.IsEveryLibrary() {
		t.Errorf("the device is %+v, %v; want the person, neither administering nor editing metadata (the "+
			"device scope) but reaching every library their role does", principal, err)
	}
}

// A code the paid service issued, entered on the core, shows the check code made of the device's
// key, the same entered again; a match keeps the session for the device's client origin under the
// code's person, with the device scope, and passes its code on (D293 §2, D292 §4).
func TestPairing_AServiceCodeChecksByTheDevicesKeyAndHandsTheSessionOff(t *testing.T) {
	t.Parallel()
	world := serviceCodesWorld(t)
	service, codes, id := world.service, world.codes, world.id
	ctx := t.Context()
	claim, err := service.ClaimPairing(ctx, id, id, "ABCD EFGH", "Living room TV")
	// The person is shown what asked, and how long ago (the device code audit's M3).
	if claim.Platform != "Tizen" || service.clock.Now().Sub(claim.AskedAt) != 40*time.Second {
		t.Errorf("the claim shows %q asked at %v; want Tizen, 40 s ago", claim.Platform, claim.AskedAt)
	}
	device := codes.waiting["abcd-efgh"]
	want, checkErr := proof.DeviceCheckCode(device.claim.PublicKey, device.nonce, codes.claims["abcd-efgh"][0])
	if err != nil || checkErr != nil || claim.CheckCode != want {
		t.Fatalf("entering the service's code: %+v, %v; want the check code %q of the device's key and both "+
			"nonces (D321)", claim, err, want)
	}
	again, err := service.ClaimPairing(ctx, id, id, "abcd-efgh", "Living room TV")
	if err != nil || again != claim {
		t.Errorf("entering it again: %+v, %v; want the same claim", again, err)
	}
	if err = service.AnswerPairing(ctx, id, "abcd-efgh", true); err != nil {
		t.Fatal(err)
	}
	handOffCode := codes.activated["abcd-efgh"]
	keys := service.HandOffOfferKeys(proof.DeviceCodePerson("abcd-efgh"))
	if handOffCode == "" || !slices.ContainsFunc(keys, func(key []byte) bool {
		return string(key) == string(proof.HandOffOfferKey(handOffCode))
	}) {
		t.Fatalf("activated with %q, the code's person proving by %d keys; want its hand-off's", handOffCode, len(keys))
	}
	issued, err := service.TakeHandOff(ctx, handOffCode)
	if err != nil {
		t.Fatal(err)
	}
	wantDeviceScoped(t, service, issued, id)
	// The outcome names the person, and the core's log is told who activated what (M1, M4).
	wantTold := []Activation{{
		ByUserID: id, ForUserID: id, DeviceName: "Living room TV", Platform: "Tizen",
		IsThroughService: true,
	}}
	if !slices.Equal(codes.people, []string{"partner"}) || !slices.Equal(*world.told, wantTold) {
		t.Errorf("sealed for %v, told %+v; want partner, and %+v", codes.people, *world.told, wantTold)
	}
}

// No match ends a service's code there, and a code neither the core nor the service issued is one no
// device waits with (D293 §2).
func TestPairing_AServiceCodeNotMatchedEndsThereAndAnUnknownOneIsNone(t *testing.T) {
	t.Parallel()
	service, codes, id := serviceCodesFixture(t)
	ctx := t.Context()
	if _, err := service.ClaimPairing(ctx, id, id, "abcd-efgj", "Phone"); err != nil {
		t.Fatal(err)
	}
	err := service.AnswerPairing(ctx, id, "abcd-efgj", false)
	if err != nil || !slices.Equal(codes.refused, []string{"abcd-efgj"}) || len(codes.activated) != 0 {
		t.Errorf("no match: %v, refused %v, activated %v; want the code ended at the service", err, codes.refused,
			codes.activated)
	}
	if _, err = service.ClaimPairing(ctx, id, id, "zzzz-zzzz", "Other"); !errors.Is(err, ErrNoPairing) {
		t.Errorf("a code neither issued: %v; want ErrNoPairing", err)
	}
}

// A device code's hand-off works for three minutes, as an old TV may take a while to reach the core,
// and a claim the service refuses as one too many is the person's too many codes (the device code
// audit's L2, H2).
func TestPairing_AServiceCodesHandOffWaitsThreeMinutesAndTooManyClaimsSlowThePerson(t *testing.T) {
	t.Parallel()
	built := newFixture(t)
	ctx := t.Context()
	id := built.createUser(t, "partner", "pw", RoleUser)
	codes := newFakeServiceCodes(map[string]fakeDevice{"abcd-efgh": newWaitingDevice(t, "", 0)})
	built.service.ClaimDeviceCodesFrom(codes)
	if _, err := built.service.ClaimPairing(ctx, id, id, "abcd-efgh", "TV"); err != nil {
		t.Fatal(err)
	}
	if err := built.service.AnswerPairing(ctx, id, "abcd-efgh", true); err != nil {
		t.Fatal(err)
	}
	built.clock.Advance(handOffLife + time.Minute)
	if _, err := built.service.TakeHandOff(ctx, codes.activated["abcd-efgh"]); err != nil {
		t.Errorf("two minutes on, the TV's hand-off: %v; want it still waiting", err)
	}
	codes.claimErr = discoveryproto.ErrTooManyClaims
	if _, err := built.service.ClaimPairing(ctx, id, id, "abcd-efgj", "TV"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("a claim the service refused as too many: %v; want ErrRateLimited", err)
	}
}

// How long a typed code waits for its device's nonce is a setting (the constants sweep, 2026-10-20).
func TestPairing_ATypedServiceCodeWaitsAsLongAsTheSettingSays(t *testing.T) {
	t.Parallel()
	built := newFixture(t)
	ctx := t.Context()
	id := built.createUser(t, "partner", "pw", RoleUser)
	silent := newWaitingDevice(t, "", 0)
	silent.silent = true
	built.service.ClaimDeviceCodesFrom(newFakeServiceCodes(map[string]fakeDevice{"abcd-efgh": silent}))
	settingstest.Set(t, built.settings, settings.DeviceCodeRevealWaitSeconds, "4")
	startedAt := built.clock.Now()
	if _, err := built.service.ClaimPairing(ctx, id, id, "abcd-efgh", "TV"); !errors.Is(err, ErrDeviceSilent) {
		t.Errorf("a device that never reveals, the wait at 4 s: %v; want ErrDeviceSilent", err)
	}
	if waited := built.clock.Now().Sub(startedAt); waited < 4*time.Second || waited >= revealWait {
		t.Errorf("with the wait at 4 s, gave up after %v", waited)
	}
}

// A typed code's check code waits for the device to reveal the nonce it committed to: a device that
// does not within revealWait, or reveals one that breaks its commitment, is refused and its code
// ended, the core's own nonce chosen afresh and claimed with (D321; the device code audit's M2).
func TestPairing_ATypedServiceCodeWaitsForTheDevicesNonceAndRefusesOneThatBreaksItsCommitment(t *testing.T) {
	t.Parallel()
	built := newFixture(t)
	ctx := t.Context()
	id := built.createUser(t, "partner", "pw", RoleUser)
	silent, liar := newWaitingDevice(t, "", 0), newWaitingDevice(t, "", 0)
	silent.silent, liar.isLiar = true, true
	codes := newFakeServiceCodes(map[string]fakeDevice{"abcd-efgh": silent, "abcd-efgj": liar})
	built.service.ClaimDeviceCodesFrom(codes)
	startedAt := built.clock.Now()
	if _, err := built.service.ClaimPairing(ctx, id, id, "abcd-efgh", "TV"); !errors.Is(err, ErrDeviceSilent) {
		t.Errorf("a device that never reveals: %v; want ErrDeviceSilent", err)
	}
	if waited := built.clock.Now().Sub(startedAt); waited < revealWait {
		t.Errorf("gave up after %v; want %v", waited, revealWait)
	}

	if _, err := built.service.ClaimPairing(ctx, id, id, "abcd-efgj", "TV"); !errors.Is(err, ErrNotThatDevice) {
		t.Errorf("a device revealing a nonce that breaks its commitment: %v; want ErrNotThatDevice", err)
	}
	if !slices.Equal(codes.refused, []string{"abcd-efgh", "abcd-efgj"}) || len(codes.activated) != 0 {
		t.Errorf("refused %v, activated %v; want both codes ended, neither activated", codes.refused, codes.activated)
	}
	if nonce := codes.claims["abcd-efgj"][0]; len(nonce) != 22 || nonce == codes.claims["abcd-efgh"][0] {
		t.Errorf("the core claimed with nonces %q and %q; want a fresh 128-bit one each", nonce,
			codes.claims["abcd-efgh"][0])
	}
}

// A code scanned from the device's QR code is claimed with no nonce and needs no check code: the core
// activates only the key the QR code's hash names, listing the device by its platform when no name is
// given, and ends a code whose key is another (D321).
func TestPairing_AScannedCodeActivatesOnlyTheKeyItsHashNamesWithNoCheckCode(t *testing.T) {
	t.Parallel()
	world := serviceCodesWorld(t)
	service, codes, id := world.service, world.codes, world.id
	ctx := t.Context()
	hash, err := proof.DeviceKeyHash(codes.waiting["abcd-efgh"].claim.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := service.ClaimScannedPairing(ctx, id, id, "abcd-efgh", hash, "")
	if err != nil || claim.CheckCode != "" || claim.DeviceName != "Tizen" {
		t.Fatalf("a scanned code: %+v, %v; want no check code, the device named by its platform", claim, err)
	}
	if nonces := codes.claims["abcd-efgh"]; !slices.Equal(nonces, []string{""}) {
		t.Errorf("claimed with nonces %q; want one claim, with none", nonces)
	}
	if err = service.AnswerPairing(ctx, id, "abcd-efgh", true); err != nil || codes.activated["abcd-efgh"] == "" {
		t.Errorf("activating the scanned code: %v, activated %v", err, codes.activated)
	}
	if _, err = service.ClaimScannedPairing(ctx, id, id, "abcd-efgj", hash, ""); !errors.Is(err, ErrNotThatDevice) {
		t.Errorf("a code whose key the hash does not name: %v; want ErrNotThatDevice", err)
	}
	if !slices.Equal(codes.refused, []string{"abcd-efgj"}) {
		t.Errorf("refused %v; want the code with another key ended", codes.refused)
	}
}
