// SPDX-License-Identifier: MPL-2.0

/**
 * How long work runs before it hands the main thread back: past 50 ms a task is a long one, and a
 * key press waits behind it (Google's break-up-long-tasks; the owner, 2026-11-11).
 */
export const pieceBudgetMs = 50;

/**
 * Hands the main thread back for a moment, so a key press or a frame waiting runs: scheduler.yield,
 * which resumes ahead of other tasks, where the browser has it (Chromium 129, Firefox 142), and a
 * zero timeout where it does not (Safari, Firefox 140; D271's floors).
 */
export function yieldToMain(): Promise<void> {
  if ("scheduler" in globalThis && "yield" in globalThis.scheduler) {
    return globalThis.scheduler.yield();
  }
  return new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
}

/** What inPieces works through, and how it tells the time and hands the main thread back. */
export interface Pieces {
  /** How many items there are to show. */
  readonly count: number;
  /** How many more each piece shows. */
  readonly piece: number;
  /** Shows the first shown items, its work done before it returns. */
  readonly showUpTo: (shown: number) => void;
  /** Whether to go on: false once what shows them is gone. */
  readonly isCurrent: () => boolean;
  readonly now?: () => number;
  readonly yieldNow?: () => Promise<void>;
}

/**
 * Shows count items a piece at a time, piece after piece until pieceBudgetMs has passed, then hands
 * the main thread back and goes on, so no task it runs is long by more than a piece. None to show
 * is still shown once, as none: what shows them may mark them arrived (an empty last page ends a
 * list). True once all are shown; false where isCurrent stopped it first.
 */
export async function inPieces(pieces: Pieces): Promise<boolean> {
  const now = pieces.now ?? (() => performance.now());
  const yieldNow = pieces.yieldNow ?? yieldToMain;
  let deadline = now() + pieceBudgetMs;
  let shown = 0;
  do {
    if (!pieces.isCurrent()) {
      return false;
    }
    shown = Math.min(pieces.count, shown + Math.max(1, pieces.piece));
    pieces.showUpTo(shown);
    if (shown < pieces.count && now() >= deadline) {
      await yieldNow();
      deadline = now() + pieceBudgetMs;
    }
  } while (shown < pieces.count);
  return true;
}
