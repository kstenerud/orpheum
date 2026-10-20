// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

var (
	// ErrUnknownSetting is returned for a key the registry does not declare.
	ErrUnknownSetting = errors.New("no such setting")
	// ErrInvalidValue is returned for a value of the wrong JSON type, or out of its bounds, and
	// for a setting read as the wrong kind.
	ErrInvalidValue = errors.New("the value has the wrong type or is out of bounds")
	// ErrNotExposed is returned when the settings page is asked to change a setting it does not
	// show; /config changes every setting.
	ErrNotExposed = errors.New("the setting is not on the settings page; change it through /config")
	// ErrNoServerSetting is returned when a setting is named that the registry does not declare:
	// Reset names the setting it resets, where Set's ErrUnknownSetting is a value it was given
	// that cannot be set.
	ErrNoServerSetting = errors.New("no such server setting")
	// ErrUnanswered is returned when a question nobody has answered is read as a value.
	ErrUnanswered = errors.New("the setting is a question nobody has answered yet")
)

// Entry is a setting as a settings page or /config shows it: the value in force, and whether a
// person set it. A setting set to the value it starts with is still modified: it no longer
// follows its starting value when a release changes it (D200).
type Entry struct {
	Definition  Definition
	Value       json.RawMessage
	IsModified  bool
	SetByUserID *int64
	SetAt       int64
}

// Service reads and writes settings.
type Service struct {
	database *store.Database
	clock    clock.Clock
	logger   *slog.Logger

	mutex          sync.Mutex
	reactionsByKey map[Key][]func(context.Context) error // guarded by mutex
	// machineZone is the machine's own time zone, the server's while time_zone is empty (D246 §7).
	machineZone localtime.Zone // guarded by mutex
}

// UseMachineZone is the machine's own time zone, which the server's is while time_zone is empty:
// read once by cmd/orpheum, the one reader of the environment (D204, D246 §7).
func (service *Service) UseMachineZone(zone localtime.Zone) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.machineZone = zone
}

// OnChange runs react after a person sets or resets a server setting, once the change is
// committed: for what is derived from a setting and must follow it at once, not at some later
// event (agent-failures A4). A reaction that fails is logged; the setting stays changed.
func (service *Service) OnChange(key Key, react func(context.Context) error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.reactionsByKey == nil {
		service.reactionsByKey = map[Key][]func(context.Context) error{}
	}
	service.reactionsByKey[key] = append(service.reactionsByKey[key], react)
}

// runReactions runs the reactions to server settings that were written.
func (service *Service) runReactions(ctx context.Context, keys []Key) {
	service.mutex.Lock()
	var reactions []func(context.Context) error
	var reactionKeys []Key
	for _, key := range keys {
		for _, react := range service.reactionsByKey[key] {
			reactions, reactionKeys = append(reactions, react), append(reactionKeys, key)
		}
	}
	service.mutex.Unlock()
	for index, react := range reactions {
		if err := react(ctx); err != nil {
			service.logger.ErrorContext(ctx, "a setting changed, and what follows from it could not follow",
				"key", string(reactionKeys[index]), "err", err)
		}
	}
}

// NewService returns a settings Service.
func NewService(database *store.Database, clockSource clock.Clock, logger *slog.Logger) *Service {
	return &Service{database: database, clock: clockSource, logger: logger.With("component", "settings")}
}

// heal writes the starting value of a setting whose row is missing — removed while the server
// runs, since every start writes all of them — and says so (D202). A missing row is never an
// error and never a value that exists only in memory.
func (service *Service) heal(ctx context.Context, definition Definition) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.InsertSettingIfMissing(ctx, sqlcgen.InsertSettingIfMissingParams{
			Key: string(definition.Key), ValueJSON: compactJSON(definition.StartJSON),
			SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("write the starting value of %s: %w", definition.Key, err)
	}
	service.logger.WarnContext(ctx, "a setting had no row, so its starting value was written",
		"key", definition.Key, "value", string(definition.StartJSON))
	return nil
}

// List returns every setting with its value in force. With exposedOnly it returns only what the
// settings page shows.
func (service *Service) List(ctx context.Context, exposedOnly bool) ([]Entry, error) {
	storedByKey, err := service.storedSettings(ctx)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, definition := range Registry() {
		if exposedOnly && !definition.IsExposed {
			continue
		}
		row, isSeeded := storedByKey[string(definition.Key)]
		if !isSeeded {
			if healErr := service.heal(ctx, definition); healErr != nil {
				return nil, healErr
			}
			row = storedSetting{Value: compactJSON(definition.StartJSON), SetAt: service.clock.Now().UnixMilli()}
		}
		entries = append(entries, Entry{
			Definition: definition, Value: json.RawMessage(row.Value),
			IsModified: row.IsModified, SetByUserID: row.SetByUserID, SetAt: row.SetAt,
		})
	}
	return entries, nil
}

type storedSetting struct {
	Value       string
	SetByUserID *int64
	SetAt       int64
	IsModified  bool
}

func (service *Service) storedSettings(ctx context.Context) (map[string]storedSetting, error) {
	rows, err := service.database.Read().ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	storedByKey := map[string]storedSetting{}
	for _, row := range rows {
		storedByKey[row.Key] = storedSetting{
			Value: row.ValueJSON, SetByUserID: row.SetByUserID, SetAt: row.SetAt, IsModified: row.IsModified == 1,
		}
	}
	return storedByKey, nil
}

// Int reads an integer server setting.
func (service *Service) Int(ctx context.Context, key Key) (int64, error) {
	var value int64
	err := service.read(ctx, key, KindInteger, &value)
	return value, err
}

// Duration reads an integer server setting that is a length of time, in the unit its definition
// names: the one place a setting's count of seconds, hours or days becomes a time.Duration. A
// setting with no unit is refused as ErrInvalidValue.
func (service *Service) Duration(ctx context.Context, key Key) (time.Duration, error) {
	definition, known := Lookup(key)
	if known && definition.Unit == 0 {
		return 0, fmt.Errorf("%s is not a length of time: %w", key, ErrInvalidValue)
	}
	count, err := service.Int(ctx, key)
	if err != nil {
		return 0, err
	}
	return definition.LengthOf(count), nil
}

// String reads a string server setting.
func (service *Service) String(ctx context.Context, key Key) (string, error) {
	var value string
	err := service.read(ctx, key, KindString, &value)
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
	if definition.Kind != kind {
		return fmt.Errorf("%s is a %s setting, read as a %s one: %w", key, definition.Kind, kind, ErrInvalidValue)
	}
	row, err := store.OrNotFound(service.database.Read().GetSetting(ctx, string(key)))
	if errors.Is(err, store.ErrNotFound) {
		if healErr := service.heal(ctx, definition); healErr != nil {
			return healErr
		}
		row.ValueJSON = compactJSON(definition.StartJSON)
	} else if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	if row.ValueJSON == "null" {
		return fmt.Errorf("%s: %w", key, ErrUnanswered)
	}
	if err = json.Unmarshal([]byte(row.ValueJSON), target); err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	return nil
}

// ValueError is a value Set refused, with the key it was given for, so the refusal can say which
// setting was wrong. It wraps ErrUnknownSetting, ErrNotExposed or ErrInvalidValue.
type ValueError struct {
	Key Key
	Err error
}

func (refused *ValueError) Error() string { return refused.Err.Error() }

func (refused *ValueError) Unwrap() error { return refused.Err }

// Set stores values, each checked against the registry, in one transaction: all of them or
// none. The person who changed them is setByUserID, nil for the dev seed. With exposedOnly, a key
// the settings page does not show is refused. A value it refuses is a *ValueError.
func (service *Service) Set(
	ctx context.Context, values map[string]json.RawMessage, setByUserID *int64, exposedOnly bool,
) error {
	for key, value := range values {
		if err := checkValue(Key(key), value, exposedOnly); err != nil {
			return &ValueError{Key: Key(key), Err: err}
		}
	}
	setAt := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for key, value := range values {
			write := settingWrite{
				key: key, value: compactJSON(value), setByUserID: setByUserID, setAt: setAt, isModified: true,
			}
			if err := upsert(ctx, queries, write); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	keys := make([]Key, 0, len(values))
	for key := range values {
		keys = append(keys, Key(key))
	}
	service.runReactions(ctx, keys)
	return nil
}

// Reset writes a setting's starting value back and clears its modified mark, so it follows
// its starting value again when a release changes it: "set to default".
func (service *Service) Reset(ctx context.Context, key Key, setByUserID *int64) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrNoServerSetting)
	}
	setAt := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return upsert(ctx, queries, settingWrite{
			key: string(key), value: compactJSON(definition.StartJSON), setByUserID: setByUserID, setAt: setAt,
		})
	})
	if err != nil {
		return err
	}
	service.runReactions(ctx, []Key{key})
	return nil
}

// settingWrite is one setting being written.
type settingWrite struct {
	key         string
	value       string
	setByUserID *int64
	setAt       int64
	isModified  bool
}

func upsert(ctx context.Context, queries *sqlcgen.Queries, write settingWrite) error {
	var modified int64
	if write.isModified {
		modified = 1
	}
	return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
		Key: write.key, ValueJSON: write.value, SetByUserID: write.setByUserID, SetAt: write.setAt,
		IsModified: modified,
	})
}

// checkValue refuses an unknown key, a hidden key on the settings page, or a value of the wrong
// kind or out of bounds.
func checkValue(key Key, value json.RawMessage, exposedOnly bool) error {
	definition, known := Lookup(key)
	if !known {
		return fmt.Errorf("%q: %w", key, ErrUnknownSetting)
	}
	if exposedOnly && !definition.IsExposed {
		return fmt.Errorf("%s: %w", key, ErrNotExposed)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s: %w: %w", key, ErrInvalidValue, err)
	}
	if !fits(decoded, definition) {
		return fmt.Errorf("%s: %w: want %s", key, ErrInvalidValue, describeKind(definition))
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
		text, isString := decoded.(string)
		return isString && (definition.Pattern == nil || definition.Pattern.MatchString(text)) &&
			(definition.IsValid == nil || definition.IsValid(text))
	}
	return false
}

func describeKind(definition Definition) string {
	if definition.Kind == KindInteger {
		return fmt.Sprintf("an integer from %d to %d", definition.Minimum, definition.Maximum)
	}
	return "a " + string(definition.Kind)
}

// compactJSON stores JSON without insignificant whitespace, so equal values are equal text.
func compactJSON(value json.RawMessage) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, value); err != nil {
		return string(value) // checkValue has already parsed it; this cannot happen
	}
	return buffer.String()
}

// Zone is the server's time zone: the time_zone setting's, or the machine's own while the admin
// has chosen none (D246 §7).
func (service *Service) Zone(ctx context.Context) (localtime.Zone, error) {
	name, err := service.String(ctx, TimeZone)
	if err != nil {
		return localtime.Zone{}, err
	}
	if name == "" {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		return service.machineZone, nil
	}
	zone, err := localtime.Named(name)
	if err != nil {
		return localtime.Zone{}, fmt.Errorf("%s: %w", TimeZone, err)
	}
	return zone, nil
}

// DurationReader reads a length-of-time setting: Service.
type DurationReader interface {
	Duration(ctx context.Context, key Key) (time.Duration, error)
}

// DurationOrStarting is key's length of time, read from reader, or its starting value when it cannot
// be read or there is no reader, said in logger: for a loop that goes on whatever the database does.
func DurationOrStarting(ctx context.Context, reader DurationReader, key Key, logger *slog.Logger) time.Duration {
	if reader == nil {
		return StartingDuration(key)
	}
	length, err := reader.Duration(ctx, key)
	if err != nil {
		length = StartingDuration(key)
		logger.WarnContext(ctx, "could not read a setting; going on at its starting value", "key", string(key),
			"startingValue", length.String(), "err", err)
	}
	return length
}
