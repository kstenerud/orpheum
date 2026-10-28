// SPDX-License-Identifier: MPL-2.0

import "./modal-dialog.css";

import { use, useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";

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

/** Opens dialog as the browser's own modal, unless it is open, its first field focused if asked. */
function openModal(dialog: HTMLDialogElement, shouldFocusFirstField: boolean): void {
  if (!dialog.open) {
    dialog.showModal();
    if (shouldFocusFirstField) {
      dialog.querySelector<HTMLElement>(firstFieldSelector)?.focus();
    }
  }
}

/**
 * Gives focus back to what had it before the dialog opened, as it closes, while that is still on the
 * page and focus is nowhere else: in the dialog going, or lost to the page with it. The browser gives
 * it back itself only to a dialog closed while on the page, and the page closes one by removing it,
 * so focus fell to the page's start, and a keyboard or a remote began again from there (D361 §3).
 * Focus something else took meanwhile, as a page moved to, stays where it is.
 */
function giveFocusBack(dialog: HTMLDialogElement, focusedBefore: Element | null): void {
  const now = document.activeElement;
  const isLost = now === null || now === document.body || dialog.contains(now);
  if (isLost && focusedBefore instanceof HTMLElement && focusedBefore.isConnected) {
    focusedBefore.focus();
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

/**
 * A modal dialog, the one way the app shows one (the owner, 2026-10-11: "these dialogs should be
 * modal ... darken the rest"): opened as the browser's own modal, over a darkened page that takes no
 * press, with focus held inside it until it closes, the arrows moving among its own controls alone
 * (focus-navigation.ts), and given back to what had it before once it closes. Escape closes it, told to onClose, and goes no further, so a menu or pop-up
 * it opened from stays open; so does a remote's or a controller's Back (input-layer.tsx), and the
 * browser's own Back, which takes an entry held in the history while it is open (D367 §10).
 */
export function ModalDialog(props: {
  /** Its name for a screen reader: its heading, or its message where it has none. */
  readonly label: string;
  readonly onClose: () => void;
  /** "alertdialog" for a notice that asks only to be read. */
  readonly role?: "dialog" | "alertdialog";
  /**
   * Whether a press outside it closes it: a drop-down's, which the page behind it waits on and
   * takes no press from, so a control under it half hidden is never pressed by mistake (the owner,
   * 2026-10-11).
   */
  readonly shouldCloseOnOutsidePress?: boolean;
  /**
   * Whether it opens with its first field to type in focused, not its first control: a form whose
   * point is typing, as an invitation's who-it-is-for (the owner, 2026-10-12).
   */
  readonly shouldFocusFirstField?: boolean;
  readonly className?: string;
  readonly children: ReactNode;
}): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  // The latest of what it is told, read as it closes: a page passes a new onClose at each render,
  // and opening the dialog again at each would reset its focus.
  const toldRef = useRef(props);
  useEffect(() => {
    toldRef.current = props;
  });
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    const focusedBefore = document.activeElement;
    openModal(dialog, toldRef.current.shouldFocusFirstField === true);
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
        toldRef.current.shouldCloseOnOutsidePress === true &&
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
      dialog.removeEventListener("cancel", cancel);
      dialog.removeEventListener("keydown", keepEscape);
      dialog.removeEventListener("click", pressOutside);
      dialog.close();
      giveFocusBack(dialog, focusedBefore);
    };
  }, []);
  useBackCloses(ref, toldRef);
  useHistoryBackCloses(toldRef);
  const classes = ["modal-dialog", props.className].filter(Boolean).join(" ");
  return (
    <dialog ref={ref} className={classes} role={props.role} aria-label={props.label}>
      {props.children}
    </dialog>
  );
}
