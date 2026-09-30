// SPDX-License-Identifier: GPL-3.0-or-later

package providerstest

import (
	"log/slog"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
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
