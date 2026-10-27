// SPDX-License-Identifier: MPL-2.0

import { useSyncExternalStore } from "react";

/**
 * The film's volume in this tab, from 0 to 1: one value the player follows and a slider anywhere
 * sets, the host's player and the session's controls alike (D281 §5).
 */
let tabFilmVolume = 1;
const listeners = new Set<() => void>();

/** Sets the film's volume, kept within 0 and 1, and tells whoever follows it. */
export function setFilmVolume(volume: number): void {
  const next = Math.min(1, Math.max(0, volume));
  if (next === tabFilmVolume) {
    return;
  }
  tabFilmVolume = next;
  for (const listener of listeners) {
    listener();
  }
}

/** The film's volume now. */
export function filmVolume(): number {
  return tabFilmVolume;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The film's volume, re-rendering as it changes. */
export function useFilmVolume(): number {
  return useSyncExternalStore(subscribe, filmVolume, filmVolume);
}

/**
 * Makes video play at the film's volume, and takes a change made at the video's own controls as
 * the film's volume; returns the stopping. An iPhone's volume is its buttons' alone: there the
 * video does not move (WebKit's audio policy for iOS).
 */
export function followFilmVolume(video: HTMLVideoElement): () => void {
  const apply = (): void => {
    // eslint-disable-next-line no-param-reassign -- following the tab's volume is the function's job
    video.volume = tabFilmVolume;
  };
  const taken = (): void => {
    setFilmVolume(video.volume);
  };
  apply();
  const unsubscribe = subscribe(apply);
  video.addEventListener("volumechange", taken);
  return () => {
    unsubscribe();
    video.removeEventListener("volumechange", taken);
  };
}
