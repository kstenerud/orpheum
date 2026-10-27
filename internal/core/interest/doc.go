// SPDX-License-Identifier: AGPL-3.0-or-later

// Package interest orders the server's work by what people play and look at (D324): it keeps, in
// memory, the film or series page each session shows, or the series' season, and at each tick
// raises the work of what is played to a play's priority, of what is looked at below it, the latest
// look highest, and of what anyone has watched, for good, to the shown-interest rung above the
// background. A look that ends releases its work, which the next demotion gives the
// shown-interest rung.
//
// # Invariants
//
//   - A session shows one page at a time; a look not said again within
//     interest_lapses_after_seconds lapses, and the latest begun is first. A series' season's page
//     is a page of its own. Enforced by TestRegister_KeepsEachSessionsPageTheLatestFirstUntilItLapses
//     and TestRegister_ASeasonsPageIsALookOfItsOwn.
//   - What is played is raised at a play's priority, what is looked at below it the latest
//     highest, a film with its live files; a look that ended is released, and so is what left a look
//     that goes on, and what anyone watched is raised for good to the shown-interest rung (D324 §2,
//     §3); what another look still raises is not released. Enforced by
//     TestRaiser_RaisesThePlayedTheLookedAtAndTheWatched, TestRaiser_ALookThatGoesOnReleasesWhatLeftIt
//     and TestRaiser_ALookAtASeasonRaisesItsEpisodes.
//   - Work raised to a person's tier that waits for room on a node, a slot, a device's readers or a
//     scan's quick pass, is brought back, so its claim bumps the lowest work of a lower tier holding
//     the room (D324 §4, D255 §2); work given to a node is told its new priority.
//     Enforced by TestRaiser_BringsBackRaisedWorkWaitingForRoomOnANode, and the bump in
//     internal/core/nodes by TestAssign_WorkOfAHigherTierBumpsTheLowestWhereNothingIsFree.
//   - A look at a series' own page raises the series and its person's Next Up alone, as that
//     episode's own page would (the owner, 2026-10-25), by library.MembersAhead over the ordering its
//     person watches it in (library.ViewingOrdering; D159, D160): the one partway through, the
//     latest watched of several; else the one
//     after the latest finished; else the first; none past the last (D324 §3). Enforced by
//     TestRaiser_ALookAtASeriesRaisesItsNextUpAlone, and the rule in internal/core/library by
//     TestMembersAhead_StartAtThePersonsPlaceInTheOrdering and
//     TestMembersOf_ReadThePersonsPositionsAgainstTheThresholds; the series' page says the same
//     episode by TestBrowse_ASeriesSaysItsPersonsNextEpisode in internal/core/api.
//   - A look at a series' season raises the series and series_episodes_ahead of that season's
//     episodes, from its person's place among them, the same way; else, its last finished or none
//     watched, from its first (D324 §3, the owner, 2026-10-24). Enforced by
//     TestMembersAheadInGroup_StartAtThePersonsPlaceInTheGroup in internal/core/library and
//     TestRaiser_ALookAtASeasonRaisesItsEpisodes; the page saying its season by
//     TestInterest_ASeasonsPageSaysItsSeason in internal/core/api. A season is a group of the
//     ordering its person watches the series in, and an episode that ordering leaves out is never
//     raised (episode-orderings.md §7 Q4). Enforced by
//     TestRaiser_ALookAtASeriesFollowsTheOrderingItsPersonWatchesItIn.
//   - A look raises the work, not the files, of the episodes whose stills its page shows, as a
//     title's page raises its own artwork: a series' own page each season's still
//     (library.SeasonStillMembers), a season's every one of its episodes' (the owner, Fire TV,
//     2026-10-25). Enforced by TestRaiser_ALookAtASeriesRaisesTheWorkOfTheStillsItShows.
//   - A look asks for the read of the unread files of what it shows, once for the look: a series'
//     its Next Up's, a season's every one of its episodes', in their order (D304 §4; the owner,
//     2026-10-25); a look said again asks for none, as a read asked for while its scan lists asks for
//     one more scan after it. Enforced by TestRaiser_ALookAtASeriesRaisesItsNextUpAlone and
//     TestRaiser_ALookAtASeasonAsksForTheReadOfEachOfItsEpisodesOnce.
//   - A list's tiles in view are a session's look of their own, beside its page, the same look as the
//     list scrolls, and raise the work of the items they show, their pictures', and no file's read;
//     tiles scrolled away from are released (D362 §4). Enforced by
//     TestRegister_ASessionsTilesInViewAreALookBesideItsPage and TestRaiser_TilesInViewRaiseTheirItemsWork.
//   - Interest never lowers a job: a play's stays a play's. Enforced in internal/core/jobs by
//     TestInterestJobs_RaiseTheWorkOfInterestAndLetItFallToTheShownInterestRung.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary, and transitively by TestArch_CoreNeverImportsNode and TestArch_CoreNeverExecs.
//
// # Read before changing
//
//   - D324: work follows interest; docs/contributors/design/plans/interest-priority.md.
//   - D234 §3: a job's priority and its "wanted until", which a play keeps and interest uses.
//   - D309: a player's interest in its film's chunks, the playback's own (api's sayInterest).
//
// # Where to change
//
//   - register.go: each session's look, said by PUT and DELETE /interest/{id}, and its tiles in view,
//     by PUT and DELETE /tiles/interest (api's page_interest.go).
//   - raise.go: what a look, a playback and a watching raise, the reads a look asks for, and the
//     loop that raises them.
//   - internal/core/library's next_up.go: a collection's members ahead of its person, its Next Up
//     first (D160).
package interest
