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

/** How many pages of this tab show a Watch Together guest's group's film (GroupFilmPage). */
let groupsFilmsShown = 0;

/**
 * Notes, while the calling page shows a Watch Together guest the group's film, that the film is the
 * group's, not one the person plays: a cast started over it carries no film, and its receiver takes
 * the guest's part in the session, following the group (D371 §4). Taken for the person's own, on the
 * one Play page every film plays on (D384 §2), it played apart from the group at the person's own
 * place, the guest's page working the group, not the receiver (the owner's S23, staging, 2026-10-30).
 */
export function useNotesTheGroupsFilmShown(): void {
  useEffect(() => {
    groupsFilmsShown += 1;
    return () => {
      groupsFilmsShown -= 1;
    };
  }, []);
}

/** Whether a page of this tab shows a Watch Together guest the group's film now. */
export function isTheGroupsFilmShown(): boolean {
  return groupsFilmsShown > 0;
}

/** Where this tab played fileId last, in ms; undefined where it was not the film it played last. */
export function positionPlayedHere(fileId: number): number | undefined {
  return played?.fileId === fileId ? played.positionMs() : undefined;
}
