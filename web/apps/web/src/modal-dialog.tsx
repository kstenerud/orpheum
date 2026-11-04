// SPDX-License-Identifier: MPL-2.0

import "./modal-dialog.css";

import { useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { IconButton } from "./icon-button.tsx";
import { useModalLayer } from "./modal-layer.tsx";
import { CloseIcon } from "./player-icons.tsx";

/** What a modal dialog is told. */
interface ModalDialogProps {
  /** Its name for a screen reader: its heading, or its message where it has none. */
  readonly label: string;
  readonly onClose: () => void;
  /** "alertdialog" for a notice that asks only to be read. */
  readonly role?: "dialog" | "alertdialog";
  /**
   * Whether a press outside it leaves it open. A press outside closes every other: a menu, a picker,
   * a notice, a dialog whose choices are kept as they are made (the owner, 2026-10-30: "a single
   * click outside of any modal view dismisses"), the page behind taking no press, so a control under
   * it half hidden is never pressed by mistake (2026-10-11). One stays open where closing would
   * answer for the person: a question whose closing declines, as a cast's "play here?".
   */
  readonly shouldStayOpenOnOutsidePress?: boolean;
  /**
   * Whether it opens with its first field to type in focused, not its first control: a form whose
   * point is typing, as an invitation's who-it-is-for (the owner, 2026-10-12).
   */
  readonly shouldFocusFirstField?: boolean;
  readonly className?: string;
  readonly children: ReactNode;
}

/**
 * A modal dialog, the one way the app shows one (the owner, 2026-10-11: "these dialogs should be
 * modal ... darken the rest"): the modal layer (modal-layer.tsx: the browser's own modal, Escape,
 * Back, a press outside, focus given back) in the middle of a darkened screen, the arrows moving
 * among its own controls alone (focus-navigation.ts), with the standard Close, a cross at its
 * top-right, and its own width. A pop-up is not one (popover.tsx, D408 §2).
 */
export function ModalDialog(props: ModalDialogProps): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  // The latest of what it is told, read as it closes: a page passes a new onClose at each render,
  // and opening the dialog again at each would reset its focus.
  const toldRef = useRef(props);
  useEffect(() => {
    toldRef.current = props;
  });
  useModalLayer(ref, toldRef);
  const classes = ["modal-dialog", props.className].filter(Boolean).join(" ");
  return (
    // tabIndex -1: a dialog taller than the window scrolls, and a box that scrolls is a Tab stop of its
    // own in Chromium and Firefox, reached as nothing (the Cast and Watch Together dialogs, 2026-11-04).
    <dialog ref={ref} className={classes} role={props.role} aria-label={props.label} tabIndex={-1}>
      {props.children}
      {/* After what it holds, so opening focuses that first; drawn at the top-right. */}
      <StandardClose toldRef={toldRef} />
    </dialog>
  );
}

/**
 * Every dialog's Close, a cross at its top-right, which closes it (the owner, 2026-10-30: "This
 * should be a standard widget so they all behave the same"); a pop-up has none (D408 §2).
 */
function StandardClose(props: {
  readonly toldRef: RefObject<{ readonly onClose: () => void }>;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="modal-dialog__close">
      <IconButton
        label={t("dropDown.close")}
        icon={<CloseIcon />}
        onClick={() => {
          props.toldRef.current.onClose();
        }}
      />
    </span>
  );
}
