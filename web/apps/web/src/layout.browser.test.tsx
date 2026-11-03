// SPDX-License-Identifier: MPL-2.0

/**
 * The layout primitives as a real browser lays them out (layout.tsx, layout.css; the 2026-10-05
 * architecture research's R2): a Stack's column and a Cluster's wrapping row, each gap a step of the
 * space scale, each shrinking below its content where it is a flex item.
 */

import "./tokens.css";
import "./base.css";

import type { CSSProperties } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Cluster, Stack, type SpaceStep } from "./layout.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** Each step of the space scale in pixels, at the browser's 16px rem (tokens.css). */
const stepPx: Readonly<Record<SpaceStep, number>> = {
  "1": 4,
  "2": 8,
  "3": 12,
  "4": 16,
  "5": 24,
  "6": 32,
  "7": 48,
};

/** How far layout's rounding may put a box. */
const roundingPx = 1;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A box of the given size, a primitive's child. */
function Box(props: { readonly name: string; readonly style: CSSProperties }): React.JSX.Element {
  return <div data-box={props.name} style={props.style} />;
}

/** The box named name's place, drawn. */
function boxOf(name: string): DOMRect {
  const box = document.querySelector(`[data-box="${name}"]`);
  if (box === null) {
    throw new Error(`the box ${name} is not drawn`);
  }
  return box.getBoundingClientRect();
}

/** The drawn element of class name. */
function drawn(name: string): DOMRect {
  const element = document.querySelector(`.${name}`);
  if (element === null) {
    throw new Error(`no .${name} is drawn`);
  }
  return element.getBoundingClientRect();
}

const square: CSSProperties = { width: 80, height: 40 };

describe("Stack", () => {
  for (const [step, px] of Object.entries(stepPx)) {
    it(`sets its children in a column, ${step} a gap of ${String(px)}px`, async () => {
      mounted = await mount(
        <Stack space={step as SpaceStep}>
          <Box name="first" style={square} />
          <Box name="second" style={square} />
        </Stack>,
      );
      const [first, second] = [boxOf("first"), boxOf("second")];
      expect(second.left).toBeCloseTo(first.left, 0);
      expect(second.top - first.bottom).toBeCloseTo(px, 0);
    });
  }

  it("shrinks below its content where it is a flex item, never widening its row", async () => {
    mounted = await mount(
      <div style={{ display: "flex", width: 100 }}>
        <Stack space="2">
          <Box name="wide" style={{ width: 300, height: 10 }} />
        </Stack>
      </div>,
    );
    expect(Math.abs(drawn("stack").width - 100)).toBeLessThanOrEqual(roundingPx);
  });
});

describe("Cluster", () => {
  for (const [step, px] of Object.entries(stepPx)) {
    it(`sets its children in a row, ${step} a gap of ${String(px)}px`, async () => {
      mounted = await mount(
        <Cluster space={step as SpaceStep}>
          <Box name="first" style={square} />
          <Box name="second" style={square} />
        </Cluster>,
      );
      const [first, second] = [boxOf("first"), boxOf("second")];
      expect(second.top).toBeCloseTo(first.top, 0);
      expect(second.left - first.right).toBeCloseTo(px, 0);
    });
  }

  it("wraps a child that does not fit onto the next row, the gap between the rows too", async () => {
    mounted = await mount(
      <div style={{ width: 200 }}>
        <Cluster space="4">
          <Box name="first" style={square} />
          <Box name="second" style={square} />
          <Box name="third" style={square} />
        </Cluster>
      </div>,
    );
    const [first, second, third] = [boxOf("first"), boxOf("second"), boxOf("third")];
    expect(second.top).toBeCloseTo(first.top, 0);
    expect(third.left).toBeCloseTo(first.left, 0);
    expect(third.top - first.bottom).toBeCloseTo(stepPx["4"], 0);
  });

  it("centres its children across the row unless told otherwise", async () => {
    mounted = await mount(
      <Cluster space="2">
        <Box name="tall" style={{ width: 20, height: 60 }} />
        <Box name="short" style={{ width: 20, height: 20 }} />
      </Cluster>,
    );
    const [tall, short] = [boxOf("tall"), boxOf("short")];
    expect(short.top - tall.top).toBeCloseTo(20, 0);
  });

  it("lines its children up at the row's start where asked", async () => {
    mounted = await mount(
      <Cluster space="2" align="start">
        <Box name="tall" style={{ width: 20, height: 60 }} />
        <Box name="short" style={{ width: 20, height: 20 }} />
      </Cluster>,
    );
    expect(boxOf("short").top).toBeCloseTo(boxOf("tall").top, 0);
  });

  it("shrinks below its content where it is a flex item, never widening its row", async () => {
    mounted = await mount(
      <div style={{ display: "flex", width: 100 }}>
        <Cluster space="2">
          <Box name="wide" style={{ width: 300, height: 10 }} />
        </Cluster>
      </div>,
    );
    expect(Math.abs(drawn("cluster").width - 100)).toBeLessThanOrEqual(roundingPx);
  });
});

describe("a primitive's space", () => {
  it("is a step of the scale, never a length, which tsc refuses", () => {
    // @ts-expect-error -- a length is not a step of the space scale
    const length = <Stack space="1rem">{null}</Stack>;
    // @ts-expect-error -- nor is a step the scale does not have
    const offTheScale = <Cluster space="8">{null}</Cluster>;
    expect([length.type, offTheScale.type]).toEqual([Stack, Cluster]);
  });
});
