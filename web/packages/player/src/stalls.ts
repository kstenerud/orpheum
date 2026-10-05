// SPDX-License-Identifier: MPL-2.0

/**
 * How long a player must wait for data before it counts as a stall: a wait shorter than this is the
 * browser catching its breath, which a person barely sees.
 */
export const stallAfterMs = 1000;

/** The part of a video element stall-watching reads. */
export type WatchedMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "paused" | "seeking" | "addEventListener" | "removeEventListener"
>;

/** One stall: how many there have been, this one included, and where the player was. */
export interface Stall {
  readonly count: number;
  readonly positionMs: number;
}

/**
 * Counts a player's stalls, telling onStall each (D247 §2): a wait for data, while it plays,
 * lasting stallAfterMs, after it has played since its last seek. A seek's own loading is not a
 * stall, nor is the first load before it has played. Returns the stopping.
 */
export function watchStalls(media: WatchedMedia, onStall: (stall: Stall) => void): () => void {
  let count = 0;
  let hasPlayedSinceSeek = false;
  let pending: ReturnType<typeof setTimeout> | undefined;
  const settle = (): void => {
    clearTimeout(pending);
    pending = undefined;
  };
  const listeners: [string, () => void][] = [
    [
      "playing",
      () => {
        hasPlayedSinceSeek = true;
        settle();
      },
    ],
    ["canplay", settle],
    ["pause", settle],
    [
      "seeking",
      () => {
        hasPlayedSinceSeek = false;
        settle();
      },
    ],
    [
      "waiting",
      () => {
        if (!hasPlayedSinceSeek || media.paused || media.seeking || pending !== undefined) {
          return;
        }
        const positionMs = media.currentTime * 1000;
        pending = setTimeout(() => {
          pending = undefined;
          count++;
          onStall({ count, positionMs });
        }, stallAfterMs);
      },
    ],
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  return () => {
    settle();
    for (const [event, listener] of listeners) {
      media.removeEventListener(event, listener);
    }
  };
}
