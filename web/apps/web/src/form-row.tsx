// SPDX-License-Identifier: MPL-2.0

import "./form-row.css";

import type { JSX, ReactNode, SyntheticEvent } from "react";

/**
 * A form's submit handler that does action in place of the browser's own submission, which would
 * load another page. Every form in the client submits through it.
 */
export function submitWith(action: () => void): (event: SyntheticEvent) => void {
  return (event) => {
    event.preventDefault();
    action();
  };
}

/**
 * A small form inside a page: its heading, fields and button in a row that wraps on a narrow
 * screen. Adding a device, changing a password, adding a person and a one-field rename all look
 * the same, from one set of styles.
 */
export function FormRow(props: {
  readonly onSubmit: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <form className="form-row" onSubmit={submitWith(props.onSubmit)}>
      {props.children}
    </form>
  );
}
