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
    name: "Your name",
    join: "Join",
    badLink:
      "This link does not work: it was used already, withdrawn, or the session is over. Ask the person who sent it for a new one.",
    viewerHeading: "Watching together",
    stage: "The film",
    volume: "Volume",
    leave: "Leave",
    connecting: "Connecting…",
    reconnecting: "Connection lost. Reconnecting…",
    here: "Here: {{names, list}}",
    away: "{{name}} (reconnecting)",
    someone: "Someone",
    hostPlaying: "Watch Together is on: this plays for everyone in your session.",
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
      buffering: "{{name}} is buffering… everyone waits",
      dropped: "{{name}} dropped out — everyone waits",
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
