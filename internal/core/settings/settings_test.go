// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"errors"
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

func entryFor(t *testing.T, entries []Entry, key string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Definition.Key == key {
			return entry
		}
	}
	t.Fatalf("no entry for %s", key)
	return Entry{}
}

func TestRegistry_EverySettingHasADefaultOrAQuestion(t *testing.T) {
	t.Parallel()
	if len(Registry()) == 0 {
		t.Fatal("the registry is empty; the check would pass vacuously")
	}
	for _, definition := range Registry() {
		hasDefault, hasQuestion := definition.Default != nil, definition.Question != ""
		if hasDefault == hasQuestion {
			t.Errorf("%s: a setting has a default or a question, exactly one (D182)", definition.Key)
		}
		if hasDefault {
			if err := check(definition.Key, definition.Scope, definition.Default); err != nil {
				t.Errorf("%s: its own default is refused: %v", definition.Key, err)
			}
		}
	}
}

func TestList_ShowsDefaultsQuestionsAndWhatWasSet(t *testing.T) {
	t.Parallel()
	service, owner := newService(t)
	ctx := t.Context()
	entries, err := service.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if entry := entryFor(t, entries, "subtitles_on_by_default"); !entry.IsDefault || string(entry.Value) != "true" {
		t.Errorf("an unset setting with a default: %+v, want the default, marked as one", entry)
	}
	if entry := entryFor(t, entries, "update_check"); entry.Value != nil || entry.Definition.Question == "" {
		t.Errorf("an unanswered question: %+v, want no value and the question", entry)
	}

	err = service.Set(ctx, nil, map[string]json.RawMessage{
		"subtitles_on_by_default": json.RawMessage(" false "),
		"update_check":            json.RawMessage("false"),
	}, &owner)
	if err != nil {
		t.Fatal(err)
	}
	entries, err = service.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := entryFor(t, entries, "subtitles_on_by_default")
	if entry.IsDefault || string(entry.Value) != "false" || entry.SetBy == nil || *entry.SetBy != owner {
		t.Errorf("a set setting: %+v, want false, set by the owner", entry)
	}
	if entry := entryFor(t, entries, "update_check"); string(entry.Value) != "false" {
		t.Errorf("an answered question: %+v, want its answer", entry)
	}
}

func TestSet_RefusesAnUnknownKeyTheWrongScopeOrKindAndChangesNothing(t *testing.T) {
	t.Parallel()
	service, owner := newService(t)
	ctx := t.Context()
	cases := []struct {
		name   string
		owner  *int64
		values map[string]json.RawMessage
		want   error
	}{
		{
			"an unknown key", nil,
			map[string]json.RawMessage{"transcode_quality": json.RawMessage("1")},
			ErrUnknownSetting,
		},
		{
			"a server key as a user's", &owner,
			map[string]json.RawMessage{"update_check": json.RawMessage("true")},
			ErrWrongScope,
		},
		{
			"a string for a bool", nil,
			map[string]json.RawMessage{"update_check": json.RawMessage(`"yes"`)},
			ErrWrongKind,
		},
		{"not JSON", nil, map[string]json.RawMessage{"update_check": json.RawMessage(`yes`)}, ErrWrongKind},
		{"one bad among good", nil, map[string]json.RawMessage{
			"dialogue_boost_default": json.RawMessage("true"),
			"update_check":           json.RawMessage("1"),
		}, ErrWrongKind},
	}
	for _, tc := range cases {
		if err := service.Set(ctx, tc.owner, tc.values, nil); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	entries, err := service.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDefault {
			t.Errorf("%s was stored although its request was refused", entry.Definition.Key)
		}
	}
}
