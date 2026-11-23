// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's words for what needs an admin, by the page it belongs to (D538 §12): each
 * item's line, as the Needs attention page, a page's Needs you and the menu's marks say it
 * (attention-lines.ts), kept apart so the catalogue fits in one read (en.ts). The catalogue has no
 * plural forms: a count's line has its own words for one.
 */
export const enAttention = {
  attention: {
    job: "What needs you, and where to fix it.",
    goTo: "Go to {{page}}",
    needsAttention: "{{name}}, needs attention",
    elsewhere: "{{count}} more things need you elsewhere",
    elsewhereOne: "1 more thing needs you elsewhere",
    folderOffline: "{{libraryName}}: the folder {{rootPath}} on {{nodeName}} is offline.",
    diskOffline: "{{libraryName}}: the disk at {{mountPath}} in {{rootPath}} is missing.",
    folderWaiting:
      "{{libraryName}}: the scan of {{rootPath}} waits for you before removing {{count}} files.",
    folderWaitingOne:
      "{{libraryName}}: the scan of {{rootPath}} waits for you before removing 1 file.",
    unread: "{{libraryName}}: {{count}} files or folders could not be read.",
    unreadOne: "{{libraryName}}: 1 file or folder could not be read.",
    rescanGaveUp:
      "{{libraryName}}: its rescan gave up, because the library changed every time it was read.",
    needsMatch: "{{count}} films and series need you to choose their match.",
    needsMatchOne: "1 film or series needs you to choose its match.",
    seriesReadWrongly: "{{count}} series may have had their file names read wrongly.",
    seriesReadWronglyOne: "1 series may have had its file names read wrongly.",
    filmsWithoutSound: "{{count}} films have no sound track, so they play in silence.",
    filmsWithoutSoundOne: "1 film has no sound track, so it plays in silence.",
    finding: "{{check}}: {{detail}}",
    workFailing: "{{count}} jobs failed in the last {{hours}} hours.",
    workFailingOne: "1 job failed in the last {{hours}} hours.",
    destinationDown: "{{name}} is not answering: {{reason}}.",
    certificateMissing: "The certificate for {{name}} has not been obtained.",
    unknownFailures:
      "{{count}} kinds of work failed in a way the server doesn't know how to handle.",
    unknownFailuresOne: "1 kind of work failed in a way the server doesn't know how to handle.",
    machineLocked: "{{nodeName}} is locked: it will not run until its confinement is fixed.",
    machineOutdated: "{{nodeName}} runs an older build than the server.",
    machineSilent: "{{nodeName}} has not answered since {{since}}.",
    keyRefused: "{{name}} refused its key or login.",
    keyMissing: "{{name}} has no key or login set, so the server can't use it.",
    secretNeedsReentry: "{{name}} must be entered again.",
    updateAvailable: "{{version}} is available.",
    securityFix: "Your version lacks a security fix, in {{version}}: {{detail}}",
    updateCheckFailed: "The last check for updates failed: {{detail}}",
    composePinned: "Your compose file pins version {{version}}, so it does not update.",
    composeNewer: "A newer compose file is available.",
    stoodAside:
      "Another copy of this server serves. This one stopped asking and serves only at home. Asking again takes over only once the other copy has stopped.",
    copyRefused:
      "A copy of this server tried to start from {{name}} at {{since}}, and was refused.",
    addressesExpired: "This server's addresses from away expired at {{since}}.",
    removed: "Another server was linked in this one's place at {{since}}.",
    deadEnds: "{{count}} devices could not reach this server in the last week.",
    deadEndsOne: "1 device could not reach this server in the last week.",
    protocolEnding: "The discovery stops answering this version of the server on {{until}}.",
  },
};
