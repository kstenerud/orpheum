// SPDX-License-Identifier: MPL-2.0

import { fellBehindEvent, isAskedToPlay } from "@orpheum/sdk";

/**
 * How long a player must wait for data before it counts as a stall: a wait shorter than this is the
 * browser catching its breath, which a person barely sees.
 */
export const stallAfterMs = 1000;

/**
 * How long a load, the first or a seek's, may leave a player unable to play before it counts as a
 * stall, and again each time as long: a player that cannot get going never stalls while it plays,
 * and so was never offered a lower quality (the owner's choice, 2026-10-05, D247 §2). Two come
 * within the 60 s a Watch Together group waits for its host. Counted only while the wait is the
 * link's (slowLoads).
 */
export const slowLoadAfterMs = 15_000;

/** HTMLMediaElement.HAVE_FUTURE_DATA, which a plain page may not define: it can play on. */
export const haveFutureData = 3;

/** The part of a video element stall-watching reads. */
export type WatchedMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "paused" | "seeking" | "readyState" | "addEventListener" | "removeEventListener"
>;

/** One stall: how many there have been, this one included, and where the player was. */
export interface Stall {
  readonly count: number;
  readonly positionMs: number;
}

/**
 * Times a wait for data while media plays, calling onStall with where it was once the wait has
 * lasted stallAfterMs, after it has played since its last seek: a seek's own loading is not one.
 */
function waitsWhilePlaying(
  media: WatchedMedia,
  onStall: (positionMs: number) => void,
): {
  readonly played: () => void;
  readonly sought: () => void;
  readonly settle: () => void;
  readonly waiting: () => void;
} {
  let hasPlayedSinceSeek = false;
  let pending: ReturnType<typeof setTimeout> | undefined;
  const settle = (): void => {
    clearTimeout(pending);
    pending = undefined;
  };
  return {
    played: () => {
      hasPlayedSinceSeek = true;
      settle();
    },
    sought: () => {
      hasPlayedSinceSeek = false;
      settle();
    },
    settle,
    waiting: () => {
      if (!hasPlayedSinceSeek || media.paused || media.seeking || pending !== undefined) {
        return;
      }
      const positionMs = media.currentTime * 1000;
      pending = setTimeout(() => {
        pending = undefined;
        onStall(positionMs);
      }, stallAfterMs);
    },
  };
}

/** What says whose a slow load's wait is: watchStalls's own unless a test hands it others. */
export interface LoadWaits {
  /**
   * Whether the chunk on its way waits on the core, which answered 425, being made: the server's
   * wait, for its own log, not the link's. Never, where nothing says.
   */
  readonly isAwaitingTheCore?: () => boolean;
  /**
   * Whether media, paused, is still asked to play: its start gate's play waits, or its group's
   * asks it (isAskedToPlay, @orpheum/sdk).
   */
  readonly isAskedToPlay?: (media: WatchedMedia) => boolean;
}

/**
 * Times how long media has been unable to play since a load began, calling onSlow with where it is
 * at each slowLoadAfterMs it still cannot: from the first moment it cannot play, however many seeks
 * follow, since a player catching up seeks again and again, each its own load. A slowLoadAfterMs
 * whose end finds the wait not the link's counts nothing: media paused with nothing asking it to
 * play (a film page's hidden player loading before Play, which reported a stall every 15 s,
 * staging 2026-10-29: 83 of 103), or its chunk on its way being made by the core.
 */
function slowLoads(
  media: WatchedMedia,
  onSlow: (positionMs: number) => void,
  waits: LoadWaits,
): { readonly loading: () => void; readonly loaded: () => void } {
  const isAsked = waits.isAskedToPlay ?? isAskedToPlay;
  const isLinksWait = (): boolean =>
    (!media.paused || isAsked(media)) && waits.isAwaitingTheCore?.() !== true;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const loaded = (): void => {
    clearTimeout(timer);
    timer = undefined;
  };
  const loading = (): void => {
    if (timer !== undefined) {
      return;
    }
    timer = setTimeout(() => {
      timer = undefined;
      if (media.readyState < haveFutureData) {
        if (isLinksWait()) {
          onSlow(media.currentTime * 1000);
        }
        loading();
      }
    }, slowLoadAfterMs);
  };
  return { loading, loaded };
}

/**
 * Counts a player's stalls, telling onStall each (D247 §2): a wait for data, while it plays,
 * lasting stallAfterMs, after it has played since its last seek; and a load, the first or a seek's,
 * that has left it unable to play for slowLoadAfterMs, once for each slowLoadAfterMs that ends
 * with the wait the link's (slowLoads, loadWaits); and each time a Watch Together player falls behind
 * its group (fellBehindEvent). A shorter load is not a stall. Returns the stopping.
 */
export function watchStalls(
  media: WatchedMedia,
  onStall: (stall: Stall) => void,
  loadWaits: LoadWaits = {},
): () => void {
  let count = 0;
  const stall = (positionMs: number): void => {
    count++;
    onStall({ count, positionMs });
  };
  const waits = waitsWhilePlaying(media, stall);
  const loads = slowLoads(media, stall, loadWaits);
  const listeners: [string, () => void][] = [
    [
      "playing",
      () => {
        waits.played();
        loads.loaded();
      },
    ],
    [
      "canplay",
      () => {
        waits.settle();
        loads.loaded();
      },
    ],
    ["pause", waits.settle],
    [
      fellBehindEvent,
      () => {
        waits.settle();
        stall(media.currentTime * 1000);
      },
    ],
    [
      "seeking",
      () => {
        waits.sought();
        loads.loading();
      },
    ],
    ["waiting", waits.waiting],
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  if (media.readyState < haveFutureData) {
    loads.loading(); // the first load
  }
  return () => {
    waits.settle();
    loads.loaded();
    for (const [event, listener] of listeners) {
      media.removeEventListener(event, listener);
    }
  };
}
