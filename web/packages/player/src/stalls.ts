// SPDX-License-Identifier: MPL-2.0

import { fellBehindEvent } from "@orpheum/sdk";

/**
 * How long a player must wait for data before it counts as a stall: a wait shorter than this is the
 * browser catching its breath, which a person barely sees.
 */
export const stallAfterMs = 1000;

/**
 * How long a load, the first or a seek's, may leave a player unable to play before it counts as a
 * stall, and again each time as long: a player that cannot get going never stalls while it plays,
 * and so was never offered a lower quality (the owner's choice, 2026-10-05, D247 §2). Two, the
 * second stall's move, come within the 60 s a Watch Together group waits for its host.
 */
export const slowLoadAfterMs = 15_000;

/** HTMLMediaElement.HAVE_FUTURE_DATA, which a plain page may not define: it can play on. */
const haveFutureData = 3;

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

/**
 * Times how long media has been unable to play since a load began, calling onSlow at each
 * slowLoadAfterMs it still cannot: from the first moment it cannot play, however many seeks
 * follow, since a player catching up seeks again and again, each its own load.
 */
function slowLoads(
  media: WatchedMedia,
  onSlow: () => void,
): { readonly loading: () => void; readonly loaded: () => void } {
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
        onSlow();
        loading();
      }
    }, slowLoadAfterMs);
  };
  return { loading, loaded };
}

/**
 * Counts a player's stalls, telling onStall each (D247 §2): a wait for data, while it plays,
 * lasting stallAfterMs, after it has played since its last seek; and a load, the first or a seek's,
 * that has left it unable to play for slowLoadAfterMs, once for each slowLoadAfterMs; and each
 * time a Watch Together player falls behind its group (fellBehindEvent). A shorter load is not a
 * stall. Returns the stopping.
 */
export function watchStalls(media: WatchedMedia, onStall: (stall: Stall) => void): () => void {
  let count = 0;
  const stall = (positionMs: number): void => {
    count++;
    onStall({ count, positionMs });
  };
  const waits = waitsWhilePlaying(media, stall);
  const loads = slowLoads(media, () => {
    stall(media.currentTime * 1000);
  });
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
