// SPDX-License-Identifier: MPL-2.0

/**
 * How long before the film's end the offer shows, in ms, where nothing says where its credits start
 * (plans/up-next.md §2): the length of a typical episode's end credits. A justified constant, kept
 * for a file with no credits segment (D339); one with a segment has the offer at its credits (D139).
 */
export const upNextLeadMs = 30_000;

/**
 * Where a film stands, for where Up next shows: at atMs of durationMs, or ended; and where its
 * credits start, where a segment of its file says so (plans/media-segments.md).
 */
export interface Ending {
  readonly atMs: number;
  readonly durationMs: number;
  readonly hasEnded: boolean;
  readonly creditsStartMs?: number | undefined;
}

/**
 * Whether the film is near its end, where Up next shows: from its credits' start where it has a
 * credits segment (D139, D339), else within upNextLeadMs of its end; or ended. The caster's player
 * and remote and a Chromecast show and go together by it (D471): the last two know no segments, so
 * a cast's receiver gives none either (plans/up-next.md §9).
 */
export function isNearItsEnd(ending: Ending): boolean {
  if (ending.hasEnded) {
    return true;
  }
  if (ending.creditsStartMs !== undefined) {
    return ending.atMs >= ending.creditsStartMs;
  }
  return ending.durationMs > 0 && ending.durationMs - ending.atMs <= upNextLeadMs;
}
