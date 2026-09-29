// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RetiredKey is a key a release stopped reading, kept so an unused row can say why it is unused.
type RetiredKey struct {
	Key     string
	Release string
	Reason  string
}

// RetiredKeys lists every setting a release stopped reading. A row with one of these keys is left
// in place — an older release on a downgrade still reads it (D181) — until a person deletes it.
func RetiredKeys() []RetiredKey {
	return []RetiredKey{}
}

// UnusedSetting is a setting row no setting of this release reads: a key a release retired, or
// one no release has used, which is most likely a typo.
type UnusedSetting struct {
	Scope     Scope
	UserID    *int64
	Key       string
	ValueJSON string
	Retired   *RetiredKey // nil when no release ever used the key
}

func (unused UnusedSetting) String() string {
	if unused.UserID != nil {
		return fmt.Sprintf("%s (user %d)", unused.Key, *unused.UserID)
	}
	return unused.Key
}

// ErrInUse is returned for an attempt to delete a setting this release reads: reset it instead.
var ErrInUse = errors.New("the setting is in use; set it to default instead of deleting it")

// Unused lists the setting rows no setting of this release reads, without changing anything.
func (service *Service) Unused(ctx context.Context) ([]UnusedSetting, error) {
	rows, err := service.database.Read().ListAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list setting keys: %w", err)
	}
	return unusedIn(rows), nil
}

// DeleteUnused removes a row no setting reads (ownerUserID nil for a server row). A setting this release
// reads is refused: it is set to default, never deleted (D202).
func (service *Service) DeleteUnused(ctx context.Context, key string, ownerUserID *int64) error {
	if _, known := Lookup(Key(key)); known {
		return fmt.Errorf("%s: %w", key, ErrInUse)
	}
	scope := ScopeServer
	if ownerUserID != nil {
		scope = ScopeUser
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		params := sqlcgen.DeleteSettingParams{Scope: string(scope), UserID: ownerUserID, Key: key}
		return queries.DeleteSetting(ctx, params)
	})
}

func unusedIn(rows []sqlcgen.ListAllSettingsRow) []UnusedSetting {
	retired := map[string]RetiredKey{}
	for _, retiredKey := range RetiredKeys() {
		retired[retiredKey.Key] = retiredKey
	}
	unused := []UnusedSetting{}
	for _, row := range rows {
		if _, known := Lookup(Key(row.Key)); known {
			continue
		}
		setting := UnusedSetting{Scope: Scope(row.Scope), UserID: row.UserID, Key: row.Key, ValueJSON: row.ValueJson}
		if retiredKey, isRetired := retired[row.Key]; isRetired {
			setting.Retired = &retiredKey
		}
		unused = append(unused, setting)
	}
	return unused
}
