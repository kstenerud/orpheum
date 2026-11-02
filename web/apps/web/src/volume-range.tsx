// SPDX-License-Identifier: MPL-2.0

import "./volume-range.css";

import { useRef, type JSX } from "react";

import { fractionAlong, useTouchAnywhere } from "./range-touch.ts";

/** How finely a volume moves: a hundredth of the volume as it came. */
const stepsPerUnit = 100;

/**
 * The volume, from 0 to most, a touch at clientX sets on a slider spanning rect: where the finger
 * is along it, in hundredths.
 */
export function volumeAt(
  clientX: number,
  rect: { readonly left: number; readonly width: number },
  most: number,
): number {
  return Math.round(fractionAlong(clientX, rect) * most * stepsPerUnit) / stepsPerUnit;
}

/**
 * A volume slider, from 0 to most, 1 as it came, named by label where no label element wraps it. A
 * touch anywhere on it sets the volume where the finger is and follows it (useTouchAnywhere): Safari
 * on an iPhone moves a slider only by a touch that starts on its thumb. A film's timeline is the
 * other slider (timeline.tsx); every other is one of these (player-widgets.test.ts).
 */
export function VolumeRange(props: {
  readonly value: number;
  readonly most: number;
  readonly onChange: (value: number) => void;
  readonly label?: string;
  /** Whether it stands upright, full at its top, as the overlay's 🔈 pop-up holds it (D407 §10). */
  readonly isUpright?: boolean | undefined;
}): JSX.Element {
  const { most, onChange } = props;
  const inputRef = useRef<HTMLInputElement>(null);
  useTouchAnywhere(inputRef, (clientX, rect) => volumeAt(clientX, rect, most), onChange);
  return (
    <input
      ref={inputRef}
      type="range"
      className={props.isUpright === true ? "volume-range volume-range--upright" : "volume-range"}
      aria-label={props.label}
      min={0}
      max={most * stepsPerUnit}
      value={Math.round(props.value * stepsPerUnit)}
      onChange={(event) => {
        onChange(Number(event.target.value) / stepsPerUnit);
      }}
    />
  );
}
