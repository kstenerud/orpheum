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
    none: "None of your other devices has Orpheum open now. Open it on the device you want to watch on, signed in as you, and it shows here.",
    controlledFrom: "Controlled from {{name}}",
    takeBack: "Take back control",
    takeBackExplain: "Press again to take back control from {{name}}.",
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
