// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's casting part (D337, D364): the casting page and its list of devices to
 * cast to, the receiver's question, line and ready page, and the remote of a session's film, kept
 * apart so the catalogue fits in one read (en.ts).
 */
export const enCast = {
  cast: {
    page: "Cast",
    heading: "Cast to",
    connectedNow: "Open now · last used {{used}}",
    noTabInFront:
      "Open, but no Orpheum tab is in front: bring one to the front there to cast to it",
    notInFront:
      "{{name}} has no Orpheum tab in front. Bring an Orpheum tab to the front on it, then cast again.",
    none: "None of your other devices has Orpheum open now. Open it on the device you want to watch on, signed in as you, and it shows here.",
    controlledFrom: "Controlled from {{name}} · Back, or OK twice, to take control here",
    asked: {
      question: "{{device}} wants to use this device for playback",
      use: "Use",
      notNow: "Not now",
    },
    asking: "Asking {{name}}…",
    answered: {
      declined: "{{name}} declined.",
      unanswered: "{{name}} did not answer.",
      ended: "The cast to {{name}} ended before it started.",
    },
    soundOn: {
      television: "Press OK to turn the sound on",
      touch: "Tap to turn the sound on",
      pointer: "Click to turn the sound on",
    },
    ready: "Ready, controlled from {{name}}",
    notReceiving: "This device is not being cast to.",
    toHome: "Go to the home page",
    aFilm: "A film",
    playingOn: "Playing on {{name}}",
    playingFilmOn: "Playing {{film}} on {{name}}",
    backToTheFilm: "Back to the film",
    playback: {
      direct: "Plays the file as it is",
      converted: "Plays it converted",
      quality: "Quality {{rate}}",
      link: "Link about {{rate}}",
    },
    readyOn: "Ready on {{name}}. What you play now plays there.",
    stop: "Stop casting",
    playHere: "Play here from {{at}}",
    playHereInstead: "Play on this device instead",
    sendingOn: "Sending it to the device you cast to…",
    over: {
      replaced: "Another of your devices now controls {{name}}.",
      gone: "The cast is over.",
    },
  },
  remote: {
    playing: "Playing",
    paused: "Paused",
    waiting: "Starting…",
  },
};
