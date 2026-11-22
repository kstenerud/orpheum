// SPDX-License-Identifier: MPL-2.0

import "./action-row.css";

import type { JSX, ReactNode } from "react";

/**
 * The buttons that end a form, a dialog or a card, in one order everywhere (standards/ui.md, the
 * parts; D538 §1): the way back first ("Cancel"), then the main action, so focus meets them in that
 * order and the drawn order matches it at every width. Wide, they sit at the row's end, the main
 * one last; narrower than 36rem of their own container, they are a column of full-width buttons, the
 * main one at the bottom, nearest the thumb. Not for a toolbar, nor a page's actions (Page's).
 */
export function ActionRow(props: {
  /** The main action: a submit, or a dangerous action's first ask. */
  readonly main: ReactNode;
  /** The way back, "Cancel", where there is one. */
  readonly secondary?: ReactNode;
}): JSX.Element {
  return (
    <div className="action-row">
      <div className="action-row__buttons">
        {props.secondary}
        {props.main}
      </div>
    </div>
  );
}
