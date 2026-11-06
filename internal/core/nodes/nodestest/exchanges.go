// SPDX-License-Identifier: AGPL-3.0-or-later

package nodestest

import (
	"crypto/rand"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// NewExchanges are the nodes' exchanges a test drives, built from parts as the server builds them
// (nodes.NewExchanges), with what a test leaves unset filled: attempt tokens from crypto/rand, a
// discarded log, a minute's silence, a cache of the test's own and seeded settings. The clock is the
// test's to give.
func NewExchanges(tb testing.TB, parts nodes.ExchangeParts) *nodes.Exchanges {
	tb.Helper()
	if parts.Clock == nil {
		tb.Fatal("nodestest.NewExchanges: the test gives the exchanges' clock")
	}
	if parts.Random == nil {
		parts.Random = rand.Reader
	}
	if parts.Logger == nil {
		parts.Logger = slog.New(slog.DiscardHandler)
	}
	if parts.MaxSilence == 0 {
		parts.MaxSilence = time.Minute
	}
	if parts.Cache == nil {
		parts.Cache = storetest.OpenCache(tb)
	}
	if parts.Settings == nil {
		parts.Settings = settingstest.Seeded(tb, storetest.OpenMigrated(tb))
	}
	return nodes.NewExchanges(parts)
}
