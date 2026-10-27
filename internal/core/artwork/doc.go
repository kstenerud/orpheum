// SPDX-License-Identifier: AGPL-3.0-or-later

// Package artwork is the core's artwork (D212): it fetches the images an item's source names,
// has the node re-encode them into our own WebP, and stores what the node answers under
// content-addressed names in the cache directory, recording each in the item's artwork rows in
// persistent.db. It is the artwork table's one writer (the kinds of image are library's
// vocabulary, which identify shares): identify finds
// the images an item's source names and those beside the media and reports them, and the
// server's wiring hands them to Take (identify and artwork never import each other). It serves
// them: a stored size as it is, JPEG cut from the WebP of its size, and a size not made by
// redirecting to the nearest one there. It is the one core package that decodes an image
// (D212 §5; the depguard rule core-no-image).
//
// # Invariants
//
//   - A node's re-encode is recorded as it arrives, whether or not its job still waits (D238 §2):
//     each size asked for, in WebP, under the SHA-256 of the image it is of. The item's image is
//     its job's to record. TestRecordArtwork_StoresTheSizesAskedForByTheSourceTheCommandNames.
//
//   - This package writes the artwork table and nothing else does; it imports no identify, which
//     reaches it only through the server's wiring. TestArch_EachTableHasOneWritingPackage; the
//     import, review.
//
//   - Of the images a pass or a match found, what an item holds, stored or refused, is not asked
//     for again; a rematch retires the earlier work's images and a pass the images no longer
//     beside the media, never a person's choice (D212, D215 §1, D223 §9).
//     TestTake_QueuesOnlyTheImagesTheItemDoesNotHold, TestTake_QueuesOneJobPerItemThatFetchesWhatItNames,
//     TestTake_ARematchRetiresTheEarlierWorksImagesTheNewOneLacks,
//     TestTake_AnImageNoLongerBesideTheMediaIsRetired.
//
//   - The artwork store takes bytes from one place: the node's answer to an artwork command
//     (D212 §8). A download is an Unsanitised value whose bytes only the node command reads.
//     TestService_StoresOnlyTheNodesAnswer.
//
//   - What the node answers is checked before it is stored: a size that was asked for, in
//     WebP. TestService_StoresOnlyTheNodesAnswer.
//
//   - An image the node refuses, or the source no longer has, is recorded with no hash, so it is
//     not fetched again and an older image stays shown. TestService_RecordsARefusalAndKeepsTheOlderImage.
//
//   - A job with no node connected waits without spending an attempt. TestService_WaitsForANode.
//
//   - A job fetches what its images need from their providers, then gives every image it still
//     wants to one node in one command, by its own id, each image named by a key, as a scan is given
//     a root (the owner, 2026-10-02); the node's one answer resolves it, and a job cancelled while
//     the node holds it has the node told to stop. TestHandle_GivesAJobsImagesToTheNodeInOneCommand,
//     TestService_ACancelledJobTellsTheNodeToStopItByItsID.
//
//   - The cache is held to its ceiling in D212 §6's order — a removed item's images (after their
//     grace, or sooner under pressure), sizes made on request and JPEG, other sizes, bases — least
//     recently served first, down to 90% of it; a live item's poster-wall size and the smallest
//     size of each shape are never evicted, and past the ceiling what they need is reported with
//     the ceiling to set (D212 §7). TestSweep_EvictsInTheOrderOfD212DownToLowWaterAndNeverAWallSize,
//     TestSweep_UnderTheCeilingOnlyARemovedItemsImagesGoAndOnlyAfterTheirGrace,
//     TestSweep_HighWaterWhenWhatIsNeverEvictedExceedsTheCeiling.
//
//   - When an image was served is kept to the day, in memory, and written in a batch, never on
//     the request path. TestFlush_WritesWhenImagesWereServedAndLoadHoldingsReadsItBack.
//
//   - An evicted size is cut again by the node from the image's base, our own WebP, without
//     fetching; only a size larger than the base, or an image with nothing held, is fetched
//     again. TestHandle_AnEvictedSizeIsCutFromTheBaseWithoutFetching.
//
//   - An image beside the media is read and re-encoded by the node that holds it, sent its path
//     and never its bytes, and stored under the hash that node read; a person's choice shows
//     first, then the source the library prefers (D215).
//     TestService_AnImageBesideTheMediaIsReadByItsNodeAndShownOverTheProviders.
//
//   - A missing image is looked into once: its record is refreshed after the CDN stops answering
//     from its cached 404, and a second miss gives it up (D213 §1). The recheck is derived from
//     its refusal, which DueRechecks reads for the refresh sweep, never held as a job (D231 §3).
//     TestService_AMissingImageIsRecheckedOnceAfterTheCDNForgetsThenGivenUp.
//     One first answer is kept with no recheck, the exception to go.md §21's "never missing for
//     good on the first answer": a size made again from a stored image's source, when the source
//     answers 404 or with other bytes, is noted as one that cannot be made (a .none file), keyed
//     by the source's SHA-256. What it notes is that those bytes cannot be had again, which no
//     later answer changes: a provider's new image is a new hash, which its item's next refresh
//     finds, and the image is shown meanwhile at its nearest stored size.
//
//   - An image that could not be stored keeps why and when, the item's own failure (D231 §4); a
//     job that gave up records each image it named that its item does not hold that way (§10).
//     TestRecordFailure_AnArtworkJobThatGaveUpHoldsItsImagesAsRefusedWithWhy.
//
//   - A refused image beside the media is owed again once a scan has seen its file since: a disk
//     that was away brings its images back (G9).
//     TestTake_ARefusedImageBesideTheMediaIsOwedAgainOnceAScanHasSeenItsFile.
//
//   - Where the library shows the images beside the media first, a provider's image of a kind
//     beside an item's media is not owed, after a pass or a match; in a TMDB-only library it is,
//     to replace it (D215 §1).
//     TestTake_InALibraryThatShowsTheMediasOwnImagesAProvidersOfAKindBesideItIsNotOwed.
//
//   - An image server its provider service finds down or asking us to wait parks the job on it,
//     at no cost of an attempt, and holds nothing of the image; the queue probes the host
//     (D231 §5, §6, which replace D213 §5's breaker). A plain 500 is the image's own failure,
//     retried on RetrySchedule.
//     TestService_AnImageServerItsProviderParksPassesTheParkingOnAndRecordsNothing. With no node
//     connected, a download waits for any node's hello. TestService_WaitsForANode.
//
//   - The node is never on the request path: a size not made is answered with the nearest one
//     there, and only a size made on request, whose source may be large enough, is queued; one
//     the source cannot make, or a source that is gone or changed, is noted and never queued
//     again (D212 §3, §5). TestServe_ASizeNotMadeRedirectsToTheNearestAndOnlyAnOnRequestSizeIsQueued,
//     TestServe_NothingIsQueuedForASizeTheSourceCannotMake,
//     TestHandle_MakesAnOnRequestSizeAndNotesOneThatCannotBeMade,
//     TestHandle_ASourceThatChangedMakesNoSizeUnderTheOldHash.
//
//   - The core decodes only its own WebP, under the pixel limit, to cut JPEG, and keeps what it
//     cut. TestServe_AJPEGIsCutFromTheWebPOfItsSizeAndKept (integration tier).
//
//   - A frame still is the last resort (D362 §2, §3): after a pass, one is queued, at the
//     background's priority, for each film with no poster or backdrop, episode with no still and other
//     video with neither, once every other source was tried: not awaiting a match, a fetch or a
//     season's listing, not ambiguous or failed, and no image its sources name of those kinds still to
//     come. TestTake_AFilmGetsAFrameStillOnlyOnceEverySourceWasTriedAndGaveNoPicture,
//     TestTake_AnEpisodeAndAnOtherVideoGetAFrameStillWhenNothingElseShowsThem.
//
//   - A frame still's job gives the node the video by its root and payload hash, its picture stream
//     and a tenth of its length, and keeps the still as the item's still; it takes none where the item
//     has come to show a picture of its kind, and a video with no picture or no frame is held as one
//     with no frame, never asked again until it changes; a failure is asked again once a scan has seen
//     the video since (D362 §1, §5). TestHandle_AFrameStillIsTakenFromTheVideoAndKeptAsTheItemsStill,
//     TestHandle_AVideoWithNoFrameIsHeldSoAndAPictureShownTakesNone,
//     TestTake_AFrameStillIsNotAskedAgainButAFailureIsOnceAScanSeesTheVideo.
//
// # Read before changing
//
//   - D212: the whole design; D99 as D212 §5 narrows it.
//   - D362: a still of a video's frames, the last resort, and when it is planned.
//   - principles §22: clean at the boundary.
//   - docs/contributors/design/plans/artwork.md.
//
// # Where to change
//
//   - images.go: the images a source names (Ref), what a pass found (Take), and every write to the artwork
//     table; source.go: the unsanitised download and the fetcher interface; service.go: the queue's
//     jobs, the node command and its answer, and the store; serve.go: answering a request;
//     retry.go: fetching and parking on the host, the retry schedule, a missing image and its
//     due recheck;
//     make.go: making sizes again; ceiling.go: the cache's ceiling, eviction and pressure.
//   - handoff.go: the make_artwork_sizes job, fetching what each image needs and giving every one
//     still wanted to a node in one command, answered once for all (Assignee).
//   - frames.go: the frame stills: which videos are owed one after a pass, the video and place a job
//     gives the node, and keeping what it answers (D362).
package artwork
