// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

var (
	// ErrUnknownSetting is returned for a key the registry does not declare.
	ErrUnknownSetting = errors.New("no such setting")
	// ErrWrongScope is returned for a server setting set as a user's, or the reverse.
	ErrWrongScope = errors.New("the setting belongs to the other scope")
	// ErrWrongKind is returned for a value of the wrong JSON type, or out of its bounds.
	ErrWrongKind = errors.New("the value has the wrong type or is out of bounds")
	// ErrNotExposed is returned when the settings page is asked to change a setting it does not
	// show; /config changes every setting.
	ErrNotExposed = errors.New("the setting is not on the settings page; change it through /config")
	// ErrUnanswered is returned when a question nobody has answered is read as a value.
	ErrUnanswered = errors.New("the setting is a question nobody has answered yet")
)

// Entry is a setting as a settings page or /config shows it: the value in force, and whether a
// person set it. A setting set to the value it starts with is still modified: it no longer
// follows its starting value when a release changes it (D200).
type Entry struct {
	Definition Definition
	Value      json.RawMessage
	IsModified bool
	SetBy      *int64
	SetAt      int64
}

// Service reads and writes settings.
type Service struct {
	database *store.Database
	clock    clock.Clock
	logger   *slog.Logger
}

// NewService returns a settings Service.
func NewService(database *store.Database, now clock.Clock, logger *slog.Logger) *Service {
	return &Service{database: database, clock: now, logger: logger.With("component", "settings")}
}

// heal writes the starting value of a setting whose row is missing — removed while the server
// runs, since every start writes all of them — and says so (D202). A missing row is never an
// error and never a value that exists only in memory.
func (service *Service) heal(ctx context.Context, definition Definition) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.InsertServerSettingIfMissing(ctx, sqlcgen.InsertServerSettingIfMissingParams{
			Key: string(definition.Key), Value: compact(definition.Start), SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("write the starting value of %s: %w", definition.Key, err)
	}
	service.logger.WarnContext(ctx, "a setting had no row, so its starting value was written",
		"key", definition.Key, "value", string(definition.Start))
	return nil
}

// List returns every setting of a scope with its value in force: owner is the user for the user
// scope and nil for the server's. With exposedOnly it returns only what the settings page shows.
func (service *Service) List(ctx context.Context, owner *int64, exposedOnly bool) ([]Entry, error) {
	scope, stored, err := service.stored(ctx, owner)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, definition := range Registry() {
		if definition.Scope != scope || (exposedOnly && !definition.Exposed) {
			continue
		}
		row, isSeeded := stored[string(definition.Key)]
		if !isSeeded {
			if healErr := service.heal(ctx, definition); healErr != nil {
				return nil, healErr
			}
			row = storedSetting{Value: compact(definition.Start), SetAt: service.clock.Now().UnixMilli()}
		}
		entries = append(entries, Entry{
			Definition: definition, Value: json.RawMessage(row.Value),
			IsModified: row.IsModified, SetBy: row.SetBy, SetAt: row.SetAt,
		})
	}
	return entries, nil
}

type storedSetting struct {
	Value      string
	SetBy      *int64
	SetAt      int64
	IsModified bool
}

func (service *Service) stored(ctx context.Context, owner *int64) (Scope, map[string]storedSetting, error) {
	stored := map[string]storedSetting{}
	if owner == nil {
		rows, err := service.database.Read().ListServerSettings(ctx)
		if err != nil {
			return ScopeServer, nil, fmt.Errorf("list server settings: %w", err)
		}
		for _, row := range rows {
			stored[row.Key] = storedSetting{
				Value: row.Value, SetBy: row.SetBy, SetAt: row.SetAt, IsModified: row.IsModified == 1,
			}
		}
		return ScopeServer, stored, nil
	}
	rows, err := service.database.Read().ListUserSettings(ctx, owner)
	if err != nil {
		return ScopeUser, nil, fmt.Errorf("list user settings: %w", err)
	}
	for _, row := range rows {
		stored[row.Key] = storedSetting{
			Value: row.Value, SetBy: row.SetBy, SetAt: row.SetAt, IsModified: row.IsModified == 1,
		}
	}
	return ScopeUser, stored, nil
}

// Int reads an integer server setting.
func (service *Service) Int(ctx context.Context, key Key) (int64, error) {
	var value int64
	err := service.read(ctx, key, KindInteger, &value)
	return value, err
}

// Bool reads a boolean server setting.
func (service *Service) Bool(ctx context.Context, key Key) (bool, error) {
	var value bool
	err := service.read(ctx, key, KindBool, &value)
	return value, err
}

func (service *Service) read(ctx context.Context, key Key, kind Kind, target any) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrUnknownSetting)
	}
	if definition.Kind != kind || definition.Scope != ScopeServer {
		return fmt.Errorf("%s is a %s %s setting, read as a %s server one: %w",
			key, definition.Scope, definition.Kind, kind, ErrWrongKind)
	}
	row, err := store.One(service.database.Read().GetServerSetting(ctx, string(key)))
	if errors.Is(err, store.ErrNotFound) {
		if healErr := service.heal(ctx, definition); healErr != nil {
			return healErr
		}
		row.Value = compact(definition.Start)
	} else if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	if row.Value == "null" {
		return fmt.Errorf("%s: %w", key, ErrUnanswered)
	}
	if err = json.Unmarshal([]byte(row.Value), target); err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	return nil
}

// Set stores values, each checked against the registry, in one transaction: all of them or
// none. The owner is the user for user settings and nil for server settings; setBy is who
// changed them, nil for the dev seed. With exposedOnly, a key the settings page does not show is
// refused.
func (service *Service) Set(
	ctx context.Context, owner *int64, values map[string]json.RawMessage, setBy *int64, exposedOnly bool,
) error {
	scope := ScopeServer
	if owner != nil {
		scope = ScopeUser
	}
	for key, value := range values {
		if err := check(Key(key), scope, value, exposedOnly); err != nil {
			return err
		}
	}
	now := service.clock.Now().UnixMilli()
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for key, value := range values {
			change := change{owner: owner, key: key, value: compact(value), setBy: setBy, setAt: now, isModified: true}
			if err := upsert(ctx, queries, change); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
		return nil
	})
}

// Reset writes a server setting's starting value back and clears its modified mark, so it follows
// its starting value again when a release changes it: "set to default".
func (service *Service) Reset(ctx context.Context, key Key, setBy *int64) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrUnknownSetting)
	}
	if definition.Scope != ScopeServer {
		return fmt.Errorf("%s: %w", key, ErrWrongScope)
	}
	now := service.clock.Now().UnixMilli()
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return upsert(ctx, queries, change{
			key: string(key), value: compact(definition.Start), setBy: setBy, setAt: now,
		})
	})
}

// change is one setting being written.
type change struct {
	owner      *int64
	key        string
	value      string
	setBy      *int64
	setAt      int64
	isModified bool
}

func upsert(ctx context.Context, queries *sqlcgen.Queries, change change) error {
	var modified int64
	if change.isModified {
		modified = 1
	}
	if change.owner == nil {
		return queries.UpsertServerSetting(ctx, sqlcgen.UpsertServerSettingParams{
			Key: change.key, Value: change.value, SetBy: change.setBy, SetAt: change.setAt, IsModified: modified,
		})
	}
	return queries.UpsertUserSetting(ctx, sqlcgen.UpsertUserSettingParams{
		UserID: change.owner, Key: change.key, Value: change.value, SetBy: change.setBy, SetAt: change.setAt,
		IsModified: modified,
	})
}

// check refuses an unknown key, the wrong scope, a hidden key on the settings page, or a value
// of the wrong kind or out of bounds.
func check(key Key, scope Scope, value json.RawMessage, exposedOnly bool) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrUnknownSetting)
	}
	if definition.Scope != scope {
		return fmt.Errorf("%s: %w", key, ErrWrongScope)
	}
	if exposedOnly && !definition.Exposed {
		return fmt.Errorf("%s: %w", key, ErrNotExposed)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s: %w: %w", key, ErrWrongKind, err)
	}
	if !fits(decoded, definition) {
		return fmt.Errorf("%s: %w: want %s", key, ErrWrongKind, describeKind(definition))
	}
	return nil
}

// fits reports whether a decoded JSON value is of the setting's kind and within its bounds.
// JSON null is never a value one sets: an answer to a question is a value, and Reset unanswers.
func fits(decoded any, definition Definition) bool {
	switch definition.Kind {
	case KindBool:
		_, isBool := decoded.(bool)
		return isBool
	case KindInteger:
		number, isNumber := decoded.(float64)
		return isNumber && number == float64(int64(number)) &&
			int64(number) >= definition.Minimum && int64(number) <= definition.Maximum
	case KindString:
		_, isString := decoded.(string)
		return isString
	}
	return false
}

func describeKind(definition Definition) string {
	if definition.Kind == KindInteger {
		return fmt.Sprintf("an integer from %d to %d", definition.Minimum, definition.Maximum)
	}
	return "a " + string(definition.Kind)
}

// compact stores JSON without insignificant whitespace, so equal values are equal text.
func compact(value json.RawMessage) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, value); err != nil {
		return string(value) // check has already parsed it; this cannot happen
	}
	return buffer.String()
}
