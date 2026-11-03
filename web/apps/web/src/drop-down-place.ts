// SPDX-License-Identifier: MPL-2.0

/**
 * Where a drop-down's open list goes on the screen (drop-down.tsx), at the button that opened it, so
 * what it belongs to is plain (the owner, 2026-10-28: "the chooser opening a window in a different
 * location feels weird ... try to make them overlap the chooser somehow, so that it's obvious what
 * they belong to"). On a big screen, a television's or a computer's, as a select's list opens: its
 * chosen place laid over the button. On a phone's, a pop-up whose top is the button's. Either way
 * inside the room a pop-up may cover, the screen less a little at each edge (the owner, 2026-11-03).
 * Where that room's edge would cut a big screen's list, it is cut there and scrolls within itself, as
 * a select's does, so its chosen place stays over the button, while the cut list keeps at least half
 * its own height; past that, and on a phone's, it is moved up or across instead (the owner,
 * 2026-10-29). A pop-up with a part of its own to lay over the button (overOpenerPlace) goes there
 * where it fits.
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
  /**
   * The part of the screen a list may cover: the screen less the room kept at its edges, a little
   * and the device's safe area (tokens.css's --screen-edge-room-*; the owner, 2026-11-03: "a (very
   * small) margin at the edge of the screen that popups aren't allowed to cover").
   */
  readonly room: ScreenBox;
  /** Whether the screen is a phone's (phone-width.ts): the list's top at the button's. */
  readonly isPhone: boolean;
}

/** start, moved where it must so that size fits inside span, given as its start and its length. */
function within(start: number, size: number, span: readonly [number, number]): number {
  return Math.max(span[0], Math.min(start, span[0] + span[1] - size));
}

/**
 * The least part of its own height a big screen's list keeps where the room's edge cuts it; one cut
 * shorter is moved instead (the owner, 2026-10-29). A justified constant, the owner's rule.
 */
export const leastShownPartOfList = 0.5;

/**
 * Where the list goes: its top-left corner, in CSS pixels from the screen's, and the height it is
 * shown at, its own unless the room's edge cuts it, where it scrolls within itself for the rest.
 */
export function listPlace(placing: ListPlacing): {
  readonly left: number;
  readonly top: number;
  readonly height: number;
} {
  const { opener, list, room } = placing;
  const left = within(opener.left, list.width, [room.left, room.width]);
  const moved = (top: number): { left: number; top: number; height: number } => ({
    left,
    top: within(top, list.height, [room.top, room.height]),
    height: list.height,
  });
  if (placing.isPhone || placing.focusedMiddle === undefined) {
    return moved(opener.top);
  }
  const top = opener.top + opener.height / 2 - placing.focusedMiddle;
  const shownTop = Math.max(top, room.top);
  const shownBottom = Math.min(top + list.height, room.top + room.height);
  const shownHeight = shownBottom - shownTop;
  return shownHeight >= list.height * leastShownPartOfList
    ? { left, top: shownTop, height: shownHeight }
    : moved(top);
}

/** What laying a pop-up's part over its button goes by. */
export interface OverOpenerPlacing {
  /** The button that opened it. */
  readonly opener: ScreenBox;
  /** The pop-up's own size. */
  readonly popUp: { readonly width: number; readonly height: number };
  /** Where its part laid over the button is, from the pop-up's top-left corner. */
  readonly part: ScreenBox;
  /** The part of the screen it may cover (ListPlacing's room). */
  readonly room: ScreenBox;
}

/**
 * Where a pop-up goes whose part is laid over its button, centre on centre, as 🔈's speaker is
 * (player-menus.tsx; the owner, 2026-11-03: "the speaker icon inside the slider ends up physically
 * at a similar location to the button it spawned from"), so that it looks like the button grown:
 * its top-left corner, kept inside the room across and above its bottom, the room winning over the
 * part's place by the little a button near the screen's edge has under it; undefined where it would
 * cross the room's top, there being no room above the button, for the pop-up to be laid as any
 * other (listPlace).
 */
export function overOpenerPlace(
  placing: OverOpenerPlacing,
): { readonly left: number; readonly top: number } | undefined {
  const { opener, popUp, part, room } = placing;
  const top = Math.min(
    opener.top + opener.height / 2 - (part.top + part.height / 2),
    room.top + room.height - popUp.height,
  );
  if (top < room.top) {
    return undefined;
  }
  const left = opener.left + opener.width / 2 - (part.left + part.width / 2);
  return { left: within(left, popUp.width, [room.left, room.width]), top };
}
