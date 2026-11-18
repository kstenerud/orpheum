// SPDX-License-Identifier: MPL-2.0

/**
 * hls.js's part in WebKit's AirPlay handover (D523 §1, §4): a playback offers the grant's alternative
 * once hls.js has attached, as hls.js removes every `<source>` as it attaches; stops loading as the
 * flag rises, as the lab's design B did; and lets go of the element once WebKit has moved it to the
 * alternative, hls.js 1.7.4 detaching itself and leaving the element alone (isMediaSourceReplaced).
 */

import Hls from "hls.js";

import {
  airPlayAlternativeOf,
  hasAirPlayAlternative,
  isOnWirelessTarget,
  offerAirPlayAlternative,
  wirelessChangedEvent,
} from "./airplay.ts";

/** What a playback offers AirPlay (D523 §4). */
export interface OffersAirPlay {
  /** A grant's master playlist, AirPlay's alternative once hls.js attaches; or none. */
  readonly airPlayAlternativeUrl?: string | undefined;
}

/** The part of hls.js the handover listens to and stops. */
export type HandingOverHls = Pick<Hls, "on" | "off" | "stopLoad">;

/**
 * Whether WebKit has moved video to the AirPlay alternative from hls.js's source, so the
 * detach that follows is hls.js letting go, not the playback's end or a recovery's.
 */
export function playsAirPlayAlternative(video: HTMLVideoElement): boolean {
  const alternative = airPlayAlternativeOf(video);
  return (
    alternative !== undefined && video.currentSrc !== "" && video.currentSrc === alternative.src
  );
}

/**
 * Lets AirPlay take video over from hls: offers alternativeUrl, where given, each time hls.js
 * attaches; stops hls.js loading as the target becomes wireless with an alternative offered; and
 * tells letGo once hls.js has detached itself as WebKit plays the alternative, the playback's own
 * listeners then to stop, so nothing it runs acts on the element during AirPlay. Returns the end of it.
 */
export function letAirPlayTakeOver(
  hls: HandingOverHls,
  video: HTMLVideoElement,
  handOver: { readonly alternativeUrl?: string | undefined; readonly letGo: () => void },
): () => void {
  const { alternativeUrl } = handOver;
  const attached = (): void => {
    if (alternativeUrl !== undefined) {
      offerAirPlayAlternative(video, alternativeUrl);
    }
  };
  const wirelessChanged = (): void => {
    if (isOnWirelessTarget(video) && hasAirPlayAlternative(video)) {
      hls.stopLoad();
    }
  };
  const detached = (): void => {
    if (playsAirPlayAlternative(video)) {
      handOver.letGo();
    }
  };
  hls.on(Hls.Events.MEDIA_ATTACHED, attached);
  hls.on(Hls.Events.MEDIA_DETACHED, detached);
  video.addEventListener(wirelessChangedEvent, wirelessChanged);
  return () => {
    hls.off(Hls.Events.MEDIA_ATTACHED, attached);
    hls.off(Hls.Events.MEDIA_DETACHED, detached);
    video.removeEventListener(wirelessChangedEvent, wirelessChanged);
  };
}

/**
 * A playback's end of its listeners, stops, each stopped once: at the playback's end, or as AirPlay
 * takes video over from hls (letAirPlayTakeOver), the alternative offered at each attach, so nothing
 * the playback runs acts on the element during AirPlay (D523 §1).
 */
export function stopsAsAirPlayTakesOver(
  playback: { readonly hls: HandingOverHls; readonly video: HTMLVideoElement },
  offers: OffersAirPlay,
  stops: (() => void)[],
): () => void {
  const { hls, video } = playback;
  const alternativeUrl = offers.airPlayAlternativeUrl;
  let hasStopped = false;
  const stopListening = (): void => {
    if (hasStopped) {
      return;
    }
    hasStopped = true;
    for (const stop of stops) {
      stop();
    }
  };
  stops.push(letAirPlayTakeOver(hls, video, { alternativeUrl, letGo: stopListening }));
  return stopListening;
}
