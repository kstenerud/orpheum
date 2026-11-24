// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's casting part (D337, D364): the casting page and its list of devices to
 * cast to, the receiver's question, line and ready page, and the remote of a session's film, added
 * as the first module that asks for one loads (cast-words.ts), never with the first page
 * (home-imports.test.ts holds it).
 */
export const enCast = {
  cast: {
    page: "Cast",
    heading: "Cast to",
    notInFront:
      "{{name}} has no Orpheum tab in front. Bring an Orpheum tab to the front on it, then cast again.",
    receivingAnother: "{{name}} is showing another cast. Cast to it once that cast ends.",
    castingHere:
      "{{name}} is casting to this device, so this device cannot cast to it. Stop that cast on {{name}} first.",
    chromecast: "Chromecast",
    airPlay: "AirPlay",
    playingOnAirPlay: "Playing on AirPlay",
    airPlayControls: "AirPlay controls",
    chromecastFailed: "The Chromecast could not take the cast. Try again.",
    showCastLog: "Show cast log",
    none: "None of your other devices can take a cast now. Open Orpheum on the device you want to watch on, signed in as you, with its tab in front, and it shows here.",
    controlledFrom: "Controlled from {{name}}",
    asking: "Asking {{name}}…",
    cancelRequest: "Cancel casting request",
    answered: {
      declined: "{{name}} declined.",
      unanswered: "{{name}} did not answer.",
      ended: "The cast to {{name}} ended before it started.",
      stopped: "You cancelled the casting request to {{name}}.",
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
    on: "On: {{name}}",
    playingFilmOn: "Playing {{film}} on {{name}}",
    castingTo: "Casting to {{name}}. It plays what the group plays.",
    backToTheFilm: "Back to the film",
    playback: {
      direct: "Original",
      converted: "Converted",
      atRate: "{{how}} {{rate}}",
      playing: "Playing: ",
      connection: "Connection: ",
    },
    filmLevel: "The film's volume there, under the TV's own",
    soundOffset: {
      inStep: "Sound there: in step with the picture",
      laterBy: "Sound there: {{ms}} ms later",
      earlierBy: "Sound there: {{ms}} ms earlier",
      field: "Sound timing there, in ms: more is later, less is earlier",
      thatDevice: "That device",
      thatDeviceEarlier: "That device earlier",
      thatDeviceLater: "That device later",
    },
    readyOn: "Ready on {{name}}. What you play now plays there.",
    stop: "Stop casting",
    stopHere: "Stop casting here",
    sendingOn: "Sending it to the device you cast to…",
    over: {
      lost: "The cast ended: the server no longer knows it.",
    },
  },
  remote: {
    playing: "Playing",
    paused: "Paused",
    waiting: "Starting…",
  },
};
