// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Jobs raises and releases the work interest is in: jobs.InterestJobs.
type Jobs interface {
	Raise(ctx context.Context, interested jobs.Interested, priority int64, until time.Time) ([]jobs.Raised, error)
	Release(ctx context.Context, interested jobs.Interested) error
}

// Nodes tells the node holding a raised job its new priority: nodes.Exchanges.
type Nodes interface {
	Reprioritize(jobID, priority int64)
}

// Parts are what the raiser reads and raises.
type Parts struct {
	Register *Register
	Jobs     Jobs
	Nodes    Nodes
	// Playing are the files played now: the playback service's NowPlaying.
	Playing  func() []int64
	Database *store.Database
	Settings Settings
	Clock    clock.Clock
	Logger   *slog.Logger
}

// Settings are the settings the raiser reads: settings.Service.
type Settings interface {
	settings.DurationReader
	library.IntSettings
}

// Raiser orders the work by interest (D324 §2): what is played at a play's priority, what is looked
// at below it, the latest look highest, and what anyone has watched a rung above the background.
type Raiser struct {
	parts Parts
	// lastLooked is what the last pass raised for each look, which a look that ended releases.
	lastLooked map[int64]jobs.Interested
}

// NewRaiser returns the raiser.
func NewRaiser(parts Parts) *Raiser {
	return &Raiser{parts: parts, lastLooked: map[int64]jobs.Interested{}}
}

// Raise raises the work of what is played and looked at now, kept wanted for interest's lapse, and
// releases the work of what was looked at and is no longer.
func (raiser *Raiser) Raise(ctx context.Context) error {
	lapse := settings.DurationOrStarting(ctx, raiser.parts.Settings, settings.InterestLapsesAfterSeconds,
		raiser.parts.Logger)
	until := raiser.parts.Clock.Now().Add(lapse)
	if err := raiser.raise(ctx, jobs.Interested{FileIDs: raiser.parts.Playing()}, jobs.PriorityPlayNow,
		until); err != nil {
		return err
	}
	looked := map[int64]jobs.Interested{}
	for _, look := range raiser.parts.Register.Looks(lapse) {
		if _, isRaised := looked[look.ItemID]; isRaised {
			continue // another session's look at it, which began earlier
		}
		interested, err := raiser.worksOf(ctx, look)
		if err != nil {
			return err
		}
		priority := max(jobs.PriorityLookedAt-int64(len(looked)), jobs.PriorityPersonsRequest+1)
		if err := raiser.raise(ctx, interested, priority, until); err != nil {
			return err
		}
		looked[look.ItemID] = interested
	}
	for itemID, interested := range raiser.lastLooked {
		// A look that goes on releases what left it: a series' episode its person has moved past.
		gone := without(interested, looked[itemID])
		if len(gone.ItemIDs)+len(gone.FileIDs) == 0 {
			continue
		}
		if err := raiser.parts.Jobs.Release(ctx, gone); err != nil {
			return fmt.Errorf("release item %d's work: %w", itemID, err)
		}
	}
	raiser.lastLooked = looked
	return nil
}

// without is what interested names that kept does not.
func without(interested, kept jobs.Interested) jobs.Interested {
	gone := func(ids, keptIDs []int64) []int64 {
		var left []int64
		for _, id := range ids {
			if !slices.Contains(keptIDs, id) {
				left = append(left, id)
			}
		}
		return left
	}
	return jobs.Interested{
		ItemIDs: gone(interested.ItemIDs, kept.ItemIDs), FileIDs: gone(interested.FileIDs, kept.FileIDs),
	}
}

// RaiseWatched raises, for good, the work of every live file anyone has watched, and its film's or
// episode's, to the rung above the background (the owner, 2026-10-21).
func (raiser *Raiser) RaiseWatched(ctx context.Context) error {
	rows, err := raiser.parts.Database.Read().ListWatchedLiveFiles(ctx)
	if err != nil {
		return fmt.Errorf("list the files anyone watched: %w", err)
	}
	var interested jobs.Interested
	for _, row := range rows {
		interested.FileIDs = append(interested.FileIDs, row.FileID)
		interested.ItemIDs = append(interested.ItemIDs, row.TitleID)
	}
	return raiser.raise(ctx, interested, jobs.PriorityShownInterest, time.Time{})
}

// raise raises interested's work and tells each node holding a raised job.
func (raiser *Raiser) raise(ctx context.Context, interested jobs.Interested, priority int64, until time.Time) error {
	raised, err := raiser.parts.Jobs.Raise(ctx, interested, priority, until)
	if err != nil {
		return fmt.Errorf("raise the work of interest: %w", err)
	}
	for _, job := range raised {
		if job.IsAssigned {
			raiser.parts.Nodes.Reprioritize(job.ID, job.Priority)
		}
	}
	return nil
}

// worksOf is what a look at an item raises (D324 §3): a film's or episode's own jobs and its live
// files'; a series' own, and its next episodes' and their files' from its person's place in it.
func (raiser *Raiser) worksOf(ctx context.Context, look Look) (jobs.Interested, error) {
	itemID := look.ItemID
	queries := raiser.parts.Database.Read()
	kind, err := store.OrNotFound(queries.GetItemKind(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Interested{}, nil // gone since its page said it was shown
	}
	if err != nil {
		return jobs.Interested{}, fmt.Errorf("read item %d's kind: %w", itemID, err)
	}
	interested := jobs.Interested{ItemIDs: []int64{itemID}}
	if kind != "title" {
		return raiser.withEpisodesAhead(ctx, look, interested)
	}
	files, err := queries.ListLiveFilesOfTitle(ctx, itemID)
	if err != nil {
		return jobs.Interested{}, fmt.Errorf("list title %d's files: %w", itemID, err)
	}
	for _, file := range files {
		interested.FileIDs = append(interested.FileIDs, file.ID)
	}
	return interested, nil
}

// raiseWatchedEvery is how often the watched files' work is raised: new work for them is queued
// at the background's priority, and minutes of it waiting there is nothing against the hours a
// library's sweep takes.
const raiseWatchedEvery = 12

// Run raises at each tick, and what anyone watched at every raiseWatchedEvery-th, until ctx ends.
func (raiser *Raiser) Run(ctx context.Context, tick <-chan time.Time) {
	for ticks := 0; ; ticks++ {
		if ticks%raiseWatchedEvery == 0 {
			if err := raiser.RaiseWatched(ctx); err != nil && ctx.Err() == nil {
				raiser.parts.Logger.WarnContext(ctx, "could not raise the watched files' work; trying again",
					"err", err)
			}
		}
		if err := raiser.Raise(ctx); err != nil && ctx.Err() == nil {
			raiser.parts.Logger.WarnContext(ctx, "could not raise the work of interest; trying again", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

// withEpisodesAhead is interested with the series' episodes ahead of its person (episodesAhead):
// each episode and its live files.
func (raiser *Raiser) withEpisodesAhead(
	ctx context.Context, look Look, interested jobs.Interested,
) (jobs.Interested, error) {
	thresholds, err := library.ReadResumeThresholds(ctx, raiser.parts.Settings)
	if err != nil {
		return jobs.Interested{}, err
	}
	count, err := raiser.parts.Settings.Int(ctx, settings.SeriesEpisodesAhead)
	if err != nil {
		return jobs.Interested{}, fmt.Errorf("read %s: %w", settings.SeriesEpisodesAhead, err)
	}
	rows, err := raiser.parts.Database.Read().ListSeriesEpisodeFilesWithPositions(ctx,
		sqlcgen.ListSeriesEpisodeFilesWithPositionsParams{CollectionID: look.ItemID, UserID: look.UserID})
	if err != nil {
		return jobs.Interested{}, fmt.Errorf("list series %d's episodes: %w", look.ItemID, err)
	}
	for _, ahead := range episodesAhead(episodesOf(rows, thresholds), int(count)) {
		interested.ItemIDs = append(interested.ItemIDs, ahead.titleID)
		interested.FileIDs = append(interested.FileIDs, ahead.fileIDs...)
	}
	return interested, nil
}
