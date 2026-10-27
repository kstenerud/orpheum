// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

/**
 * The film this tab plays, or last played, and where it stands: a cast started from the casting page
 * moves it to the receiver at its position (D364 §1), though the Play page is left to reach that page.
 * Kept by the tab, in memory, while the page lives.
 */
interface PlayedHere {
  readonly fileId: number;
  readonly positionMs: () => number;
}

let played: PlayedHere | undefined;

/**
 * Notes, while the calling Play page plays fileId on this device (isOwn), where it stands, by
 * positionMs, asked as it is needed; as the page goes, the position it was left at stays.
 */
export function useNotesPlayingHere(
  isOwn: boolean,
  fileId: number,
  positionMs: () => number,
): void {
  useEffect(() => {
    if (!isOwn) {
      return undefined;
    }
    played = { fileId, positionMs };
    return () => {
      const leftAtMs = positionMs();
      played = { fileId, positionMs: () => leftAtMs };
    };
  }, [isOwn, fileId, positionMs]);
}

/** Where this tab played fileId last, in ms; undefined where it was not the film it played last. */
export function positionPlayedHere(fileId: number): number | undefined {
  return played?.fileId === fileId ? played.positionMs() : undefined;
}
