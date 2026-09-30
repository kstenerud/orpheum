// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RetrySchedule is how a make_artwork_sizes job that failed through no fault of its image waits before
// each retry, before it fails: a minute, five, half an hour, two hours, twelve, then daily for a
// week (D213 §5).
func RetrySchedule() []time.Duration {
	schedule := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}
	for range 7 {
		schedule = append(schedule, 24*time.Hour)
	}
	return schedule
}

// breakAfterFailures is how many failures in a row pause a source's host (D213 §5).
const breakAfterFailures = 5

// breaker pauses a host that keeps failing: after breakAfter failures in a row no image is asked
// of it until a time, and then one is, a probe, each pause the next step of RetrySchedule.
type breaker struct {
	mutex        sync.Mutex
	failures     int
	scheduleStep int
	openUntil    time.Time
}

// fetch downloads an image through its source's breaker. A rate limit and an open breaker wait
// without costing the job an attempt; a 404 is the image's own answer, not the host failing.
func (service *Service) fetch(ctx context.Context, fetcher Fetcher, source, ref string) (Unsanitised, error) {
	gate := service.breakerOf(source)
	now := service.parts.Clock.Now()
	gate.mutex.Lock()
	openUntil := gate.openUntil
	gate.mutex.Unlock()
	if now.Before(openUntil) {
		return Unsanitised{}, &jobs.RetryAtError{
			At: openUntil, Err: fmt.Errorf("%s's image server is failing; it is left alone until then", source),
		}
	}
	image, err := fetcher.Fetch(ctx, ref)
	if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); isLimited {
		return Unsanitised{}, &jobs.RetryAtError{At: now.Add(limited.RetryAfter), Err: err}
	}
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if err == nil || errors.Is(err, ErrNoImage) {
		gate.failures, gate.scheduleStep = 0, 0
		return image, err
	}
	gate.failures++
	if gate.failures >= breakAfterFailures {
		schedule := RetrySchedule()
		gate.openUntil = now.Add(schedule[min(gate.scheduleStep, len(schedule)-1)])
		gate.scheduleStep++
		service.parts.Logger.WarnContext(ctx, "an image server keeps failing; pausing it", "source", source,
			"failures", gate.failures, "until", gate.openUntil)
	}
	return Unsanitised{}, fmt.Errorf("fetch %s %s: %w", source, ref, err)
}

func (service *Service) breakerOf(source string) *breaker {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	gate, found := service.breakers[source]
	if !found {
		gate = &breaker{}
		service.breakers[source] = gate
	}
	return gate
}

// recordMissing records an image its source says is not there. The first time, the record that named
// it is refreshed once, after the source's CDN stops answering from its cached 404, and the
// image asked for again; the second time it is given up (D213 §1). An older image stays shown.
func (service *Service) recordMissing(ctx context.Context, fetcher Fetcher, want wantedImage, missErr error) error {
	misses, err := service.parts.Database.Read().CountEarlierMisses(ctx, sqlcgen.CountEarlierMissesParams{
		ItemID: want.itemID, Kind: want.ref.Kind, Source: want.source, SourceRef: &want.ref.SourceRef,
	})
	if err != nil {
		return fmt.Errorf("count item %d's earlier misses: %w", want.itemID, err)
	}
	if refusedErr := service.recordRefusal(ctx, want, missErr.Error()); refusedErr != nil {
		return refusedErr
	}
	if misses > 0 {
		return nil // rechecked once already: given up
	}
	at := service.parts.Clock.Now().Add(fetcher.RecheckAfter())
	if err := service.parts.Rechecker.Recheck(ctx, want.itemID, at); err != nil {
		return fmt.Errorf("ask for item %d's recheck: %w", want.itemID, err)
	}
	return nil
}
