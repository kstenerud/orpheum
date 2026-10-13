// SPDX-License-Identifier: AGPL-3.0-or-later

package auth_test

import (
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/auth/authtest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// These tests are outside package auth so they can use authtest.SetupCodes, which imports auth.

// laptop is the device the setup tests sign in on.
var laptop = auth.NewDevice{Name: "Firefox on Linux", Platform: "web"}

// newSetupService is an auth service over a migrated database with every setting seeded.
func newSetupService(t *testing.T) *auth.Service {
	t.Helper()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	return auth.NewService(database, fakeClock, rand.Reader, settingstest.Seeded(t, database))
}

func TestSetup_ACodeAnEarlierReleaseStoredInAnotherAlphabetIsReplaced(t *testing.T) {
	t.Parallel()
	service := newSetupService(t)
	codes := &authtest.SetupCodes{Code: "7KQM-2XHP-9RTW"}
	code, err := service.EnsureSetupCode(t.Context(), codes)
	if err != nil || code == "7KQM-2XHP-9RTW" || codes.Code != code {
		t.Errorf("EnsureSetupCode = %q, %v, stored %q; want a new safe32 code, stored", code, err, codes.Code)
	}
}

func TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce(t *testing.T) {
	t.Parallel()
	service := newSetupService(t)
	ctx := t.Context()
	codes := &authtest.SetupCodes{}
	code, err := service.EnsureSetupCode(ctx, codes)
	if err != nil {
		t.Fatal(err)
	}
	safe32Group := `[0-9a-hjkmnp-tv-z]{4}`
	if !regexp.MustCompile(`^` + safe32Group + `-` + safe32Group + `-` + safe32Group + `$`).MatchString(code) {
		t.Errorf("setup code %q is not three groups of four safe32 characters (D223 §2)", code)
	}
	again, err := service.EnsureSetupCode(ctx, codes)
	if err != nil || again != code {
		t.Errorf("a restart before setup: code %q, %v; want the same %q", again, err, code)
	}

	admin := auth.NewUser{Name: "owner", Password: "right"}
	_, err = service.CompleteSetup(ctx, codes, "AAAA-AAAA-AAAA", admin)
	if !errors.Is(err, auth.ErrWrongSetupCode) {
		t.Errorf("a wrong code: %v, want auth.ErrWrongSetupCode", err)
	}
	// Typed as a person reads it from the log: in capitals, with spaces, an O for a 0.
	typed := strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(code), "-", " "), "0", "O")
	id, err := service.CompleteSetup(ctx, codes, typed, admin)
	if err != nil {
		t.Fatalf("the right code: %v", err)
	}
	issued, err := service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := service.Authenticate(ctx, issued.Token)
	if err != nil || principal.UserID != id || !principal.IsAdmin() {
		t.Errorf("the first user is %+v, %v; want an admin", principal, err)
	}
	if codes.Code != "" {
		t.Error("the setup code survived setup")
	}
	_, err = service.CompleteSetup(ctx, codes, code, auth.NewUser{Name: "intruder", Password: "x"})
	if !errors.Is(err, auth.ErrSetupComplete) {
		t.Errorf("setup a second time: %v, want auth.ErrSetupComplete", err)
	}
	if code, err := service.EnsureSetupCode(ctx, codes); err != nil || code != "" {
		t.Errorf("after setup, EnsureSetupCode = %q, %v; want no code", code, err)
	}
}
