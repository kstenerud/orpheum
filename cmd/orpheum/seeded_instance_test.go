// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// startServingCold is startServing on an instance as default-config leaves it, with no
// persistent.db: the server migrates one from empty and seeds every setting's production starting
// value, as a new installation does.
func startServingCold(t *testing.T, config Config, captured capturedLog, awaited string) runningServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return serveOn(t, listener, config, captured, awaited, nil)
}

// seedPersistentDatabase gives an instance with no persistent.db a copy of storetest's migrated
// template, with every setting's starting value and argon2id's costs lowered as settingstest.Seeded
// lowers them: a serve then neither replays every migration step nor hashes each password at
// 64 MiB and three passes, which cost each serving test seconds (test-weight.md). Seeded's
// lowered costs are rows a person set, so the server's sweep of settings leaves them.
func seedPersistentDatabase(t *testing.T, config Config) {
	t.Helper()
	path := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	switch _, err := os.Stat(path); {
	case err == nil:
		return
	case !errors.Is(err, os.ErrNotExist):
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.Instance.PersistentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	storetest.WriteMigrated(t, path)
	database, err := store.Open(t.Context(), path, storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	settingstest.Seeded(t, database)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}
