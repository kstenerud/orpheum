// SPDX-License-Identifier: AGPL-3.0-or-later

// Package interest orders the server's work by what people play and look at (D324): it keeps, in
// memory, the film or series page each session shows, and at each tick raises the work of what is
// played to a play's priority, of what is looked at below it, the latest look highest, and of what
// anyone has watched, for good, to the shown-interest rung above the background. A look that ends
// releases its work, which the next demotion gives the shown-interest rung.
//
// # Invariants
//
//   - A session shows one page at a time; a look not said again within
//     interest_lapses_after_seconds lapses, and the latest begun is first.
//     Enforced by TestRegister_KeepsEachSessionsPageTheLatestFirstUntilItLapses.
//   - What is played is raised at a play's priority, what is looked at below it the latest
//     highest, a film with its live files; a look that ended is released, and so is what left a look
//     that goes on, and what anyone watched is raised for good to the shown-interest rung (D324 §2,
//     §3). Enforced by TestRaiser_RaisesThePlayedTheLookedAtAndTheWatched and
//     TestRaiser_ALookThatGoesOnReleasesWhatLeftIt.
//   - A look at a series raises series_episodes_ahead episodes from its person's place: the one
//     partway through, the latest watched of several; else the one after the latest finished; else
//     the first (D324 §3). Enforced by TestEpisodesAhead_StartAtThePersonsPlaceInTheSeries and
//     TestEpisodesOf_ReadThePersonsPositionsAgainstTheThresholds.
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
//   - register.go: each session's look, said by PUT and DELETE /interest/{id} (api's page_interest.go).
//   - raise.go: what a look, a playback and a watching raise, and the loop that raises them.
//   - series.go: a series' episodes ahead of its person.
package interest
