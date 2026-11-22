// SPDX-License-Identifier: MPL-2.0

import "./number-input.css";

import { useId, useState, type JSX, type KeyboardEvent, type ReactNode } from "react";

import { IconButton } from "./icon-button.tsx";
import { rowAttribute } from "./focus-engine.ts";
import { StepArrowIcon } from "./icons.tsx";
import { Cluster, Stack } from "./layout.tsx";

/** What a number input is given: its value, its bounds and step, and the words it is said in. */
export interface NumberInputProps {
  /** What the number is, in the person's words: the field's accessible name. */
  readonly label: string;
  /** The value as a person reads it ("Sound on this device: 550 ms later"), shown and said. */
  readonly valueText: string;
  /** The value, a whole number from min to max. */
  readonly value: number;
  readonly min: number;
  readonly max: number;
  /** How far each arrow moves it. */
  readonly step: number;
  /** The unit, after the field ("ms"). */
  readonly unit: string;
  /** What the down arrow does, in the person's words: its accessible name. */
  readonly stepDownLabel: string;
  /** What the up arrow does, in the person's words: its accessible name. */
  readonly stepUpLabel: string;
  /** Where its words go, or a text label leading it instead; its words lead its row if none. */
  readonly layout?: NumberInputLayout;
  /** Hears a new value, already within min and max: an arrow's step, or a number typed. */
  readonly onChange: (value: number) => void;
}

/**
 * How a number input is laid out:
 * - wordsLead: its words lead its one row, the field and arrows after them;
 * - wordsAbove: its words have a line of their own above the field and arrows, so these stay where
 *   they are as the words' length changes (a cast's remote's sound timing);
 * - labelled: a row of a menu, rowLabel's text leading it and the input right beside it, in the row
 *   itself (the owner, 2026-11-22: ⁝'s "Sound on this device", and a file's own timing after it);
 *   the label never changes, so neither does where the field and arrows are. Its words are then
 *   said, not shown: a screen reader still hears each value. In a pop-up, Left and Right move along
 *   the row, from the field's end to each arrow and back, before they close it (rowAttribute).
 */
export type NumberInputLayout =
  | { readonly kind: "wordsLead" }
  | { readonly kind: "wordsAbove" }
  | { readonly kind: "labelled"; readonly rowLabel: string };

/** A whole number, signed or not, with the typographic minus a phone's keyboard may give. */
const wholeNumber = /^[+\-−]?\d+$/u;

/**
 * The number text says, held within min and max; undefined where it is no whole number, which the
 * field refuses, showing its value again.
 */
export function numberTyped(text: string, min: number, max: number): number | undefined {
  const trimmed = text.trim();
  if (!wholeNumber.test(trimmed)) {
    return undefined;
  }
  const typed = Number(trimmed.replace("−", "-"));
  return Math.min(Math.max(typed, min), max);
}

/**
 * A number typed into a field, or stepped by the down and up arrows beside it (the owner,
 * 2026-11-22: "a field to enter a number, with an up and down arrow to step up/down"). The field is
 * a text field, so a remote and a keyboard move through it as every one-line field: Left and Right
 * move its caret and leave it at its ends, to the arrows on its right, and Up and Down always leave
 * the row (focus-engine.ts's isArrowTheFields); the arrows are buttons a remote's OK, a click and a
 * tap press, never disabled at a bound, which on a television would drop the focus with nowhere to
 * go, but held there. What is typed applies on Enter or on leaving the field, as every setting's
 * typing does; what is no whole number is refused, the field showing the value again, and a number
 * past a bound is held at it. Its words, valueText, are an output, so a screen reader says the value
 * each time it changes, an arrow's press too; the field says its name, its value and its unit.
 */
export function NumberInput(props: NumberInputProps): JSX.Element {
  const { value, min, max, step, onChange } = props;
  const unitId = useId();
  const moveTo = (wanted: number): void => {
    const next = Math.min(Math.max(wanted, min), max);
    if (next !== value) {
      onChange(next);
    }
  };
  return (
    <LaidOut layout={props.layout ?? { kind: "wordsLead" }} valueText={props.valueText}>
      <NumberField
        label={props.label}
        unitId={unitId}
        value={value}
        min={min}
        max={max}
        onApply={moveTo}
      />
      <span id={unitId} className="number-input__unit">
        {props.unit}
      </span>
      <StepArrow
        label={props.stepDownLabel}
        direction="down"
        onPress={() => {
          moveTo(value - step);
        }}
      />
      <StepArrow
        label={props.stepUpLabel}
        direction="up"
        onPress={() => {
          moveTo(value + step);
        }}
      />
    </LaidOut>
  );
}

/**
 * A number input's field: what is typed shows until Enter or leaving the field applies it, a whole
 * number held within min and max, and anything else is refused, the field showing value again.
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
      className="number-input__field"
      // A phone's number pad has no minus on some phones, so a field that can go below zero takes the
      // whole keyboard.
      inputMode={min < 0 ? "text" : "numeric"}
      enterKeyHint="done"
      size={widestOf(min, max)}
      aria-label={props.label}
      aria-describedby={props.unitId}
      value={typing ?? String(props.value)}
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

/** One of a number input's arrows, an icon button named label that steps it down or up. */
function StepArrow(props: {
  readonly label: string;
  readonly direction: "up" | "down";
  readonly onPress: () => void;
}): JSX.Element {
  return (
    <IconButton
      label={props.label}
      icon={<StepArrowIcon direction={props.direction} />}
      onClick={props.onPress}
    />
  );
}

/** A number input's field, unit and arrows, children, with its words, laid out as layout says. */
function LaidOut(props: {
  readonly layout: NumberInputLayout;
  readonly valueText: string;
  readonly children: ReactNode;
}): JSX.Element {
  const { layout, children: parts } = props;
  if (layout.kind === "labelled") {
    // A row of a menu, whose Left and Right move along it in a pop-up rather than close it.
    return (
      <div {...{ [rowAttribute]: "" }}>
        <Cluster space="2">
          <span className="number-input__label">{layout.rowLabel}</span>
          {parts}
          <output className="number-input__said">{props.valueText}</output>
        </Cluster>
      </div>
    );
  }
  const words = <output className="number-input__words">{props.valueText}</output>;
  return layout.kind === "wordsAbove" ? (
    <Stack space="1">
      {words}
      <Cluster space="2">{parts}</Cluster>
    </Stack>
  ) : (
    <Cluster space="2">
      {words}
      {parts}
    </Cluster>
  );
}

/** How many characters the widest value from min to max is, so the field never changes width. */
function widestOf(min: number, max: number): number {
  return Math.max(String(min).length, String(max).length);
}
