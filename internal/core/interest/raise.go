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
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
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

// Waiting brings back the work waiting on a destination: jobs.Queue.
type Waiting interface {
	Unpark(ctx context.Context, destination jobs.Destination)
}

// Reads has a title's files its listing stored unread read before what else waits, for a person
// who looks at its page or its series' (D304 §4): playback.Service's ReadWhenViewed.
type Reads interface {
	ReadWhenViewed(ctx context.Context, titleID int64) error
}

// Parts are what the raiser reads and raises.
type Parts struct {
	Register *Register
	Jobs     Jobs
	Nodes    Nodes
	Waiting  Waiting
	Reads    Reads
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
	// lastLooked is what the last pass raised for each page looked at, which a look that ended
	// releases.
	lastLooked map[page]jobs.Interested
	// asked are the titles whose reads each look going on has asked for, so a look said again does
	// not ask again: a read asked for while its scan lists asks for one more scan after it.
	asked map[askedRead]bool
	// raiseFailing and watchedFailing are a lasting failure of the raise and of the watched files'
	// raise, said once a minute while it lasts rather than at every 5 s tick (the audit of
	// 2026-10-25, C12).
	raiseFailing, watchedFailing slogx.Failing
}

// askedRead is a title whose read a look asked for: the look, by its page, its person and when it
// began, and the title.
type askedRead struct {
	shown         page
	userID, since int64
	titleID       int64
}

// page is what a look shows: a film's or series' page, or one season's of a series, or the tiles of
// a session's list, named by its session.
type page struct {
	itemID, season int64
	tilesOf        string
}

// ErrMissingPart is a raiser built without one of its parts.
var ErrMissingPart = errors.New("the raiser is missing a part")

// NewRaiser returns the raiser, or ErrMissingPart for parts that leave one out: a server wired
// without one would fail only once its first raise reached it.
func NewRaiser(parts Parts) (*Raiser, error) {
	for _, part := range []struct {
		name      string
		isMissing bool
	}{
		{"Register", parts.Register == nil},
		{"Jobs", parts.Jobs == nil},
		{"Nodes", parts.Nodes == nil},
		{"Waiting", parts.Waiting == nil},
		{"Reads", parts.Reads == nil},
		{"Playing", parts.Playing == nil},
		{"Database", parts.Database == nil},
		{"Settings", parts.Settings == nil},
		{"Clock", parts.Clock == nil},
		{"Logger", parts.Logger == nil},
	} {
		if part.isMissing {
			return nil, fmt.Errorf("%s: %w", part.name, ErrMissingPart)
		}
	}
	return &Raiser{parts: parts, lastLooked: map[page]jobs.Interested{}, asked: map[askedRead]bool{}}, nil
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
	looked := map[page]jobs.Interested{}
	asked := map[askedRead]bool{}
	var stillLooked jobs.Interested
	for _, look := range raiser.parts.Register.Looks(lapse) {
		shown := page{itemID: look.ItemID, season: look.Season, tilesOf: look.tilesOf}
		if _, isRaised := looked[shown]; isRaised {
			continue // another session's look at it, which began earlier
		}
		interested, toRead, err := raiser.worksOf(ctx, look)
		if err != nil {
			return err
		}
		raiser.askForReads(ctx, look, toRead, asked)
		priority := max(jobs.PriorityLookedAt-int64(len(looked)), jobs.PriorityPersonsRequest+1)
		if err := raiser.raise(ctx, interested, priority, until); err != nil {
			return err
		}
		looked[shown] = interested
		stillLooked.ItemIDs = append(stillLooked.ItemIDs, interested.ItemIDs...)
		stillLooked.FileIDs = append(stillLooked.FileIDs, interested.FileIDs...)
	}
	for shown, interested := range raiser.lastLooked {
		// A look that goes on releases what left it: a series' episode its person has moved past.
		// What another look still raises stays: the series of a season its session moved on from.
		gone := without(interested, stillLooked)
		if len(gone.ItemIDs)+len(gone.FileIDs) == 0 {
			continue
		}
		if err := raiser.parts.Jobs.Release(ctx, gone); err != nil {
			return fmt.Errorf("release item %d's work: %w", shown.itemID, err)
		}
	}
	raiser.lastLooked = looked
	raiser.asked = asked
	return nil
}

// askForReads asks, once for each look, for the read of each of titleIDs' files its listing stored
// unread, in their order, and records them in asked: a look said again asks for none it asked for.
// A read that could not be asked for is not asked again in that look; its page's Play asks again.
func (raiser *Raiser) askForReads(ctx context.Context, look Look, titleIDs []int64, asked map[askedRead]bool) {
	for _, titleID := range titleIDs {
		key := askedRead{
			shown: page{itemID: look.ItemID, season: look.Season, tilesOf: look.tilesOf}, userID: look.UserID,
			since: look.Since.UnixNano(), titleID: titleID,
		}
		asked[key] = true
		if raiser.asked[key] {
			continue
		}
		if err := raiser.parts.Reads.ReadWhenViewed(ctx, titleID); err != nil && ctx.Err() == nil {
			raiser.parts.Logger.WarnContext(ctx, "could not ask for the read of a title looked at", "titleId", titleID,
				"err", err)
		}
	}
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

// raise raises interested's work and tells each node holding a job it raised. Work of a person's
// tier that waits for room on a node, its slot or its device's readers full or a scan's quick pass
// on its disk, is brought back at each pass, so its claim bumps the lowest work of a lower tier
// holding the room (D324 §4, D255 §2): background work parks the room again as often as it is
// brought back, and the person's work, parked there before it was raised, would wait with it.
func (raiser *Raiser) raise(ctx context.Context, interested jobs.Interested, priority int64, until time.Time) error {
	raised, err := raiser.parts.Jobs.Raise(ctx, interested, priority, until)
	if err != nil {
		return fmt.Errorf("raise the work of interest: %w", err)
	}
	var waitingOn []jobs.Destination
	for _, job := range raised {
		switch {
		case job.IsAssigned:
			if job.HasRisen {
				raiser.parts.Nodes.Reprioritize(job.ID, job.Priority)
			}
		case job.Priority >= jobs.PriorityPersonsRequest && isRoomOnANode(job.Destination) &&
			!slices.Contains(waitingOn, job.Destination):
			waitingOn = append(waitingOn, job.Destination)
		}
	}
	for _, destination := range waitingOn {
		raiser.parts.Waiting.Unpark(ctx, destination)
	}
	return nil
}

// isRoomOnANode is whether destination is room on a node, which work in it holds: a slot, a
// device's readers, a disk a scan's quick pass is on, or a disk's last reader kept for faster work.
func isRoomOnANode(destination jobs.Destination) bool {
	parts := jobs.PartsOf(destination)
	return parts.IsSlot || parts.IsQuickPass || parts.IsLastReader
}

// worksOf is what a look at an item raises (D324 §3), and the titles whose unread files it asks to
// have read: a film's or episode's own jobs and its live files', whose page asks for their read
// itself; a series' own, and its Next Up's and its files', whose read it asks for, as that
// episode's page would; a season's, the series' own and the next episodes' of the season, from its
// person's place in it, and it asks for the read of every one of its episodes; a list's tiles, the
// items they show (D362 §4).
func (raiser *Raiser) worksOf(ctx context.Context, look Look) (jobs.Interested, []int64, error) {
	if look.tilesOf != "" {
		// A list's tiles in view raise the work of the items they show, their pictures', and ask for
		// no read: a tile shows no file (D362 §4).
		return jobs.Interested{ItemIDs: slices.Clone(look.Tiles)}, nil, nil
	}
	itemID := look.ItemID
	queries := raiser.parts.Database.Read()
	kind, err := store.OrNotFound(queries.GetItemKind(ctx, itemID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Interested{}, nil, nil // gone since its page said it was shown
	}
	if err != nil {
		return jobs.Interested{}, nil, fmt.Errorf("read item %d's kind: %w", itemID, err)
	}
	interested := jobs.Interested{ItemIDs: []int64{itemID}}
	if kind != "title" {
		return raiser.withMembersAhead(ctx, look, interested)
	}
	files, err := queries.ListLiveFilesOfTitle(ctx, itemID)
	if err != nil {
		return jobs.Interested{}, nil, fmt.Errorf("list title %d's files: %w", itemID, err)
	}
	for _, file := range files {
		interested.FileIDs = append(interested.FileIDs, file.ID)
	}
	return interested, nil, nil
}

// raiseWatchedEvery is how often the watched files' work is raised: new work for them is queued
// at the background's priority, and minutes of it waiting there is nothing against the hours a
// library's sweep takes.
const raiseWatchedEvery = 12

// Run raises at each tick, and what anyone watched at every raiseWatchedEvery-th, until ctx ends.
func (raiser *Raiser) Run(ctx context.Context, tick <-chan time.Time) {
	for ticks := 0; ; ticks++ {
		if ticks%raiseWatchedEvery == 0 {
			raiser.sayWatched(ctx, raiser.RaiseWatched(ctx))
		}
		raiser.sayRaise(ctx, raiser.Raise(ctx))
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

// withMembersAhead is interested with what a look at a series raises, each member and its live
// files, and the titles whose reads it asks for: on the series' own page, its person's Next Up
// alone, as that episode's own page raises it (the owner, 2026-10-25), and its read; on a season's,
// series_episodes_ahead of that season's from their place in it, and the read of all its members.
// The members whose stills the page shows have their own work raised, not their files', as a
// title's page raises its artwork: on the series' page each season's still (SeasonStillMembers), on
// a season's every one of its episodes'.
// The ordering is the one its person watches the series in (library.ViewingOrdering, D159), and a
// season is a group of it: an episode it leaves out is never raised (episode-orderings.md §7 Q4).
func (raiser *Raiser) withMembersAhead(
	ctx context.Context, look Look, interested jobs.Interested,
) (jobs.Interested, []int64, error) {
	queries := raiser.parts.Database.Read()
	ordering, err := library.ViewingOrdering(ctx, queries, look.UserID, look.ItemID)
	if err != nil {
		return jobs.Interested{}, nil, err
	}
	members, err := library.ReadMembers(ctx, queries, raiser.parts.Settings, look.ItemID, look.UserID, ordering)
	if err != nil {
		return jobs.Interested{}, nil, err
	}
	ahead := library.MembersAhead(members, 1)
	toRead := ahead
	// shown are the members whose stills the page shows, whose own work, their artwork's, it raises.
	shown, err := library.SeasonStillMembers(ctx, queries, members)
	if err != nil {
		return jobs.Interested{}, nil, err
	}
	if look.Season != NoSeason {
		count, countErr := raiser.parts.Settings.Int(ctx, settings.SeriesEpisodesAhead)
		if countErr != nil {
			return jobs.Interested{}, nil, fmt.Errorf("read %s: %w", settings.SeriesEpisodesAhead, countErr)
		}
		ahead = library.MembersAheadInGroup(members, look.Season, int(count))
		toRead = library.MembersOfGroup(members, look.Season)
		shown = toRead
	}
	for _, each := range ahead {
		interested.ItemIDs = append(interested.ItemIDs, each.TitleID)
		interested.FileIDs = append(interested.FileIDs, each.FileIDs...)
	}
	for _, each := range shown {
		if !slices.Contains(interested.ItemIDs, each.TitleID) {
			interested.ItemIDs = append(interested.ItemIDs, each.TitleID)
		}
	}
	titleIDs := make([]int64, 0, len(toRead))
	for _, each := range toRead {
		titleIDs = append(titleIDs, each.TitleID)
	}
	return interested, titleIDs, nil
}

// sayRaise logs a tick's failure to raise, while it lasts once a minute with how often it failed
// (slogx.Failing), and the tick that raises again after one.
func (raiser *Raiser) sayRaise(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	now, logger := raiser.parts.Clock.Now(), raiser.parts.Logger
	if err == nil {
		if isEnded, lasted := raiser.raiseFailing.Succeed(now); isEnded {
			logger.InfoContext(ctx, "the work of interest is raised again", lasted...)
		}
		return
	}
	if isSaid, lasting := raiser.raiseFailing.Fail(now); isSaid {
		logger.WarnContext(ctx, "could not raise the work of interest; trying again",
			append([]any{"err", err}, lasting...)...)
	}
}

// sayWatched is sayRaise for the watched files' raise.
func (raiser *Raiser) sayWatched(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	now, logger := raiser.parts.Clock.Now(), raiser.parts.Logger
	if err == nil {
		if isEnded, lasted := raiser.watchedFailing.Succeed(now); isEnded {
			logger.InfoContext(ctx, "the watched files' work is raised again", lasted...)
		}
		return
	}
	if isSaid, lasting := raiser.watchedFailing.Fail(now); isSaid {
		logger.WarnContext(ctx, "could not raise the watched files' work; trying again",
			append([]any{"err", err}, lasting...)...)
	}
}
