// SPDX-License-Identifier: AGPL-3.0-or-later

package settingstest

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
)

// Seeded writes every setting's starting value into database, lowers argon2id's costs to the
// minimum, and returns the service.
func Seeded(tb testing.TB, database *store.Database) *settings.Service {
	tb.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC))
	service := settings.NewService(database, fakeClock, slog.New(slog.NewTextHandler(tb.Output(), nil)))
	if _, err := service.Seed(tb.Context()); err != nil {
		tb.Fatalf("seed the settings: %v", err)
	}
	Set(tb, service, settings.Argon2MemoryKiB, "8")
	Set(tb, service, settings.Argon2Iterations, "1")
	Set(tb, service, settings.Argon2Parallelism, "1")
	return service
}

// Set changes one server setting to a JSON value through the raw path, as /config does.
func Set(tb testing.TB, service *settings.Service, key settings.Key, value string) {
	tb.Helper()
	encoded := map[string]json.RawMessage{string(key): json.RawMessage(value)}
	if err := service.Set(tb.Context(), encoded, nil, false); err != nil {
		tb.Fatalf("set %s to %s: %v", key, value, err)
	}
}
