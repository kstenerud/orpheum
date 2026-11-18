// SPDX-License-Identifier: MPL-2.0

/**
 * A film handed to an AirPlay receiver by WebKit itself (D523): the grant's master playlist is added
 * to the page's element as an HLS `<source>` alternative (offerAirPlayAlternative), and Safari's
 * picker is shown within the press (showAirPlayPicker). As the person picks a receiver, WebKit moves
 * the element to the alternative and plays it there from where it stood; hls.js 1.7.4 lets go of the
 * element as it does (chunked.ts). Whether AirPlay plays is the element's wireless flag alone
 * (followAirPlay): on as it rises, off once it has stayed false airPlayOffSettlesMs. A sound changed
 * during AirPlay is reloaded on the receiver from a new grant (reloadOnAirPlay).
 */

/** The page, which offers AirPlay where it has WebKit's event for a target's availability (Safari). */
export type AirPlayPage = object;

/** The event an element tells as its playback target becomes wireless, or stops being. */
export const wirelessChangedEvent = "webkitcurrentplaybacktargetiswirelesschanged";

/** An element WebKit says the playback target of: wireless, an AirPlay receiver, or not. */
export interface WirelessTargeted {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- WebKit's own name
  readonly webkitCurrentPlaybackTargetIsWireless?: boolean;
}

/**
 * Whether media plays on a wireless target now, an AirPlay receiver, as WebKit says; false in a
 * browser that says nothing. The one reading of it: AirPlay's state (followAirPlay), a hidden page's
 * pause and a Watch Together guest's sync (D522 §2) each ask it, of any element, typed as WebKit's or
 * not.
 */
export function isOnWirelessTarget(media: object): boolean {
  return (
    "webkitCurrentPlaybackTargetIsWireless" in media &&
    media.webkitCurrentPlaybackTargetIsWireless === true
  );
}

/**
 * How long the wireless flag stays false before AirPlay counts as off (D523 §2): it fell for
 * 100–150 ms at WebKit's switch to the receiver and at a sound's reload (the lab's E1, E3). The film
 * goes back to hls.js after the same wait (D523 §4).
 */
export const airPlayOffSettlesMs = 1000;

/** The MIME type the alternative is offered as: the element's own HLS, which a receiver plays. */
export const airPlayAlternativeType = "application/x-mpegURL";

/** The attribute that marks the `<source>` the page added as AirPlay's alternative. */
const alternativeMark = "data-airplay-alternative";

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
 * Offers url, a grant's master playlist, to AirPlay as video's HLS alternative, replacing one offered
 * before, and lets the element play remotely, which hls.js turns off as it attaches under Managed
 * Media Source (D523 §1). hls.js removes every `<source>` as it attaches, so a playback offers it
 * once attached (chunked.ts); a late offer, at the press, works too (the lab's E3, C).
 */
export function offerAirPlayAlternative(video: HTMLVideoElement, url: string): void {
  for (const old of video.querySelectorAll(`source[${alternativeMark}]`)) {
    old.remove();
  }
  const source = video.ownerDocument.createElement("source");
  source.type = airPlayAlternativeType;
  source.src = url;
  source.setAttribute(alternativeMark, "");
  video.append(source);
  const element = video;
  element.disableRemotePlayback = false;
  element.setAttribute("x-webkit-airplay", "allow");
}

/** AirPlay's alternative offered in video (offerAirPlayAlternative), if one is. */
export function airPlayAlternativeOf(video: HTMLVideoElement): HTMLSourceElement | undefined {
  return video.querySelector<HTMLSourceElement>(`source[${alternativeMark}]`) ?? undefined;
}

/** Whether video has AirPlay's alternative offered (offerAirPlayAlternative). */
export function hasAirPlayAlternative(video: HTMLVideoElement): boolean {
  return airPlayAlternativeOf(video) !== undefined;
}

/**
 * Shows Safari's AirPlay picker for video, within the press it is called in: to start AirPlay, or as
 * AirPlay's controls while it plays (D523 §6), every choice the person's own. False where the element
 * has none or it threw.
 */
export function showAirPlayPicker(video: object): boolean {
  if (!("webkitShowPlaybackTargetPicker" in video)) {
    return false;
  }
  const show = video.webkitShowPlaybackTargetPicker;
  if (typeof show !== "function") {
    return false;
  }
  try {
    show.call(video);
    return true;
  } catch {
    return false;
  }
}

/** What following AirPlay tells: its start, as the flag rises, and its end, once it stayed down. */
export interface AirPlayFollowed {
  readonly onOn: () => void;
  readonly onOff: () => void;
}

/**
 * Follows whether video plays on AirPlay, by its wireless flag alone (D523 §2): on as the flag rises,
 * off once it has stayed false airPlayOffSettlesMs, a rise meanwhile keeping it on. No press sets it:
 * every change of route is the person's, made in a dialog the page cannot see. Returns the end of it.
 */
export function followAirPlay(video: HTMLVideoElement, told: AirPlayFollowed): () => void {
  let isOn = isOnWirelessTarget(video);
  let settling: ReturnType<typeof setTimeout> | undefined;
  function changed(): void {
    clearTimeout(settling);
    settling = undefined;
    if (isOnWirelessTarget(video)) {
      if (!isOn) {
        isOn = true;
        told.onOn();
      }
      return;
    }
    if (isOn) {
      settling = setTimeout(() => {
        settling = undefined;
        isOn = false;
        told.onOff();
      }, airPlayOffSettlesMs);
    }
  }
  video.addEventListener(wirelessChangedEvent, changed);
  if (isOn) {
    told.onOn();
  }
  return () => {
    clearTimeout(settling);
    video.removeEventListener(wirelessChangedEvent, changed);
  };
}

/**
 * Reloads the film on the receiver from url, a grant for another sound, where it stands, playing or
 * paused as it was (D523 §3): the route and the position hold, with under 0.5 s of silence (the lab's
 * E1). The alternative is url too, for the next AirPlay.
 */
export function reloadOnAirPlay(video: HTMLVideoElement, url: string): void {
  const where = { positionS: video.currentTime, wasPaused: video.paused };
  offerAirPlayAlternative(video, url);
  video.addEventListener(
    "loadedmetadata",
    () => {
      const element = video;
      element.currentTime = where.positionS;
      if (!where.wasPaused) {
        video.play().catch(() => undefined); // a refusal leaves it paused, for the person's press
      }
    },
    { once: true },
  );
  const element = video;
  element.src = url;
  video.load();
}
