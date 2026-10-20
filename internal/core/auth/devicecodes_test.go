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

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// fakeServiceCodes is the paid service's device codes as the core sees them: the devices waiting, by
// code, and how the core ended each it claimed.
type fakeServiceCodes struct {
	waiting   map[string]discoveryproto.DeviceCodeClaimResponse
	activated map[string]string
	refused   []string
}

func (fake *fakeServiceCodes) ClaimDeviceCode(
	_ context.Context, code string,
) (discoveryproto.DeviceCodeClaimResponse, bool, error) {
	waiting, isWaiting := fake.waiting[code]
	return waiting, isWaiting, nil
}

func (fake *fakeServiceCodes) ActivateDeviceCode(_ context.Context, code, publicKey, handOffCode string) error {
	if publicKey != fake.waiting[code].PublicKey {
		return errors.New("sealed to another key than the device's")
	}
	fake.activated[code] = handOffCode
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

// serviceCodesFixture is the auth service of a core whose paid service has a television waiting with abcd-efgh and a
// phone with abcd-efgj, and an admin to enter them.
func serviceCodesFixture(t *testing.T) (*Service, *fakeServiceCodes, int64) {
	t.Helper()
	built := newFixture(t)
	id := built.createUser(t, "partner", "pw", RoleAdmin)
	codes := &fakeServiceCodes{
		waiting: map[string]discoveryproto.DeviceCodeClaimResponse{
			"abcd-efgh": {PublicKey: newDevicePublicKey(t), Platform: "Tizen"},
			"abcd-efgj": {PublicKey: newDevicePublicKey(t)},
		},
		activated: map[string]string{},
	}
	built.service.ClaimDeviceCodesFrom(codes)
	return built.service, codes, id
}

// A code the paid service issued, entered on the core, shows the check code made of the device's
// key, the same entered again; a match keeps the session for the device's client origin under the
// code's person, with the device scope, and passes its code on (D293 §2, D292 §4).
func TestPairing_AServiceCodeChecksByTheDevicesKeyAndHandsTheSessionOff(t *testing.T) {
	t.Parallel()
	service, codes, id := serviceCodesFixture(t)
	ctx := t.Context()
	claim, err := service.ClaimPairing(ctx, id, id, "ABCD EFGH", "Living room TV")
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
	principal, err := service.Authenticate(ctx, issued.Token)
	if err != nil || principal.UserID != id || principal.IsAdmin() || !principal.Reach.IsEveryLibrary() {
		t.Errorf("the device is %+v, %v; want the person, without administering (the device scope) but "+
			"reaching every library their role does", principal, err)
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
