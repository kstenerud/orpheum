// SPDX-License-Identifier: AGPL-3.0-or-later

package relaycred

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef"

func testSecret(t *testing.T, key string) Secret {
	t.Helper()
	secret, err := NewSecret([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestParse_ReadsBackWhatMakeWroteAndRefusesAnythingElse(t *testing.T) {
	t.Parallel()
	made := Credential{
		ExpiresAt: time.Unix(1790000000, 0).UTC(), Role: RoleClient, Household: "household-01", Kbps: 3000,
	}
	username, _, err := Make(testSecret(t, testKey), made)
	if err != nil {
		t.Fatal(err)
	}
	if username != "1790000000:client:household-01:3000" {
		t.Errorf("username %q", username)
	}
	read, err := Parse(username)
	if err != nil || read != made {
		t.Errorf("read back %+v, %v; want %+v", read, err, made)
	}
	for _, refused := range []string{
		"", "1790000000:client:household-01", "1790000000:client:household-01:3000:x",
		"1790000000:guest:household-01:3000", "1790000000:client:short:3000",
		"1790000000:client:" + strings.Repeat("a", 33) + ":3000", "1790000000:client:house.hold1:3000",
		"1790000000:client:household-01:0", "1790000000:client:household-01:1000001",
		"1790000000:client:household-01:+3000", "1790000000:client:household-01:03000",
		"01790000000:client:household-01:3000", "-5:client:household-01:3000", "x:client:household-01:3000",
	} {
		if _, err := Parse(refused); err == nil {
			t.Errorf("Parse(%q) read a credential; want it refused", refused)
		}
	}
	for _, unwritable := range []Credential{
		{ExpiresAt: made.ExpiresAt, Role: "guest", Household: made.Household, Kbps: 1},
		{ExpiresAt: made.ExpiresAt, Role: RoleCore, Household: "a:b:c:d:e:f", Kbps: 1},
		{ExpiresAt: made.ExpiresAt, Role: RoleCore, Household: made.Household, Kbps: MaxKbps + 1},
		{ExpiresAt: time.Unix(0, 0), Role: RoleCore, Household: made.Household, Kbps: 1},
	} {
		if username, _, err := Make(testSecret(t, testKey), unwritable); err == nil {
			t.Errorf("made %q of %+v; want it refused", username, unwritable)
		}
	}
}

func TestPassword_IsTheTURNRESTFormsVector(t *testing.T) {
	t.Parallel()
	// The vector was computed apart from this package, by Python's hmac and base64 modules.
	got := Password(testSecret(t, testKey), "1790000000:client:household-01:3000")
	if want := "h1JHT3ZyfPwzc4Nc82Yt6W6EEOw="; got != want {
		t.Errorf("password %q, want %q", got, want)
	}
}

func TestMatches_RefusesAChangedUsernameOrAnotherSecret(t *testing.T) {
	t.Parallel()
	secret := testSecret(t, testKey)
	username, password, err := Make(secret, Credential{
		ExpiresAt: time.Unix(1790000000, 0), Role: RoleClient, Household: "household-01", Kbps: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !Matches(secret, username, password) {
		t.Fatal("a credential does not match itself")
	}
	for _, changed := range []string{
		"1790000001:client:household-01:3000", "1790000000:core:household-01:3000",
		"1790000000:client:household-02:3000", "1790000000:client:household-01:30000",
	} {
		if Matches(secret, changed, password) {
			t.Errorf("the password matches the changed username %q", changed)
		}
	}
	if Matches(testSecret(t, strings.Repeat("z", 32)), username, password) {
		t.Error("the password matches under another secret")
	}
}

func TestSecret_IsLongEnoughAndNeverPrints(t *testing.T) {
	t.Parallel()
	if _, err := NewSecret([]byte(strings.Repeat("k", MinSecretBytes-1))); err == nil {
		t.Error("a secret of 31 bytes was taken")
	}
	secret := testSecret(t, testKey)
	encoded, err := json.Marshal(struct {
		S Secret `json:"s"`
	}{secret})
	if err != nil {
		t.Fatal(err)
	}
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("x", slog.Any("secret", secret))
	for _, shown := range []string{
		fmt.Sprintf("%v %+v %#v %s %q %x", secret, secret, secret, secret, secret, secret),
		string(encoded), logged.String(),
	} {
		if strings.Contains(shown, "0123456789") || strings.Contains(shown, "3031323334") {
			t.Errorf("the secret shows in %q", shown)
		}
	}
}
