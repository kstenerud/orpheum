// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's Watch Together part: the viewer's page (together) and the host's
 * screen (watchTogether), kept apart so the catalogue fits in one read (en.ts).
 */
export const enTogether = {
  together: {
    joinHeading: "Watch together",
    joinExplain:
      "You were invited to watch together. Choose the name the others will see you by, then join.",
    noLink: "This address has no invitation in it. Open the link you were given.",
    checkingLink: "Checking your invitation…",
    // Asking the server for the person's sessions failed: asked again by its button.
    askAgain: "Try again",
    cannotFollow:
      "This browser cannot keep in step with the others. On an iPhone or iPad, update to iOS 17.1 or later, then open the link again.",
    name: "Your name",
    join: "Join",
    leaveOtherToJoin: "Leave {{host}}'s session to join this one?",
    leaveOwnToJoin: "Leave your own session to join this one? It ends for everyone in it.",
    leaveAndJoin: "Leave and join",
    stayInOther: "Don't join",
    badLink:
      "This link does not work: invitations are short-lived, and this one has expired, was used already or was withdrawn, or the session is over. Ask the person who sent it for a new one.",
    viewerHeading: "Watching together",
    stage: "The film",
    waitsForAFilm: "Waiting for the host to start a film.",
    soundOn: "Tap for sound",
    soundBack: "Tap to turn the sound back on",
    invitedBy: "{{host}} invited you to watch together.",
    invitedBySomeone: "You were invited to watch together.",
    tapToContinue: "Tap to continue",
    fullscreen: "Full screen",
    volume: "Volume",
    volumeButtons: "Use your phone's volume buttons for the film.",
    leave: "Leave",
    connecting: "Connecting…",
    cannotKeepUp:
      "{{name}}'s connection carries about {{link}}, and even the lowest quality needs {{lowest}}: their film will keep pausing.",
    cannotKeepUpUnmeasured:
      "Even the lowest quality, {{lowest}}, is too much for {{name}}'s connection: their film will keep pausing.",
    reconnecting: "Connection lost. Reconnecting…",
    here: "Here: {{names, list}}",
    // A member with no device on the session's page: wandered elsewhere in the app, or their page
    // closed or lost (D372 §9); the roster cannot tell which.
    away: "{{name}} (away)",
    status: {
      needsTap: "{{name}}: needs to tap the page to start",
      loading: "{{name}}: loading the film",
      catchingUp: "{{name}}: catching up",
      buffering: "{{name}}: waiting for the film to load",
      suspended: "{{name}}: paused on their device",
      watching: "{{name}}: watching",
    },
    someone: "Someone",
    debug: {
      heading: "Devices in the session",
      device: "{{name}}: device {{id}} ({{role}})",
      leading: "▶ leading",
      thisDevice: "this device",
      line: "{{state}} · leader {{leader}} · why {{why}} · this device {{device}}",
      leader: "device {{id}} ({{name}})",
      none: "none",
    },
    hostPlaying: "Watch Together is on: this plays for everyone in your session.",
    cameIn: "{{name}} joined",
    wentOut: "{{name}} left",
    voice: {
      label: "Voice",
      talk: "Talk",
      headphones: "Use headphones, so the film does not reach your microphone.",
      earpiece: "To hear the video while the microphone is on, please connect your headphones.",
      ok: "OK",
      tapToHear: "Tap to hear everyone's voices",
      starting: "Asking for your microphone…",
      stopTalking: "Stop talking",
      talking: "The others hear you.",
      personVolume: "{{name}}'s voice",
      volumes: "Voices",
      connecting: "Voice: connecting…",
      alone: "Voice: no one else is here yet.",
      connectingTo: "Voice: connecting to {{names, list}}…",
      browserReason: "The browser said: {{reason}}",
      connectedTo: "Voice: hearing {{names, list}}",
      failure: {
        refused:
          "Your microphone was not allowed. Allow it for this site in the browser's settings. On a phone, the browser app itself must also be allowed the microphone, in the phone's settings (Apps, then the browser, then Permissions). Then press Talk again.",
        none: "No microphone was found. Connect one, then press Talk again.",
        busy: "Your microphone is in use by another app, or could not start. Close that app, then press Talk again.",
        unsupported:
          "This page cannot use a microphone. Voice needs the https:// address, in a browser that supports it.",
        failed: "Your microphone could not start. Press Talk to try again.",
      },
    },
    over: {
      hostEnded: "The host ended the session. You can close this page.",
      idle: "The session was closed because nothing had happened in it for hours. You can close this page.",
      ejected: "The host removed you from the session. You can close this page.",
      movedAway:
        "You joined this session on another device, by a new link from the host, so it ended here. You can close this page.",
      left: "You left the session. You can close this page.",
      gone: "This session is over. You can close this page.",
      uploadFull: "The host's connection can't carry everyone right now.",
      takenBack: "Control was taken back. You can close this page.",
      replaced: "Another device took over. You can close this page.",
      stopped: "Casting stopped. You can close this page.",
      receiverGone: "The device that played was gone too long. You can close this page.",
      joinAgain: "Join again",
    },
    // Said on the home page a signed-in guest is taken to as their session ends (notice.ts); sent
    // away for the host's connection (uploadFull), they stay on the session's page with Join again.
    overNotice: {
      hostEnded: "The host ended the Watch Together session.",
      idle: "The Watch Together session was closed because nothing had happened in it for hours.",
      ejected: "The host removed you from the Watch Together session.",
      movedAway: "You joined the Watch Together session on another device, so it ended here.",
      left: "You left the Watch Together session.",
      gone: "The Watch Together session is over.",
      // A signed-in guest's session gone without a word to their page: a restart of the server.
      notThere: "The Watch Together session is no longer there.",
      // Said to the host's other pages as they end the session on one (D389 §1).
      endedElsewhere: "The Watch Together session was ended on another of your devices.",
      // Said to the host's page as they move the session to another of their devices (D442 §2).
      movedElsewhere: "You moved the Watch Together session to another of your devices.",
      takenBack: "Control was taken back, so the Watch Together session ended here.",
      replaced: "Another device took over the Watch Together session.",
      stopped: "Casting stopped, so the Watch Together session ended here.",
      receiverGone:
        "The device that played was gone too long, so the Watch Together session ended.",
    },
    event: {
      played: "{{name}} played from {{at}}",
      paused: "{{name}} paused at {{at}}",
      seeked: "{{name}} jumped from {{from}} to {{to}}",
      changedItem: "{{name}} put on a film",
      stopped: "{{name}} stopped the film",
      waitingFor: "Waiting for {{name}}'s player at {{at}}",
      leaderPlays: "{{name}}'s screen plays from {{at}}",
      everyoneLeft: "Everyone's screen left; paused at {{at}}",
      everyoneLeftWhilePlaying:
        "Everyone's screen left; held at {{at}}, and plays on when a screen is back",
      resumedOnReturn: "{{name}}'s screen is back; playing on from {{at}}",
      changedRate: "{{name}} changed the speed",
    },
  },
  watchTogether: {
    heading: "Watch Together",
    explain:
      "Start a session, then make a link for each person you want to watch with. While it is on, what you play plays for them too.",
    start: "Start a session",
    cannotHost:
      "This browser cannot keep in step with a group, so it cannot host one. On an iPhone or iPad, update to iOS 17.1 or later.",
    mayNotHost: "Your role may not host Watch Together. An administrator can change that.",
    over: "That session is over.",
    // The session the person hosts on another of their devices (D442).
    onElsewhere: "Your session is on, on another of your devices.",
    onElsewhereNamed: "Your session is on, on {{device}}.",
    moveHere: "Move it here",
    onExplain:
      "Your session is on. Browse and play as usual: whatever you play plays on everyone's screen, and goes on while you browse. When you stop it, their screens go black.",
    nothingPlays: "Nothing is playing",
    backToTheFilm: "Back to the film",
    stopTheFilm: "Stop the film for everyone",
    castingTo: "Casting to {{name}}.",
    makeLinkHeading: "Invite someone",
    filmVolume: "Film volume",
    note: "Who it is for (only you see this)",
    noteExample: "Grandma",
    mayPlayPause: "They may play and pause for everyone",
    maySeek: "They may move the position for everyone",
    rights: {
      both: "may play, pause and move the position",
      playPause: "may play and pause",
      seek: "may move the position",
      none: "watches only",
    },
    makeLink: "Make a link",
    issued: "The link for {{note}}",
    qrLabel: "The link for {{note}}, as a QR code",
    people: "People",
    nobody: "No one is invited yet.",
    member: "{{name}} ({{note}})",
    // Said after a member with no device on the session's page (D372 §9).
    memberAway: "away",
    eject: "Remove",
    ejectConsequence: "{{name}} leaves the session and can't come back by their link.",
    ejectConfirm: "Remove them",
    relink: "New link",
    relinkExplain:
      "{{name}} stays in until the new link is used. Whoever joins by it takes their place, and the devices they came in on are cut off.",
    showLink: "Show link",
    waitingLink: "A link for {{note}}, not used yet",
    linkUntil: "good until {{time}}",
    withdraw: "Withdraw",
    withdrawConsequence: "No one can join by this link.",
    withdrawConfirm: "Withdraw it",
    end: "End the session",
    endConsequence: "Everyone's screen goes black and their links stop working.",
    endConfirm: "End it",
  },
} as const;
