// SPDX-License-Identifier: MPL-2.0

import { heardVolumeOf, setHeardVolume } from "@orpheum/player";
import { useSyncExternalStore } from "react";

import { browserStorage } from "./browser-storage.ts";

/** Where this browser keeps the film's volume, 0 to 1, as a number. */
export const filmVolumeKey = "orpheum.sound.filmVolume";

/**
 * The film's volume on this device, from 0 to 1: one value every film's player follows and a slider
 * anywhere sets, the host's player and the session's controls alike (D281 §5), kept in the browser so
 * the next film, and the next visit, play at it (the owner, 2026-11-26: "The volume control should
 * persist per-device"). Undefined until first read.
 */
let tabFilmVolume: number | undefined;
const listeners = new Set<() => void>();

/** The volume the browser keeps; full where it keeps none, or something that is no volume. */
function keptFilmVolume(): number {
  const kept = browserStorage().getItem(filmVolumeKey);
  const volume = kept === null || kept.trim() === "" ? Number.NaN : Number(kept);
  return Number.isFinite(volume) && volume >= 0 && volume <= 1 ? volume : 1;
}

/** Sets the film's volume, kept within 0 and 1, and tells whoever follows it. */
export function setFilmVolume(volume: number): void {
  const next = Math.min(1, Math.max(0, volume));
  if (next === filmVolume()) {
    return;
  }
  tabFilmVolume = next;
  browserStorage().setItem(filmVolumeKey, String(next));
  for (const listener of listeners) {
    listener();
  }
}

/** The film's volume now. */
export function filmVolume(): number {
  tabFilmVolume ??= keptFilmVolume();
  return tabFilmVolume;
}

/** Forgets the volume read, so the next read is the browser's again: a test's page loaded anew. */
export function forgetFilmVolumeRead(): void {
  tabFilmVolume = undefined;
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
    setHeardVolume(video, filmVolume());
  };
  const taken = (): void => {
    // The volume the person set, not the element's, which a lowering by levelling is in (D436).
    setFilmVolume(heardVolumeOf(video));
  };
  apply();
  const unsubscribe = subscribe(apply);
  video.addEventListener("volumechange", taken);
  return () => {
    unsubscribe();
    video.removeEventListener("volumechange", taken);
  };
}
