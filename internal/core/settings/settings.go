// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Scope is whose a setting is.
type Scope string

// The scopes, as the setting table's CHECK names them.
const (
	ScopeServer Scope = "server"
	ScopeUser   Scope = "user"
)

// Kind is a setting value's JSON type.
type Kind string

// The kinds a setting may have.
const (
	KindBool    Kind = "bool"
	KindInteger Kind = "integer"
	KindString  Kind = "string"
)

// Definition is one setting: its scope and kind, and either a default that is safe to be wrong
// and shown as a default, or a question asked in the admin UI (D182, D183). A row exists in the
// setting table only when someone set a value.
type Definition struct {
	Key      string
	Scope    Scope
	Kind     Kind
	Default  json.RawMessage // nil when the setting is a question
	Question string          // what the UI asks when there is no default
}

// Registry is every setting. M1 declares the three that dev/seed.toml sets; each later setting
// arrives with its feature.
func Registry() []Definition {
	return []Definition{
		{
			Key: "update_check", Scope: ScopeServer, Kind: KindBool,
			Question: "Check GitHub for a new release once a day? The check sends nothing but the request.",
		},
		{Key: "subtitles_on_by_default", Scope: ScopeServer, Kind: KindBool, Default: json.RawMessage("true")},
		{Key: "dialogue_boost_default", Scope: ScopeServer, Kind: KindBool, Default: json.RawMessage("false")},
	}
}

// Lookup finds a setting's definition.
func Lookup(key string) (Definition, bool) {
	for _, definition := range Registry() {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

var (
	// ErrUnknownSetting is returned for a key the registry does not declare.
	ErrUnknownSetting = errors.New("no such setting")
	// ErrWrongScope is returned for a server setting set as a user's, or the reverse.
	ErrWrongScope = errors.New("the setting belongs to the other scope")
	// ErrWrongKind is returned for a value of the wrong JSON type.
	ErrWrongKind = errors.New("the value has the wrong type")
)

// Entry is a setting as the settings page shows it: the value in force and where it came from.
type Entry struct {
	Definition Definition
	Value      json.RawMessage // nil while a question is unanswered
	IsDefault  bool
	SetBy      *int64
	SetAt      int64
}

// Service reads and writes settings.
type Service struct {
	database *store.Database
	clock    clock.Clock
}

// NewService returns a settings Service.
func NewService(database *store.Database, now clock.Clock) *Service {
	return &Service{database: database, clock: now}
}

// List returns every setting of a scope with its value in force; owner is the user for the
// user scope and nil for the server's.
func (service *Service) List(ctx context.Context, owner *int64) ([]Entry, error) {
	scope, stored, err := service.stored(ctx, owner)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, definition := range Registry() {
		if definition.Scope != scope {
			continue
		}
		entry := Entry{Definition: definition, Value: definition.Default, IsDefault: true}
		if row, isSet := stored[definition.Key]; isSet {
			entry.Value, entry.IsDefault = json.RawMessage(row.Value), false
			entry.SetBy, entry.SetAt = row.SetBy, row.SetAt
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

type storedSetting struct {
	Value string
	SetBy *int64
	SetAt int64
}

func (service *Service) stored(ctx context.Context, owner *int64) (Scope, map[string]storedSetting, error) {
	stored := map[string]storedSetting{}
	if owner == nil {
		rows, err := service.database.Read().ListServerSettings(ctx)
		if err != nil {
			return ScopeServer, nil, fmt.Errorf("list server settings: %w", err)
		}
		for _, row := range rows {
			stored[row.Key] = storedSetting{Value: row.Value, SetBy: row.SetBy, SetAt: row.SetAt}
		}
		return ScopeServer, stored, nil
	}
	rows, err := service.database.Read().ListUserSettings(ctx, owner)
	if err != nil {
		return ScopeUser, nil, fmt.Errorf("list user settings: %w", err)
	}
	for _, row := range rows {
		stored[row.Key] = storedSetting{Value: row.Value, SetBy: row.SetBy, SetAt: row.SetAt}
	}
	return ScopeUser, stored, nil
}

// Set stores values, each checked against the registry, in one transaction: all of them or
// none. The owner is the user for user settings and nil for server settings; setBy is who
// changed them, nil for the setup wizard or the dev seed.
func (service *Service) Set(ctx context.Context, owner *int64, values map[string]json.RawMessage, setBy *int64) error {
	scope := ScopeServer
	if owner != nil {
		scope = ScopeUser
	}
	for key, value := range values {
		if err := check(key, scope, value); err != nil {
			return err
		}
	}
	now := service.clock.Now().UnixMilli()
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for key, value := range values {
			change := change{owner: owner, key: key, value: compact(value), setBy: setBy, setAt: now}
			if err := upsert(ctx, queries, change); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
		return nil
	})
}

// change is one setting being written.
type change struct {
	owner *int64
	key   string
	value string
	setBy *int64
	setAt int64
}

func upsert(ctx context.Context, queries *sqlcgen.Queries, change change) error {
	if change.owner == nil {
		return queries.UpsertServerSetting(ctx, sqlcgen.UpsertServerSettingParams{
			Key: change.key, Value: change.value, SetBy: change.setBy, SetAt: change.setAt,
		})
	}
	return queries.UpsertUserSetting(ctx, sqlcgen.UpsertUserSettingParams{
		UserID: change.owner, Key: change.key, Value: change.value, SetBy: change.setBy, SetAt: change.setAt,
	})
}

// check refuses an unknown key, the wrong scope, or a value of the wrong kind.
func check(key string, scope Scope, value json.RawMessage) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrUnknownSetting)
	}
	if definition.Scope != scope {
		return fmt.Errorf("%s: %w", key, ErrWrongScope)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s: %w: %w", key, ErrWrongKind, err)
	}
	if !isKind(decoded, definition.Kind) {
		return fmt.Errorf("%s: %w: want %s", key, ErrWrongKind, definition.Kind)
	}
	return nil
}

func isKind(decoded any, kind Kind) bool {
	switch kind {
	case KindBool:
		_, isBool := decoded.(bool)
		return isBool
	case KindInteger:
		number, isNumber := decoded.(float64)
		return isNumber && number == float64(int64(number))
	case KindString:
		_, isString := decoded.(string)
		return isString
	}
	return false
}

// compact stores JSON without insignificant whitespace, so equal values are equal text.
func compact(value json.RawMessage) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, value); err != nil {
		return string(value) // check has already parsed it; this cannot happen
	}
	return buffer.String()
}
