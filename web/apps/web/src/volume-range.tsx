// SPDX-License-Identifier: MPL-2.0

import "./volume-range.css";

import { useEffect, useRef, type JSX } from "react";

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
  if (rect.width <= 0) {
    return 0;
  }
  const along = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
  return Math.round(along * most * stepsPerUnit) / stepsPerUnit;
}

/**
 * A volume slider, from 0 to most, 1 as it came. A touch anywhere on it sets the volume where the
 * finger is and follows it: Safari on an iPhone moves a slider only by a touch that starts on its
 * thumb, and the owner's volumes would not move unless hit just so (2026-10-12).
 */
export function VolumeRange(props: {
  readonly value: number;
  readonly most: number;
  readonly onChange: (value: number) => void;
}): JSX.Element {
  const { most, onChange } = props;
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const input = inputRef.current;
    if (input === null) {
      return undefined;
    }
    const follow = (event: TouchEvent): void => {
      const touch = event.touches[0];
      if (touch === undefined) {
        return;
      }
      event.preventDefault(); // the finger moves the volume, not the page
      onChange(volumeAt(touch.clientX, input.getBoundingClientRect(), most));
    };
    input.addEventListener("touchstart", follow, { passive: false });
    input.addEventListener("touchmove", follow, { passive: false });
    return () => {
      input.removeEventListener("touchstart", follow);
      input.removeEventListener("touchmove", follow);
    };
  }, [most, onChange]);
  return (
    <input
      ref={inputRef}
      type="range"
      className="volume-range"
      min={0}
      max={most * stepsPerUnit}
      value={Math.round(props.value * stepsPerUnit)}
      onChange={(event) => {
        onChange(Number(event.target.value) / stepsPerUnit);
      }}
    />
  );
}
