// SPDX-License-Identifier: MPL-2.0

/**
 * How long before the film's end the offer shows, in ms, where nothing says where its credits start
 * (plans/up-next.md §2): the length of a typical episode's end credits. A justified constant, until
 * detected credits place the offer (D82, D139).
 */
export const upNextLeadMs = 30_000;

/** Where a film stands, for where Up next shows: at atMs of durationMs, or ended. */
export interface Ending {
  readonly atMs: number;
  readonly durationMs: number;
  readonly hasEnded: boolean;
}

/**
 * Whether the film is within upNextLeadMs of its end, or has ended: where Up next shows, on the
 * caster's player and remote and on a Chromecast alike (D471), so they show and go together.
 */
export function isNearItsEnd(ending: Ending): boolean {
  return (
    ending.hasEnded || (ending.durationMs > 0 && ending.durationMs - ending.atMs <= upNextLeadMs)
  );
}
