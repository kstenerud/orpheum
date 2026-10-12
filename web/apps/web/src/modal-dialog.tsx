// SPDX-License-Identifier: MPL-2.0

import "./modal-dialog.css";

import { useEffect, useRef, type JSX, type ReactNode } from "react";

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

/** A field a person types in: not a checkbox, a radio or a button. */
const firstFieldSelector =
  'input:not([type="checkbox"]):not([type="radio"]):not([type="button"]):not([type="submit"]), textarea';

/**
 * A modal dialog, the one way the app shows one (the owner, 2026-10-11: "these dialogs should be
 * modal ... darken the rest"): opened as the browser's own modal, over a darkened page that takes no
 * press, with focus held inside it until it closes. Escape closes it, told to onClose, and goes no
 * further, so a menu or pop-up it opened from stays open.
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
    if (!dialog.open) {
      dialog.showModal();
      if (toldRef.current.shouldFocusFirstField === true) {
        dialog.querySelector<HTMLElement>(firstFieldSelector)?.focus();
      }
    }
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
    };
  }, []);
  const classes = ["modal-dialog", props.className].filter(Boolean).join(" ");
  return (
    <dialog ref={ref} className={classes} role={props.role} aria-label={props.label}>
      {props.children}
    </dialog>
  );
}
