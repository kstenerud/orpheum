// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's status part: the status tray at the banner's end (status), the page
 * of what needs the admin (needsAttention), D280, its failures of unknown cause (unknownFailures,
 * D509, D511), and the job view (jobs), kept apart so the catalogue fits in one read (en.ts).
 */
export const enStatus = {
  status: {
    tray: "What is on",
    together: "Watch Together",
    togetherOn: "Watch Together is on: its controls",
    togetherElsewhere: "Watch Together is on, on another of your devices",
    cast: "Cast to a device",
    castingReady: "Casting to {{name}}, ready",
    castingFilm: "Casting {{film}} to {{name}}",
    castFrom: "Cast from {{name}}",
    castFromAnother: "Cast from another of your devices",
    onAirPlay: "Playing on AirPlay",
    close: "Close",
    needsAttention: "Something needs your attention",
  },
  needsAttention: {
    heading: "Needs attention",
    nothing: "Nothing needs your attention.",
    seriesReadWrongly:
      ": {{count}} of its episodes have no number, though no specials or extras folder holds them. Their file names may have been read wrongly; renaming them as S01E04 places them.",
    filmWithoutSound:
      ": its video has no sound track, so it plays in silence. The file may be a video-only download whose sound was never merged in.",
    moreSeriesReadWrongly: "And {{count}} more series whose file names may have been read wrongly.",
    moreFilmsWithoutSound: "And {{count}} more films with no sound track.",
  },
  unknownFailures: {
    heading: "Failures of unknown cause",
    about:
      "Work that failed in a way the server doesn't know how to handle yet. What it was for stays unavailable until the server next starts, when it is tried once more.",
    counts:
      "{{lastHour}} in the last hour, {{sinceStart}} since the server started, the last {{at}}.",
    raised: "Its alert is raised.",
    times: "Failed {{count}} times, on {{version}}, the first {{first}} and the last {{last}}.",
    earlierStart: "From an earlier start of the server.",
    error: "Error",
    output: "Output",
    noOutput: "No output.",
    copyReport: "Copy report",
    copied: "Copied: every failure, in full.",
    copyRefused: "This browser wouldn't copy it: select the report below instead.",
  },
  jobs: {
    heading: "Jobs",
    none: "No work has been done yet.",
    target: "{{kind}} #{{target}}",
    // A season's job names the season, a group of the series' aired ordering, by its id: the job
    // view is given no series' name nor season number for it, as for no other target.
    seasonTarget: "{{kind}}: season group #{{season}}",
    kind: {
      fetchMetadata: "Looking up",
      makeArtworkSizes: "Making artwork sizes",
      extractSubtitle: "Extracting subtitles",
      readThrough: "Reading a video through",
      makeChunks: "Making a film's chunks",
      convertFetchedSubtitle: "Converting a downloaded subtitle",
      scan: "Listing a folder",
      examine: "Reading new files",
      backup: "Backing up",
      renewCertificate: "Renewing the server's certificate",
      hashWholeFile: "Comparing a file with its likely copy",
      makeSystemVideos: "Making the server's own videos",
      findRepeats: "Finding a season's intros and credits",
    },
    state: {
      queued: "Waiting",
      running: "Running",
      done: "Done",
      failed: "Failed",
      cancelled: "Cancelled",
    },
    progress: "How far {{job}} is",
    reason: "Why: {{reason}}",
    queuedAt: "Queued {{at}}",
    lastFailure: "Last error: {{failure}}",
    waitingFor: "Waiting for {{name}}: {{reason}} since {{since}}, next check {{next}}",
    waitingForSignal: "Waiting for {{name}}: {{reason}} since {{since}}",
    onNode: {
      waiting: "Waiting for a free slot on the media machine",
      paused: "Paused while someone watches",
    },
    cancel: "Stop",
    cancelLabel: "Stop {{job}}",
    cancelQuestion: "Stop {{job}}?",
    cancelConsequence: "What it has done so far is lost.",
    cancelConfirm: "Stop the job",
    alreadyEnded: "That job has already ended.",
    active: "Running and waiting",
    finished: "Ended",
    day: "{{day}} ({{count}})",
    filterState: "State",
    filterKind: "Kind",
    every: "Every one",
    noneShown: "No job is of that state and kind.",
  },
};
