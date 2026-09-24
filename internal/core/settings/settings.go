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
	// ErrNotSeeded is matched by every NotSeededError: a setting with no row. There is no
	// fallback in code (D200).
	ErrNotSeeded = errors.New("the setting has no value")
	// ErrUnanswered is returned when a question nobody has answered is read as a value.
	ErrUnanswered = errors.New("the setting is a question nobody has answered yet")
)

// NotSeededError is a setting with no row: never seeded, or its row was removed. It says what the
// starting value is, so a person can set just that one, and how to write every missing one.
type NotSeededError struct {
	Key   Key
	Start json.RawMessage
}

func (missing *NotSeededError) Error() string {
	return fmt.Sprintf("setting %s has no value; its starting value is %s. Set just this one with PUT "+
		"/api/v1/config, or write every missing setting with `orpheum seed-settings` or POST /api/v1/config/seed; "+
		"the server also writes them at start", missing.Key, missing.Start)
}

// Is makes errors.Is(err, ErrNotSeeded) match.
func (missing *NotSeededError) Is(target error) bool {
	return target == ErrNotSeeded
}

func notSeeded(definition Definition) error {
	return &NotSeededError{Key: definition.Key, Start: definition.Start}
}

// Entry is a setting as a settings page or /config shows it: the value in force, and whether it
// differs from the value a new server starts with.
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
}

// NewService returns a settings Service.
func NewService(database *store.Database, now clock.Clock) *Service {
	return &Service{database: database, clock: now}
}

// Seed writes the starting value of every server setting that has no row: all of them on a new
// server, on an upgrade the keys the new release introduced, and any row that was removed. A row
// that exists is never changed, so a person's choice survives. It returns the keys it wrote. The
// server seeds at every start; a person can also seed with `orpheum seed-settings` or
// POST /config/seed.
func (service *Service) Seed(ctx context.Context) ([]Key, error) {
	written := []Key{}
	now := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		before, err := queries.ListServerSettings(ctx)
		if err != nil {
			return fmt.Errorf("list server settings: %w", err)
		}
		present := map[string]bool{}
		for _, row := range before {
			present[row.Key] = true
		}
		for _, definition := range Registry() {
			if definition.Scope != ScopeServer || present[string(definition.Key)] {
				continue
			}
			err := queries.InsertServerSettingIfMissing(ctx, sqlcgen.InsertServerSettingIfMissingParams{
				Key: string(definition.Key), Value: compact(definition.Start), SetAt: now,
			})
			if err != nil {
				return fmt.Errorf("seed %s: %w", definition.Key, err)
			}
			written = append(written, definition.Key)
		}
		return nil
	})
	return written, err
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
			return nil, notSeeded(definition)
		}
		entries = append(entries, Entry{
			Definition: definition, Value: json.RawMessage(row.Value),
			IsModified: row.Value != compact(definition.Start), SetBy: row.SetBy, SetAt: row.SetAt,
		})
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
		return notSeeded(definition)
	}
	if err != nil {
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
			change := change{owner: owner, key: key, value: compact(value), setBy: setBy, setAt: now}
			if err := upsert(ctx, queries, change); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
		return nil
	})
}

// Reset writes a server setting's starting value back, as about:config's reset does.
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
