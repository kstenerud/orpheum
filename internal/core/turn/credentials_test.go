// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// testLifetime is how long a member's relay credentials last where no setting says: the setting's start.
var testLifetime = settings.StartingDuration(settings.VoiceRelayCredentialHours)

// lifetimeSettings answers voice_relay_credential_hours with its hours.
type lifetimeSettings struct{ hours int }

func (lifetime lifetimeSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	if key != settings.VoiceRelayCredentialHours {
		return 0, errors.New("not asked for")
	}
	return time.Duration(lifetime.hours) * time.Hour, nil
}

// How long a member's relay credentials last is a setting (the constants sweep, 2026-10-20).
func TestICEServersFor_LastAsLongAsTheSettingSays(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)
	issuer := &Issuer{Address: "203.0.113.4:3478", Secrets: newMemorySecrets(), Settings: lifetimeSettings{hours: 2}}
	if err := EnsureSecret(t.Context(), issuer.Secrets, rand.Reader); err != nil {
		t.Fatal(err)
	}
	servers, err := issuer.ICEServersFor(t.Context(), Membership{SessionID: 4, MemberID: 9}, now)
	if err != nil || len(servers) != 1 || !servers[0].Credentials.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Errorf("the relay's servers = %+v (%v); want credentials good for the setting's two hours", servers, err)
	}
}

// Credentials name their membership and expiry and carry a password derived from the secret, never
// the secret itself (D194).
func TestIssue_GivesADerivedPasswordNeverTheSecret(t *testing.T) {
	t.Parallel()
	const canary = "CANARY-turn-5b2d-0123456789WXYZ"
	now := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC)
	issued := Issue(secrets.NewValue(canary), Membership{SessionID: 4, MemberID: 9}, now, testLifetime)
	if strings.Contains(issued.Username+issued.Password, canary) {
		t.Fatalf("the credentials carry the secret: %+v", issued)
	}
	held, expiresAt, err := parseUsername(issued.Username)
	membership := held.membership
	if err != nil || held.isFound || membership != (Membership{SessionID: 4, MemberID: 9}) ||
		!expiresAt.Equal(now.Add(testLifetime)) || !issued.ExpiresAt.Equal(expiresAt) {
		t.Errorf("the username %q reads as %+v until %v (%v); want session 4's member 9 for a day",
			issued.Username, membership, expiresAt, err)
	}
	if other := Issue(secrets.NewValue("another"), membership, now, testLifetime); other.Password == issued.Password {
		t.Error("two secrets gave the same password")
	}
}
