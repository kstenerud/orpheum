// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's status part: the status tray at the banner's end (status) and the page
 * of what needs the admin (needsAttention), D280, kept apart so the catalogue fits in one read
 * (en.ts).
 */
export const enStatus = {
  status: {
    tray: "What is on",
    together: "Watch Together is on: its controls",
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
    toSystem: "The System page",
    toLibraries: "The Libraries",
    toIdentify: "Needs a match",
  },
};
