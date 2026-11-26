// SPDX-License-Identifier: MPL-2.0

import "./ends-place.css";

import { createContext, use, useState, type JSX, type ReactNode } from "react";

/** Where the controls under the film draw when it ends (EndsUnderTheHeading); null: nowhere. */
const EndsPlaceContext = createContext<HTMLElement | null>(null);

/**
 * A page's heading over its player (children), and directly under the heading the place where the
 * controls under the film held upright draw when it ends (page-controls.tsx; the owner, 2026-11-26:
 * "The "Ends" line ... should be under the backlink at the top"). Empty while the overlay is the
 * controls, which draws Ends under its own way up.
 */
export function EndsUnderTheHeading(props: {
  readonly heading: ReactNode;
  readonly children: ReactNode;
}): JSX.Element {
  const [place, setPlace] = useState<HTMLElement | null>(null);
  return (
    <>
      {props.heading}
      <div ref={setPlace} className="ends-place" />
      <EndsPlaceContext value={place}>{props.children}</EndsPlaceContext>
    </>
  );
}

/** The place under the page's heading where the film's end is drawn; null where the page has none. */
export function useEndsPlace(): HTMLElement | null {
  return use(EndsPlaceContext);
}
