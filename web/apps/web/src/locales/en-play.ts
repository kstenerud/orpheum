// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's play part: the player's screen, what plays and why, its failures and its
 * menus (play), kept apart so the catalogue fits in one read (en.ts).
 */
export const enPlay = {
  play: {
    updateAfterFilm: "A new version of this app is ready: it loads as you leave the film.",
    direct: "Playing the original file, as it is.",
    directFailed: "This browser could not play the original file: {{reason}}.",
    prepared: "Playing a copy prepared for this browser, because {{reasons}}.",
    preparing: "Preparing — ready in a few seconds.",
    preparedFailed: "The prepared copy could not play: {{reason}}.",
    ladderFailed: "The film could not play: {{reason}}.",
    lowerQualityStarting:
      "Your connection could not keep up, so this moves to a lower quality: {{height}}p, {{rate}}.",
    ladderStarting: "Starting at {{height}}p, {{rate}}. The quality follows your connection.",
    ladderPlaying: "Playing at {{height}}p, {{rate}}. The quality follows your connection.",
    lowerQuality:
      "Playing a lower quality for your connection: {{height}}p, {{rate}}. It goes back up when the connection allows.",
    cannotKeepUpUnmeasured:
      "Even the lowest quality, {{lowest}}, is too much for this connection: it will keep pausing.",
    cannotKeepUp:
      "Your connection carries about {{link}}, and even the lowest quality needs {{lowest}}: it will keep pausing.",
    failure: {
      hlsUnsupported: "this browser cannot play streamed video",
      aborted: "loading it was stopped",
      network: "it could not be fetched from the server",
      decode: "this browser could not decode it",
      unsupported: "this browser does not play its format",
      tooBusy:
        "the server cannot make this quality as fast as it plays while it makes others for other viewers; try again in a while",
      unknown: "the browser gave no reason",
    },
    boost: "Dialogue boost",
    level: {
      switch: "Level loudness",
      raised: "Raised {{db}} dB, to the level of other films.",
      lowered: "Lowered {{db}} dB, to the level of other films.",
      atLevel: "Already at the level of other films.",
      asMixed: "Playing as it was mixed.",
      notMeasured: "Its loudness is not measured yet, so it plays as it is.",
      deviceVolume:
        "This device sets its own volume, so only a copy prepared after measuring is levelled here.",
    },
    startOver: "Start over",
    stoppedByAdmin: "The administrator stopped this: {{message}}",
    positionNotKept: "Where you are is no longer being kept for next time. {{reason}}",
    reopening: "The server restarted. Opening the film again where you were…",
    audio: {
      label: "Sound",
      unknownLanguage: "Unknown language",
      commentary: "commentary",
      described: "described for the blind and partially sighted",
    },
    subtitles: {
      label: "Subtitles",
      off: "Off",
      unknownLanguage: "Unknown language",
      forced: "forced",
      hearingImpaired: "for the deaf and hard of hearing",
      earlier: "Earlier",
      later: "Later",
      offset: "Subtitle offset: {{seconds}} s",
      failed: "The subtitles could not be shown: {{reason}}.",
      fetched: "downloaded, unverified",
    },
    findSubtitles: {
      open: "Find subtitles…",
      heading: "Find subtitles",
      language: "Language",
      search: "Search OpenSubtitles",
      allowance: "{{remaining}} of {{allowed}} downloads left today.",
      unverified:
        "Downloaded subtitles aren't checked against the sound, so one can run early or late, or be timed for a different cut. If it is, the Earlier and Later buttons shift it.",
      none: "OpenSubtitles has no {{language}} subtitles for this video.",
      noneForThisFile: "OpenSubtitles has no {{language}} subtitles made for this exact file.",
      fileHashOnly:
        "This video isn't identified yet, so only subtitles made for this exact file were searched for. Once it is identified, a search also finds those made for other copies of it.",
      madeForThisFile: "made for this very file",
      machineTranslated: "machine-translated",
      downloads: "downloads: {{count}}",
      fetch: "Download",
      quotaSpent: "Today's downloads are spent; they start again at {{when}}.",
      nothingToSearchBy:
        "This file is not identified yet, so there is nothing to ask OpenSubtitles by. Identify it first, or scan it again.",
      reason: {
        noLogin: "No OpenSubtitles account is set up. An admin can add one in Settings.",
        loginRefused:
          "OpenSubtitles refused the account's login. An admin can change it in Settings.",
        notConfigured: "This server does not download subtitles.",
        unavailable: "OpenSubtitles is not answering right now. Try again later.",
        unavailableSince: "OpenSubtitles has not been answering{{since}}{{next}}.",
        unreadable: "That subtitle could not be read, so it was not kept. Choose another.",
        unknown: "OpenSubtitles could not be reached.",
      },
    },
    reason: {
      container: "the browser cannot open its container",
      videoCodec: "the browser cannot decode its video",
      audioCodec: "the browser cannot decode its sound",
      bandwidth: "it is too large for the connection",
      dialogueBoost:
        "this device plays stereo, so its surround sound is mixed here and the dialogue boost can act",
      audioTrack: "the sound you chose is not the one the file plays first",
      userChoice: "you asked for it to be prepared",
    },
  },
};
