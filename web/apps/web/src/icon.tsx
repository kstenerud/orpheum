// SPDX-License-Identifier: MPL-2.0

import "./icon.css";

import type { JSX, ReactNode } from "react";

/**
 * Every icon's drawing (the owner, 2026-10-27: "all of our icons should ideally use the same common
 * code for display"): decoration beside words that say what it means, hidden from a screen reader
 * and never a stop for the keyboard, its shapes coloured by where it is drawn. Its marks are the
 * text colour, `currentColor`; a two-tone icon's ground behind them is `--icon-ground`. The region
 * an icon is drawn in sets `color` and `--icon-ground`, and nothing else colours it (icon.test.ts).
 * The QR code alone draws its own SVG, as a scanner needs black on white.
 */
export function Icon(props: {
  /** The icon's own class, which sizes it and, for a cue, sets its colours. */
  readonly className: string;
  /** The icon's drawing box; a 24-unit square unless its shapes were drawn in another. */
  readonly viewBox?: string;
  /** The icon's shapes alone. */
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <svg
      className={`icon ${props.className}`}
      viewBox={props.viewBox ?? "0 0 24 24"}
      aria-hidden="true"
      focusable="false"
    >
      {props.children}
    </svg>
  );
}
