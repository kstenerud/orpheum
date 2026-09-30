// SPDX-License-Identifier: GPL-3.0-or-later

// Package artwork is the core's artwork (D212): it fetches the images an item's source names,
// has the node re-encode them into our own WebP, and stores what the node answers under
// content-addressed names in the cache directory, recording each in the item's artwork rows in
// library.db through identify.RecordImage, since identify writes that table. It serves them: a
// stored size as it is, JPEG cut from the WebP of its size, and a size not made by redirecting to
// the nearest one there. It is the one core package that decodes an image
// (D212 §5; the depguard rule core-no-image).
//
// # Invariants
//
//   - The artwork store takes bytes from one place: the node's answer to an artwork command
//     (D212 §8). A download is an Unsanitised value whose bytes only the node command reads.
//     TestService_StoresOnlyTheNodesAnswer.
//   - What the node answers is checked before it is stored: a size that was asked for, in
//     WebP. TestService_StoresOnlyTheNodesAnswer.
//   - An image the node refuses, or the source no longer has, is recorded with no hash, so it is
//     not fetched again and an older image stays shown. TestService_RecordsARefusalAndKeepsTheOlderImage.
//   - A job with no node connected waits without spending an attempt. TestService_WaitsForANode.
//   - A job waits for the node's answer by its own id, and one cancelled while it waits tells the
//     node to stop it. TestService_ACancelledJobTellsTheNodeToStopItByItsID.
//   - The cache is held to its ceiling in D212 §6's order — a removed item's images (after their
//     grace, or sooner under pressure), sizes made on request and JPEG, other sizes, bases — least
//     recently served first, down to 90% of it; a live item's poster-wall size and the smallest
//     size of each shape are never evicted, and past the ceiling what they need is reported with
//     the ceiling to set (D212 §7). TestSweep_EvictsInTheOrderOfD212DownToLowWaterAndNeverAWallSize,
//     TestSweep_UnderTheCeilingOnlyARemovedItemsImagesGoAndOnlyAfterTheirGrace,
//     TestSweep_HighWaterWhenWhatIsNeverEvictedExceedsTheCeiling.
//   - When an image was served is kept to the day, in memory, and written in a batch, never on
//     the request path. TestFlush_WritesWhenImagesWereServedAndLoadHoldingsReadsItBack.
//   - An evicted size is cut again by the node from the image's base, our own WebP, without
//     fetching; only a size larger than the base, or an image with nothing held, is fetched
//     again. TestHandle_AnEvictedSizeIsCutFromTheBaseWithoutFetching.
//   - An image beside the media is read and re-encoded by the node that holds it, sent its path
//     and never its bytes, and stored under the hash that node read; a person's choice shows
//     first, then the source the library prefers (D215).
//     TestService_AnImageBesideTheMediaIsReadByItsNodeAndShownOverTheProviders.
//   - A missing image is looked into once: its record is refreshed after the CDN stops answering
//     from its cached 404, and a second miss gives it up (D213 §1).
//     TestService_AMissingImageIsRecheckedOnceAfterTheCDNForgetsThenGivenUp.
//   - A host that fails five times in a row is left alone, probed once per step of
//     RetrySchedule; a rate limit waits without costing an attempt (D213 §5).
//     TestService_AFailingHostIsPausedAfterFiveFailuresAndProbedAfterEachStep,
//     TestService_ARateLimitWaitsWithoutCostingAnAttempt.
//   - The node is never on the request path: a size not made is answered with the nearest one
//     there, and only a size made on request, whose source may be large enough, is queued; one
//     the source cannot make, or a source that is gone or changed, is noted and never queued
//     again (D212 §3, §5). TestServe_ASizeNotMadeRedirectsToTheNearestAndOnlyAnOnRequestSizeIsQueued,
//     TestServe_NothingIsQueuedForASizeTheSourceCannotMake,
//     TestHandle_MakesAnOnRequestSizeAndNotesOneThatCannotBeMade,
//     TestHandle_ASourceThatChangedMakesNoSizeUnderTheOldHash.
//   - The core decodes only its own WebP, under the pixel limit, to cut JPEG, and keeps what it
//     cut. TestServe_AJPEGIsCutFromTheWebPOfItsSizeAndKept (integration tier).
//
// # Read before changing
//
//   - D212: the whole design; D99 as D212 §5 narrows it.
//   - principles §22: clean at the boundary.
//   - docs/contributors/design/plans/artwork.md.
//
// # Where to change
//
//   - source.go: the unsanitised download and the fetcher interface; service.go: the queue's
//     jobs, the node command and its answer, and the store; serve.go: answering a request;
//     retry.go: the host's breaker, the retry schedule and the recheck of a missing image;
//     make.go: making sizes again; ceiling.go: the cache's ceiling, eviction and pressure.
package artwork
