// SPDX-License-Identifier: MPL-2.0

import { useCallback, useRef, useSyncExternalStore, type RefObject } from "react";

/** Where a video stands, its length, and whether it is paused or ended, as it says them. */
export interface VideoStanding {
  readonly atMs: number;
  readonly durationMs: number;
  readonly isPaused: boolean;
  readonly hasEnded: boolean;
}

/** One video's reading: the last, and who hears each of its events. */
interface Reading {
  standing: VideoStanding;
  readonly hearers: Map<string, Set<(standing: VideoStanding) => void>>;
  readonly listeners: Map<string, () => void>;
}

/** Each video's one reading. */
const readings = new WeakMap<HTMLVideoElement, Reading>();

/** Where video stands now. */
function standingOf(video: HTMLVideoElement): VideoStanding {
  return {
    atMs: video.currentTime * 1000,
    durationMs: Number.isFinite(video.duration) ? video.duration * 1000 : 0,
    isPaused: video.paused,
    hasEnded: video.ended,
  };
}

function readingOf(video: HTMLVideoElement): Reading {
  const known = readings.get(video);
  if (known !== undefined) {
    return known;
  }
  const reading: Reading = {
    standing: standingOf(video),
    hearers: new Map(),
    listeners: new Map(),
  };
  readings.set(video, reading);
  return reading;
}

/**
 * Hears where video stands at each of events, from the one reading every hearer of the video takes:
 * one listener on the video for each event, however many hear it, reading the video once per event.
 * The bar's standing and Up next's each listened at every timeupdate and read the video apart (the
 * efficiency audit's finding 7). Returns the way to stop hearing.
 */
export function hearStanding(
  video: HTMLVideoElement,
  events: readonly string[],
  heard: (standing: VideoStanding) => void,
): () => void {
  const reading = readingOf(video);
  for (const event of events) {
    let hearers = reading.hearers.get(event);
    if (hearers === undefined) {
      const theseHearers = new Set<(standing: VideoStanding) => void>();
      hearers = theseHearers;
      reading.hearers.set(event, theseHearers);
      const listener = (): void => {
        reading.standing = standingOf(video);
        for (const each of [...theseHearers]) {
          each(reading.standing);
        }
      };
      reading.listeners.set(event, listener);
      video.addEventListener(event, listener);
    }
    hearers.add(heard);
  }
  return () => {
    for (const event of events) {
      const hearers = reading.hearers.get(event);
      hearers?.delete(heard);
      if (hearers?.size === 0) {
        const listener = reading.listeners.get(event);
        if (listener !== undefined) {
          video.removeEventListener(event, listener);
        }
        reading.hearers.delete(event);
        reading.listeners.delete(event);
      }
    }
  };
}

/** What a component takes of where a video stands, and when it draws again. */
export interface StandingTaken<T> {
  /** What it takes of a reading. */
  readonly take: (standing: VideoStanding, video: HTMLVideoElement) => T;
  /** Whether two takings mean the same to it: drawn again only when they do not. */
  readonly isSame: (one: T, other: T) => boolean;
  /** What it holds until it first reads. */
  readonly initial: T;
  /** Whether it reads at once as it starts to hear, not waiting for the first event. */
  readonly isReadAtOnce: boolean;
}

/**
 * What a component takes of where the video in videoRef stands (taken), heard at events from its one
 * reading (hearStanding). taken and events are kept outside the component, the same at each render.
 */
export function useVideoStanding<T>(
  videoRef: RefObject<HTMLVideoElement | null>,
  events: readonly string[],
  taken: StandingTaken<T>,
): T {
  const takenRef = useRef(taken.initial);
  const subscribe = useCallback(
    (changed: () => void) => {
      const video = videoRef.current;
      if (video === null) {
        return () => undefined;
      }
      const heard = (standing: VideoStanding): void => {
        const next = taken.take(standing, video);
        if (!taken.isSame(takenRef.current, next)) {
          takenRef.current = next;
          changed();
        }
      };
      const stop = hearStanding(video, events, heard);
      if (taken.isReadAtOnce) {
        heard(standingOf(video));
      }
      return stop;
    },
    [videoRef, events, taken],
  );
  const read = useCallback(() => takenRef.current, []);
  return useSyncExternalStore(subscribe, read, read);
}
