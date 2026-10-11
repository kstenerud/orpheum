// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// Credentials name their membership and expiry and carry a password derived from the secret, never
// the secret itself (D194).
func TestIssue_GivesADerivedPasswordNeverTheSecret(t *testing.T) {
	t.Parallel()
	const canary = "CANARY-turn-5b2d-0123456789WXYZ"
	now := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC)
	issued := Issue(secrets.NewValue(canary), Membership{SessionID: 4, MemberID: 9}, now)
	if strings.Contains(issued.Username+issued.Password, canary) {
		t.Fatalf("the credentials carry the secret: %+v", issued)
	}
	membership, expiresAt, err := parseUsername(issued.Username)
	if err != nil || membership != (Membership{SessionID: 4, MemberID: 9}) ||
		!expiresAt.Equal(now.Add(credentialLifetime)) || !issued.ExpiresAt.Equal(expiresAt) {
		t.Errorf("the username %q reads as %+v until %v (%v); want session 4's member 9 for a day",
			issued.Username, membership, expiresAt, err)
	}
	if other := Issue(secrets.NewValue("another"), membership, now); other.Password == issued.Password {
		t.Error("two secrets gave the same password")
	}
}
