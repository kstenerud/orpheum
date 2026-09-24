// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/tmdb"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// metadata is the fetch side of identification: the providers, the job queue and its runner.
type metadata struct {
	providers identify.Providers
	queue     *jobs.Queue
	stop      func()
}

// startMetadata builds the providers and the queue, returns the jobs a crash left running to it,
// and runs the fetch jobs until stop.
func startMetadata(ctx context.Context, opened started, tmdbBase string, logger *slog.Logger) (metadata, error) {
	timeout, err := opened.settings.Int(ctx, settings.ProviderTimeoutS)
	if err != nil {
		return metadata{}, err
	}
	httpClient, err := outbound.NewClient(time.Duration(timeout) * time.Second)
	if err != nil {
		return metadata{}, err
	}
	providers := identify.Providers{"tmdb": tmdb.New(httpClient, tmdbBase, opened.secrets)}
	queue := jobs.NewQueue(opened.cache, clock.System{}, opened.settings, logger)
	requeued, err := queue.RequeueRunning(ctx)
	if err != nil {
		return metadata{}, err
	}
	if requeued > 0 {
		logger.InfoContext(ctx, "returned jobs a stop left running to the queue", "jobs", requeued)
	}
	matcher := identify.NewMatcher(opened.database, opened.cache, providers, clock.System{}, logger)
	runner := queue.NewRunner(jobs.KindFetch, matcher.Handle)
	running, cancel := context.WithCancel(ctx)
	var done sync.WaitGroup
	ticker := time.NewTicker(time.Second)
	done.Go(func() {
		defer ticker.Stop()
		if err := runner.Run(running, ticker.C); err != nil {
			logger.ErrorContext(ctx, "the fetch queue stopped", "err", err)
		}
	})
	return metadata{providers: providers, queue: queue, stop: func() { cancel(); done.Wait() }}, nil
}

// queueMatches queues the first fetch of each film a pass left awaiting its match: once per
// title (D170); a later one is a person's request.
func queueMatches(ctx context.Context, queue *jobs.Queue, titles []int64, logger *slog.Logger) {
	queued := 0
	for _, titleID := range titles {
		ever, err := queue.EverQueued(ctx, jobs.KindFetch, titleID)
		if err == nil && !ever {
			_, err = queue.Enqueue(ctx, jobs.New{
				Kind: jobs.KindFetch, Subject: titleID, Reason: "the first match for a newly identified title",
			})
			queued++
		}
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a title's match", "titleId", titleID, "err", err)
			return
		}
	}
	if queued > 0 {
		logger.InfoContext(ctx, "queued titles for matching", "titles", queued)
	}
}

var _ tmdb.Keys = (*secrets.Store)(nil)
