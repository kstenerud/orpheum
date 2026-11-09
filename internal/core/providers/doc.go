// SPDX-License-Identifier: AGPL-3.0-or-later

// Package providers is the outside hosts as services (D231 §5, the owner 2026-09-30): each host
// the server calls, TMDB's API (tmdb-api), TMDB's image server (tmdb-images) and OpenSubtitles,
// is one Service that owns its health, the request rate we keep to, its quota remaining and when
// it resets, and its credentials' state. A caller says how much its request matters, someone
// waiting, a person's request or background work, and asks; the service answers now, at a time,
// parked (why), or quota spent until a time. The clients live beside it: tmdb/ and
// opensubtitles/, each asking its service before every request.
//
// A host's health is its destination in the job queue (internal/core/jobs), so a host a person's
// call finds down parks the queue's work for it, and one a job finds down is parked for the
// person's next call too; within a job the queue records what the job's answer says of the host,
// and outside one the service records it itself.
//
// # Invariants
//
//   - Background work keeps below its share of a host's rate (tmdbBackgroundRateSharePercent,
//     starting at 80% of tmdbRequestsPerSecond's 40, D231 §5), so a person's request always
//     finds room, and every caller keeps to the whole rate.
//     TestAsk_BackgroundKeepsBelowItsShareOfTheRateAndAPersonFindsRoom.
//   - A host's limits are the settings, read at each request, so an admin's change takes effect
//     at the next. TestAsk_KeepsToTheLimitsTheSettingsSayAtEachRequest.
//   - Background work never spends the last downloads held back for people
//     (opensubtitlesDownloadsReservedForPeoplePercent of the day's allowance the host said,
//     starting at 25%, rounded up to whole downloads), and nobody spends past the quota the host
//     said, until its reset.
//     TestAsk_BackgroundNeverSpendsTheLastDownloadsHeldBackForPeople.
//   - A person's call to a parked host always tries, and counts as its probe: a failure backs the
//     next probe off, a success unparks it (D231 §6); background is answered parked, with why and
//     since when. TestCall_APersonsCallToAParkedHostAlwaysTriesAndIsItsProbe.
//   - A host that asks us to wait (a 429) is waited for by every caller, and a wait under our own
//     rate is waited in place. TestCall_AHostThatAsksUsToWaitIsWaitedForByEveryone,
//     TestCall_AWaitUnderOurOwnRateIsWaitedInPlace.
//   - Within a job, a host unreachable (a connection, DNS, a timeout, 502, 503, 504), asking us
//     to wait or refusing or lacking its credentials parks the job on it, which the queue records
//     (D231 §5); a plain 500 or a 404 is the request's own.
//     TestCall_InAJobAFailureThatSaysTheHostIsDownParksTheJobAndTheQueueRecordsIt.
//   - A host parked for its credentials comes back when a person changes them, and no other park
//     does. TestCredentialsChanged_BringsBackAHostParkedForItsKeyAndNoOtherPark.
//   - A job's requests matter by its priority: someone waiting to watch, a person's request, or
//     the background. TestJobImportance_IsSomeoneWaitingToWatchAPersonsRequestOrTheBackground.
//   - Time and waits come from a clock.Clock, so every rate and wait is testable. Enforced by the
//     forbidigo rule tagged [clock].
//
// # Read before changing
//
//   - D231 §5, §6: destinations, what counts as down, parking and probing, and the provider
//     services with their reservations.
//   - D170, D213: provider etiquette, rate limits and back-off.
//   - docs/contributors/design/research/provider-etiquette.md: each host's documented limits.
//
// # Where to change
//
//   - service.go: the service, its answers and what it records of each call. bucket.go: the
//     token bucket a rate is kept with. importance.go: how much a request matters. tmdb/ and
//     opensubtitles/: the clients, each host's limits, and its service's construction.
package providers
