// SPDX-License-Identifier: MPL-2.0

import "./stepper.css";

import { useId, useState, type JSX, type KeyboardEvent } from "react";

import { IconButton } from "./icon-button.tsx";
import { rowAttribute } from "./focus-engine.ts";
import { StepIcon } from "./icons.tsx";

/** What a stepper is given: its value, its bounds and step, and the words it is said in. */
export interface StepperProps {
  /** What the number is, shown with it ("Sound on this device"): the name of its group. */
  readonly label: string;
  /** What the field holds, in the person's words, its unit and its sense too: its accessible name. */
  readonly fieldLabel: string;
  /** The value as a person reads it ("Sound on this device: 550 ms later"): its caption. */
  readonly valueText: string;
  /** The value, a whole number from min to max. */
  readonly value: number;
  readonly min: number;
  readonly max: number;
  /** How far − and + move it. */
  readonly step: number;
  /** The unit, after the field ("ms"). */
  readonly unit: string;
  /** What − does, in the person's words: its accessible name. */
  readonly stepDownLabel: string;
  /** What + does, in the person's words: its accessible name. */
  readonly stepUpLabel: string;
  /** Hears a new value, already within min and max: a step of − or +, or a number typed. */
  readonly onChange: (value: number) => void;
}

/** A whole number, signed or not, with the typographic minus a phone's keyboard may give. */
const wholeNumber = /^[+\-−]?\d+$/u;

/** The minus sign a stepper shows a value below zero with (U+2212), not the hyphen. */
const minusSign = "−";

/**
 * The number text says, held within min and max; undefined where it is no whole number, which the
 * field refuses, showing its value again.
 */
export function numberTyped(text: string, min: number, max: number): number | undefined {
  const trimmed = text.trim();
  if (!wholeNumber.test(trimmed)) {
    return undefined;
  }
  const typed = Number(trimmed.replace(minusSign, "-"));
  return Math.min(Math.max(typed, min), max);
}

/** A value as the field shows it: below zero with the minus sign, not a hyphen. */
export function numberShown(value: number): string {
  return value < 0 ? `${minusSign}${String(-value)}` : String(value);
}

/**
 * A number typed into a field, or stepped by − and + either side of it, one joined control, its
 * label leading it, which never changes, so the control never moves as the value does (D538 §9:
 * the number input's next form; the owner, 2026-11-22: "a field to enter a number, with an up and
 * down arrow to step up/down"). It is a group (a fieldset) named by its label, a row of controls (rowAttribute),
 * so in a pop-up Left and Right move along it, from − to the field to +, and close the pop-up only
 * past its ends. The field is a text field, so a remote and a keyboard move through it as every
 * one-line field: Left and Right move its caret and leave it at its ends, and Up and Down always
 * leave the row (focus-engine.ts's isArrowTheFields), never stepping it, as a remote's Up and Down
 * leave every row. − and + are buttons a remote's OK, a click and a tap press; at a bound the one
 * that can go no further is marked unavailable, and does nothing, but stays in the focus order,
 * which a disabled button would leave, dropping a television's focus with nowhere to go. What is
 * typed applies on Enter or on leaving the field, as every setting's typing does; what is no whole
 * number is refused, the field showing the value again, and a number past a bound is held at it.
 * Its caption, valueText, is an output describing the group, so a screen reader says the value each
 * time it changes, a step's too; it is shown under the control only while the value is not 0, where
 * there is nothing to spell out.
 */
export function Stepper(props: StepperProps): JSX.Element {
  const { value, min, max, step, onChange } = props;
  const labelId = useId();
  const unitId = useId();
  const captionId = useId();
  const moveTo = (wanted: number): void => {
    const next = Math.min(Math.max(wanted, min), max);
    if (next !== value) {
      onChange(next);
    }
  };
  return (
    <div className="stepper">
      <span id={labelId} className="stepper__label">
        {props.label}
      </span>
      <fieldset
        className="stepper__control"
        aria-labelledby={labelId}
        aria-describedby={captionId}
        {...{ [rowAttribute]: "" }}
      >
        <Sign
          label={props.stepDownLabel}
          direction="less"
          isAtBound={value <= min}
          onPress={() => {
            moveTo(value - step);
          }}
        />
        <span className="stepper__middle">
          <NumberField
            label={props.fieldLabel}
            unitId={unitId}
            value={value}
            min={min}
            max={max}
            onApply={moveTo}
          />
          <span id={unitId} className="stepper__unit">
            {props.unit}
          </span>
        </span>
        <Sign
          label={props.stepUpLabel}
          direction="more"
          isAtBound={value >= max}
          onPress={() => {
            moveTo(value + step);
          }}
        />
      </fieldset>
      <Caption id={captionId} isShown={value !== 0} text={props.valueText} />
    </div>
  );
}

/** A stepper's caption: an output, said at each change, shown only while isShown. */
function Caption(props: {
  readonly id: string;
  readonly isShown: boolean;
  readonly text: string;
}): JSX.Element {
  return (
    <output
      id={props.id}
      className={props.isShown ? "stepper__caption" : "stepper__caption stepper__caption--unshown"}
    >
      {props.text}
    </output>
  );
}

/** A stepper's − or +: at its bound unavailable, a press doing nothing, yet still focusable. */
function Sign(props: {
  readonly label: string;
  readonly direction: "less" | "more";
  readonly isAtBound: boolean;
  readonly onPress: () => void;
}): JSX.Element {
  return (
    <IconButton
      label={props.label}
      icon={<StepIcon direction={props.direction} />}
      isUnavailable={props.isAtBound}
      onClick={props.onPress}
    />
  );
}

/**
 * A stepper's field: what is typed shows until Enter or leaving the field applies it, a whole number
 * held within min and max, and anything else is refused, the field showing value again.
 */
function NumberField(props: {
  readonly label: string;
  readonly unitId: string;
  readonly value: number;
  readonly min: number;
  readonly max: number;
  readonly onApply: (typed: number) => void;
}): JSX.Element {
  const { min, max } = props;
  // What is being typed, until it is applied; undefined shows the value.
  const [typing, setTyping] = useState<string | undefined>(undefined);
  const applyTyping = (): void => {
    if (typing === undefined) {
      return;
    }
    setTyping(undefined);
    const typed = numberTyped(typing, min, max);
    if (typed !== undefined) {
      props.onApply(typed);
    }
  };
  return (
    <input
      type="text"
      className="stepper__field"
      // A phone's number pad has no minus on some phones, so a field that can go below zero takes the
      // whole keyboard.
      inputMode={min < 0 ? "text" : "numeric"}
      enterKeyHint="done"
      size={widestOf(min, max)}
      aria-label={props.label}
      aria-describedby={props.unitId}
      value={typing ?? numberShown(props.value)}
      onChange={(event) => {
        setTyping(event.target.value);
      }}
      onBlur={applyTyping}
      onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
        if (event.key === "Enter") {
          applyTyping();
        }
      }}
    />
  );
}

/** How many characters the widest value from min to max is, so the field never changes width. */
function widestOf(min: number, max: number): number {
  return Math.max(String(min).length, String(max).length);
}
