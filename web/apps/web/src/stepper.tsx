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
  /**
   * How the field shows the value where not as the whole number it is: in a unit scale times its
   * own, to fractionDigits places, as a timing kept in ms is shown in seconds to a tenth.
   */
  readonly shownIn?: ShownIn | undefined;
  /** Whether a value above zero shows a leading "+", as a subtitle timing's does. */
  readonly isPlusShown?: boolean | undefined;
}

/** A field's unit, scale times the value's, shown to fractionDigits places: 1000 and 1 for ms as s. */
export interface ShownIn {
  readonly scale: number;
  readonly fractionDigits: number;
}

/** A value shown as the whole number it is. */
const asItIs: ShownIn = { scale: 1, fractionDigits: 0 };

/** A whole number, signed or not, with the typographic minus a phone's keyboard may give. */
const wholeNumber = /^[+\-−]?\d+$/u;

/** A number with a fraction too, its point a full stop or a comma, as a keyboard of either gives. */
const decimalNumber = /^[+\-−]?(?:\d+(?:[.,]\d*)?|[.,]\d+)$/u;

/** The minus sign a stepper shows a value below zero with (U+2212), not the hyphen. */
const minusSign = "−";

/**
 * The number text says, in the value's own unit, held within min and max; undefined where it is no
 * number the field takes, which it refuses, showing its value again: a whole number, or, where the
 * field shows a fraction (shownIn), one with a fraction, rounded to the value's unit.
 */
export function numberTyped(
  text: string,
  min: number,
  max: number,
  shownIn: ShownIn = asItIs,
): number | undefined {
  const trimmed = text.trim();
  const form = shownIn.fractionDigits > 0 ? decimalNumber : wholeNumber;
  if (!form.test(trimmed)) {
    return undefined;
  }
  const typed = Number(trimmed.replace(minusSign, "-").replace(",", "."));
  return Math.min(Math.max(Math.round(typed * shownIn.scale), min), max);
}

/**
 * A value as the field shows it: in its shown unit (shownIn), below zero with the minus sign, not a
 * hyphen, and above zero with a leading "+" where isPlusShown.
 */
export function numberShown(value: number, shownIn: ShownIn = asItIs, isPlusShown = false): string {
  const digits = (Math.abs(value) / shownIn.scale).toFixed(shownIn.fractionDigits);
  if (value < 0) {
    return `${minusSign}${digits}`;
  }
  return isPlusShown && value > 0 ? `+${digits}` : digits;
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
            shownIn={props.shownIn ?? asItIs}
            isPlusShown={props.isPlusShown === true}
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
 * A stepper's field: what is typed shows until Enter or leaving the field applies it, a number held
 * within min and max, and anything else is refused, the field showing value again.
 */
function NumberField(props: {
  readonly label: string;
  readonly unitId: string;
  readonly value: number;
  readonly min: number;
  readonly max: number;
  readonly shownIn: ShownIn;
  readonly isPlusShown: boolean;
  readonly onApply: (typed: number) => void;
}): JSX.Element {
  const { min, max, shownIn, isPlusShown } = props;
  // What is being typed, until it is applied; undefined shows the value.
  const [typing, setTyping] = useState<string | undefined>(undefined);
  const applyTyping = (): void => {
    if (typing === undefined) {
      return;
    }
    setTyping(undefined);
    const typed = numberTyped(typing, min, max, shownIn);
    if (typed !== undefined) {
      props.onApply(typed);
    }
  };
  return (
    <input
      type="text"
      className="stepper__field"
      inputMode={inputModeOf(min, shownIn)}
      enterKeyHint="done"
      size={Math.max(
        numberShown(min, shownIn, isPlusShown).length,
        numberShown(max, shownIn, isPlusShown).length,
      )}
      aria-label={props.label}
      aria-describedby={props.unitId}
      value={typing ?? numberShown(props.value, shownIn, isPlusShown)}
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

/**
 * The keyboard a phone gives the field: its whole keyboard where the value can go below zero, as
 * some phones' number pad has no minus; else a decimal pad for a fraction, or a number pad.
 */
function inputModeOf(min: number, shownIn: ShownIn): "text" | "decimal" | "numeric" {
  if (min < 0) {
    return "text";
  }
  return shownIn.fractionDigits > 0 ? "decimal" : "numeric";
}
