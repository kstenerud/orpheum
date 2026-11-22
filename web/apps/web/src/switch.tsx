// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import "./switch.css";

import { keepsOpenAttribute } from "./popover.tsx";

/** What a switch shows and does. */
export interface SwitchProps {
  /** What it turns on or off, its name. */
  readonly label: string;
  /** How the setting stands, after the label in brackets and part of its name; none if undefined. */
  readonly status?: string | undefined;
  readonly isOn: boolean;
  /** Called with the state a press asks for: the setting applies at once, with no Save. */
  readonly onSwitch: (isOn: boolean) => void;
  readonly isDisabled?: boolean | undefined;
}

/**
 * A setting that takes effect the moment it is pressed (D538, interface rule 7; the owner,
 * 2026-11-22): one row, its label leading and a track at its end whose knob changes side as well as
 * colour, so its state is never carried by colour alone. It is one `<button role="switch">`, a
 * single focus stop that a click, a tap, Space, Enter and a remote's OK all toggle; Left and Right
 * are its container's, as on any row. A setting and no choice, it never closes a pop-up holding it
 * (keepsOpenAttribute), so ⁝ stays open for the next. A choice a form's Save confirms is a Checkbox
 * (checkbox.tsx) instead.
 */
export function Switch(props: SwitchProps): JSX.Element {
  const { isOn } = props;
  return (
    <button
      type="button"
      role="switch"
      className="switch"
      aria-checked={isOn}
      disabled={props.isDisabled === true}
      {...{ [keepsOpenAttribute]: "" }}
      onClick={() => {
        props.onSwitch(!isOn);
      }}
    >
      <span className="switch__label">
        {props.label}
        {props.status === undefined ? null : (
          <span className="switch__status"> ({props.status})</span>
        )}
      </span>
      <span className="switch__track" aria-hidden="true">
        <span className="switch__knob" />
      </span>
    </button>
  );
}
