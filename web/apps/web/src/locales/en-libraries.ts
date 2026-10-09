// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's libraries part: the admin's libraries page, its roots, scans and watching,
 * and the new-library form (libraries), kept apart so the catalogue fits in one read (en.ts).
 */
export const enLibraries = {
  libraries: {
    heading: "Libraries",
    none: "No libraries yet. Create one below.",
    scan: "Scan now",
    scanAgainAfter: "Scan again when this one ends",
    scanListing: "Scanning… listing the folders.",
    scanReading: "Scanning… {{count}} files found, {{read}} of {{total}} new or changed read.",
    scanThenAgain: "It scans again when this one ends.",
    scanning: "Asked {{count}} roots to scan.",
    files: "What the scanner saw",
    filing: "{{filing}}: {{count}}",
    root: "{{node}}: {{path}}",
    outcomeComplete: "scanned",
    outcomeUnreadable: "could not be read",
    outcomeWithheld: "removals wait for you",
    watching: "watched for changes",
    watchCauseNetworkFilesystem:
      "Not watched: it is on a network share, whose changes made on another machine this one never hears of.",
    watchCauseWatchLimit:
      "Not watched: it has more folders than this machine lets one user watch (fs.inotify.max_user_watches).",
    watchCauseUnavailable: "Not watched: this machine gave the node no watcher.",
    watchCauseUnreadable: "Not watched: a folder in it could not be read.",
    watchFallback: "Its periodic scan finds what changes.",
    approveRemovals: "Remove what the last scan did not find",
    diskMissing: "A disk is missing from",
    diskGone: "This disk is gone for good",
    retireRoot: "This folder is gone for good…",
    retireRootConsequence:
      "Its films leave the library and it is never scanned again; what was watched is kept.",
    repoint: "This folder moved…",
    repointTo: "It is now at",
    repointConsequence:
      "Its files move there with what was watched; the next scan reads each again to make sure it is the same.",
    repointConfirm: "Use the new place",
    repointKeep: "Keep it where it is",
    repointed: "Now at the new place, with {{count}} files. A scan of it is under way.",
    retireRootConfirm: "Retire the folder",
    delete: "Delete the library…",
    deleteConsequence:
      "Its films and series are hidden and its folders never scanned again; what was watched is kept, and a film that turns up in another library brings its history back.",
    deleteConfirm: "Delete it",
    createHeading: "Create a library",
    name: "Name",
    kind: "What it holds",
    kindMovies: "Films",
    kindShows: "Series",
    kindMixed: "Both",
    metadataSource: "Where information comes from",
    sourceNfoThenTmdb: "NFO files where there are any, and TMDB for the rest",
    sourceTmdb: "TMDB only; NFO files are ignored",
    sourceNfo: "NFO files only; nothing is fetched",
    changeDetection: "How changes are noticed",
    changesWatch: "Watch the folders",
    changesInterval: "Check every so many hours",
    changesManual: "Only when I ask",
    checkEvery: "Hours between checks",
    writable: "May the server write beside the media",
    writableHelp:
      "Read-only unless you allow it; while read-only, nothing is saved beside your files.",
    node: "Machine",
    nodeAway: "{{name}} (not connected)",
    roots: "Folders this library reads",
    rootsHelp:
      "Tick at least one. Only the folders the node's configuration lists (node.root_paths) appear here; to add one, list it there and restart the node.",
    noNodes:
      "No node has connected since the server started. Start one, and its folders show here.",
    noRootsYet:
      "{{name}} has not said which folders it offers yet. Reload this page in a moment; if none appear, its configuration lists none (node.root_paths).",
    create: "Create and scan",
  },
};
