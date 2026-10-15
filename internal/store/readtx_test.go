// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// settingValue is one setting's stored value, read through queries.
func settingValue(ctx context.Context, t *testing.T, queries *sqlcgen.Queries) string {
	t.Helper()
	row, err := queries.GetSetting(ctx, "probe")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	return row.ValueJSON
}

// setSetting writes the setting's value through the writer.
func setSetting(ctx context.Context, t *testing.T, database *Database, value string) {
	t.Helper()
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: "probe", ValueJSON: value, SetAt: 1})
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestReadTx_SeesOneCommitWhateverCommitsMeanwhile(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	ctx := t.Context()
	setSetting(ctx, t, database, `"before"`)
	var first, second string
	err := database.ReadTx(ctx, func(queries *sqlcgen.Queries) error {
		first = settingValue(ctx, t, queries)
		setSetting(ctx, t, database, `"after"`)
		second = settingValue(ctx, t, queries)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadTx: %v", err)
	}
	if first != `"before"` || second != `"before"` {
		t.Errorf("inside one read transaction the setting read %s, then %s; want the first commit's both times",
			first, second)
	}
	if after := settingValue(ctx, t, database.Read()); after != `"after"` {
		t.Errorf("after the read transaction the setting reads %s; want the later commit's", after)
	}
}

func TestCacheReadTx_SeesOneCommitWhateverCommitsMeanwhile(t *testing.T) {
	t.Parallel()
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	ctx := t.Context()
	keep := func(day int64) {
		t.Helper()
		err := cache.Write(ctx, func(queries *cachegen.Queries) error {
			return queries.MarkArtServed(ctx, cachegen.MarkArtServedParams{
				SourceSha256Hex: "ab", LastServedEpochDay: day,
			})
		})
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	served := func(queries *cachegen.Queries) int64 {
		t.Helper()
		rows, err := queries.ListArtServed(ctx)
		if err != nil || len(rows) != 1 {
			t.Fatalf("ListArtServed: %v, %d rows", err, len(rows))
		}
		return rows[0].LastServedEpochDay
	}
	keep(1)
	var first, second int64
	err := cache.ReadTx(ctx, func(queries *cachegen.Queries) error {
		first = served(queries)
		keep(2)
		second = served(queries)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadTx: %v", err)
	}
	if first != 1 || second != 1 {
		t.Errorf("inside one read transaction the day read %d, then %d; want the first commit's both times",
			first, second)
	}
}
