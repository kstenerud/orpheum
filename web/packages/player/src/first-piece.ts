// SPDX-License-Identifier: MPL-2.0

/**
 * The first arriving piece of each of the page's chunk loads, its first progress, which the bitrate
 * controller measures nothing of (D382 §2): a transport that delivers in pieces, the WebRTC path's
 * 128 KiB, brings the first with the first byte, and a chunk given up with only that measured a
 * 20 Mb/s link at 680 kb/s, the Fire TV dropping to its lowest quality (staging, 2026-10-30). hls.js
 * keeps only the load's head and its bytes so far, so the first progress is heard on its request.
 */

import type { FirstPiece } from "./abr-choice.ts";
import { isLinkCapped } from "./link-shaping.ts";

/** How many loads' first pieces are kept, the latest: more than hls.js has on its way at once. */
export const firstPiecesKept = 16;

/** A request as hearing its first progress needs it: an XMLHttpRequest, or a test's. */
export type ProgressingRequest = Pick<EventTarget, "addEventListener" | "removeEventListener">;

/** How many bytes a progress event says have arrived; 0 where it says nothing. */
function loadedOf(event: Event): number {
  const loaded: unknown = Reflect.get(event, "loaded");
  return typeof loaded === "number" ? loaded : 0;
}

/** The first piece of each load followed, by its URL, the latest firstPiecesKept. */
export class FirstPieces {
  readonly #pieces = new Map<string, FirstPiece>();
  readonly #nowMs: () => number;

  /** nowMs is the clock hls.js times its loads by, performance.now's. */
  constructor(nowMs: () => number) {
    this.#nowMs = nowMs;
  }

  /** A load of url began on request: its first progress with bytes is its first piece. */
  follow(request: ProgressingRequest, url: string): void {
    this.#pieces.delete(url);
    const onProgress = (event: Event): void => {
      const loaded = loadedOf(event);
      if (loaded <= 0) {
        return;
      }
      request.removeEventListener("progress", onProgress);
      this.#pieces.set(url, { atMs: this.#nowMs(), bytes: loaded });
      for (const oldest of this.#pieces.keys()) {
        if (this.#pieces.size <= firstPiecesKept) {
          break;
        }
        this.#pieces.delete(oldest);
      }
    };
    request.addEventListener("progress", onProgress);
  }

  /**
   * The first piece of the latest load of url; undefined before it came, for one not followed, or
   * while the debug cap shapes the loads (isLinkCapped): the piece came on the real link and the
   * cap's bytes come after it, and a piece of 100–220 KB of a 1.6 MB chunk left every chunk given
   * up at 300 kb/s too few bytes after it to measure the link, its film held at its top quality
   * (staging, 2026-11-17).
   */
  readonly of = (url: string): FirstPiece | undefined =>
    isLinkCapped() ? undefined : this.#pieces.get(url);
}
