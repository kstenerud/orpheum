// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RetrySchedule is how a make_artwork_sizes job that failed through no fault of its image waits before
// each retry, before it fails: artworkSecondsBeforeEachRetry, which starts as a minute, five,
// half an hour, two hours, twelve, then daily for a week (D213 §5).
func RetrySchedule(ctx context.Context, config Settings) ([]time.Duration, error) {
	listed, err := config.String(ctx, settings.ArtworkSecondsBeforeEachRetry)
	if err != nil {
		return nil, fmt.Errorf("read artworkSecondsBeforeEachRetry: %w", err)
	}
	var schedule []time.Duration
	for field := range strings.SplitSeq(listed, ",") {
		seconds, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("artworkSecondsBeforeEachRetry %q: %w", listed, err)
		}
		schedule = append(schedule, time.Duration(seconds)*time.Second)
	}
	return schedule, nil
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

// recordMissing records an image its source says is not there, as missing at its source. The
// first time, the record that named it is refreshed once, after the source's CDN stops answering
// from its cached 404, and the image asked for again; the second time it is given up (D213 §1).
// The recheck is derived from this row by the refresh sweep (DueRechecks), not queued here. An
// older image stays shown.
func (service *Service) recordMissing(ctx context.Context, want wantedImage, missErr error) error {
	service.parts.Logger.WarnContext(ctx, "an image is missing at its source", "itemId", want.itemID,
		"source", want.source, "sourceRef", want.ref.SourceRef)
	return service.record(ctx, want, nil, &refusal{why: missErr.Error(), isMissingAtSource: true})
}

// DueRechecks are the items with an image their source said was missing that are due their one
// recheck: refused a CDN's 404 lifetime ago or more, and not looked at again since (D213 §1). The
// refresh sweep queues a refresh of each that asks for its images again (identify's
// QueueRechecks, through the server's wiring).
func (service *Service) DueRechecks(ctx context.Context) ([]int64, error) {
	now := service.parts.Clock.Now()
	var due []int64
	for _, source := range slices.Sorted(maps.Keys(service.parts.Fetchers)) {
		refusedBefore := now.Add(-service.parts.Fetchers[source].RecheckAfter()).UnixMilli()
		items, err := service.parts.Database.Read().ListItemsDueRecheck(ctx, sqlcgen.ListItemsDueRecheckParams{
			Source: source, RefusedBefore: &refusedBefore,
		})
		if err != nil {
			return nil, fmt.Errorf("list the items due a recheck of %s's images: %w", source, err)
		}
		due = append(due, items...)
	}
	return due, nil
}
