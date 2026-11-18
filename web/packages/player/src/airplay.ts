// SPDX-License-Identifier: MPL-2.0

/**
 * A film handed to an AirPlay receiver from the page's own element (D473; plans/airplay.md's Way B):
 * hls.js lets go of the element, which is given the grant's master playlist at its position, Safari's
 * own HLS playing it, and Safari's picker is shown within the press. The player's controls go on
 * driving the element, now the receiver's remote, and its time is the receiver's. AirPlay ends as the
 * element's target stops being wireless (the person choosing the phone, in the picker Stop shows or
 * in Control Center, or the receiver dropping an idle film), at stop, or when no receiver is picked
 * within airPlayPickWithinMs; the element is
 * then emptied, so the phone does not go on loading the grant's playlist (the lab's second run,
 * 2026-11-13), and the page is told where it stood, to hand it back to hls.js.
 */

/** The page, which offers AirPlay where it has WebKit's event for a target's availability (Safari). */
export type AirPlayPage = object;

/** The event an element tells as its playback target becomes wireless, or stops being. */
export const wirelessChangedEvent = "webkitcurrentplaybacktargetiswirelesschanged";

/** How long a receiver may take to be picked before the film is handed back to the page. */
export const airPlayPickWithinMs = 30_000;

/** The element AirPlay plays from: its time, its playing, its source, and WebKit's AirPlay. */
export type AirPlayVideo = Pick<
  HTMLVideoElement,
  | "currentTime"
  | "src"
  | "disableRemotePlayback"
  | "paused"
  | "pause"
  | "play"
  | "load"
  | "setAttribute"
  | "removeAttribute"
> & {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- WebKit's own name
  readonly webkitCurrentPlaybackTargetIsWireless?: boolean;
  readonly webkitShowPlaybackTargetPicker?: () => void;
  addEventListener(type: string, listener: () => void): void;
  removeEventListener(type: string, listener: () => void): void;
};

/** Where the film stood as AirPlay ended, in s, and whether it was paused there. */
export interface AirPlayLeft {
  readonly positionS: number;
  readonly wasPaused: boolean;
}

/**
 * A film playing on AirPlay. askToStop, within a press, shows the picker for the person to choose this
 * device, and AirPlay ends as they do: only they can bring its sound back from the receiver (D521
 * §1). stop ends it at once, for an end no one pressed. Either way the page is told where it stood
 * (onEnded).
 */
export interface AirPlaying {
  readonly askToStop: () => void;
  readonly stop: () => void;
}

/**
 * Whether the page offers AirPlay from video: Safari's, which has WebKit's target availability event
 * and the element's picker.
 */
export function isAirPlayOffered(video: object, page: AirPlayPage): boolean {
  return (
    "WebKitPlaybackTargetAvailabilityEvent" in page &&
    "webkitShowPlaybackTargetPicker" in video &&
    typeof video.webkitShowPlaybackTargetPicker === "function"
  );
}

/**
 * Hands the film in video to AirPlay from the grant's master playlist at url, within the press it is
 * called in: hls.js lets go of it first (letGoOfHls), and onEnded is told, once, where it stood as
 * AirPlay ended. Returns the playing, which askToStop and stop end.
 */
export function playOnAirPlay(
  video: AirPlayVideo,
  url: string,
  hand: { readonly letGoOfHls: () => void; readonly onEnded: (left: AirPlayLeft) => void },
): AirPlaying {
  const positionS = video.currentTime;
  hand.letGoOfHls();
  const element = video;
  element.disableRemotePlayback = false;
  element.setAttribute("x-webkit-airplay", "allow");
  element.src = url;
  element.currentTime = positionS;
  let hasGoneWireless = false;
  let isEnded = false;
  const unpicked = setTimeout(() => {
    if (!hasGoneWireless) {
      end();
    }
  }, airPlayPickWithinMs);
  function end(): void {
    if (isEnded) {
      return;
    }
    isEnded = true;
    clearTimeout(unpicked);
    element.removeEventListener(wirelessChangedEvent, onWirelessChanged);
    const left = { positionS: element.currentTime, wasPaused: element.paused };
    // Remote playback off again, as before the handover, which brings the picture back to the page.
    // The sound stays on the AirPlay route the picker chose, which only the person can end (the
    // owner, VQ362, 2026-11-18; docs/contributors/idiosyncrasies.md).
    element.disableRemotePlayback = true;
    element.removeAttribute("x-webkit-airplay");
    element.pause();
    element.removeAttribute("src");
    element.load();
    hand.onEnded(left);
  }
  function onWirelessChanged(): void {
    if (element.webkitCurrentPlaybackTargetIsWireless === true) {
      hasGoneWireless = true;
      clearTimeout(unpicked);
    } else if (hasGoneWireless) {
      end();
    }
  }
  function askToStop(): void {
    if (isEnded) {
      return;
    }
    if (!hasGoneWireless) {
      end(); // no receiver is picked, so nothing plays the sound but this device
      return;
    }
    try {
      element.webkitShowPlaybackTargetPicker?.();
    } catch {
      end();
    }
  }
  const playing = { askToStop, stop: end };
  element.addEventListener(wirelessChangedEvent, onWirelessChanged);
  try {
    element.webkitShowPlaybackTargetPicker?.();
  } catch {
    end();
    return playing;
  }
  element.play().catch(() => undefined); // a refusal leaves it paused, for the person's press
  return playing;
}
