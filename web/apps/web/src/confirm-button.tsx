// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { ModalDialog } from "./modal-dialog.tsx";

/** Puts focus on a button as it is drawn: the ask's Keep it (D421). */
function focusAsDrawn(button: HTMLButtonElement | null): void {
  button?.focus();
}

/**
 * A button for what cannot be undone: pressed, it says what will happen and asks again, and only
 * the second press acts; "Keep it" puts things back as they were. Both it and what it says are in
 * the irreversible red (D278 §4): outlined first, filled to confirm. Asking, Keep it comes first and
 * has focus (D421), so a press too many keeps things as they are.
 */
export function ConfirmButton(props: {
  /** What the first button says. */
  readonly label: string;
  /** What a screen reader says of the first button, where its label alone does not name what it acts on. */
  readonly labelForReader?: string;
  /** Whether it cannot be pressed now, as a role some people hold. */
  readonly isDisabled?: boolean;
  /** What will happen, said before the admin confirms. */
  readonly consequence: string;
  /** What the confirming button says. */
  readonly confirmLabel: string;
  readonly onConfirm: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isAsking, setIsAsking] = useState(false);
  if (!isAsking) {
    return (
      <button
        type="button"
        className="irreversible-outline"
        aria-label={props.labelForReader}
        disabled={props.isDisabled}
        onClick={() => {
          setIsAsking(true);
        }}
      >
        {props.label}
      </button>
    );
  }
  return (
    <span>
      <span className="irreversible-words">{props.consequence}</span>{" "}
      <button
        type="button"
        ref={focusAsDrawn}
        onClick={() => {
          setIsAsking(false);
        }}
      >
        {t("common.keepIt")}
      </button>{" "}
      <button
        type="button"
        className="irreversible"
        onClick={() => {
          setIsAsking(false);
          props.onConfirm();
        }}
      >
        {props.confirmLabel}
      </button>
    </span>
  );
}

/**
 * A dialog asking before what cannot be undone, for an action under a "…" (more-actions.tsx): its
 * heading, what will happen, in the irreversible red, "Keep it", the least harmful choice, first and
 * focused as it opens (D421), and the action, filled red (ui-audit-2026-11-04.md §3, A library), so a
 * remote's OK pressed once too often keeps things as they are. Keep it, Escape or Back close it, sending
 * nothing.
 */
export function ConfirmDialog(props: {
  readonly heading: string;
  /** What will happen, said before the admin confirms. */
  readonly consequence: string;
  /** What the confirming button says. */
  readonly confirmLabel: string;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ModalDialog label={props.heading} onClose={props.onClose}>
      <h2>{props.heading}</h2>
      <p className="irreversible-words">{props.consequence}</p>
      <p>
        <button type="button" {...arrivalMark} onClick={props.onClose}>
          {t("common.keepIt")}
        </button>{" "}
        <button
          type="button"
          className="irreversible"
          onClick={() => {
            props.onClose();
            props.onConfirm();
          }}
        >
          {props.confirmLabel}
        </button>
      </p>
    </ModalDialog>
  );
}
