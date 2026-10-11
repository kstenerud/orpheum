// SPDX-License-Identifier: MPL-2.0

import { useEffect, useSyncExternalStore } from "react";

/**
 * Whether a Watch Together guest's page shows a film now: a reload for a new client waits for it to
 * stop, and comes at once while the screen is black, before joining or after the session ended. A
 * guest's tab that never reloaded kept an old client through every update of a day's testing (the
 * owner, 2026-10-11).
 */
let filmsShowing = 0;
const listeners = new Set<() => void>();

function changeBy(delta: number): void {
  filmsShowing += delta;
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

const isShowing = (): boolean => filmsShowing > 0;

/** Whether a guest's film shows now, re-rendering as one starts or stops. */
export function useIsFilmShowing(): boolean {
  return useSyncExternalStore(subscribe, isShowing, isShowing);
}

/** Counts the component that calls it as a film showing, while it is mounted. */
export function useShowsAFilm(): void {
  useEffect(() => {
    changeBy(1);
    return () => {
      changeBy(-1);
    };
  }, []);
}
