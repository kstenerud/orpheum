// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * A button for what cannot be undone: pressed, it says what will happen and asks again, and only
 * the second press acts; "Keep it" puts things back as they were.
 */
export function ConfirmButton(props: {
  /** What the first button says. */
  readonly label: string;
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
      {props.consequence}{" "}
      <button
        type="button"
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
