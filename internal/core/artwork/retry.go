// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"fmt"
	"time"

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

// fetch downloads an image from its source's image server. A server that is down or asks us to
// wait answers, through its provider service, that the job parks on it (D231 §5, §6, which
// replace D213 §5's breaker); a 404 is the image's own answer, not the host failing.
func (service *Service) fetch(ctx context.Context, fetcher Fetcher, ref string) (Unsanitised, error) {
	image, err := fetcher.Fetch(ctx, ref)
	if err != nil {
		return Unsanitised{}, fmt.Errorf("fetch %s: %w", ref, err)
	}
	return image, nil
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
