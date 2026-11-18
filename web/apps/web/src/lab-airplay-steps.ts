// SPDX-License-Identifier: MPL-2.0

/**
 * The AirPlay lab's steps (plans/airplay.md's spike), apart from its page: the grant in the media's
 * path, Safari's AirPlay picker on an element, and what the element says of its target, each step
 * logged. Safari alone has the picker; elsewhere each step says it is missing.
 */

import { followAirPlay, offerAirPlayAlternative, showAirPlayPicker } from "@orpheum/player";
import { failureText } from "@orpheum/sdk";

import type { AirPlayGrant, Log } from "./airplay.ts";

/** A media element with Safari's AirPlay picker (WebKit's own, in no standard's DOM types). */
interface WebKitAirPlay {
  readonly webkitShowPlaybackTargetPicker: () => void;
}

/** Whether video has Safari's AirPlay picker. */
function hasPicker(video: HTMLVideoElement): video is HTMLVideoElement & WebKitAirPlay {
  return (
    "webkitShowPlaybackTargetPicker" in video &&
    typeof video.webkitShowPlaybackTargetPicker === "function"
  );
}

/** Shows Safari's AirPlay picker for video, within the press it is called in; false where missing. */
export function showPicker(video: HTMLVideoElement, log: Log): boolean {
  if (!hasPicker(video)) {
    log("no webkitShowPlaybackTargetPicker here: AirPlay's picker is Safari's");
    return false;
  }
  try {
    video.webkitShowPlaybackTargetPicker();
    log("picker shown");
    return true;
  } catch (cause: unknown) {
    log(`the picker failed: ${failureText(cause)}`);
    return false;
  }
}

/** Logs what video says of its AirPlay target and its playing, until the returned stop. */
export function watchTarget(video: HTMLVideoElement, name: string, log: Log): () => void {
  const heard: Record<string, (event: Event) => void> = {
    webkitcurrentplaybacktargetiswirelesschanged: () => {
      const isWireless: unknown = Reflect.get(video, "webkitCurrentPlaybackTargetIsWireless");
      log(`${name}: webkitcurrentplaybacktargetiswirelesschanged, wireless ${String(isWireless)}`);
    },
    webkitplaybacktargetavailabilitychanged: (event) => {
      const availability: unknown = Reflect.get(event, "availability");
      log(`${name}: webkitplaybacktargetavailabilitychanged, ${String(availability)}`);
    },
    loadedmetadata: () => {
      log(`${name}: loadedmetadata, ${video.duration.toFixed(0)} s`);
    },
    playing: () => {
      log(`${name}: playing at ${video.currentTime.toFixed(1)} s`);
    },
    error: () => {
      log(`${name}: error ${String(video.error?.code)} ${video.error?.message ?? ""}`);
    },
  };
  for (const [kind, listener] of Object.entries(heard)) {
    video.addEventListener(kind, listener);
  }
  return () => {
    for (const [kind, listener] of Object.entries(heard)) {
      video.removeEventListener(kind, listener);
    }
  };
}

/** What a way is given: the page's element, hls.js's, and the grant fetched for its playback. */
export interface AirPlayWay {
  readonly main: HTMLVideoElement;
  readonly grant: AirPlayGrant;
  readonly log: Log;
}

/**
 * Way A: a second element, the page's own beside it with AirPlay allowed, given the grant's address
 * as Safari's own HLS at the page's element's position, and its picker shown, within the press; the
 * page's element is paused.
 */
export function airPlayFromSecondElement(way: AirPlayWay, toSecond: HTMLVideoElement): void {
  const { main } = way;
  const second = toSecond;
  const position = main.currentTime;
  main.pause();
  second.src = way.grant.url;
  second.currentTime = position;
  way.log(`way A: a second element given the grant at ${position.toFixed(1)} s`);
  showPicker(second, way.log);
  playOn(second, "way A", way.log);
}

/**
 * Way B, the product's (D523): WebKit's own handover, the grant fetched before the press offered as
 * the page's element's HLS alternative and its picker shown within the press; hls.js lets go of the
 * element itself as WebKit moves it to the receiver. AirPlay's start and its settled end are logged,
 * as the element's wireless flag says them (followAirPlay). Returns the end of the following.
 */
export function airPlayFromTheSameElement(way: AirPlayWay): () => void {
  const { main, log } = way;
  log(`way B: the grant offered as the element's alternative at ${main.currentTime.toFixed(1)} s`);
  offerAirPlayAlternative(main, way.grant.url);
  const stopFollowing = followAirPlay(main, {
    onOn: () => {
      log(`way B: on AirPlay at ${main.currentTime.toFixed(1)} s`);
    },
    onOff: () => {
      const how = main.paused ? "paused" : "playing";
      log(`way B: off AirPlay at ${main.currentTime.toFixed(1)} s, ${how}`);
    },
  });
  if (showAirPlayPicker(main)) {
    log("picker shown");
  } else {
    log("no webkitShowPlaybackTargetPicker here: AirPlay's picker is Safari's");
  }
  return stopFollowing;
}

/** Plays video, logging a refusal under the way's name. */
function playOn(video: HTMLVideoElement, wayName: string, log: Log): void {
  video.play().catch((cause: unknown) => {
    log(`${wayName}: play() refused: ${failureText(cause)}`);
  });
}

/** Ends video's playing, its source taken away. */
export function endPlaying(video: HTMLVideoElement): void {
  video.pause();
  video.removeAttribute("src");
  video.load();
}
