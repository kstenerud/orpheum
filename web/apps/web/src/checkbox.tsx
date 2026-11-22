// SPDX-License-Identifier: MPL-2.0

import { useId, type JSX } from "react";

import "./checkbox.css";

import { TickIcon } from "./icons.tsx";

/** What a checkbox shows and does. */
export interface CheckboxProps {
  readonly label: string;
  /** A line under the label saying more; none if undefined. */
  readonly hint?: string | undefined;
  readonly isChecked: boolean;
  readonly onChange: (isChecked: boolean) => void;
  readonly name?: string | undefined;
  readonly isDisabled?: boolean | undefined;
}

/**
 * A yes or no that a form's Save, Add or Continue confirms (D538, interface rules 7 and 14): a
 * native checkbox redrawn, its whole row its label, so a press anywhere on it toggles it, and its
 * hint, if any, describing it. A setting that applies at once is a Switch (switch.tsx) instead.
 */
export function Checkbox(props: CheckboxProps): JSX.Element {
  const hintId = useId();
  return (
    <div className="checkbox">
      <label className="checkbox__row">
        <span className="checkbox__frame">
          <input
            type="checkbox"
            className="checkbox__box"
            name={props.name}
            checked={props.isChecked}
            disabled={props.isDisabled === true}
            aria-describedby={props.hint === undefined ? undefined : hintId}
            onChange={(event) => {
              props.onChange(event.currentTarget.checked);
            }}
          />
          <TickIcon />
        </span>
        {props.label}
      </label>
      {props.hint === undefined ? null : (
        <span id={hintId} className="checkbox__hint">
          {props.hint}
        </span>
      )}
    </div>
  );
}

/** One of a radio group's choices: what it is sent as, and what it says. */
export interface RadioChoice<T extends string> {
  readonly value: T;
  readonly label: string;
  readonly hint?: string | undefined;
}

/**
 * One choice of a few, shown as rows (D538, interface rule 14): a fieldset whose legend names the
 * group, each choice a native radio redrawn, its whole row its label. Tab enters the group at the
 * one chosen, and the arrow keys move and choose, as a remote's Up and Down do.
 */
export function RadioRows<T extends string>(props: {
  readonly legend: string;
  readonly name: string;
  readonly choices: readonly RadioChoice<T>[];
  readonly chosen: T;
  readonly onChoose: (chosen: T) => void;
}): JSX.Element {
  const hintId = useId();
  return (
    <fieldset className="radio-rows">
      <legend className="radio-rows__legend">{props.legend}</legend>
      {props.choices.map((choice) => (
        <div key={choice.value} className="checkbox">
          <label className="checkbox__row">
            <span className="checkbox__frame">
              <input
                type="radio"
                className="checkbox__box checkbox__box--radio"
                name={props.name}
                value={choice.value}
                checked={choice.value === props.chosen}
                aria-describedby={
                  choice.hint === undefined ? undefined : `${hintId}-${choice.value}`
                }
                onChange={() => {
                  props.onChoose(choice.value);
                }}
              />
            </span>
            {choice.label}
          </label>
          {choice.hint === undefined ? null : (
            <span id={`${hintId}-${choice.value}`} className="checkbox__hint">
              {choice.hint}
            </span>
          )}
        </div>
      ))}
    </fieldset>
  );
}
