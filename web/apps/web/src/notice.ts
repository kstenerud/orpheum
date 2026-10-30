// SPDX-License-Identifier: MPL-2.0

import { useSyncExternalStore } from "react";

import type { Navigation } from "./navigation.ts";

/**
 * A word for the person on the page they are taken to, such as why what they were in ended (the
 * owner, 2026-10-29: a signed-in guest whose Watch Together session ended is taken home, "maybe with
 * a popup telling them that the play together session ended"). One at a time, in this tab: a new one
 * replaces the last. The shell shows it, a pop-up (notice-note.tsx), until it is closed or the screen
 * changes.
 */
export interface Notice {
  /** What it says, in the person's words. */
  readonly text: string;
}

let shown: Notice | undefined;
const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** Shows text as the notice, in place of any before it. */
export function showNotice(text: string): void {
  shown = { text };
  tellChanged();
}

/**
 * Takes the page home, in its place, and shows text there as the notice: home first, so the notice
 * pops up on the screen it stays on (notice-note.tsx). What ended under the page has nowhere better
 * to go: a signed-in guest's Watch Together session, a cast the server no longer knows (D386 §2).
 */
export function homeTold(navigation: Pick<Navigation, "replace">, text: string): void {
  navigation.replace({ screen: "home" });
  showNotice(text);
}

/** Takes notice away, when it is still the one shown. */
export function dismissNotice(notice: Notice): void {
  if (shown !== notice) {
    return;
  }
  shown = undefined;
  tellChanged();
}

/** The notice shown now, if any. */
export function currentNotice(): Notice | undefined {
  return shown;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The notice shown now, re-rendering as one is shown or taken away. */
export function useNotice(): Notice | undefined {
  return useSyncExternalStore(subscribe, currentNotice, currentNotice);
}
