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

// metadata is the fetch side of identification: the pass, the job queue, the matcher that runs
// its fetch jobs, and the chooser a person uses.
type metadata struct {
	pass    *identify.Pass
	queue   *jobs.Queue
	matcher *identify.Matcher
	chooser *identify.Chooser
}

// newMetadata builds the providers, the pass and the queue, and returns the jobs a crash left
// running to the queue.
func newMetadata(ctx context.Context, opened started, tmdbBase string, logger *slog.Logger) (metadata, error) {
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
	pass := identify.NewPass(opened.database, clock.System{}, logger, providers)
	matcher := identify.NewMatcher(opened.database, opened.cache, pass, clock.System{}, logger)
	return metadata{
		pass: pass, queue: queue, matcher: matcher,
		chooser: identify.NewChooser(matcher, pass),
	}, nil
}

// run works the fetch jobs until the returned stop is called.
func (fetching metadata) run(ctx context.Context, logger *slog.Logger) (stop func()) {
	runner := fetching.queue.NewRunner(jobs.KindFetch, fetching.matcher.Handle)
	running, cancel := context.WithCancel(ctx)
	var done sync.WaitGroup
	ticker := time.NewTicker(time.Second)
	done.Go(func() {
		defer ticker.Stop()
		if err := runner.Run(running, ticker.C); err != nil {
			logger.ErrorContext(ctx, "the fetch queue stopped", "err", err)
		}
	})
	return func() { cancel(); done.Wait() }
}

// queueMatches queues what a pass left owing: the first match of each film and series, once
// per item (D170); and a matched series' season listings, whenever a new episode needs one
// (D209 §3). A later match is a person's request.
func queueMatches(ctx context.Context, queue *jobs.Queue, report identify.Report, logger *slog.Logger) {
	queued := 0
	for _, itemID := range report.AwaitingMatch {
		ever, err := queue.EverQueued(ctx, jobs.KindFetch, itemID)
		if err == nil && !ever {
			_, err = queue.Enqueue(ctx, jobs.New{
				Kind: jobs.KindFetch, Subject: itemID, Reason: "the first match for a newly identified title or series",
			})
			queued++
		}
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a match", "itemId", itemID, "err", err)
			return
		}
	}
	for _, seriesID := range report.AwaitingListings {
		added, err := queue.Enqueue(ctx, jobs.New{
			Kind: jobs.KindFetch, Subject: seriesID, Reason: "a season listing a new episode needs",
		})
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a season listing", "seriesId", seriesID, "err", err)
			return
		}
		if added {
			queued++
		}
	}
	if queued > 0 {
		logger.InfoContext(ctx, "queued fetches", "jobs", queued)
	}
}

var _ tmdb.Keys = (*secrets.Store)(nil)
