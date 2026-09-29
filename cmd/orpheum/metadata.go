// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/tmdb"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/playback/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// metadata is the fetch side of identification: the pass, the job queue, the matcher that runs
// its fetch jobs, the chooser a person uses, the refresher, and the artwork its matches name.
type metadata struct {
	pass      *identify.Pass
	queue     *jobs.Queue
	matcher   *identify.Matcher
	chooser   *identify.Chooser
	refresher *identify.Refresher
	settings  *settings.Service
	artwork   *artwork.Service
	// playback runs the prepare jobs, which share the one queue (cut-a §5.3).
	playback *playback.Service
	// subtitler fetches subtitles on request (D224).
	subtitler *opensubtitles.Client
	// measureDue is signalled after each identification pass, which follows every scan: new files
	// may want measuring (D226 §2).
	measureDue chan struct{}
}

// newMetadata builds the providers, the pass and the queue, and returns the jobs a crash left
// running to the queue.
func newMetadata(ctx context.Context, opened started, outside wiring, logger *slog.Logger) (metadata, error) {
	providerTimeoutSeconds, err := opened.settings.Int(ctx, settings.ProviderTimeoutS)
	if err != nil {
		return metadata{}, err
	}
	httpClient, err := outbound.NewClient(time.Duration(providerTimeoutSeconds)*time.Second, version)
	if err != nil {
		return metadata{}, err
	}
	providers := identify.Providers{"tmdb": tmdb.New(httpClient, outside.tmdbBaseURL, opened.secrets)}
	subtitler, err := opensubtitles.New(httpClient, outside.openSubtitlesBaseURL, opened.secrets, clock.System{})
	if err != nil {
		return metadata{}, err
	}
	queue := jobs.NewQueue(opened.cache, clock.System{}, opened.settings, logger)
	requeued, err := queue.RequeueRunning(ctx)
	if err != nil {
		return metadata{}, err
	}
	if requeued > 0 {
		logger.InfoContext(ctx, "returned jobs a stop left running to the queue", "jobs", requeued)
	}
	pass := identify.NewPass(opened.database, clock.System{}, logger, providers, opened.settings)
	if err := followSortLocale(ctx, pass, opened.settings, logger); err != nil {
		return metadata{}, err
	}
	pass.AfterPass(func(ctx context.Context, report identify.Report) { queueMatches(ctx, queue, report, logger) })
	measureDue := make(chan struct{}, 1)
	pass.AfterPass(func(context.Context, identify.Report) {
		select {
		case measureDue <- struct{}{}:
		default: // a sweep is already due
		}
	})
	matcher := identify.NewMatcher(opened.database, opened.cache, pass, clock.System{}, logger)
	return metadata{
		pass: pass, queue: queue, matcher: matcher, settings: opened.settings, subtitler: subtitler,
		measureDue: measureDue,
		chooser:    identify.NewChooser(matcher, pass),
		refresher:  identify.NewRefresher(opened.database, queue, opened.settings, clock.System{}, logger),
	}, nil
}

// jobRunners are the job queue's runners: each kind, its handler, its retry schedule when it has
// its own, and how many of its jobs run at once.
type jobRunners []struct {
	kind     jobs.Kind
	handler  jobs.Handler
	schedule []time.Duration
	atOnce   int
}

// runners are the kinds of job the server works. Prepares run as many at once as a node has
// slots (cut-a §5.3), so a second person's copy is not held behind the first's.
func (fetching metadata) runners() jobRunners {
	return jobRunners{
		{jobs.KindFetch, fetching.matcher.Handle, nil, 1},
		{jobs.KindArtwork, fetching.artwork.Handle, artwork.RetrySchedule(), 1},
		{jobs.KindPrepare, fetching.playback.HandlePrepare, nil, nodeproto.EncodeSlots + nodeproto.RemuxSlots},
		{jobs.KindExtractSubtitle, fetching.playback.HandleSubtitle, nil, 1},
		{jobs.KindConvertFetchedSubtitle, fetching.playback.HandleFetchedSubtitle, nil, 1},
		{jobs.KindMeasureLoudness, fetching.playback.HandleLoudness, nil, 1},
	}
}

// run works the fetch and artwork jobs, and looks for data due a refresh when it starts and every
// metadata_refresh_check_every_h (D213 §3), until the returned stop is called.
func (fetching metadata) run(ctx context.Context, logger *slog.Logger) (stop func()) {
	running, cancel := context.WithCancel(ctx)
	var done sync.WaitGroup
	for _, queue := range fetching.runners() {
		for range queue.atOnce {
			kind, runner := queue.kind, fetching.queue.NewRunner(queue.kind, queue.handler)
			if queue.schedule != nil {
				runner.RetryOn(queue.schedule)
			}
			done.Go(func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				if err := runner.Run(running, ticker.C); err != nil {
					logger.ErrorContext(ctx, "a job queue stopped", "kind", string(kind), "err", err)
				}
			})
		}
	}
	done.Go(func() { fetching.sweep(running, logger) })
	done.Go(func() { fetching.queueMeasurements(running, logger) })
	done.Go(func() { keepArtwork(running, fetching.artwork, logger) })
	return func() { cancel(); done.Wait() }
}

// sweep queues the rolling refresh: now, then every metadata_refresh_check_every_h, read afresh each time
// so a change takes effect at the next look.
func (fetching metadata) sweep(ctx context.Context, logger *slog.Logger) {
	for {
		queued, err := fetching.refresher.Sweep(ctx)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "could not look for data due a refresh", "err", err)
		}
		if queued > 0 {
			logger.InfoContext(ctx, "queued refreshes of data that is due", "items", queued)
		}
		hours, err := fetching.settings.Int(ctx, settings.MetadataRefreshCheckEveryH)
		if err != nil {
			hours = 24
		}
		wait := time.NewTimer(time.Duration(hours) * time.Hour)
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
	}
}

// measureSweepEvery is how often the server looks for files to measure when no scan has said so.
const measureSweepEvery = time.Hour

// queueMeasurements queues a loudness measurement for each file with none (D226 §2): now, after
// each identification pass, and every measureSweepEvery.
func (fetching metadata) queueMeasurements(ctx context.Context, logger *slog.Logger) {
	for {
		queued, err := fetching.playback.QueueMeasurements(ctx)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "could not look for files to measure", "err", err)
		}
		if queued > 0 {
			logger.InfoContext(ctx, "queued loudness measurements", "files", queued)
		}
		wait := time.NewTimer(measureSweepEvery)
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-fetching.measureDue:
			wait.Stop()
		case <-wait.C:
		}
	}
}

// followSortLocale keeps every sort key in the sort_locale setting's locale: now, for a change
// made while the server was stopped or a release's new starting value, and whenever a person
// changes it (D47; agent-failures A4).
func followSortLocale(ctx context.Context, pass *identify.Pass, config *settings.Service, logger *slog.Logger) error {
	written, err := pass.RefreshSortKeys(ctx)
	if err != nil {
		return err
	}
	if written > 0 {
		logger.InfoContext(ctx, "sorted every title and series in the sort locale", "items", written)
	}
	config.OnChange(settings.SortLocale, func(ctx context.Context) error {
		written, err := pass.RefreshSortKeys(ctx)
		if err == nil {
			logger.InfoContext(ctx, "the sort locale changed; sorted every title and series again", "items", written)
		}
		return err
	})
	return nil
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
