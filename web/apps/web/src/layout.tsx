// SPDX-License-Identifier: MPL-2.0

import "./layout.css";

import type { JSX, ReactNode } from "react";

/**
 * A step of the space scale (tokens.css's --space-1 to --space-7), the only space a layout primitive
 * takes: a name, never a length, so tsc holds every gap to the scale (the 2026-10-05 architecture
 * research's R2).
 */
export type SpaceStep = "1" | "2" | "3" | "4" | "5" | "6" | "7";

/** How a Cluster's children line up across its row. */
export type ClusterAlign = "start" | "center" | "end" | "baseline";

/**
 * Its children in a column, the gap between them a step of the space scale. It lays out its direct
 * children alone, sets nothing inside them, and shrinks below its content where it is a flex or grid
 * item, so a screen never needs the overflow fix (R2). It answers to its container, never the
 * viewport.
 */
export function Stack(props: {
  readonly space: SpaceStep;
  readonly children: ReactNode;
}): JSX.Element {
  return <div className={`stack layout-space-${props.space}`}>{props.children}</div>;
}

/**
 * Its children in a row that wraps where its container is too narrow for them, the gap between them,
 * across and down, a step of the space scale, lined up across the row by align (center by default).
 * Like Stack, it lays out its direct children alone and shrinks below its content (R2).
 */
export function Cluster(props: {
  readonly space: SpaceStep;
  readonly align?: ClusterAlign;
  readonly children: ReactNode;
}): JSX.Element {
  const align = props.align ?? "center";
  return (
    <div className={`cluster cluster--align-${align} layout-space-${props.space}`}>
      {props.children}
    </div>
  );
}
