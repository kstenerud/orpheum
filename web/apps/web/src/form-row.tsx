// SPDX-License-Identifier: MPL-2.0

import "./form-row.css";

import type { JSX, ReactNode, SyntheticEvent } from "react";

/**
 * A small form inside a page: its heading, fields and button in a row that wraps on a narrow
 * screen. Adding a device, changing a password, adding a person and a one-field rename all look
 * the same, from one set of styles.
 */
export function FormRow(props: {
  readonly onSubmit: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSubmit();
  };
  return (
    <form className="form-row" onSubmit={submit}>
      {props.children}
    </form>
  );
}
