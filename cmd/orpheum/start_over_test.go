// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A database below the baseline is refused by default, and with database.when_unmigratable =
// "start_over", staging's, it is set aside in the backups directory, named by when, and the core
// starts with an empty one, saying so (D249 §4).
func TestOpenAndMigrateOrStartOver_SetsAsideADatabaseItCannotBringForwardOnlyWhenTold(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		t.Fatal(err)
	}
	database, err := openAndMigrate(t.Context(), config, directories, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	steps, err := store.Steps()
	if err != nil {
		t.Fatal(err)
	}
	belowBaseline := "PRAGMA user_version = " + strconv.Itoa(steps[0].Version-1)
	storetest.ExecRaw(t, directories.PersistentDatabasePath(), belowBaseline)
	now := clocktest.NewFake(time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC))
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))

	config.Database.WhenUnmigratable = whenRefuse
	_, err = openAndMigrateOrStartOver(t.Context(), config, directories, now, nil, logger)
	if !errors.Is(err, store.ErrBeforeBaseline) {
		t.Fatalf("refusing, a database below the baseline opened with %v; want ErrBeforeBaseline", err)
	}

	config.Database.WhenUnmigratable = whenStartOver
	database, err = openAndMigrateOrStartOver(t.Context(), config, directories, now, nil, logger)
	if err != nil {
		t.Fatalf("starting over: %v", err)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	state, err := database.State(t.Context())
	if err != nil || state.Version != steps[len(steps)-1].Version {
		t.Errorf("the new database is at schema %d (%v); want %d", state.Version, err, steps[len(steps)-1].Version)
	}
	aside := filepath.Join(directories.BackupDirectory(), "persistent-set-aside-20261006T123000Z.db")
	if _, statErr := os.Stat(aside); statErr != nil {
		t.Errorf("the old database is not at %s: %v", aside, statErr)
	}
	if !strings.Contains(logged.String(), "level=WARN") || !strings.Contains(logged.String(), aside) {
		t.Errorf("the log does not warn where the old database went:\n%s", logged.String())
	}
}
