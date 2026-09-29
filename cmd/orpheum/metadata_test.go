// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestFollowSortLocale_KeysEveryNameAtStartAndOnEveryChange(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)",
		"INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified')",
		"INSERT INTO item_text (item_id, language, is_original, name) VALUES (10, 'und', 1, 'Metropolis')")
	pass := identify.NewPass(database, clock.System{}, slog.New(slog.DiscardHandler), nil, config)
	if err := followSortLocale(t.Context(), pass, config, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	missing := func(locale string) int64 {
		t.Helper()
		count, err := database.Read().CountItemsMissingSortKey(t.Context(), locale)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	if missing("en") != 0 {
		t.Error("at start, a name was left with no sort key in the sort locale")
	}
	swedish := map[string]json.RawMessage{string(settings.SortLocale): json.RawMessage(`"sv"`)}
	err := config.Set(t.Context(), nil, swedish, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if missing("sv") != 0 {
		t.Error("after the locale changed, a name had no sort key in it; the change must not wait for a scan")
	}
}

func TestRunners_PreparesRunAsManyAtOnceAsANodeHasSlots(t *testing.T) {
	t.Parallel()
	for _, runner := range (metadata{}).runners() {
		want := 1
		if runner.kind == jobs.KindPrepare {
			// A second person's copy is not held behind the first's.
			want = nodeproto.EncodeSlotCount + nodeproto.RemuxSlotCount
		}
		if runner.atOnce != want {
			t.Errorf("%s jobs run %d at once, want %d (cut-a §5.3)", runner.kind, runner.atOnce, want)
		}
	}
}
