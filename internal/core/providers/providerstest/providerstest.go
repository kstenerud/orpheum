// SPDX-License-Identifier: AGPL-3.0-or-later

package providerstest

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// Health is the job queue's parked destinations, over jobs held in memory: a provider service's
// providers.Health, with the queue's own parking, probe back-off and unparking.
type Health = jobqueue.Queue[int64]

// NewHealth is a queue in memory whose destinations a test's provider services keep their hosts'
// health in, on clockSource, with no jitter.
func NewHealth(tb testing.TB, clockSource clock.Clock) *Health {
	tb.Helper()
	return jobqueue.New(&jobqueuetest.Memory[int64]{}, clockSource, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), Logger: slog.New(slog.DiscardHandler),
	})
}

// Settings are the settings a provider service reads its limits from: each at its starting value,
// unless a test sets it here.
type Settings map[settings.Key]int64

// Int is the setting's value: the test's, or its starting value.
func (fixed Settings) Int(_ context.Context, key settings.Key) (int64, error) {
	if value, isSet := fixed[key]; isSet {
		return value, nil
	}
	definition, known := settings.Lookup(key)
	if !known {
		return 0, fmt.Errorf("%q: %w", key, settings.ErrUnknownSetting)
	}
	var start int64
	if err := strictjson.Unmarshal(definition.StartJSON, &start); err != nil {
		return 0, fmt.Errorf("%s's starting value: %w", key, err)
	}
	return start, nil
}
