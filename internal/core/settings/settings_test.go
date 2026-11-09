// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func newService(t *testing.T) (*Service, int64) {
	t.Helper()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	return NewService(database, fakeClock, testLogger(t)), storetest.InsertUser(t, database, "owner", "admin")
}

// testLogger writes into the test's own output.
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(t.Output(), nil))
}

func newSeededService(t *testing.T) (*Service, int64) {
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

func jsonByKey(pairs ...string) map[string]json.RawMessage {
	encoded := map[string]json.RawMessage{}
	for index := 0; index+1 < len(pairs); index += 2 {
		encoded[pairs[index]] = json.RawMessage(pairs[index+1])
	}
	return encoded
}

// A setting is read on every playback request, and a waiting chunk request reads several four times
// a second: a lookup that built the registry, compiling its patterns, cost 2 ms and 1.5 MB a read,
// and stalled the owner's playback on a busy machine (2026-10-05).
func TestLookup_FindsADefinitionWithoutBuildingTheRegistry(t *testing.T) {
	definition, known := Lookup(MediaTokenIdleExpiryHours)
	if !known || definition.Key != MediaTokenIdleExpiryHours {
		t.Fatalf("Lookup(%s) = %+v, %v; want its definition", MediaTokenIdleExpiryHours, definition, known)
	}
	if _, known = Lookup("no_such_setting"); known {
		t.Error("Lookup found a key the registry does not have")
	}
	allocations := testing.AllocsPerRun(100, func() { Lookup(MediaTokenIdleExpiryHours) })
	if allocations != 0 {
		t.Errorf("a lookup allocates %.0f times; want none: it builds the registry again", allocations)
	}
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
		if definition.Description == "" {
			t.Errorf("%s says nothing about what it is", definition.Key)
		}
		if definition.IsQuestion() != (definition.Question != "") {
			t.Errorf("%s: a setting starts as null exactly when it is a question", definition.Key)
		}
		if !definition.IsQuestion() {
			if err := checkValue(definition.Key, definition.StartJSON, false); err != nil {
				t.Errorf("%s: its own starting value is refused: %v", definition.Key, err)
			}
		}
	}
}

// The owner tunes values in use, and a good one becomes the starting value; to weigh one against
// another, each says where it came from.
func TestRegistry_EveryStartingValueSaysWhereItCameFrom(t *testing.T) {
	t.Parallel()
	for _, definition := range Registry() {
		if definition.StartSource == "" {
			t.Errorf("%s says nothing of where its starting value came from: a decision, a best guess and "+
				"its basis, or a measurement", definition.Key)
		}
	}
}

func TestSeed_WritesEveryStartingValueAndNeverOverwrites(t *testing.T) {
	t.Parallel()
	service, owner := newService(t)
	ctx := t.Context()
	report, err := service.Seed(ctx)
	if err != nil || len(report.Inserted) != len(Registry()) || len(report.Unused) != 0 {
		t.Fatalf("the first seed = %+v, %v; want every setting written and nothing unknown", report, err)
	}
	entries, err := service.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(Registry()) {
		t.Errorf("after seeding, %d settings, want all %d", len(entries), len(Registry()))
	}
	for _, entry := range entries {
		if entry.IsModified || entry.SetByUserID != nil {
			t.Errorf("a seeded setting reads as changed: %+v", entry)
		}
	}
	if err = service.Set(ctx, jsonByKey("signinFailureLimit", "9"), &owner, false); err != nil {
		t.Fatal(err)
	}
	assertSeedKeepsTheLimitAt(t, service, 9)
}

// assertSeedKeepsTheLimitAt seeds again and checks a person's choice survived it.
func assertSeedKeepsTheLimitAt(t *testing.T, service *Service, want int64) {
	t.Helper()
	report, err := service.Seed(t.Context())
	if err != nil || len(report.Inserted) != 0 {
		t.Fatalf("seeding a seeded database wrote %v, %v; want nothing", report.Inserted, err)
	}
	if limit, err := service.Int(t.Context(), SignInFailureLimit); err != nil || limit != want {
		t.Errorf("after seeding again, the limit = %d, %v; a person's choice must survive every start", limit, err)
	}
}

func TestList_SaysWhatWasChangedAndShowsOnlyTheImportantOnesWhenAsked(t *testing.T) {
	t.Parallel()
	service, owner := newSeededService(t)
	ctx := t.Context()
	page, err := service.List(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range page {
		if !entry.Definition.IsImportant {
			t.Errorf("the important settings list %s, which is not one", entry.Definition.Key)
		}
	}
	if entry := entryFor(t, page,
		ShouldCheckForUpdates); string(entry.Value) != "null" || entry.Definition.Question == "" {
		t.Errorf("an unanswered question: %+v, want null and the question", entry)
	}
	answers := jsonByKey("shouldCheckForUpdates", "false", "sortLocale", ` "sv" `)
	err = service.Set(ctx, answers, &owner, true)
	if err != nil {
		t.Fatal(err)
	}
	all, err := service.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	changed := entryFor(t, all, SortLocale)
	if !changed.IsModified || string(changed.Value) != `"sv"` ||
		changed.SetByUserID == nil || *changed.SetByUserID != owner {
		t.Errorf("a changed setting: %+v, want \"sv\", modified, set by the owner", changed)
	}
	if entry := entryFor(t, all, SignInFailureLimit); entry.IsModified {
		t.Errorf("an untouched setting reads as modified: %+v", entry)
	}
}

func TestRegistry_ALengthOfTimeHasTheUnitItsKeySpells(t *testing.T) {
	t.Parallel()
	unitBySuffix := map[string]time.Duration{"Seconds": time.Second, "Hours": time.Hour, "Days": 24 * time.Hour}
	lengths := 0
	for _, definition := range Registry() {
		key := string(definition.Key)
		var want time.Duration
		for suffix, unit := range unitBySuffix {
			if strings.HasSuffix(key, suffix) {
				want = unit
			}
		}
		if definition.Unit != want {
			t.Errorf("%s: its unit is %v; its key spells %v", key, definition.Unit, want)
		}
		if want != 0 {
			lengths++
			if definition.Kind != KindInteger {
				t.Errorf("%s is a length of time but not an integer", key)
			}
		}
	}
	if lengths == 0 {
		t.Fatal("no setting is a length of time; the check would pass vacuously")
	}
}

func TestDuration_ReadsASettingInItsUnitAndRefusesOneWithNone(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	for _, read := range []struct {
		key  Key
		want time.Duration
	}{
		{JobFirstRetryDelaySeconds, 30 * time.Second},
		{SessionIdleExpiryHours, 2160 * time.Hour},
		{TMDBRefreshAfterDays, 180 * 24 * time.Hour},
	} {
		if got, err := service.Duration(ctx, read.key); err != nil || got != read.want {
			t.Errorf("%s: %v, %v; want %v", read.key, got, err, read.want)
		}
	}
	if _, err := service.Duration(ctx, ArtworkWebPQuality); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("a setting that is no length of time: %v, want ErrInvalidValue", err)
	}
	if _, err := service.Duration(ctx, "no_such_setting_s"); !errors.Is(err, ErrUnknownSetting) {
		t.Errorf("an unknown key: %v, want ErrUnknownSetting", err)
	}
}

func TestStartingDuration_IsTheRegistrysStartingValueInItsUnit(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		key  Key
		want time.Duration
	}{
		{NodeSilentAfterSeconds, 30 * time.Second},
		{MetadataRefreshCheckEveryHours, 24 * time.Hour},
		{RemovedItemArtworkKeptDays, 30 * 24 * time.Hour},
		{ArtworkWebPQuality, 0},    // no length of time
		{ShouldCheckForUpdates, 0}, // a question
		{"noSuchSettingSeconds", 0},
	} {
		if got := StartingDuration(check.key); got != check.want {
			t.Errorf("StartingDuration(%s) = %v, want %v", check.key, got, check.want)
		}
	}
}

func TestReaders_RefuseAnUnansweredQuestionAndTheWrongKind(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	if _, err := service.Bool(ctx, ShouldCheckForUpdates); !errors.Is(err, ErrUnanswered) {
		t.Errorf("reading an unanswered question: %v, want ErrUnanswered", err)
	}
	if _, err := service.Int(ctx, ShouldServeDebugPProf); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("reading a bool as an integer: %v, want ErrInvalidValue", err)
	}
	if on, err := service.Bool(ctx, ShouldScanOnNodeHello); err != nil || !on {
		t.Errorf("reading a seeded bool: %v, %v; want its starting value", on, err)
	}
	if hours, err := service.Int(ctx, SessionIdleExpiryHours); err != nil || hours != 2160 {
		t.Errorf("reading a seeded integer: %d, %v; want its starting value", hours, err)
	}
}

func TestSet_RefusesAndChangesNothing(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	cases := []struct {
		name          string
		values        map[string]json.RawMessage
		importantOnly bool
		want          error
	}{
		{"an unknown key", jsonByKey("transcode_quality", "1"), false, ErrUnknownSetting},
		{"a string for a bool", jsonByKey("shouldCheckForUpdates", `"yes"`), false, ErrInvalidValue},
		{"null, which only reset writes", jsonByKey("shouldCheckForUpdates", "null"), false, ErrInvalidValue},
		{"not JSON", jsonByKey("shouldCheckForUpdates", `yes`), false, ErrInvalidValue},
		{"below the minimum", jsonByKey("signinFailureLimit", "0"), false, ErrInvalidValue},
		{"above the maximum", jsonByKey("secretLastCharsShown", "9"), false, ErrInvalidValue},
		{"a fraction", jsonByKey("signinFailureLimit", "2.5"), false, ErrInvalidValue},
		// A node posts every third of it, at least a second (the constants sweep, 2026-10-21).
		{"a node silence under three seconds", jsonByKey("nodeSilentAfterSeconds", "2"), false, ErrInvalidValue},
		{"one not important, among the important", jsonByKey("signinFailureLimit", "3"), true, ErrNotImportant},
		{
			"one bad among good",
			jsonByKey("shouldScanOnNodeHello", "true", "shouldCheckForUpdates", "1"), false, ErrInvalidValue,
		},
	}
	for _, tc := range cases {
		if err := service.Set(ctx, tc.values, nil, tc.importantOnly); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	entries, err := service.List(ctx, false)
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
	service, owner := newSeededService(t)
	ctx := t.Context()
	changes := jsonByKey("signinFailureLimit", "9", "shouldCheckForUpdates", "true")
	if err := service.Set(ctx, changes, &owner, false); err != nil {
		t.Fatal(err)
	}
	for _, key := range []Key{SignInFailureLimit, ShouldCheckForUpdates} {
		if err := service.Reset(ctx, key, &owner); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := service.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []Key{SignInFailureLimit, ShouldCheckForUpdates} {
		if entry := entryFor(t, entries, key); entry.IsModified {
			t.Errorf("after a reset, %s = %s, want its starting value", key, entry.Value)
		}
	}
	if err = service.Reset(ctx, "no_such_setting", &owner); !errors.Is(err, ErrNoServerSetting) {
		t.Errorf("resetting an unknown key: %v, want ErrNoServerSetting", err)
	}
}

func TestRead_HealsARemovedSettingAndSaysSo(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	var logged bytes.Buffer
	service := NewService(database, clocktest.NewFake(time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)),
		slog.New(slog.NewTextHandler(&logged, nil)))
	ctx := t.Context()
	if _, err := service.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, database.Path(),
		"DELETE FROM setting WHERE key IN ('signinFailureLimit', 'backupAgeWarningHours')")
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("reading a removed setting = %d, %v; want its starting value, written back", limit, err)
	}
	if _, err := service.List(ctx, false); err != nil {
		t.Errorf("List with a removed setting: %v, want it healed", err)
	}
	report, err := service.Seed(ctx)
	if err != nil || len(report.Inserted) != 0 {
		t.Errorf("after the reads healed them, Seed inserted %v, %v; want nothing: the rows are back",
			report.Inserted, err)
	}
	for _, key := range []string{"key=signinFailureLimit", "key=backupAgeWarningHours"} {
		if !strings.Contains(logged.String(), key) {
			t.Errorf("the heal of %s was not logged:\n%s", key, logged.String())
		}
	}
}

// A row under a key the settings page renamed reads as retired, naming the key it is now, and the
// new key starts at its starting value (the owner, 2026-10-21).
func TestUnused_ARenamedKeysRowSaysWhatItIsNow(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	storetest.ExecRaw(t, service.database.Path(),
		"INSERT INTO setting (key, value_json, set_at) VALUES ('resumeMinPercent', '12', 1)")
	unused, err := service.Unused(ctx)
	if err != nil || len(unused) != 1 || unused[0].Retired == nil ||
		unused[0].Retired.Reason != "renamed inProgressAfterPercent" {
		t.Fatalf("Unused = %+v, %v; want resumeMinPercent retired as renamed inProgressAfterPercent",
			unused, err)
	}
	if percent, err := service.Int(ctx, InProgressAfterPercent); err != nil || percent != 5 {
		t.Errorf("inProgressAfterPercent = %d, %v; want its starting 5, the old value not carried", percent,
			err)
	}
}

func TestUnused_ListsRowsNothingReadsAndDeletesOnlyThem(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	storetest.ExecRaw(t, service.database.Path(),
		"INSERT INTO setting (key, value_json, set_at) VALUES ('signin_falure_limit', '3', 1), ('old_key', '1', 1)")
	report, err := service.Seed(ctx)
	if err != nil || len(report.Unused) != 2 {
		t.Fatalf("Seed reported unused %v, %v; want both rows, left alone", report.Unused, err)
	}
	unused, err := service.Unused(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, row := range unused {
		found = append(found, row.String()+"="+row.ValueJSON)
		if row.Retired != nil {
			t.Errorf("%s reads as retired, but no release retired it", row.Key)
		}
	}
	if want := "old_key=1|signin_falure_limit=3"; strings.Join(found, "|") != want {
		t.Errorf("Unused = %v, want %s", found, want)
	}
	if err = service.DeleteUnused(ctx, "signinFailureLimit"); !errors.Is(err, ErrInUse) {
		t.Errorf("deleting a setting in use: %v, want ErrInUse", err)
	}
	if err = service.DeleteUnused(ctx, "signin_falure_limit"); err != nil {
		t.Fatal(err)
	}
	if err = service.DeleteUnused(ctx, "old_key"); err != nil {
		t.Fatal(err)
	}
	if unused, err = service.Unused(ctx); err != nil || len(unused) != 0 {
		t.Errorf("after deleting both, Unused = %v, %v; want none", unused, err)
	}
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("deleting the typo touched the real setting: %d, %v", limit, err)
	}
}

func TestRetiredKeys_AreNotInUseAndSayWhen(t *testing.T) {
	t.Parallel()
	for _, retired := range RetiredKeys() {
		if _, inUse := Lookup(Key(retired.Key)); inUse {
			t.Errorf("%s is both retired and in the registry", retired.Key)
		}
		if retired.Release == "" || retired.Reason == "" {
			t.Errorf("retired key %s does not say when or why", retired.Key)
		}
	}
}

func TestSet_MarksModifiedEvenAtTheStartingValueAndResetClearsIt(t *testing.T) {
	t.Parallel()
	service, owner := newSeededService(t)
	ctx := t.Context()
	if err := service.Set(ctx, jsonByKey("signinFailureLimit", "5"), &owner, false); err != nil {
		t.Fatal(err)
	}
	entries, err := service.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if entry := entryFor(t, entries, SignInFailureLimit); !entry.IsModified {
		t.Errorf("a setting a person set to its starting value reads as unmodified: %+v", entry)
	}
	if err = service.Reset(ctx, SignInFailureLimit, &owner); err != nil {
		t.Fatal(err)
	}
	entries, err = service.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if entry := entryFor(t, entries, SignInFailureLimit); entry.IsModified {
		t.Errorf("after setting it to default, the setting still reads as modified: %+v", entry)
	}
}

func TestSeed_MovesOnlyUnmodifiedSettingsToAChangedStartingValue(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	ctx := t.Context()
	// As a database from an older release looks when this release changed both starting values:
	// one row still has the old starting value, unmodified; the other was set by a person.
	storetest.ExecRaw(t, service.database.Path(),
		"UPDATE setting SET value_json = '7', is_modified = 0 WHERE key = 'signinFailureLimit'",
		"UPDATE setting SET value_json = '6', is_modified = 1 WHERE key = 'secretLastCharsShown'")
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
	if lastCharsShown, err := service.Int(ctx, SecretLastCharsShown); err != nil || lastCharsShown != 6 {
		t.Errorf("the setting a person set = %d, %v; want it left at 6", lastCharsShown, err)
	}
	if again, err := service.Seed(ctx); err != nil || len(again.Updated) != 0 {
		t.Errorf("seeding again updated %v, %v; want nothing", again.Updated, err)
	}
}

func TestOnChange_ReactsToASetAndAResetOfItsKeyOnly(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	var seen []string
	service.OnChange(SortLocale, func(ctx context.Context) error {
		locale, err := service.String(ctx, SortLocale)
		seen = append(seen, locale)
		return err
	})
	set := func(key Key, value string) {
		t.Helper()
		if err := service.Set(t.Context(), map[string]json.RawMessage{string(key): json.RawMessage(value)}, nil,
			false); err != nil {
			t.Fatal(err)
		}
	}
	set(SortLocale, `"sv"`)
	set(ShouldScanOnNodeHello, "false")
	if err := service.Reset(t.Context(), SortLocale, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"sv", "en"}) {
		t.Errorf("reactions saw %v; want the new locale after the set and the starting one after the reset", seen)
	}
	malformed := map[string]json.RawMessage{string(SortLocale): json.RawMessage(`"not a tag!"`)}
	err := service.Set(t.Context(), malformed, nil, false)
	if !errors.Is(err, ErrInvalidValue) || len(seen) != 2 {
		t.Errorf("a malformed locale: %v, reactions %v; want it refused, and no reaction", err, seen)
	}
}

func TestSet_TheSortLocaleTakesAnyWellFormedBCP47Tag(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	for value, isAccepted := range map[string]bool{
		`"en-x-foo"`: true, `"zh-Hant-TW"`: true, `"de-1996"`: true, `"not a tag!"`: false, `"e"`: false,
	} {
		err := service.Set(t.Context(), map[string]json.RawMessage{string(SortLocale): json.RawMessage(value)},
			nil, false)
		if (isAccepted && err != nil) || (!isAccepted && !errors.Is(err, ErrInvalidValue)) {
			t.Errorf("sortLocale %s: %v; want accepted %t", value, err, isAccepted)
		}
	}
}

// The web client waits as the screen-refresh settings start until it has read them (D237 §4), so
// its copy of their starting values is theirs: a setting's start changed alone would leave a
// screen waiting otherwise until the client settings arrive.
func TestRegistry_TheWebClientsWaitsBeforeItReadsThemAreTheScreenRefreshSettingsStarts(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "apps", "web", "src", "polling.ts"))
	if err != nil {
		t.Fatal(err)
	}
	waits := regexp.MustCompile(`backgroundChangeWaits: PollWaits = \{\s*` +
		`firstMs: ([\d_]+),\s*longestMs: ([\d_]+),\s*failedMs: ([\d_]+),?\s*\}`).
		FindSubmatch(source)
	if waits == nil {
		t.Fatal("polling.ts has no backgroundChangeWaits this test can read; say where the client's copy went")
	}
	for index, key := range []Key{
		ScreenRefreshFirstWaitSeconds, ScreenRefreshLongestWaitSeconds, ScreenRefreshFailedWaitSeconds,
	} {
		milliseconds, parseErr := strconv.ParseInt(strings.ReplaceAll(string(waits[index+1]), "_", ""), 10, 64)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if web, start := time.Duration(milliseconds)*time.Millisecond, StartingDuration(key); web != start {
			t.Errorf("the web client waits %v before it reads %s, which starts at %v", web, key, start)
		}
	}
}

// The web player chooses within the core's share of a link until a playback's offer says it, so its
// copy is the setting's starting value (the constants sweep, 2026-10-20).
func TestRegistry_TheWebPlayersShareBeforeAnOfferSaysIsTheLinkShareSettingsStart(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "packages", "player", "src", "abr-choice.ts"))
	if err != nil {
		t.Fatal(err)
	}
	share := regexp.MustCompile(`export const linkShareForQuality = ([\d.]+);`).FindSubmatch(source)
	if share == nil {
		t.Fatal("abr-choice.ts has no linkShareForQuality this test can read; say where the player's copy went")
	}
	web, parseErr := strconv.ParseFloat(string(share[1]), 64)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if start := StartingInt(LinkShareForQualityPercent); int64(math.Round(web*100)) != start {
		t.Errorf("the web player chooses within %v of a link before an offer says; the setting starts at %d%%",
			web, start)
	}
}

// Every setting is listed in one of the settings page's sections (the owner, 2026-10-21).
func TestRegistry_EverySettingIsInASection(t *testing.T) {
	t.Parallel()
	for _, definition := range Registry() {
		if !slices.Contains(Sections(), definition.Section) {
			t.Errorf("%s is in section %q, which is none of %v", definition.Key, definition.Section, Sections())
		}
	}
}

// The server's time zone is the machine's own until the admin chooses one, then the one chosen, a
// name the zone database knows; a misspelt one, and "Local", are refused (D246 §7).
func TestZone_IsTheMachinesUntilOneIsChosenAndAMisspeltOneIsRefused(t *testing.T) {
	t.Parallel()
	service, owner := newSeededService(t)
	ctx := t.Context()
	if zone, err := service.Zone(ctx); err != nil || zone.Name() != "UTC" {
		t.Errorf("the starting time zone with no machine's told = %v (%v); want UTC", zone.Name(), err)
	}
	tokyo, err := localtime.Named("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	service.UseMachineZone(tokyo)
	if zone, zoneErr := service.Zone(ctx); zoneErr != nil || zone.Name() != "Asia/Tokyo" {
		t.Errorf("the starting time zone on a machine in Tokyo = %v (%v); want the machine's", zone.Name(), zoneErr)
	}
	for _, refused := range []string{`"Europe/Berln"`, `"Local"`, `"../etc/passwd"`} {
		err := service.Set(ctx, jsonByKey(string(TimeZone), refused), &owner, true)
		if !errors.Is(err, ErrInvalidValue) {
			t.Errorf("timeZone %s: %v; want it refused", refused, err)
		}
	}
	if err := service.Set(ctx, jsonByKey(string(TimeZone), `"Europe/Berlin"`), &owner, true); err != nil {
		t.Fatal(err)
	}
	if zone, err := service.Zone(ctx); err != nil || zone.Name() != "Europe/Berlin" {
		t.Errorf("timeZone Europe/Berlin = %v (%v)", zone.Name(), err)
	}
}

// A database that kept the retired remote bit-rate ceiling lists its row as unused, saying when
// and why it went, rather than as a typo (D247 §11).
func TestUnused_TheRemoteCeilingsRowSaysItWasRetiredAndWhy(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	storetest.ExecRaw(t, service.database.Path(),
		"INSERT INTO setting (key, value_json, set_at) VALUES ('remoteBitRateCeilingKbps', '10000', 1)")
	unused, err := service.Unused(t.Context())
	if err != nil || len(unused) != 1 {
		t.Fatalf("Unused = %v, %v; want the ceiling's row", unused, err)
	}
	if retired := unused[0].Retired; retired == nil || !strings.Contains(retired.Reason, "D247") {
		t.Errorf("the ceiling's row reads as %+v; want retired, with D247's reason", retired)
	}
}

// chunkQualities is checked by the function internal/core/chunks reads it with, so a value the
// settings take is one the chunks can read (AGENTS.md rule 11).
func TestChunkQualities_IsCheckedByTheParserThatReadsIt(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	for value, isAccepted := range map[string]bool{
		`"4000k@1080p,800k@360p"`: true, `"800k@360p,4000k@1080p"`: false, `"800k@361p"`: false, `""`: false,
	} {
		err := service.Set(t.Context(), map[string]json.RawMessage{string(ChunkQualities): json.RawMessage(value)},
			nil, false)
		if (isAccepted && err != nil) || (!isAccepted && !errors.Is(err, ErrInvalidValue)) {
			t.Errorf("chunkQualities %s: %v; want accepted %t", value, err, isAccepted)
		}
	}
	text, err := service.String(t.Context(), ChunkQualities)
	if err != nil || text != "4000k@1080p,800k@360p" {
		t.Errorf("chunkQualities reads %q, %v; want the value set", text, err)
	}
}

// failingReader fails every read, as a database that is gone.
type failingReader struct{}

func (failingReader) Duration(context.Context, Key) (time.Duration, error) {
	return 0, errors.New("the database is gone")
}

// A loop's wait is the setting as read, or its starting value when it cannot be read or there is no
// reader at all, said in the log.
func TestDurationOrStarting_IsTheSettingOrItsStart(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	service, owner := newSeededService(t)
	if err := service.Set(t.Context(), jsonByKey(string(DiscoveryEndedCheckEverySeconds), "900"), &owner,
		false); err != nil {
		t.Fatal(err)
	}
	key := DiscoveryEndedCheckEverySeconds
	if wait := DurationOrStarting(t.Context(), service, key, logger); wait != 15*time.Minute {
		t.Errorf("read: %s; want the 900 s set", wait)
	}
	start := StartingDuration(key)
	if wait := DurationOrStarting(t.Context(), failingReader{}, key, logger); wait != start {
		t.Errorf("a failing read: %s; want the starting %s", wait, start)
	}
	if len(testutil.LinesWith(logged.String(), "settingUnread")) == 0 {
		t.Errorf("a failing read was not logged: %q", logged.String())
	}
	if wait := DurationOrStarting(t.Context(), nil, key, logger); wait != start {
		t.Errorf("no reader: %s; want the starting %s", wait, start)
	}
}

// A setting is read from memory: once the values are read, reads run no statement, however many,
// and a set or a reset is seen by the very next read (principles §11; audit 2026-10-25, S2). A
// chunk's request read about six settings, each a statement, before.
func TestRead_RunsNoStatementAndSeesEachWriteAtOnce(t *testing.T) {
	t.Parallel()
	service, owner := newSeededService(t)
	ctx, tally := querytally.New(t.Context())
	for range 100 {
		if _, err := service.Int(ctx, SignInFailureLimit); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Duration(ctx, BackupAgeWarningHours); err != nil {
			t.Fatal(err)
		}
	}
	if ran := tally.Statements(); ran != 0 {
		t.Errorf("200 reads ran %d statements (%s); want none, the values held in memory", ran, tally)
	}
	if err := service.Set(ctx, jsonByKey(string(SignInFailureLimit), "9"), &owner, false); err != nil {
		t.Fatal(err)
	}
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 9 {
		t.Errorf("read after a set = %d, %v; want 9", limit, err)
	}
	if err := service.Reset(ctx, SignInFailureLimit, &owner); err != nil {
		t.Fatal(err)
	}
	if limit, err := service.Int(ctx, SignInFailureLimit); err != nil || limit != 5 {
		t.Errorf("read after a reset = %d, %v; want its starting 5", limit, err)
	}
}

// A row of the check D437 retired reads as retired by D437, not as a typo, so an admin upgrading
// sees why their old value no longer acts (D437 §3).
func TestUnused_TheAnotherCopyCheckReadsAsRetiredByD437(t *testing.T) {
	t.Parallel()
	service, _ := newSeededService(t)
	storetest.ExecRaw(t, service.database.Path(),
		"INSERT INTO setting (key, value_json, set_at) VALUES ('discoveryAnotherCopyCheckEverySeconds', '60', 1)")
	unused, err := service.Unused(t.Context())
	if err != nil || len(unused) != 1 {
		t.Fatalf("Unused = %v, %v; want the one row", unused, err)
	}
	if retired := unused[0].Retired; retired == nil || !strings.Contains(retired.Reason, "D437") {
		t.Errorf("the retired check reads as %+v, want retired by D437", retired)
	}
}
