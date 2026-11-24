// SPDX-License-Identifier: MPL-2.0

import "./popover.css";

import {
  createContext,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Context,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { reportFailure } from "@orpheum/sdk";

import { ButtonByRole } from "./button-by-role.tsx";
import { grownPlace, listPlace, overOpenerPlace, type ScreenBox } from "./drop-down-place.ts";
import { caretOf, focusTargetsIn, rowAttribute } from "./focus-engine.ts";
import { directionOfKey, keyCodeOf } from "./input-buttons.ts";
import { commandEventName } from "./input-commands.ts";
import { useCommands } from "./input-layer.tsx";
import { useLatest } from "./latest.ts";
import { useModalLayer, type ModalLayerTold } from "./modal-layer.tsx";
import { isPhoneWidth } from "./phone-width.ts";

/** What names the opening button for a screen reader and a pointer's hover, where its content does not. */
export interface OpenerNaming {
  readonly "aria-label"?: string;
  readonly "aria-describedby"?: string;
  readonly title?: string;
}

/**
 * The attribute that marks a part of a pop-up whose buttons change something in it, a stepper's or
 * the volume's, as the player's ⁝ and 🔈 hold (player-menus.tsx): pressing one keeps the pop-up open,
 * for the next press.
 */
export const keepsOpenAttribute = "data-keeps-open";

/** The DOM event telling a pop-up what it holds changed in its place (tellPopoverContentChanged). */
const contentChangedEvent = "orpheum-popover-content-changed";

/**
 * Tells the pop-up from is in that what it holds changed in its place, a submenu's list taking its
 * list's (list-box.tsx's ListSubmenu, D490), or going: the pop-up is laid at its button again, as it
 * was at its opening (usePlacedAtOpener), so a taller list stays inside the screen.
 */
export function tellPopoverContentChanged(from: Element | null): void {
  from?.dispatchEvent(new Event(contentChangedEvent, { bubbles: true }));
}

/** What has focus inside dialog, other than dialog itself; undefined for nothing. */
function focusedIn(dialog: HTMLDialogElement): HTMLElement | undefined {
  const focused = document.activeElement;
  return focused instanceof HTMLElement && focused !== dialog && dialog.contains(focused)
    ? focused
    : undefined;
}

/** The nearest box around element, inside dialog, that scrolls its content; undefined for none. */
function scrollerOf(element: HTMLElement, dialog: HTMLDialogElement): HTMLElement | undefined {
  for (let at = element.parentElement; at !== null && at !== dialog; at = at.parentElement) {
    const { overflowY } = getComputedStyle(at);
    if (overflowY === "auto" || overflowY === "scroll") {
      return at;
    }
  }
  return undefined;
}

/** How a pop-up is laid at its button, besides the room at the screen's edges. */
interface Placing {
  /**
   * Whether it opens from its top, its top at the button's, rather than its current choice laid over
   * the button: a menu's, which has no current choice, and a pop-up that holds no list.
   */
  readonly isFromItsTop: boolean;
  /**
   * A selector of its part laid over the button, centre on centre, so that it looks like the button
   * grown, where it fits (layPartOverOpener): 🔈's speaker, the slider above it (player-menus.tsx;
   * the owner, 2026-11-03). Undefined for none.
   */
  readonly partOverOpener?: string | undefined;
  /**
   * Whether it grows out of its button as a tab at its foot, laid on the button and showing what the
   * button shows (Popover's tab, grownPlace): the player's menus over the film.
   */
  readonly isGrownFromOpener?: boolean | undefined;
}

/**
 * Lays dialog, an open pop-up, with its part that partSelector finds, if it names one, over the
 * button that opened it, centre on centre (overOpenerPlace), so that it looks like the button grown,
 * where it fits there inside the room at the screen's edges; says whether it did. Its `data-placed`
 * says `grown`.
 */
function layPartOverOpener(
  dialog: HTMLDialogElement,
  at: {
    readonly partSelector: string | undefined;
    readonly opener: DOMRect;
    readonly box: DOMRect;
    readonly room: ScreenBox;
  },
): boolean {
  const part =
    at.partSelector === undefined
      ? undefined
      : dialog.querySelector(at.partSelector)?.getBoundingClientRect();
  if (part === undefined) {
    return false;
  }
  const { box } = at;
  const place = overOpenerPlace({
    opener: at.opener,
    popUp: box,
    part: {
      left: part.left - box.left,
      top: part.top - box.top,
      width: part.width,
      height: part.height,
    },
    room: at.room,
  });
  if (place === undefined) {
    return false;
  }
  dialog.setAttribute("data-placed", "grown");
  dialog.style.setProperty("left", `${String(place.left)}px`);
  dialog.style.setProperty("top", `${String(place.top)}px`);
  return true;
}

/**
 * The part of the screen dialog, an open pop-up, may cover: the screen less the room at its edges
 * (.popover__room, tokens.css), as the browser lays that room out.
 */
function roomOf(dialog: HTMLDialogElement): ScreenBox {
  return (
    dialog.querySelector(".popover__room")?.getBoundingClientRect() ?? {
      left: 0,
      top: 0,
      width: window.innerWidth,
      height: window.innerHeight,
    }
  );
}

/**
 * Scrolls the content of dialog, an open pop-up laid at its button, so that focused, its current
 * choice, comes to the button's middle as far as it can, as a select's list opens, on a big screen;
 * and then brings it into view with the context its scroller keeps around it (its scroll-padding,
 * list-box.css): a neighbour either side, though that moves the choice off the button's middle
 * (D408 §3).
 */
function scrollFocusedToOpener(
  dialog: HTMLDialogElement,
  focused: HTMLElement,
  at: { readonly opener: DOMRect; readonly isPhone: boolean },
): void {
  const laid = focused.getBoundingClientRect();
  if (!at.isPhone) {
    scrollerOf(focused, dialog)?.scrollBy({
      top: laid.top + laid.height / 2 - (at.opener.top + at.opener.height / 2),
    });
  }
  focused.scrollIntoView({ block: "nearest", inline: "nearest" });
}

/**
 * The focused element's place in its list (its `li`, else itself) and the places either side of it,
 * where there are, as their top and bottom below the list's top, box: what a cut list must keep
 * (drop-down-place.ts's focusedAndBeside).
 */
function besideOf(focused: HTMLElement, box: DOMRect): { top: number; bottom: number } {
  const place = focused.closest("li") ?? focused;
  const above = place.previousElementSibling ?? place;
  const below = place.nextElementSibling ?? place;
  return {
    top: above.getBoundingClientRect().top - box.top,
    bottom: below.getBoundingClientRect().bottom - box.top,
  };
}

/**
 * Lays dialog, an open pop-up that grows out of its button (grownPlace), its tab on the button, and
 * says by its `data-tab-at` whether the tab is at its `start` or `end`, where the menu's corner above
 * it is square. Its `data-placed` says `grown`. What has focus is brought into view, its list cut.
 */
function layGrown(
  dialog: HTMLDialogElement,
  at: { readonly opener: DOMRect; readonly box: DOMRect; readonly room: ScreenBox },
): void {
  const { opener, box } = at;
  const place = grownPlace({ opener, popUp: box, room: at.room });
  dialog.setAttribute("data-placed", "grown");
  dialog.setAttribute("data-grow", place.isDown ? "down" : "up");
  const tabAt = tabAtOf(place.tabOffset, box.width - opener.width);
  if (tabAt === undefined) {
    dialog.removeAttribute("data-tab-at");
  } else {
    dialog.setAttribute("data-tab-at", tabAt);
  }
  dialog.style.setProperty("--popover-tab-offset", `${String(place.tabOffset)}px`);
  dialog.style.setProperty("left", `${String(place.left)}px`);
  dialog.style.setProperty("top", `${String(place.top)}px`);
  if (place.height !== undefined) {
    dialog.style.setProperty("max-height", `${String(place.height)}px`);
  }
  focusedIn(dialog)?.scrollIntoView({ block: "nearest", inline: "nearest" });
}

/**
 * Lays dialog, an open pop-up, at opener (drop-down-place.ts), inside the room at the screen's edges
 * (roomOf): its part over the button where it names one and that fits (layPartOverOpener), else as
 * a list is; on a big screen, where the room's edge cuts it, cuts it to the height shown, its content
 * scrolling within it (a list, list-box.css), and brings what has focus in it, its current choice,
 * to the button (scrollFocusedToOpener). A pop-up shown from its top (isFromItsTop) has no choice to
 * lay over the button: its top is the button's, as a phone's is, and its content is not scrolled to
 * the button.
 */
function layAtOpener(
  dialog: HTMLDialogElement,
  at: { readonly opener: HTMLElement } & Placing,
): void {
  const openerBox = at.opener.getBoundingClientRect();
  const isPhone = isPhoneWidth();
  dialog.style.setProperty("--popover-opener-width", `${String(openerBox.width)}px`);
  dialog.style.setProperty("--popover-opener-height", `${String(openerBox.height)}px`);
  dialog.style.removeProperty("max-height");
  dialog.style.removeProperty("--popover-tab-offset");
  const box = dialog.getBoundingClientRect();
  const room = roomOf(dialog);
  if (at.isGrownFromOpener === true) {
    layGrown(dialog, { opener: openerBox, box, room });
    return;
  }
  if (
    layPartOverOpener(dialog, { partSelector: at.partOverOpener, opener: openerBox, box, room })
  ) {
    return;
  }
  dialog.setAttribute("data-placed", isPhone ? "over" : "at");
  const focused = at.isFromItsTop ? undefined : focusedIn(dialog);
  const focusedBox = focused?.getBoundingClientRect();
  const { left, top, height } = listPlace({
    opener: openerBox,
    list: box,
    focusedMiddle:
      focusedBox === undefined ? undefined : focusedBox.top + focusedBox.height / 2 - box.top,
    focusedAndBeside: focused === undefined ? undefined : besideOf(focused, box),
    room,
    isPhone,
  });
  dialog.style.setProperty("left", `${String(left)}px`);
  dialog.style.setProperty("top", `${String(top)}px`);
  if (height < box.height) {
    dialog.style.setProperty("max-height", `${String(height)}px`);
  }
  if (focused !== undefined) {
    scrollFocusedToOpener(dialog, focused, { opener: openerBox, isPhone });
  }
}

/**
 * Lays the open pop-up at the button that opened it (layAtOpener), as it opens, after what it holds
 * has focused its current choice, and again as the screen changes size and as what it holds changes
 * in its place (tellPopoverContentChanged). The dialog's `data-placed`
 * says how: `at` a big screen's button, `over` a phone's, or `grown` out of it (layPartOverOpener).
 */
function usePlacedAtOpener(
  ref: RefObject<HTMLDialogElement | null>,
  openerRef: RefObject<HTMLElement | null>,
  placing: Placing,
): void {
  const { isFromItsTop, partOverOpener, isGrownFromOpener } = placing;
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    const lay = (): void => {
      if (openerRef.current !== null) {
        layAtOpener(dialog, {
          opener: openerRef.current,
          isFromItsTop,
          partOverOpener,
          isGrownFromOpener,
        });
      }
    };
    lay();
    window.addEventListener("resize", lay);
    dialog.addEventListener(contentChangedEvent, lay);
    // Grown out of its button, its bottom is the button's whatever its height: laid again as its
    // panel's size changes, what it holds arriving after it opens (a dialog's code loaded later, the
    // owner, 2026-11-23: Cast's ran off the screen's bottom the first time) or changing while open.
    const panel = dialog.querySelector(".popover__panel");
    const sized =
      isGrownFromOpener === true && panel !== null && typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(lay)
        : undefined;
    if (panel !== null) {
      sized?.observe(panel);
    }
    return () => {
      window.removeEventListener("resize", lay);
      dialog.removeEventListener(contentChangedEvent, lay);
      sized?.disconnect();
    };
  }, [ref, openerRef, isFromItsTop, partOverOpener, isGrownFromOpener]);
}

/** What has focus, in a few words for the core's log: its tag and classes, and whether in it. */
function focusedDescribed(popover: Element | null): string {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return "nothing";
  }
  const where = popover?.contains(focused) === true ? "in the list" : "outside the list";
  const named = [focused.tagName.toLowerCase(), ...focused.classList].join(".");
  return `${named} ${where}`;
}

/**
 * Tells the core's log of a Left or Right that left the pop-up open, but in a text field, which keeps
 * them (isTakenInPopUp): on the owner's Fire TV (Silk 138, spatial navigation) neither closed it
 * with focus on a place, where Chromium and Firefox close it by the same key (2026-10-28;
 * idiosyncrasies.md). It says which key came, whether something had
 * taken it before the app heard it and after, and where focus was, so the next check on the device
 * shows which layer kept it. isClosingRef says it was told to close, which it does a render later;
 * one that something in the pop-up took as its command, a submenu's opening or return (D490), kept
 * nothing.
 */
function useStuckArrowTold(
  ref: RefObject<HTMLElement | null>,
  isClosingRef: RefObject<boolean>,
): void {
  useEffect(() => {
    const checks = new Set<ReturnType<typeof setTimeout>>();
    let takenWithin = 0;
    const commanded = (event: Event): void => {
      if (event.defaultPrevented) {
        takenWithin += 1;
      }
    };
    const content = ref.current;
    content?.addEventListener(commandEventName, commanded);
    const heard = (event: KeyboardEvent): void => {
      const direction = directionOfKey(event.key, keyCodeOf(event));
      // A text field keeps Left and Right, and a row moves along them, the pop-up open (isTakenInPopUp).
      if (direction !== "left" && direction !== "right") {
        return;
      }
      if (isInTextField() || isAlongARow(direction === "left" ? -1 : 1)) {
        return;
      }
      const isTakenBefore = event.defaultPrevented;
      const focused = focusedDescribed(ref.current);
      const takenBefore = takenWithin;
      const check = setTimeout(() => {
        checks.delete(check);
        if (!isClosingRef.current && takenWithin === takenBefore) {
          reportFailure(
            new Error(
              `a drop-down stayed open at ${direction}: key ${event.key}, keyCode ${String(keyCodeOf(event))}, code ${event.code}, taken before the app ${String(isTakenBefore)}, taken after ${String(event.defaultPrevented)}, focus on ${focused}`,
            ),
          );
        }
      }, 0);
      checks.add(check);
    };
    // At the window, capturing: before the app's input layer and anything else of the page.
    globalThis.addEventListener("keydown", heard, true);
    return () => {
      content?.removeEventListener(commandEventName, commanded);
      globalThis.removeEventListener("keydown", heard, true);
      for (const check of checks) {
        clearTimeout(check);
      }
    };
  }, [ref, isClosingRef]);
}

/**
 * Closes the pop-up as the page enters or leaves full screen, a layout's change under it: on a Fire
 * TV Silk's Back leaves full screen and is no key the page hears, so it is the one press that closes
 * it there (D407 §8; idiosyncrasies.md, a modal dialog removed while open).
 */
function useFullScreenChangeCloses(close: () => void): void {
  useEffect(() => {
    document.addEventListener("fullscreenchange", close);
    return () => {
      document.removeEventListener("fullscreenchange", close);
    };
  }, [close]);
}

/**
 * Closes the pop-up when a link or a button in it is pressed (not one marked keepsOpenAttribute
 * inside it): after what it does, as a choice's route followed (link.tsx), heard at the document,
 * past the page's root. A press in a pop-up opened from it, a chooser's list in a list's Sort and
 * filter (listing-tools.tsx), is that one's alone: it closes, and this one, its whole content marked
 * to stay open, does not (D417). Heard on the pop-up itself, the close ran first, between the
 * browser's listeners, and it was gone before React heard the press at its root, so a link's own
 * handling never ran and the browser followed it (the browser test, 2026-10-28).
 */
function useChoiceCloses(ref: RefObject<HTMLElement | null>, close: () => void): void {
  useEffect(() => {
    const chosen = (event: MouseEvent): void => {
      const popover = ref.current;
      const { target } = event;
      if (popover === null || !(target instanceof Element)) {
        return;
      }
      const keeper = target.closest(`[${keepsOpenAttribute}]`);
      if (
        target.closest("dialog") === popover &&
        target.closest('a, button, [role="button"]') !== null &&
        (keeper === null || !popover.contains(keeper))
      ) {
        close();
      }
    };
    document.addEventListener("click", chosen);
    return () => {
      document.removeEventListener("click", chosen);
    };
  }, [ref, close]);
}

/**
 * A pop-up that is one control, as 🔈's volume is: Up and Down step it, wherever focus is in it, and
 * OK closes it, nothing in it moved to or pressed by a key (the owner, 2026-11-03: "When I select it
 * via keyboard and it opens, I should be able to press up and down to adjust it. Pressing select
 * should close it").
 */
export interface OneControl {
  readonly up: () => void;
  readonly down: () => void;
  /**
   * Where it is a row of controls, as 🔈's row of sliders is (D440 §2): moves to the one before
   * (-1) or after (1) at Left and Right, and says whether there was one to move to; at the row's
   * end, as in a pop-up of one control, Left and Right close it. Undefined for one control alone.
   */
  readonly across?: ((direction: -1 | 1) => boolean) | undefined;
}

/** What a pop-up is told. */
/** A Popover's placing: grown out of its button where it is given a tab, which says what it shows. */
interface PopoverProps extends Omit<Placing, "isGrownFromOpener"> {
  /** Its name for a screen reader. */
  readonly label: string;
  /** The button that opened it, which it is laid at. */
  readonly openerRef: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  /** Where it is one control, how Up and Down step it (OneControl); undefined for a column. */
  readonly oneControl?: OneControl | undefined;
  /**
   * Where it grows out of its button (Placing's isGrownFromOpener), what its tab shows: what the
   * button shows. Undefined for none.
   */
  readonly tab?: ReactNode;
  readonly children: ReactNode;
}

/**
 * Closes dialog, a pop-up grown out of its button, at a press on its tab, which lies where its
 * button is and so is pressed as the button would be to close it, or on its box beside the tab,
 * which shows nothing there: neither is its backdrop, which closes it already (modal-layer.tsx).
 */
function useGrownTabCloses(
  ref: RefObject<HTMLDialogElement | null>,
  isGrown: boolean,
  onClose: () => void,
): void {
  const onCloseRef = useLatest(onClose);
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null || !isGrown) {
      return undefined;
    }
    const pressed = (event: MouseEvent): void => {
      const { target } = event;
      if (target instanceof Element && target.closest(".popover__tab") !== null) {
        onCloseRef.current();
        return;
      }
      // Its box beside the tab; a press outside the box the modal layer closes it for.
      const box = dialog.getBoundingClientRect();
      const isInItsBox =
        event.clientX >= box.left &&
        event.clientX <= box.right &&
        event.clientY >= box.top &&
        event.clientY <= box.bottom;
      if (target === dialog && isInItsBox) {
        onCloseRef.current();
      }
    };
    dialog.addEventListener("click", pressed);
    return () => {
      dialog.removeEventListener("click", pressed);
    };
  }, [ref, isGrown, onCloseRef]);
}

/**
 * What a dialog grows out of, as a menu does out of its button (Popover's tab): the button, and what
 * its tab shows, what the button shows.
 */
export interface GrownFrom {
  readonly openerRef: RefObject<HTMLElement | null>;
  readonly tab: ReactNode;
}

/**
 * The button a modal dialog opened under it grows out of (modal-dialog.tsx): Watch Together's and
 * Cast's over the film, in the buttons row with the menus (the owner, 2026-11-23). The dialog takes
 * it from here and gives what it holds none, so a dialog opened from it is drawn as dialogs are.
 */
export const GrownFromContext: Context<GrownFrom | undefined> = createContext<
  GrownFrom | undefined
>(undefined);

/**
 * A dialog opened from openerRef's button grown out of it, its tab showing tab (GrownFromContext);
 * drawn as dialogs are where openerRef is undefined (a sign not over the film).
 */
export function GrownFromOpener(props: {
  readonly openerRef: RefObject<HTMLElement | null> | undefined;
  readonly tab: ReactNode;
  readonly children: ReactNode;
}): JSX.Element {
  const { openerRef, tab } = props;
  const grownFrom = useMemo(
    () => (openerRef === undefined ? undefined : { openerRef, tab }),
    [openerRef, tab],
  );
  return <GrownFromContext value={grownFrom}>{props.children}</GrownFromContext>;
}

const noOpener: RefObject<HTMLElement | null> = { current: null };

/**
 * Lays dialog, open, grown out of grownFrom's button as a menu is (layGrown), as the screen changes
 * size too, and closes it at a press on its tab; does nothing where grownFrom is undefined.
 */
export function useGrownOutOf(
  ref: RefObject<HTMLDialogElement | null>,
  grownFrom: GrownFrom | undefined,
  onClose: () => void,
): void {
  usePlacedAtOpener(ref, grownFrom?.openerRef ?? noOpener, {
    isFromItsTop: true,
    isGrownFromOpener: grownFrom !== undefined,
  });
  useGrownTabCloses(ref, grownFrom !== undefined, onClose);
}

/**
 * Where a grown pop-up's tab stands at one of its corners: "start" where it starts at the pop-up's
 * start, "end" where it ends at its end (lastOffset, the pop-up's width less the opener's), within a
 * pixel's rounding; undefined between, where both its shoulders are drawn.
 */
function tabAtOf(tabOffset: number, lastOffset: number): "start" | "end" | undefined {
  const edgeRoom = 1;
  if (tabOffset < edgeRoom) {
    return "start";
  }
  return tabOffset > lastOffset - edgeRoom ? "end" : undefined;
}

/**
 * The parts of a pop-up or dialog grown out of its button (popover.css): its panel, holding what it
 * shows (children), and its tab, laid on the button and showing what the button shows. panelClass
 * names the panel's own look beside the menus' (the dialog's).
 */
export function GrownParts(props: {
  readonly tab: ReactNode;
  readonly panelClass?: string | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <>
      <div
        className={
          props.panelClass === undefined ? "popover__panel" : `popover__panel ${props.panelClass}`
        }
      >
        {props.children}
      </div>
      <div className="popover__tab" aria-hidden="true">
        {props.tab}
      </div>
    </>
  );
}

/** Whether focus is in a text field, where Left and Right are the caret's (focus-engine.ts's caretOf). */
function isInTextField(): boolean {
  const focused = document.activeElement;
  return focused !== null && caretOf(focused) !== undefined;
}

/**
 * Whether Left (-1) or Right (1) moves along a row of the pop-up's column (focus-engine.ts's
 * rowAttribute), to a control of it beside the one focused, as an arrow does in any row: the input
 * layer moves it there, and the pop-up stays open.
 */
function isAlongARow(direction: -1 | 1): boolean {
  const focused = document.activeElement;
  const row = focused?.closest(`[${rowAttribute}]`);
  if (row === null || row === undefined || !(focused instanceof HTMLElement)) {
    return false;
  }
  const targets = focusTargetsIn(row);
  const at = targets.indexOf(focused);
  return at !== -1 && targets[at + direction] !== undefined;
}

/**
 * Left or Right in a pop-up (isTakenInPopUp): the caret's in a text field, a step along a row of its
 * column or of one control's, else closing it.
 */
function isAcrossTaken(
  direction: -1 | 1,
  oneControl: OneControl | undefined,
  onClose: () => void,
): boolean {
  if (isInTextField() || isAlongARow(direction)) {
    return false;
  }
  if (oneControl?.across?.(direction) !== true) {
    onClose();
  }
  return true;
}

/**
 * What a command does in a pop-up, closed by onClose: Left and Right close it, but from a text field,
 * a list's Sort and filter's name (listing-tools.tsx), where they are the caret's and, at its value's
 * edge, go where they would in a dialog's field, to nothing beside it in a column, not closing it; in
 * one that is one control, Up and Down step it and OK closes it, and in a row of them Left and Right
 * move along it before they close it at its end. Whether it took the command.
 */
function isTakenInPopUp(
  command: string,
  oneControl: OneControl | undefined,
  onClose: () => void,
): boolean {
  if (command === "left" || command === "right") {
    return isAcrossTaken(command === "left" ? -1 : 1, oneControl, onClose);
  }
  if (oneControl !== undefined && command === "select") {
    onClose();
    return true;
  }
  if (oneControl !== undefined && (command === "up" || command === "down")) {
    oneControl[command]();
    return true;
  }
  return false;
}

/** A pop-up's classes: one control's, one grown out of its button (its tab), or the plain one. */
function popoverClassOf(oneControl: OneControl | undefined, isGrown: boolean): string {
  if (isGrown) {
    return "popover popover--grown";
  }
  return oneControl === undefined ? "popover" : "popover popover--one-control";
}

/**
 * A pop-up, a quick control at the button that opened it (D408 §2): the modal layer
 * (modal-layer.tsx), so it closes at Escape, Back, the browser's Back and a press outside, which
 * presses nothing else, focus going back to its button; laid at its button (layAtOpener); as big as
 * what it holds within the screen, no minimum width and no Close (the owner, 2026-11-03: "For quick
 * control popups this doesn't make sense, especially since selecting within them closes the
 * widget"). It closes too when a link or a button in it is pressed (useChoiceCloses), at Left or
 * Right, what it holds being one column whose Up and Down move within it (the owner, 2026-10-28:
 * "moving left/right while in a chooser or menu closes it"), the press closing it alone, and as full
 * screen changes. One that is one control (OneControl) takes Up, Down and OK itself. Neither its opening nor its closing moves the page under it (the owner, 2026-10-28,
 * on a Fire TV: the season picker "shifted the page up before displaying it").
 */
export function Popover(props: PopoverProps): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const isClosingRef = useRef(false);
  const { onClose: tellClosed } = props;
  const onClose = useCallback(() => {
    isClosingRef.current = true;
    tellClosed();
  }, [tellClosed]);
  const toldRef = useLatest<ModalLayerTold>({ onClose });
  useModalLayer(ref, toldRef);
  useChoiceCloses(ref, onClose);
  useFullScreenChangeCloses(onClose);
  const isGrown = props.tab !== undefined;
  usePlacedAtOpener(ref, props.openerRef, {
    isFromItsTop: props.isFromItsTop,
    partOverOpener: props.partOverOpener,
    isGrownFromOpener: isGrown,
  });
  useGrownTabCloses(ref, isGrown, onClose);
  useStuckArrowTold(contentRef, isClosingRef);
  const content = (
    <div ref={contentRef} className="popover__content">
      {props.children}
    </div>
  );
  useCommands(contentRef, (detail, event) => {
    if (isTakenInPopUp(detail.command, props.oneControl, onClose)) {
      event.preventDefault();
    }
  });
  return (
    <dialog
      ref={ref}
      className={popoverClassOf(props.oneControl, isGrown)}
      aria-label={props.label}
    >
      {isGrown ? <GrownParts tab={props.tab}>{content}</GrownParts> : content}
      <div className="popover__room" aria-hidden="true" />
    </dialog>
  );
}

/**
 * A button that opens a pop-up (Popover) of what it holds, and closes it when pressed again: the
 * player's 🔈, and every drop-down (drop-down.tsx). It says whether the pop-up is open
 * (aria-expanded), and tells onOpenChange each time it opens or closes, as the player's overlay holds
 * its time (D407 §6). It is a button by role (ButtonByRole), not a <button>: the banner's Libraries
 * and menu and a list's Sort and filter are on the page of a long list, where Silk on a Fire TV lays
 * the page out each time a key focuses a <button> (idiosyncrasies.md).
 */
export function PopoverButton(props: {
  /** The pop-up's name for a screen reader. */
  readonly label: string;
  /** What the button shows. */
  readonly opener: ReactNode;
  readonly openerClassName: string;
  readonly openerNaming: OpenerNaming;
  /** Whether the pop-up opens from its top (Popover's isFromItsTop). */
  readonly isFromItsTop: boolean;
  /** A selector of its part laid over the button (Popover's partOverOpener). */
  readonly partOverOpener?: string | undefined;
  /** Where the pop-up is one control, how Up and Down step it (Popover's oneControl). */
  readonly oneControl?: OneControl | undefined;
  /** Whether the pop-up grows out of the button, its tab showing what the button shows (Popover's tab). */
  readonly isGrownFromOpener?: boolean | undefined;
  readonly onOpenChange?: ((isOpen: boolean) => void) | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const openerRef = useRef<HTMLDivElement>(null);
  const { onOpenChange } = props;
  const onOpenChangeRef = useLatest(onOpenChange);
  const setOpen = useCallback(
    (isNowOpen: boolean) => {
      setIsOpen(isNowOpen);
      onOpenChangeRef.current?.(isNowOpen);
    },
    [onOpenChangeRef],
  );
  const close = useCallback(() => {
    setOpen(false);
  }, [setOpen]);
  return (
    <>
      <ButtonByRole
        ref={openerRef}
        className={props.openerClassName}
        aria-haspopup="dialog"
        aria-expanded={isOpen}
        {...props.openerNaming}
        onClick={() => {
          setOpen(!isOpen);
        }}
      >
        {props.opener}
      </ButtonByRole>
      {isOpen ? (
        <Popover
          label={props.label}
          openerRef={openerRef}
          isFromItsTop={props.isFromItsTop}
          partOverOpener={props.partOverOpener}
          oneControl={props.oneControl}
          tab={props.isGrownFromOpener === true ? props.opener : undefined}
          onClose={close}
        >
          {props.children}
        </Popover>
      ) : null}
    </>
  );
}
