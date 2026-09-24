// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func newService(t *testing.T) (*Service, int64) {
	t.Helper()
	database := storetest.Open(t)
	now := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	return NewService(database, now), storetest.InsertUser(t, database, "owner", "admin")
}

func seeded(t *testing.T) (*Service, int64) {
	t.Helper()
	service, owner := newService(t)
	if _, err := service.Seed(t.Context()); err != nil {
		t.Fatal(err)
	}
	return service, owner
}

func entryFor(t *testing.T, entries []Entry, key Key) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Definition.Key == key {
			return entry
		}
	}
	t.Fatalf("no entry for %s", key)
	return Entry{}
}

func values(pairs ...string) map[string]json.RawMessage {
	encoded := map[string]json.RawMessage{}
	for index := 0; index+1 < len(pairs); index += 2 {
		encoded[pairs[index]] = json.RawMessage(pairs[index+1])
	}
	return encoded
}

func TestRegistry_EveryStartingValueIsAValidValueOrAQuestion(t *testing.T) {
	t.Parallel()
	if len(Registry()) == 0 {
		t.Fatal("the registry is empty; the check would pass vacuously")
	}
	seen := map[Key]bool{}
	for _, definition := range Registry() {
		if seen[definition.Key] {
			t.Errorf("%s is declared twice", definition.Key)
		}
		seen[definition.Key] = true
		if definition.Describe == "" {
			t.Errorf("%s says nothing about what it is", definition.Key)
		}
		if definition.Scope != ScopeServer {
			t.Errorf("%s is a user setting; seeding user settings is not built yet, so it would have no row",
				definition.Key)
		}
		if definition.IsQuestion() != (definition.Question != "") {
			t.Errorf("%s: a setting starts as null exactly when it is a question", definition.Key)
		}
		if !definition.IsQuestion() {
			if err := check(definition.Key, definition.Scope, definition.Start, false); err != nil {
				t.Errorf("%s: its own starting value is refused: %v", definition.Key, err)
			}
		}
	}
}

func TestSeed_WritesEveryStartingValueAndNeverOverwrites(t *testing.T) {
	t.Parallel()
	service, owner := newService(t)
	ctx := t.Context()
	if _, err := service.Int(ctx, SignInFailureLimit); !errors.Is(err, ErrNotSeeded) {
		t.Errorf("before seeding, reading a setting: %v, want ErrNotSeeded and no fallback", err)
	}
	if _, err := service.List(ctx, nil, false); !errors.Is(err, ErrNotSeeded) {
		t.Errorf("before seeding, List: %v, want ErrNotSeeded", err)
	}
	report, err := service.Seed(ctx)
	if err != nil || len(report.Written) != len(Registry()) || len(report.Unknown) != 0 {
		t.Fatalf("the first seed = %+v, %v; want every setting written and nothing unknown", report, err)
	}
	entries, err := service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(Registry()) {
		t.Errorf("after seeding, %d settings, want all %d", len(entries), len(Registry()))
	}
	for _, entry := range entries {
		if entry.IsModified || entry.SetBy != nil {
			t.Errorf("a seeded setting reads as changed: %+v", entry)
		}
	}
	if err = service.Set(ctx, nil, values("signin_failure_limit", "9"), &owner, false); err != nil {
		t.Fatal(err)
	}
	assertSeedKeepsTheLimitAt(t, service, 9)
}

// assertSeedKeepsTheLimitAt seeds again and checks a person's choice survived it.
func assertSeedKeepsTheLimitAt(t *testing.T, service *Service, want int64) {
	t.Helper()
	report, err := service.Seed(t.Context())
	if err != nil || len(report.Written) != 0 {
		t.Fatalf("seeding a seeded database wrote %v, %v; want nothing", report.Written, err)
	}
	if limit, err := service.Int(t.Context(), SignInFailureLimit); err != nil || limit != want {
		t.Errorf("after seeding again, the limit = %d, %v; a person's choice must survive every start", limit, err)
	}
}

func TestList_SaysWhatWasChangedAndShowsOnlyExposedOnTheSettingsPage(t *testing.T) {
	t.Parallel()
	service, owner := seeded(t)
	ctx := t.Context()
	page, err := service.List(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range page {
		if !entry.Definition.Exposed {
			t.Errorf("the settings page lists %s, which it does not show", entry.Definition.Key)
		}
	}
	if entry := entryFor(t, page, UpdateCheck); string(entry.Value) != "null" || entry.Definition.Question == "" {
		t.Errorf("an unanswered question: %+v, want null and the question", entry)
	}
	err = service.Set(ctx, nil, values("update_check", "false", "subtitles_on_by_default", " false "), &owner, true)
	if err != nil {
		t.Fatal(err)
	}
	all, err := service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	changed := entryFor(t, all, SubtitlesOnByDefault)
	if !changed.IsModified || string(changed.Value) != "false" || changed.SetBy == nil || *changed.SetBy != owner {
		t.Errorf("a changed setting: %+v, want false, modified, set by the owner", changed)
	}
	if entry := entryFor(t, all, SignInFailureLimit); entry.IsModified {
		t.Errorf("an untouched setting reads as modified: %+v", entry)
	}
}

func TestReaders_RefuseAnUnansweredQuestionAndTheWrongKind(t *testing.T) {
	t.Parallel()
	service, _ := seeded(t)
	ctx := t.Context()
	if _, err := service.Bool(ctx, UpdateCheck); !errors.Is(err, ErrUnanswered) {
		t.Errorf("reading an unanswered question: %v, want ErrUnanswered", err)
	}
	if _, err := service.Int(ctx, DebugPProf); !errors.Is(err, ErrWrongKind) {
		t.Errorf("reading a bool as an integer: %v, want ErrWrongKind", err)
	}
	if on, err := service.Bool(ctx, SubtitlesOnByDefault); err != nil || !on {
		t.Errorf("reading a seeded bool: %v, %v; want its starting value", on, err)
	}
	if hours, err := service.Int(ctx, SessionIdleExpiryH); err != nil || hours != 2160 {
		t.Errorf("reading a seeded integer: %d, %v; want its starting value", hours, err)
	}
}

func TestSet_RefusesAndChangesNothing(t *testing.T) {
	t.Parallel()
	service, owner := seeded(t)
	ctx := t.Context()
	cases := []struct {
		name        string
		owner       *int64
		values      map[string]json.RawMessage
		exposedOnly bool
		want        error
	}{
		{"an unknown key", nil, values("transcode_quality", "1"), false, ErrUnknownSetting},
		{"a server key as a user's", &owner, values("update_check", "true"), false, ErrWrongScope},
		{"a string for a bool", nil, values("update_check", `"yes"`), false, ErrWrongKind},
		{"null, which only reset writes", nil, values("update_check", "null"), false, ErrWrongKind},
		{"not JSON", nil, values("update_check", `yes`), false, ErrWrongKind},
		{"below the minimum", nil, values("signin_failure_limit", "0"), false, ErrWrongKind},
		{"above the maximum", nil, values("secret_hint_length", "9"), false, ErrWrongKind},
		{"a fraction", nil, values("signin_failure_limit", "2.5"), false, ErrWrongKind},
		{"a hidden key on the settings page", nil, values("signin_failure_limit", "3"), true, ErrNotExposed},
		{"one bad among good", nil, values("dialogue_boost_default", "true", "update_check", "1"), false, ErrWrongKind},
	}
	for _, tc := range cases {
		if err := service.Set(ctx, tc.owner, tc.values, nil, tc.exposedOnly); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	entries, err := service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsModified {
			t.Errorf("%s changed although every request was refused", entry.Definition.Key)
		}
	}
}

func TestReset_WritesTheStartingValueBack(t *testing.T) {
	t.Parallel()
	service, owner := seeded(t)
	ctx := t.Context()
	changes := values("signin_failure_limit", "9", "update_check", "true")
	if err := service.Set(ctx, nil, changes, &owner, false); err != nil {
		t.Fatal(err)
	}
	for _, key := range []Key{SignInFailureLimit, UpdateCheck} {
		if err := service.Reset(ctx, key, &owner); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []Key{SignInFailureLimit, UpdateCheck} {
		if entry := entryFor(t, entries, key); entry.IsModified {
			t.Errorf("after a reset, %s = %s, want its starting value", key, entry.Value)
		}
	}
	if err = service.Reset(ctx, "no_such_setting", &owner); !errors.Is(err, ErrUnknownSetting) {
		t.Errorf("resetting an unknown key: %v, want ErrUnknownSetting", err)
	}
}

func TestSeed_RefillsARemovedSettingAndTheErrorSaysHowToFixIt(t *testing.T) {
	t.Parallel()
	service, _ := seeded(t)
	ctx := t.Context()
	storetest.ExecRaw(t, service.database.Path(), "DELETE FROM setting WHERE key = 'signin_failure_limit'")
	_, err := service.Int(ctx, SignInFailureLimit)
	missing, isMissing := errors.AsType[*NotSeededError](err)
	if !isMissing || !errors.Is(err, ErrNotSeeded) {
		t.Fatalf("reading a removed setting: %v, want a NotSeededError", err)
	}
	wants := []string{"signin_failure_limit", "starting value is 5", "orpheum seed-settings", "PUT /api/v1/config"}
	for _, want := range wants {
		if !strings.Contains(missing.Error(), want) {
			t.Errorf("the error does not say %q: %s", want, missing.Error())
		}
	}
	if _, err = service.List(ctx, nil, false); !errors.Is(err, ErrNotSeeded) ||
		!strings.Contains(err.Error(), "starting value is 5") {
		t.Errorf("List with a removed setting: %v, want the same guidance", err)
	}
	report, err := service.Seed(ctx)
	if err != nil || len(report.Written) != 1 || report.Written[0] != SignInFailureLimit {
		t.Fatalf("seeding after a removal wrote %v, %v; want just the removed setting", report.Written, err)
	}
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("after seeding, the limit = %d, %v; want its starting value back", limit, err)
	}
}

func TestSeed_ReportsRowsNothingReadsAndLeavesThemAlone(t *testing.T) {
	t.Parallel()
	service, owner := seeded(t)
	ctx := t.Context()
	storetest.ExecRaw(t, service.database.Path(),
		"INSERT INTO setting (scope, key, value, set_at) VALUES ('server', 'signin_falure_limit', '3', 1)",
		fmt.Sprintf("INSERT INTO setting (scope, user_id, key, value, set_at) VALUES ('user', %d, 'old_key', '1', 1)",
			owner))
	report, err := service.Seed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, unknown := range report.Unknown {
		found = append(found, unknown.String())
	}
	want := []string{"signin_falure_limit", fmt.Sprintf("old_key (user %d)", owner)}
	if strings.Join(found, "|") != strings.Join(want, "|") {
		t.Errorf("Seed reported unknown rows %v, want %v", found, want)
	}
	unknown, err := service.Unknown(ctx)
	if err != nil || len(unknown) != 2 {
		t.Errorf("Unknown = %v, %v; want the same two rows, still there", unknown, err)
	}
}

func TestSet_MarksModifiedEvenAtTheStartingValueAndResetClearsIt(t *testing.T) {
	t.Parallel()
	service, owner := seeded(t)
	ctx := t.Context()
	if err := service.Set(ctx, nil, values("signin_failure_limit", "5"), &owner, false); err != nil {
		t.Fatal(err)
	}
	entries, err := service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if entry := entryFor(t, entries, SignInFailureLimit); !entry.IsModified {
		t.Errorf("a setting a person set to its starting value reads as unmodified: %+v", entry)
	}
	if err = service.Reset(ctx, SignInFailureLimit, &owner); err != nil {
		t.Fatal(err)
	}
	entries, err = service.List(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if entry := entryFor(t, entries, SignInFailureLimit); entry.IsModified {
		t.Errorf("after setting it to default, the setting still reads as modified: %+v", entry)
	}
}

func TestSeed_MovesOnlyUnmodifiedSettingsToAChangedStartingValue(t *testing.T) {
	t.Parallel()
	service, _ := seeded(t)
	ctx := t.Context()
	// As a database from an older release looks when this release changed both starting values:
	// one row still has the old starting value, unmodified; the other was set by a person.
	storetest.ExecRaw(t, service.database.Path(),
		"UPDATE setting SET value = '7', is_modified = 0 WHERE key = 'signin_failure_limit'",
		"UPDATE setting SET value = '6', is_modified = 1 WHERE key = 'secret_hint_length'")
	report, err := service.Seed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Updated) != 1 || report.Updated[0] != SignInFailureLimit {
		t.Errorf("Seed updated %v, want only the unmodified setting", report.Updated)
	}
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("the unmodified setting = %d, %v; want the new starting value 5", limit, err)
	}
	if hint, err := service.Int(ctx, SecretHintLength); err != nil || hint != 6 {
		t.Errorf("the setting a person set = %d, %v; want it left at 6", hint, err)
	}
	if again, err := service.Seed(ctx); err != nil || len(again.Updated) != 0 {
		t.Errorf("seeding again updated %v, %v; want nothing", again.Updated, err)
	}
}
