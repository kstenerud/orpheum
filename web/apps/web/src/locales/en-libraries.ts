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
    rescan: "Rescan",
    rescanHelp:
      "Scans it, then matches each film and series in it again from its files. What a person chose or edited is left alone.",
    rescanEverything: "Rescan everything",
    rescanStarted: "Rescanning: it is scanned, then each film and series in it is matched again.",
    rescanRunning: "A rescan of it is already running.",
    rescanEverythingStarted: "Rescanning {{count}} libraries of films or series.",
    rescanScanning: "Rescanning… scanning first.",
    rescanThis: "Rescan this",
    rescanThisStarted: "Rescanning: its files are scanned, then it is matched again from them.",
    rescanThisRunning: "A rescan of it, or of its library, is already running.",
    rescanMatching: "Rescanning… {{matched}} of {{count}} films and series matched again.",
    files: "What the scanner saw",
    filing: "{{filing}}: {{count}}",
    root: "{{node}}: {{path}}",
    outcomeComplete: "scanned",
    outcomeUnreadable: "could not be read",
    outcomeWithheld: "removals wait for you",
    watching: "watched for changes",
    watchCauseNetworkFilesystem:
      "Not watched: it is on a network share or a shared drive, whose changes made elsewhere (another machine, or Windows on a WSL drive) this one never hears of.",
    watchCauseWatchLimit:
      "Not watched: it has more folders than this machine lets one user watch (fs.inotify.max_user_watches).",
    watchCauseUnavailable: "Not watched: this machine gave the node no watcher.",
    watchCauseUnreadable: "Not watched: a folder in it could not be read.",
    watchFallback: "Its periodic scan finds what changes.",
    approveRemovals: "Remove what the last scan did not find",
    diskMissing: "Nothing is mounted any more at",
    diskGone: "What was mounted here is gone for good",
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
    kindOther: "Other videos: home movies, concerts, lectures, clips",
    metadataSource: "Where information comes from",
    sourceNfoThenTmdb: "NFO files where there are any, and TMDB for the rest",
    sourceTmdb: "TMDB only; NFO files are ignored",
    sourceNfo: "NFO files only; nothing is fetched",
    sourceOther:
      "Other videos are never looked up online: each is named by its file, or an NFO file beside it, and dated by when it was recorded.",
    changeDetection: "How changes are noticed",
    changesWatch: "Watch the folders",
    changesInterval: "Check every so many hours",
    changesManual: "Only when I ask",
    checkEvery: "Hours between checks",
    changeDetectionSave: "Save",
    changeDetectionSaved: "Saved: changes are noticed as chosen.",
    writable: "May the server write beside the media",
    writableHelp:
      "Read-only unless you allow it; while read-only, nothing is saved beside your files.",
    node: "Machine",
    nodeAway: "{{name}} (not connected)",
    roots: "Folders this library reads",
    rootsHelp:
      "Tick at least one: a folder the node's configuration lists (node.root_paths), or a folder inside one, chosen below. Only folders inside those can be chosen; to add one, list it there and restart the node.",
    browseInside: "Or choose a folder inside one",
    browseOpen: "Browse {{node}}: {{path}}",
    browseUp: "Up one folder",
    browseChoose: "Choose this folder",
    browseEmpty: "No folders in it.",
    browseNotAnswering:
      "This folder is not responding: the disk or share it is on did not answer in time.",
    browseMissing: "This folder is not there any more.",
    browseFailed:
      "This folder cannot be chosen: it could not be read, or it is reached through a link.",
    browseAgain: "Try again",
    addRoot: "Add a folder…",
    addRootCancel: "Cancel",
    rootAdded: "Added {{path}}. A scan of it is under way.",
    noNodes:
      "No node has connected since the server started. Start one, and its folders show here.",
    noRootsYet:
      "{{name}} has not said which folders it offers yet. Reload this page in a moment; if none appear, its configuration lists none (node.root_paths).",
    create: "Create and scan",
  },
};
