// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * A button for what cannot be undone: pressed, it says what will happen and asks again, and only
 * the second press acts; "Keep it" puts things back as they were. Both it and what it says are in
 * the irreversible red (D278 §4): outlined first, filled to confirm.
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
        className="irreversible"
        onClick={() => {
          setIsAsking(false);
          props.onConfirm();
        }}
      >
        {props.confirmLabel}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          setIsAsking(false);
        }}
      >
        {t("common.keepIt")}
      </button>
    </span>
  );
}
