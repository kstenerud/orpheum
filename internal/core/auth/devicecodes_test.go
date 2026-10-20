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
)

// fakeServiceCodes is the paid service's device codes as the core sees them: the devices waiting, by
// code, and how the core ended each it claimed.
type fakeServiceCodes struct {
	waiting   map[string]discoveryproto.DeviceCodeClaimResponse
	activated map[string]string
	people    []string
	refused   []string
	// claimErr is what every claim fails with, when set.
	claimErr error
}

func (fake *fakeServiceCodes) ClaimDeviceCode(
	_ context.Context, code string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	if fake.claimErr != nil {
		return discoveryproto.DeviceCodeClaimResponse{}, false, fake.claimErr
	}
	waiting, isWaiting := fake.waiting[code]
	return waiting, isWaiting, nil
}

func (fake *fakeServiceCodes) ActivateDeviceCode(_ context.Context, code, publicKey, handOffCode, person string) error {
	if publicKey != fake.waiting[code].PublicKey {
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
	codes := &fakeServiceCodes{
		waiting: map[string]discoveryproto.DeviceCodeClaimResponse{
			"abcd-efgh": {PublicKey: newDevicePublicKey(t), Platform: "Tizen", AskedSecondsAgo: 40},
			"abcd-efgj": {PublicKey: newDevicePublicKey(t)},
		},
		activated: map[string]string{},
	}
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
	want, checkErr := proof.DeviceCheckCode(codes.waiting["abcd-efgh"].PublicKey)
	if err != nil || checkErr != nil || claim.CheckCode != want {
		t.Fatalf("entering the service's code: %+v, %v; want the check code %q of the device's key", claim, err, want)
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
	codes := &fakeServiceCodes{
		waiting:   map[string]discoveryproto.DeviceCodeClaimResponse{"abcd-efgh": {PublicKey: newDevicePublicKey(t)}},
		activated: map[string]string{},
	}
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
