// SPDX-License-Identifier: MPL-2.0

import "./busy-group.css";

import type { JSX, ReactNode } from "react";

/**
 * Controls that wait while a request one of them sent runs: every button, field and choice inside
 * is disabled, so a second click sends nothing (D244 §4). It looks like nothing else.
 */
export function BusyGroup(props: {
  readonly isBusy: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <fieldset className="busy-group" disabled={props.isBusy}>
      {props.children}
    </fieldset>
  );
}
