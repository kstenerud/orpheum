// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's status part: the status tray at the banner's end (status), the page
 * of what needs the admin (needsAttention), D280, and the job view (jobs), kept apart so the
 * catalogue fits in one read (en.ts).
 */
export const enStatus = {
  status: {
    tray: "What is on",
    together: "Watch Together",
    togetherOn: "Watch Together is on: its controls",
    cast: "Cast to a device",
    castingReady: "Casting to {{name}}, ready",
    castingFilm: "Casting {{film}} to {{name}}",
    close: "Close",
    needsAttention: "Something needs your attention",
  },
  needsAttention: {
    heading: "Needs attention",
    placeholder:
      "Everything that needs fixing will be summed up here. For now, this counts it, and the pages below have each with what fixes it.",
    nothing: "Nothing needs your attention.",
    offlineFolders: "{{count}} library folders are offline.",
    missingDisks: "{{count}} folders mounted inside library folders are missing.",
    waitingFolders: "{{count}} library folders' scans wait for you before removing files.",
    unreadPaths: "{{count}} files or folders could not be read.",
    waitingMatches: "{{count}} films and series need you to choose their match.",
    seriesReadWrongly:
      ": {{count}} of its episodes have no number, though no specials or extras folder holds them. Their file names may have been read wrongly; renaming them as S01E04 places them.",
    filmWithoutSound:
      ": its video has no sound track, so it plays in silence. The file may be a video-only download whose sound was never merged in.",
    toSystem: "The System page",
    toLibraries: "The Libraries",
    toIdentify: "Needs a match",
  },
  jobs: {
    heading: "Jobs",
    none: "No work has been done yet.",
    target: "{{kind}} #{{target}}",
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
    cancel: "Cancel",
    cancelLabel: "Cancel {{job}}",
    alreadyEnded: "That job has already ended.",
  },
};
