// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	coreproviders "github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// metadataParts are what the metadata jobs are made over: the databases, the settings, the secrets
// TMDB's key is read from, the clock and randomness the queue runs on, the one outbound client, where
// TMDB answers, and the log.
type metadataParts struct {
	database    *store.Database
	cache       *store.Cache
	settings    *settings.Service
	secrets     *secrets.Store
	clock       clock.Clock
	random      io.Reader
	httpClient  *http.Client
	tmdbBaseURL string
	logger      *slog.Logger
}

// metadataJobs are the job queue, the outside hosts' provider services, and the identification
// pass, the matcher that runs its fetch_metadata jobs and the refresher, over TMDB.
type metadataJobs struct {
	queue     *jobs.Queue
	hosts     hostProviders
	pass      *identify.Pass
	matcher   *identify.Matcher
	refresher *identify.Refresher
}

// newMetadataJobs makes the metadata jobs as the server runs them: serve's wiring, and the
// scenarios' (scenario_test.go), so the two cannot drift.
func newMetadataJobs(parts metadataParts) metadataJobs {
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: parts.cache, Clock: parts.clock, Settings: parts.settings, Random: parts.random, Logger: parts.logger,
		ArgsMerges: jobArgsMerges(),
	})
	hosts := newHostProviders(queue, parts.secrets, parts.clock, parts.settings)
	providers := identify.Providers{"tmdb": tmdb.New(parts.httpClient, parts.tmdbBaseURL, parts.secrets, hosts.tmdbAPI)}
	pass := identify.NewPass(parts.database, parts.clock, parts.logger, providers, parts.settings)
	return metadataJobs{
		queue: queue, hosts: hosts, pass: pass,
		matcher:   identify.NewMatcher(parts.database, parts.cache, pass, parts.clock, parts.logger),
		refresher: identify.NewRefresher(parts.database, queue, parts.settings, parts.clock, parts.logger),
	}
}

// hostProviders are the outside hosts' provider services (D231 §5).
type hostProviders struct {
	tmdbAPI, tmdbImages, openSubtitles *coreproviders.Service
}

// all are the services, in the order the System page lists them.
func (hosts hostProviders) all() []*coreproviders.Service {
	return []*coreproviders.Service{hosts.tmdbAPI, hosts.tmdbImages, hosts.openSubtitles}
}

// newHostProviders builds each outside host's provider service, its health kept in the job queue,
// and has a person's change of a host's key or login try it again.
func newHostProviders(
	queue *jobs.Queue, secretStore *secrets.Store, clockSource clock.Clock, settingsReader coreproviders.Settings,
) hostProviders {
	hosts := hostProviders{
		tmdbAPI:       tmdb.APIService(queue, clockSource, settingsReader),
		tmdbImages:    tmdb.ImagesService(queue, clockSource, settingsReader),
		openSubtitles: opensubtitles.Service(queue, clockSource, settingsReader),
	}
	secretStore.OnChange(func(ctx context.Context, name secrets.Name) {
		switch name {
		case secrets.TMDBAPIKey:
			hosts.tmdbAPI.CredentialsChanged(ctx)
		case secrets.OpenSubtitlesUsername, secrets.OpenSubtitlesPassword:
			hosts.openSubtitles.CredentialsChanged(ctx)
		case secrets.TURNSecret, secrets.DNSProviderToken: // no outside host we call uses them yet
		}
	})
	return hosts
}

// jobArgsMerges are the kinds whose repeat requests merge their arguments their own way (D231 §8):
// a fetch's, a backup's and a read-through's, whose packages say how. The others keep the queued
// job's.
func jobArgsMerges() map[jobs.Kind]jobs.ArgsMerge {
	return map[jobs.Kind]jobs.ArgsMerge{
		jobs.KindFetchMetadata: identify.FetchArgsMerge(), jobs.KindBackup: backup.JobArgsMerge(),
		jobs.KindReadThrough: readthrough.ArgsMerge(),
	}
}
