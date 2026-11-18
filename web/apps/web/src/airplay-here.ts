// SPDX-License-Identifier: MPL-2.0

/**
 * The film this tab plays that AirPlay is offered for (airPlayHere), and whether it plays on AirPlay
 * now (useIsOnAirPlay, D523 §2): what Cast's menu, its sign over the film and a guest's Cast dialog
 * read. Apart from airplay.ts and free of the player, so the home page's tray reads it without
 * loading the player (home-imports.test.ts).
 */

import { useSyncExternalStore } from "react";

import type { AirPlayGrants } from "./airplay.ts";

/** The film this tab plays that AirPlay is offered for, as the Cast menu reaches it. */
export interface AirPlayHere {
  /** The playback's media token, which its grant is asked for by. */
  readonly mediaToken: string;
  /** Who its grant is asked for and revoked by. */
  readonly grants: AirPlayGrants;
  /** The audio track the film plays now, by its place, which AirPlay plays too; none, its starting one. */
  readonly audioTrackIndex?: () => number | undefined;
  /** Whether the film plays on AirPlay now, as the element's wireless flag says (D523 §2). */
  readonly isPlaying: () => boolean;
  /**
   * Within a press, offers the grant at url to AirPlay as the element's alternative and shows the
   * picker: WebKit moves the film to the receiver the person picks (D523 §1).
   */
  readonly start: (url: string) => void;
  /** Within a press, shows AirPlay's picker, AirPlay's own controls while it plays (D523 §6). */
  readonly showControls: () => void;
}

let offered: AirPlayHere | undefined;
const watchers = new Set<() => void>();

/** Tells every watcher that this tab's film, or whether it plays on AirPlay, changed. */
export function tellAirPlayChanged(): void {
  for (const watcher of watchers) {
    watcher();
  }
}

function watchOffered(watcher: () => void): () => void {
  watchers.add(watcher);
  return () => {
    watchers.delete(watcher);
  };
}

/** The film this tab offers AirPlay for now, if any. */
export function airPlayHere(): AirPlayHere | undefined {
  return offered;
}

/**
 * The film this tab offers AirPlay for, re-rendering as another is offered or it goes: a menu or
 * dialog left open as the film changes offers the new one's.
 */
export function useAirPlayHere(): AirPlayHere | undefined {
  return useSyncExternalStore(watchOffered, airPlayHere, airPlayHere);
}

/** Whether this tab's film plays on AirPlay now (D523 §2). */
function isOnAirPlayNow(): boolean {
  return offered?.isPlaying() === true;
}

/**
 * Whether this tab's film plays on AirPlay now, re-rendering as it starts or ends (D523 §2): what
 * Cast's menu, its sign over the film and a guest's Cast dialog show (D523 §6, §7).
 */
export function useIsOnAirPlay(): boolean {
  return useSyncExternalStore(watchOffered, isOnAirPlayNow, isOnAirPlayNow);
}

/** Offers AirPlay for here, the film this tab plays, till the returned function takes it back. */
export function offerAirPlayHere(here: AirPlayHere): () => void {
  offered = here;
  tellAirPlayChanged();
  return () => {
    if (offered === here) {
      offered = undefined;
      tellAirPlayChanged();
    }
  };
}
