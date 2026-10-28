// SPDX-License-Identifier: MPL-2.0

/**
 * Where a drop-down's open list goes on the screen (drop-down.tsx), at the button that opened it, so
 * what it belongs to is plain (the owner, 2026-10-28: "the chooser opening a window in a different
 * location feels weird ... try to make them overlap the chooser somehow, so that it's obvious what
 * they belong to"). On a big screen, a television's or a computer's, as a select's list opens: its
 * chosen place laid over the button where it fits. On a phone's, a pop-up whose top is the button's.
 * Either way inside the screen, moved up or across where the button's place leaves it no room.
 */

/** A box on the screen, in CSS pixels from its top-left corner, as getBoundingClientRect gives it. */
export interface ScreenBox {
  readonly left: number;
  readonly top: number;
  readonly width: number;
  readonly height: number;
}

/** What placing a list goes by. */
export interface ListPlacing {
  /** The button that opened it. */
  readonly opener: ScreenBox;
  /** The list's own size. */
  readonly list: { readonly width: number; readonly height: number };
  /**
   * How far below the list's top the middle of its focused place is, the current choice: what a big
   * screen lays over the button's middle. Undefined where nothing in it is focused.
   */
  readonly focusedMiddle: number | undefined;
  /** The screen's size. */
  readonly screen: { readonly width: number; readonly height: number };
  /** Whether the screen is a phone's (phone-width.ts): the list's top at the button's. */
  readonly isPhone: boolean;
}

/**
 * The room kept between a list and the screen's edge, in CSS pixels: enough that its border and
 * shadow show. A justified constant, the list's own geometry.
 */
export const screenEdgeRoomPx = 8;

/** start, kept from beyond end, where the list's size fits between the room at each edge. */
function withinScreen(start: number, size: number, screenSize: number): number {
  const most = screenSize - size - screenEdgeRoomPx;
  return Math.max(screenEdgeRoomPx, Math.min(start, most));
}

/** Where the list's top-left corner goes, in CSS pixels from the screen's. */
export function listPlace(placing: ListPlacing): { readonly left: number; readonly top: number } {
  const { opener, list, screen } = placing;
  const openerMiddle = opener.top + opener.height / 2;
  const top =
    placing.isPhone || placing.focusedMiddle === undefined
      ? opener.top
      : openerMiddle - placing.focusedMiddle;
  return {
    left: withinScreen(opener.left, list.width, screen.width),
    top: withinScreen(top, list.height, screen.height),
  };
}
