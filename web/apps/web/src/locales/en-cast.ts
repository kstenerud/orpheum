// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's casting part (D337): the list of devices to cast to, the receiver's line
 * and prompt, and the remote, kept apart so the catalogue fits in one read (en.ts).
 */
export const enCast = {
  cast: {
    open: "Cast to another device",
    heading: "Cast to",
    close: "Close",
    connectedNow: "Open now · last used {{used}}",
    noTabInFront:
      "Open, but no Orpheum tab is in front: bring one to the front there to cast to it",
    notInFront:
      "{{name}} has no Orpheum tab in front. Bring an Orpheum tab to the front on it, then cast again.",
    none: "None of your other devices has Orpheum open now. Open it on the device you want to watch on, signed in as you, and it shows here.",
    controlledFrom: "Controlled from {{name}} · Back, or OK twice, to take control here",
    asked: {
      film: "{{device}} wants to play {{film}} here",
      aFilm: "{{device}} wants to play a film here",
      play: "Play",
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
    remote: "Remote",
    aFilm: "A film",
    playingOn: "Playing on {{name}}",
    playing: "Playing",
    paused: "Paused",
    waiting: "Starting…",
    play: "Play",
    pause: "Pause",
    back: "Back 10 s",
    forward: "Forward 10 s",
    timeline: "Where the film is",
    stop: "Stop casting",
    playHere: "Play here from {{at}}",
    notCasting: "This device is not casting anything.",
    sendingOn: "Sending it to the device you cast to…",
    over: {
      takenBack: "{{name}} took control back.",
      replaced: "Another of your devices now controls {{name}}.",
      stopped: "Casting stopped.",
      filmEnded: "The film ended on {{name}}.",
      gone: "The cast is over.",
    },
  },
};
