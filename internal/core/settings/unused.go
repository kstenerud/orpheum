// SPDX-License-Identifier: AGPL-3.0-or-later

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
	return []RetiredKey{
		{
			Key: "remote_bit_rate_ceiling_kbps", Release: "M4",
			Reason: "nothing knows where a client is; a slow link shows itself by its stalls (D247 §11)",
		},
	}
}

// UnusedSetting is a setting row no setting of this release reads: a key a release retired, or
// one no release has used, which is most likely a typo.
type UnusedSetting struct {
	Key       string
	ValueJSON string
	Retired   *RetiredKey // nil when no release ever used the key
}

func (unused UnusedSetting) String() string {
	return unused.Key
}

// ErrInUse is returned for an attempt to delete a setting this release reads: reset it instead.
var ErrInUse = errors.New("the setting is in use; set it to default instead of deleting it")

// Unused lists the setting rows no setting of this release reads, without changing anything.
func (service *Service) Unused(ctx context.Context) ([]UnusedSetting, error) {
	rows, err := service.database.Read().ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list setting keys: %w", err)
	}
	return unusedIn(rows), nil
}

// DeleteUnused removes a row no setting reads. A setting this release reads is refused: it is set
// to default, never deleted (D202).
func (service *Service) DeleteUnused(ctx context.Context, key string) error {
	if _, known := Lookup(Key(key)); known {
		return fmt.Errorf("%s: %w", key, ErrInUse)
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.DeleteSetting(ctx, key)
	})
}

func unusedIn(rows []sqlcgen.Setting) []UnusedSetting {
	retired := map[string]RetiredKey{}
	for _, retiredKey := range RetiredKeys() {
		retired[retiredKey.Key] = retiredKey
	}
	unused := []UnusedSetting{}
	for _, row := range rows {
		if _, known := Lookup(Key(row.Key)); known {
			continue
		}
		setting := UnusedSetting{Key: row.Key, ValueJSON: row.ValueJSON}
		if retiredKey, isRetired := retired[row.Key]; isRetired {
			setting.Retired = &retiredKey
		}
		unused = append(unused, setting)
	}
	return unused
}
