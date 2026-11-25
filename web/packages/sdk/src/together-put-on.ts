// SPDX-License-Identifier: MPL-2.0

import type { GroupCommand } from "./together-connection.ts";

/**
 * What a player leading its session commands to put its film on (D555 §3): the Play page's host and
 * the web app's receiver (together-host.tsx's putOn) and the Google Cast receiver (its CastDevice)
 * alike: the film at positionMs, as the title itemId it was opened as (B0 question 20), then paused
 * there where it is to wait paused (a cast started from a film paused, the owner, 2026-11-08: "It
 * should maintain the paused status of the caster"). When each sends them is its own.
 */
export function putOnCommands(
  film: { readonly fileId: number; readonly itemId?: number | undefined },
  positionMs: number,
  isPaused: boolean,
): readonly GroupCommand[] {
  const { fileId, itemId } = film;
  return [
    { command: "changeItem", fileId, ...(itemId === undefined ? {} : { itemId }), positionMs },
    ...(isPaused ? [{ command: "pause" } as const] : []),
  ];
}
