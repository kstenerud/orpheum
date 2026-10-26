// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A raise failing at every 5 s tick, cache.db being unwritable, says so once, then once a minute with
// how often it failed, and once as it raises again; the watched files' raise likewise, apart. Each
// was logged at every tick (the audit of 2026-10-25, C12).
func TestRun_SaysALastingRaiseFailureOnceAMinute(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	now := clocktest.NewFake(time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	logged := &testutil.Log{}
	raiser := mustRaiser(t, Parts{
		Register: NewRegister(now), Jobs: &raising{}, Nodes: &waitingRoom{}, Waiting: &waitingRoom{},
		Reads: &readsAsked{}, Playing: func() []int64 { return nil },
		Database: database, Settings: settingstest.Seeded(t, database), Clock: now,
		Logger: slog.New(slog.NewTextHandler(logged, nil)),
	})
	unwritable := errors.New("cache.db cannot be written")
	for range 30 { // 2.5 minutes of ticks
		raiser.sayRaise(t.Context(), unwritable)
		now.Advance(5 * time.Second)
	}
	raiser.sayWatched(t.Context(), unwritable)
	raiser.sayRaise(t.Context(), nil)
	text := logged.String()
	if strings.Count(text, "could not raise the work of interest") != 3 || !strings.Contains(text, "failureCount=25") ||
		strings.Count(text, "raised again") != 1 || strings.Count(text, "watched files' work") != 1 {
		t.Errorf("30 failed raises 5 s apart, a watched one, then a raise logged:\n%s\nwant the raise's failure "+
			"at 0, 60 and 120 s, the last counting 25, then its end; and the watched one's failure once", text)
	}
}
