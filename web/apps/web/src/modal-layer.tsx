// SPDX-License-Identifier: MPL-2.0

import "./modal-layer.css";

import { use, useEffect, useLayoutEffect, type RefObject } from "react";

import { arriveWhenDrawn, choiceArrivalIn, scrollIntoViewGently } from "./focus-navigation.ts";
import { useCommands } from "./input-layer.tsx";
import { ServicesContext } from "./services.tsx";

/** Whether a press at event's point falls outside dialog's own box, on its backdrop. */
function isOutside(dialog: HTMLDialogElement, event: MouseEvent): boolean {
  const box = dialog.getBoundingClientRect();
  return (
    event.clientX < box.left ||
    event.clientX > box.right ||
    event.clientY < box.top ||
    event.clientY > box.bottom
  );
}

/**
 * Opens dialog as the browser's own modal, unless it is open: its first field focused if asked and
 * drawn; else its arrival from its marks (D421, focus-navigation.ts choiceArrivalIn), its first
 * choice not marked a dangerous action, or, where every choice is, the dialog itself, never its
 * Close; and that arrival again as more is drawn (arriveWhenDrawn), as a list still read when it
 * opened (Cast to). The browser's own opening focuses the first control, which may be a dangerous
 * one or the Close. Returns the stopping of the watch for its arrival.
 */
function openModal(dialog: HTMLDialogElement, shouldFocusFirstField: boolean): () => void {
  if (dialog.open) {
    return () => undefined;
  }
  dialog.showModal();
  const field = shouldFocusFirstField
    ? dialog.querySelector<HTMLElement>(firstFieldSelector)
    : null;
  if (field !== null) {
    field.focus();
    return () => undefined;
  }
  (choiceArrivalIn(dialog) ?? dialog).focus();
  return arriveWhenDrawn(
    () => dialog,
    (element) => {
      element.focus();
    },
    choiceArrivalIn,
  );
}

/**
 * What each open dialog gives focus back to as it closes, the nearest first: what had focus as it
 * opened, then, where that was in another dialog, what that one gives it back to.
 */
const focusReturns = new WeakMap<HTMLDialogElement, readonly Element[]>();

/**
 * What a dialog opening with focusedBefore focused gives focus back to: focusedBefore, then, where it
 * is in another open dialog, what that one would. A dialog opened from a drop-down's place takes the
 * list's place (the owner, 2026-10-29: "they should swap in/out"), the list closing as it opens, so
 * the place is gone when it closes, and focus goes back to the drop-down's button.
 */
function focusReturnsFrom(focusedBefore: Element | null): readonly Element[] {
  if (focusedBefore === null) {
    return [];
  }
  const under = focusedBefore.closest("dialog");
  return [focusedBefore, ...(under === null ? [] : (focusReturns.get(under) ?? []))];
}

/**
 * Gives focus back to the first of returns still on the page, outside any dialog closed, as the
 * dialog closes, where focus was nowhere else as it began to (isLost): in the dialog going, or lost
 * to the page with it. The
 * browser gives it back itself only to a dialog closed while on the page, and the page closes one by
 * removing it, so focus fell to the page's start, and a keyboard or a remote began again from there
 * (D361 §3). Focus something else took meanwhile, as a page moved to, stays where it is. Given back
 * without scrolling the page, which brings it into view only where it is out of it: the browser's own
 * focus scrolled the page to a chooser's button the opening had moved off the screen (the owner on a
 * Fire TV, 2026-10-28), and centres what it brings in.
 */
function giveFocusBack(
  dialog: HTMLDialogElement,
  returns: readonly Element[],
  isLost: boolean,
): void {
  const to = returns.find(
    (each) =>
      each.isConnected && !dialog.contains(each) && each.closest("dialog:not([open])") === null,
  );
  if (isLost && to instanceof HTMLElement) {
    to.focus({ preventScroll: true });
    scrollIntoViewGently(to);
  }
}

/** Closes the dialog on a remote's or a controller's Back, told to onClose as Escape's is. */
function useBackCloses(
  ref: RefObject<HTMLDialogElement | null>,
  toldRef: RefObject<{ readonly onClose: () => void }>,
): void {
  useCommands(ref, (detail, event) => {
    if (detail.command === "back") {
      event.preventDefault();
      toldRef.current.onClose();
    }
  });
}

/**
 * Closes the dialog on the browser's own Back, told to onClose: an entry held over the page's
 * (navigation.ts's holdBack) while it is open, which Back takes first, and which the dialog closed
 * another way takes away again. A Fire TV's remote Back in Silk is that step, never a key (the owner,
 * 2026-10-28: "I can only exit without choosing by pressing the back button, which actually brings me
 * back a page"). Where no services run, as a test of the dialog alone, there is no history to hold.
 */
function useHistoryBackCloses(toldRef: RefObject<{ readonly onClose: () => void }>): void {
  const navigation = use(ServicesContext)?.navigation;
  useEffect(
    () =>
      navigation?.holdBack(() => {
        toldRef.current.onClose();
      }),
    [navigation, toldRef],
  );
}

/** A field a person types in: not a checkbox, a radio or a button. */
const firstFieldSelector =
  'input:not([type="checkbox"]):not([type="radio"]):not([type="button"]):not([type="submit"]), textarea';

/** What the modal layer under a dialog or a pop-up is told, read as it closes. */
export interface ModalLayerTold {
  readonly onClose: () => void;
  /** Whether a press outside it leaves it open (ModalDialog's shouldStayOpenOnOutsidePress). */
  readonly shouldStayOpenOnOutsidePress?: boolean | undefined;
  /** Whether it opens with its first field to type in focused, not its first control. */
  readonly shouldFocusFirstField?: boolean | undefined;
}

/**
 * The parts every transient surface shares, a dialog (modal-dialog.tsx) and a pop-up (popover.tsx)
 * alike (D408 §1): the element in ref opened as the browser's own modal, over a page that takes no
 * press and does not scroll (modal-layer.css), focus held inside it; closed, told to onClose, by
 * Escape, which goes no further, so a pop-up it opened from stays open; by a remote's or a
 * controller's Back (input-layer.tsx); by the browser's own Back, which takes an entry held in the
 * history while it is open (D367 §10); and by a press outside it, unless told to stay; focus given
 * back to what had it before once it closes, or, where that went with a drop-down whose place opened
 * it, to the drop-down's button. toldRef holds the latest of what it is told: a page passes a new
 * onClose at each render, and opening it again at each would reset its focus. It keeps every width
 * and chrome to what composes it.
 */
export function useModalLayer(
  ref: RefObject<HTMLDialogElement | null>,
  toldRef: RefObject<ModalLayerTold>,
): void {
  // Opened as it is laid out, before the effects of what it holds: a list focusing its current
  // choice (list-box.tsx) then keeps focus from the browser's own opening, which focuses the first.
  useLayoutEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    const returns = focusReturnsFrom(document.activeElement);
    focusReturns.set(dialog, returns);
    const stopArriving = openModal(dialog, toldRef.current.shouldFocusFirstField === true);
    const cancel = (event: Event): void => {
      event.preventDefault(); // the page, not the browser, closes it, by unmounting it
      toldRef.current.onClose();
    };
    const keepEscape = (event: KeyboardEvent): void => {
      if (event.key === "Escape") {
        event.stopPropagation();
      }
    };
    // A press on the backdrop reaches the dialog itself, outside its box.
    const pressOutside = (event: MouseEvent): void => {
      if (
        toldRef.current.shouldStayOpenOnOutsidePress !== true &&
        event.target === dialog &&
        isOutside(dialog, event)
      ) {
        toldRef.current.onClose();
      }
    };
    dialog.addEventListener("cancel", cancel);
    dialog.addEventListener("keydown", keepEscape);
    dialog.addEventListener("click", pressOutside);
    return () => {
      stopArriving();
      dialog.removeEventListener("cancel", cancel);
      dialog.removeEventListener("keydown", keepEscape);
      dialog.removeEventListener("click", pressOutside);
      // Read before it closes: closed while still drawn, the browser itself gives focus back, with
      // no scroll to bring it into view.
      const now = document.activeElement;
      const isLost = now === null || now === document.body || dialog.contains(now);
      dialog.close();
      giveFocusBack(dialog, returns, isLost);
    };
  }, [ref, toldRef]);
  useBackCloses(ref, toldRef);
  useHistoryBackCloses(toldRef);
}
