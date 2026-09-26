// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const canary = "CANARY-7f3a9c-secret-value"

func TestValue_NeverShowsItsContents(t *testing.T) {
	t.Parallel()
	value := NewValue(canary)
	type carrier struct {
		Key Value `json:"key"`
	}
	var logged bytes.Buffer
	textLogger := slog.New(slog.NewTextHandler(&logged, nil))
	jsonLogger := slog.New(slog.NewJSONHandler(&logged, nil))
	textLogger.Info("secret", "key", value)
	jsonLogger.Info("secret", "key", value, "carrier", carrier{Key: value})
	marshalled, err := json.Marshal(carrier{Key: value})
	if err != nil {
		t.Fatal(err)
	}
	text, err := value.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	renderings := map[string]string{
		"%q":           fmt.Sprintf("%q", value),
		"%x":           fmt.Sprintf("%x", value),
		"%+v":          fmt.Sprintf("%+v", carrier{Key: value}),
		"%#v":          fmt.Sprintf("%#v", carrier{Key: value}),
		"String":       value.String(),
		"GoString":     value.GoString(),
		"MarshalJSON":  string(marshalled),
		"MarshalText":  string(text),
		"slog handler": logged.String(),
	}
	for how, rendered := range renderings {
		if strings.Contains(rendered, canary) || strings.Contains(rendered, fmt.Sprintf("%x", canary)) {
			t.Errorf("%s shows the secret: %s", how, rendered)
		}
		if !strings.Contains(rendered, redacted) {
			t.Errorf("%s = %q, want it to say %s", how, rendered, redacted)
		}
	}
	if value.Reveal() != canary {
		t.Errorf("Reveal() = %q, want the secret: the redaction must not lose it", value.Reveal())
	}
}

func TestKeyFile_RoundTripsAndRefusesAnotherFormat(t *testing.T) {
	t.Parallel()
	key, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseKeyFile(key.EncodeKeyFile())
	if err != nil {
		t.Fatalf("ParseKeyFile: %v", err)
	}
	if parsed != key || parsed.ID() <= 0 {
		t.Errorf("the key did not survive its file, or has id %d", parsed.ID())
	}
	other, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if other.ID() == key.ID() {
		t.Error("two random keys share an id")
	}
	for name, contents := range map[string]string{
		"no header":   "c2VjcmV0\n",
		"short key":   keyFileHeader + "\nc2hvcnQ=\n",
		"not base64":  keyFileHeader + "\n!!!\n",
		"other magic": "some-other-key v1\n" + strings.Repeat("A", 44) + "\n",
	} {
		if _, err := ParseKeyFile([]byte(contents)); err == nil {
			t.Errorf("%s: ParseKeyFile succeeded, want a refusal", name)
		}
	}
}

type fixture struct {
	database *store.Database
	settings *settings.Service
	secrets  *Store
	key      Key
	userID   int64
	otherID  int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	database := storetest.Open(t)
	key, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	return fixture{
		database: database,
		settings: seeded,
		secrets:  NewStore(database, key, now, rand.Reader, seeded),
		key:      key,
		userID:   storetest.InsertUser(t, database, "owner", "admin"),
		otherID:  storetest.InsertUser(t, database, "partner", "user"),
	}
}

func TestStore_SealsAndOpens(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	if err := fixture.secrets.Set(ctx, TMDBAPIKey, nil, NewValue(canary), &fixture.userID); err != nil {
		t.Fatal(err)
	}
	opened, err := fixture.secrets.Open(ctx, TMDBAPIKey, nil)
	if err != nil || opened.Reveal() != canary {
		t.Fatalf("Open = %v, %v; want the value set", opened, err)
	}
	row, err := fixture.database.Read().GetSealedSecret(ctx, sqlcgen.GetSealedSecretParams{
		Scope: string(ScopeServer), Name: string(TMDBAPIKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.Ciphertext, []byte(canary)) || bytes.Contains(row.Nonce, []byte(canary)) {
		t.Error("the stored row holds the secret in plain text")
	}
	if row.KeyID != fixture.key.ID() {
		t.Errorf("row key_id = %d, want the sealing key's %d", row.KeyID, fixture.key.ID())
	}
	if _, err := fixture.secrets.Open(ctx, OpenSubtitlesAPIKey, &fixture.userID); !errors.Is(err, ErrNotSet) {
		t.Errorf("Open(unset) error = %v, want ErrNotSet", err)
	}
	if err := fixture.secrets.Clear(ctx, TMDBAPIKey, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.secrets.Open(ctx, TMDBAPIKey, nil); !errors.Is(err, ErrNotSet) {
		t.Errorf("Open(cleared) error = %v, want ErrNotSet", err)
	}
}

func TestStore_ASealedValueCopiedToAnotherRowDoesNotOpen(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	if err := fixture.secrets.Set(ctx, OpenSubtitlesPassword, &fixture.userID, NewValue(canary), nil); err != nil {
		t.Fatal(err)
	}
	row, err := fixture.database.Read().GetSealedSecret(ctx, sqlcgen.GetSealedSecretParams{
		Scope: string(ScopeUser), UserID: &fixture.userID, Name: string(OpenSubtitlesPassword),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSealedSecret(ctx, sqlcgen.UpsertSealedSecretParams{
			Scope: row.Scope, UserID: &fixture.otherID, Name: row.Name, KeyID: row.KeyID,
			Nonce: row.Nonce, Ciphertext: row.Ciphertext, SetAt: row.SetAt,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.secrets.Open(ctx, OpenSubtitlesPassword, &fixture.otherID); !errors.Is(err, ErrNeedsReentry) {
		t.Errorf("Open(another user's copied ciphertext) error = %v, want ErrNeedsReentry", err)
	}
	if opened, err := fixture.secrets.Open(ctx, OpenSubtitlesPassword, &fixture.userID); err != nil ||
		opened.Reveal() != canary {
		t.Errorf("the original row: %v, %v; want it still opens", opened, err)
	}
}

func TestStore_AnotherKeyMarksSecretsNeedingReentry(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	if err := fixture.secrets.Set(ctx, TMDBAPIKey, nil, NewValue(canary), nil); err != nil {
		t.Fatal(err)
	}
	if status := serverStatus(t, fixture.secrets, TMDBAPIKey); !status.IsSet || status.NeedsReentry {
		t.Errorf("under its own key: %+v, want set and not needing re-entry", status)
	}

	restoredWithoutKey, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := clocktest.NewFake(time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC))
	afterRestore := NewStore(fixture.database, restoredWithoutKey, now, rand.Reader, fixture.settings)
	if status := serverStatus(t, afterRestore, TMDBAPIKey); !status.IsSet || !status.NeedsReentry {
		t.Errorf("under a new key: %+v, want set and needing re-entry", status)
	}
	if _, err = afterRestore.Open(ctx, TMDBAPIKey, nil); !errors.Is(err, ErrNeedsReentry) {
		t.Errorf("Open under a new key: error = %v, want ErrNeedsReentry", err)
	}
	needing, err := afterRestore.NeedingReentry(ctx)
	if err != nil || len(needing) != 1 || needing[0].Name != TMDBAPIKey {
		t.Errorf("NeedingReentry = %+v, %v; want the TMDB key", needing, err)
	}
	if err = afterRestore.Set(ctx, TMDBAPIKey, nil, NewValue("re-entered"), nil); err != nil {
		t.Fatal(err)
	}
	needing, err = afterRestore.NeedingReentry(ctx)
	if err != nil || len(needing) != 0 {
		t.Errorf("after re-entering, NeedingReentry = %+v, %v; want none", needing, err)
	}
}

func serverStatus(t *testing.T, secrets *Store, name Name) Status {
	t.Helper()
	statuses, err := secrets.Statuses(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return findStatus(t, statuses, name)
}

func TestStore_ShowsAHintOnlyForAnAPIKey(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	const apiKey = "0123456789abcdefWXYZ"
	if err := fixture.secrets.Set(ctx, OpenSubtitlesAPIKey, &fixture.userID, NewValue(apiKey), nil); err != nil {
		t.Fatal(err)
	}
	if err := fixture.secrets.Set(ctx, OpenSubtitlesPassword, &fixture.userID, NewValue(canary), nil); err != nil {
		t.Fatal(err)
	}
	statuses, err := fixture.secrets.Statuses(ctx, &fixture.userID)
	if err != nil {
		t.Fatal(err)
	}
	if hint := findStatus(t, statuses, OpenSubtitlesAPIKey).Hint; hint != "WXYZ" {
		t.Errorf("API key hint = %q, want its last four characters", hint)
	}
	settingstest.Set(t, fixture.settings, settings.SecretHintChars, "2")
	if err = fixture.secrets.Set(ctx, OpenSubtitlesAPIKey, &fixture.userID, NewValue(apiKey), nil); err != nil {
		t.Fatal(err)
	}
	statuses, err = fixture.secrets.Statuses(ctx, &fixture.userID)
	if err != nil {
		t.Fatal(err)
	}
	if hint := findStatus(t, statuses, OpenSubtitlesAPIKey).Hint; hint != "YZ" {
		t.Errorf("with secret_hint_chars 2, the hint = %q, want the last two characters", hint)
	}
	if hint := findStatus(t, statuses, OpenSubtitlesPassword).Hint; hint != "" {
		t.Errorf("password hint = %q, want none", hint)
	}
	if status := findStatus(t, statuses, OpenSubtitlesUsername); status.IsSet {
		t.Errorf("an unset secret reports %+v", status)
	}
}

func TestStore_RefusesAnUnknownNameOrTheWrongScope(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	cases := []struct {
		name  Name
		owner *int64
		want  error
	}{
		{name: "plex_token", owner: nil, want: ErrUnknownSecret},
		{name: TMDBAPIKey, owner: &fixture.userID, want: ErrWrongScope},
		{name: OpenSubtitlesPassword, owner: nil, want: ErrWrongScope},
	}
	for _, tc := range cases {
		if err := fixture.secrets.Set(ctx, tc.name, tc.owner, NewValue(canary), nil); !errors.Is(err, tc.want) {
			t.Errorf("Set(%s) error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func findStatus(t *testing.T, statuses []Status, name Name) Status {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			return status
		}
	}
	t.Fatalf("no status for %s in %+v", name, statuses)
	return Status{}
}
