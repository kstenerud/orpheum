// SPDX-License-Identifier: MPL-2.0

import "./submit-button.css";

import type { JSX } from "react";

/**
 * A form's main button (D538, interface rule 14): base.css's submit, in the accent. It is never
 * disabled for what its form holds: pressed with something missing, the form's check
 * (field.tsx's useFormFields) says what at the field and moves focus there. While what it started
 * works (isBusy) it shows a spinner at its start, its label kept and its width the same, says so
 * (aria-busy), and takes no press, Enter in a field of its form included, yet keeps its focus:
 * a disabled button drops a remote's focus with nowhere to go, and a second press would send twice.
 */
export function SubmitButton(props: {
  readonly label: string;
  readonly isBusy: boolean;
  /** The id of the form it sends, where it stands outside it: at a dialog's foot (D538 §2). */
  readonly form?: string | undefined;
}): JSX.Element {
  return (
    <button
      type="submit"
      form={props.form}
      className="submit-button"
      aria-busy={props.isBusy ? "true" : undefined}
      onClick={(event) => {
        // A form's Enter presses its submit too (implicit submission), so this stops both.
        if (props.isBusy) {
          event.preventDefault();
        }
      }}
    >
      {props.isBusy ? <span className="submit-button__spinner" aria-hidden="true" /> : null}
      {props.label}
    </button>
  );
}
