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
    cannotFollow:
      "This browser cannot keep in step with the others. On an iPhone or iPad, update to iOS 17.1 or later, then open the link again.",
    name: "Your name",
    join: "Join",
    badLink:
      "This link does not work: it was used already, withdrawn, or the session is over. Ask the person who sent it for a new one.",
    viewerHeading: "Watching together",
    stage: "The film",
    soundOn: "Tap for sound",
    volume: "Volume",
    leave: "Leave",
    connecting: "Connecting…",
    cannotKeepUp:
      "{{name}}'s connection carries about {{link}}, and even the lowest quality needs {{lowest}}: their film will keep pausing.",
    cannotKeepUpUnmeasured:
      "Even the lowest quality, {{lowest}}, is too much for {{name}}'s connection: their film will keep pausing.",
    reconnecting: "Connection lost. Reconnecting…",
    here: "Here: {{names, list}}",
    away: "{{name}} (reconnecting)",
    someone: "Someone",
    hostPlaying: "Watch Together is on: this plays for everyone in your session.",
    voice: {
      label: "Voice",
      talk: "Talk",
      headphones: "Use headphones, so the film does not reach your microphone.",
      starting: "Asking for your microphone…",
      mute: "Mute",
      unmute: "Unmute",
      leave: "Leave voice",
      partner: "Partner",
      connecting: "Voice: connecting…",
      alone: "Voice is on. No one else has turned theirs on yet.",
      connectingTo: "Voice: connecting to {{names, list}}…",
      browserReason: "The browser said: {{reason}}",
      connectedTo: "Voice: connected to {{names, list}}",
      failure: {
        refused:
          "Your microphone was not allowed. Allow it for this site in the browser's settings. On a phone, the browser app itself must also be allowed the microphone, in the phone's settings (Apps, then the browser, then Permissions). Then press Talk again.",
        none: "No microphone was found. Connect one, then press Talk again.",
        busy: "Your microphone is in use by another app, or could not start. Close that app, then press Talk again.",
        unsupported:
          "This page cannot use a microphone. Voice needs the https:// address, in a browser that supports it.",
        failed: "Your microphone could not start. Press Talk to try again.",
        unreachable: "Voice could not connect to the others and is off. Press Talk to try again.",
      },
    },
    over: {
      hostEnded: "The host ended the session. You can close this page.",
      ejected: "The host removed you from the session. You can close this page.",
      left: "You left the session. You can close this page.",
      gone: "This session is over. You can close this page.",
    },
    event: {
      played: "{{name}} played from {{at}}",
      paused: "{{name}} paused at {{at}}",
      seeked: "{{name}} jumped from {{from}} to {{to}}",
      changedItem: "{{name}} put on a film",
      stopped: "{{name}} stopped the film",
      continuedWithout: "Continuing without {{name}}, who will catch up",
      everyoneReady: "Everyone is ready",
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
    onExplain:
      "Your session is on. Browse and play as usual: whatever you play plays on everyone's screen, and when you stop, their screens go black.",
    makeLinkHeading: "Invite someone",
    note: "Who it is for (only you see this)",
    noteExample: "Grandma",
    makeLink: "Make a link",
    issued: "The link for {{note}}",
    qrLabel: "The link for {{note}}, as a QR code",
    people: "People",
    nobody: "No one is invited yet.",
    member: "{{name}} ({{note}})",
    eject: "Remove",
    waitingLink: "A link for {{note}}, not used yet",
    withdraw: "Withdraw",
    end: "End the session",
    endConsequence: "Everyone's screen goes black and their links stop working.",
    endConfirm: "End it",
  },
} as const;
