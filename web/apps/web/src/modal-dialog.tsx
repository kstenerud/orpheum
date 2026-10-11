// SPDX-License-Identifier: MPL-2.0

import "./modal-dialog.css";

import { useEffect, useRef, type JSX, type ReactNode } from "react";

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
  readonly className?: string;
  readonly children: ReactNode;
}): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  const { onClose } = props;
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    if (!dialog.open) {
      dialog.showModal();
    }
    const cancel = (event: Event): void => {
      event.preventDefault(); // the page, not the browser, closes it, by unmounting it
      onClose();
    };
    const keepEscape = (event: KeyboardEvent): void => {
      if (event.key === "Escape") {
        event.stopPropagation();
      }
    };
    dialog.addEventListener("cancel", cancel);
    dialog.addEventListener("keydown", keepEscape);
    return () => {
      dialog.removeEventListener("cancel", cancel);
      dialog.removeEventListener("keydown", keepEscape);
      dialog.close();
    };
  }, [onClose]);
  const classes = ["modal-dialog", props.className].filter(Boolean).join(" ");
  return (
    <dialog ref={ref} className={classes} role={props.role} aria-label={props.label}>
      {props.children}
    </dialog>
  );
}
