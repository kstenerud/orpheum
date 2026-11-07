// SPDX-License-Identifier: MPL-2.0

import "./volume-range.css";

import { useRef, type JSX } from "react";

import { fractionAlong, useTouchAnywhere, type TouchPoint } from "./range-touch.ts";

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
 * The volume, from 0 to most, a touch at point sets on an upright slider spanning rect, full at its
 * top: where the finger is up it, in hundredths. A finger drags the overlay's row of sliders up and
 * down (D440 §2), as a mouse does.
 */
export function uprightVolumeAt(
  point: Pick<TouchPoint, "clientY">,
  rect: { readonly top: number; readonly height: number },
  most: number,
): number {
  // Up the slider is a span from its bottom, read as a span from its left would be.
  const fromBottom = rect.top + rect.height - point.clientY;
  return volumeAt(fromBottom, { left: 0, width: rect.height }, most);
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
  /**
   * Whether it stands upright, full at its top, as the overlay's 🔈 pop-up holds it (D407 §10), a
   * finger dragging it up and down.
   */
  readonly isUpright?: boolean | undefined;
  /**
   * Whether focus can come to it, by Tab or an arrow: not in 🔈's pop-up, one control whose Up and
   * Down step it wherever focus is (player-menus.tsx); a pointer moves it all the same.
   */
  readonly isFocusable?: boolean | undefined;
}): JSX.Element {
  const { most, onChange } = props;
  const inputRef = useRef<HTMLInputElement>(null);
  const isUpright = props.isUpright === true;
  useTouchAnywhere(
    inputRef,
    (clientX, rect, point) =>
      isUpright ? uprightVolumeAt(point, rect, most) : volumeAt(clientX, rect, most),
    onChange,
  );
  return (
    <input
      ref={inputRef}
      type="range"
      className={isUpright ? "volume-range volume-range--upright" : "volume-range"}
      aria-label={props.label}
      {...(props.isFocusable === false ? { tabIndex: -1 } : {})}
      min={0}
      max={most * stepsPerUnit}
      value={Math.round(props.value * stepsPerUnit)}
      onChange={(event) => {
        onChange(Number(event.target.value) / stepsPerUnit);
      }}
    />
  );
}
