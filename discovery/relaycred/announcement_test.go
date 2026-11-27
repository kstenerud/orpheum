// SPDX-License-Identifier: AGPL-3.0-or-later

package relaycred

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadAnnouncement_ReadsASignedOneWithinTwoMinutesAndRefusesAnythingElse(t *testing.T) {
	t.Parallel()
	secret := testSecret(t, testKey)
	signedAt := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	body, signature, err := SignAnnouncement(secret, "relay.example", signedAt)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"name":"relay.example","at":` + "1795780800000" + `}`; string(body) != want {
		t.Errorf("the body is %s; want %s", body, want)
	}
	mac := hmac.New(sha256.New, []byte(testKey))
	_, _ = mac.Write(body)
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); signature != want {
		t.Errorf("the signature is %q; want HMAC-SHA256's standard base64 %q", signature, want)
	}
	for _, now := range []time.Time{signedAt, signedAt.Add(AnnouncementSkew), signedAt.Add(-AnnouncementSkew)} {
		read, readErr := ReadAnnouncement(secret, body, signature, now)
		if readErr != nil || read != (Announcement{Name: "relay.example", At: signedAt.UnixMilli()}) {
			t.Errorf("read at %s: %+v, %v; want it as signed", now, read, readErr)
		}
	}
	other := testSecret(t, strings.Repeat("z", 32))
	_, otherSignature, err := SignAnnouncement(other, "relay.example", signedAt)
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte(strings.Replace(string(body), "relay.example", "relay.elsewhere", 1))
	refused := []struct {
		name      string
		body      []byte
		signature string
		now       time.Time
	}{
		{"too old", body, signature, signedAt.Add(AnnouncementSkew + time.Millisecond)},
		{"too new", body, signature, signedAt.Add(-AnnouncementSkew - time.Millisecond)},
		{"another secret", body, otherSignature, signedAt},
		{"a changed body", changed, signature, signedAt},
		{"no signature", body, "", signedAt},
	}
	for _, each := range refused {
		_, readErr := ReadAnnouncement(secret, each.body, each.signature, each.now)
		if !errors.Is(readErr, ErrNotAnAnnouncement) {
			t.Errorf("%s: %v; want ErrNotAnAnnouncement", each.name, err)
		}
	}
	if _, _, err := SignAnnouncement(secret, "", signedAt); err == nil {
		t.Error("an announcement with no name was signed")
	}
}

func TestSelfTestHousehold_IsAHouseholdNoDiscoveryNames(t *testing.T) {
	t.Parallel()
	if !household.MatchString(SelfTestHousehold) {
		t.Errorf("%q is not a household's name, so the relay's own credential cannot be made", SelfTestHousehold)
	}
	// A discovery names a household with 22 characters of base64url (Connect's hostedrelay).
	if len(SelfTestHousehold) == 22 {
		t.Errorf("%q is as long as a discovery's household names, so it could be one", SelfTestHousehold)
	}
}
